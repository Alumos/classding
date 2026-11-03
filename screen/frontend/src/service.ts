import type {components, paths} from '@classding/api/types';
import type {AckStatus, Notice, WSFrame} from '@classding/api/ws';
import { configureServer, loadState, saveState } from './native';
import { playChime, playNotice } from './audio';

export type {AckStatus, Notice} from '@classding/api/ws';
export type Connection = 'connecting' | 'online' | 'offline';
interface Ledger { status: AckStatus; at: number; err?: string; }
interface StoredState {
  version: 1; server: string; token: string; deviceName: string; className: string; volume: number; lastSeq: number;
  queue: Notice[]; ledger: Record<string, Ledger>;
}
export type Tool = { name: 'countdown'; end: number; seconds: number } | { name: 'rollcall'; names: string[]; selected: string; ends: number } | null;
export interface Snapshot {
  ready: boolean; busy: boolean; error: string; connection: Connection; bound: boolean; className: string; deviceName: string;
  server: string; volume: number; pending: number; current: Notice | null; startedAt: number; tool: Tool;
}
type Frame = Pick<WSFrame,'t'|'id'|'seq'> & {d?: Record<string, unknown>};
const frameTypes = ['hello','ack','welcome','notify','recall','tool','config','unbind','ping','pong','receipt','presence'] as const satisfies readonly WSFrame['t'][];
const parseFrame = (input: unknown): Frame | null => {
  if (typeof input !== 'object' || input === null || Array.isArray(input)) return null;
  const value = input as Record<string,unknown>;
  const t = frameTypes.find(type => type === value.t);
  if (!t) return null;
  const d = typeof value.d === 'object' && value.d !== null && !Array.isArray(value.d) ? value.d as Record<string,unknown> : undefined;
  return {t,id:typeof value.id === 'string' ? value.id : undefined,seq:typeof value.seq === 'number' ? value.seq : undefined,d};
};
const fresh = (): StoredState => ({version: 1, server: '', token: '', deviceName: '教室大屏', className: '', volume: 80, lastSeq: 0, queue: [], ledger: {}});
const sleep = (ms: number, signal: AbortSignal) => new Promise<void>((resolve, reject) => {
  if (signal.aborted) { reject(new DOMException('已取消', 'AbortError')); return; }
  const cancel = () => { clearTimeout(timer); reject(new DOMException('已取消', 'AbortError')); };
  const timer = setTimeout(() => { signal.removeEventListener('abort', cancel); resolve(); }, ms);
  signal.addEventListener('abort', cancel, {once:true});
});
const num = (value: unknown, fallback: number) => typeof value === 'number' && Number.isFinite(value) ? value : fallback;
const str = (value: unknown, fallback = '') => typeof value === 'string' ? value : fallback;
const abortError = (error: unknown) => error instanceof DOMException && error.name === 'AbortError';

export class ScreenService {
  private state = fresh();
  private view: Snapshot = {ready:false,busy:false,error:'',connection:'offline',bound:false,className:'',deviceName:'教室大屏',server:'',volume:80,pending:0,current:null,startedAt:0,tool:null};
  private listeners = new Set<() => void>();
  private ops: Promise<void> = Promise.resolve();
  private socket?: WebSocket;
  private retry?: ReturnType<typeof setTimeout>;
  private heartbeat?: ReturnType<typeof setInterval>;
  private attempts = 0;
  private lastFrame = 0;
  private serverOffset = 0;
  private base = '';
  private active?: AbortController;
  private suppressed = new Set<string>();
  private validated = new Set<string>();
  private helloPending: string[] = [];
  private suppressStarts = false;
  private generation = 0;
  private disposed = false;
  subscribe = (fn: () => void) => { this.listeners.add(fn); return () => { this.listeners.delete(fn); }; };
  getSnapshot = () => this.view;
  private emit(patch: Partial<Snapshot> = {}) {
    const current = patch.current === undefined ? this.view.current : patch.current;
    this.view = {...this.view, ...patch, bound: !!this.state.token, className:this.state.className, deviceName:this.state.deviceName,
      server:this.state.server, volume:this.state.volume, pending:Math.max(0, this.state.queue.length - (current ? 1 : 0))};
    this.listeners.forEach(fn => fn());
  }
  private mutate(change: () => void): Promise<void> {
    const operation = this.ops.then(async () => {
      const before = structuredClone(this.state);
      try { change(); await saveState(JSON.stringify(this.state)); this.emit(); }
      catch (error) { this.state = before; this.emit({error:`无法保存设备状态：${error instanceof Error ? error.message : String(error)}`}); this.socket?.close(); throw error; }
    });
    this.ops = operation.catch(() => {});
    return operation;
  }
  private async setBase(server: string) { this.base = await configureServer(server); }
  private async request(path: string, init?: RequestInit): Promise<Response> {
    return fetch(`${this.base}${path}`, init);
  }
  async init() {
    try {
      const raw = await loadState();
      if (raw) {
        const saved = JSON.parse(raw) as Partial<StoredState>;
        if (saved.version !== 1 || typeof saved.token !== 'string' || !Array.isArray(saved.queue) || typeof saved.ledger !== 'object' || !saved.ledger) throw new Error('本地数据格式无法识别');
        this.state = {...fresh(), ...saved} as StoredState;
        // A completed message must never be spoken again after a restart.
        this.state.queue = this.state.queue.filter(item => !['played','dismissed','failed'].includes(this.state.ledger[item.message_id]?.status));
      }
      await this.setBase(this.state.server);
      this.emit({ready:true});
      if (this.state.token) this.connect();
    } catch (error) { this.emit({ready:true,error:`初始化失败：${error instanceof Error ? error.message : String(error)}`}); }
  }
  async bind(code: string, deviceName: string, server: string) {
    if (!/^\d{6}$/.test(code)) throw new Error('请输入 6 位数字绑定码');
    this.emit({busy:true,error:''});
    let serverAccepted = false;
    try {
      await this.setBase(server);
      const payload: paths['/device/bind']['post']['requestBody']['content']['application/json'] = {code,name:deviceName.trim() || '教室大屏',os:navigator.userAgent.includes('Windows') ? 'windows' : 'browser',client_version:'0.1.0'};
      const response = await this.request('/api/v1/device/bind', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)});
      const data = await response.json().catch(() => ({})) as Partial<components['schemas']['DeviceBinding'] & components['schemas']['Error']>;
      if (!response.ok) throw new Error(data.message || `绑定失败（${response.status}）`);
      if (!data.device_token || !data.class?.name) throw new Error('服务器返回的绑定信息不完整');
      serverAccepted = true;
      await this.mutate(() => { this.state = {...fresh(),server,token:data.device_token!,deviceName:data.device?.name || deviceName.trim() || '教室大屏',className:data.class!.name!,volume:num(data.volume,80)}; });
      this.attempts = 0; this.connect();
    } catch (error) { const reason = error instanceof TypeError ? '无法连接服务器，请检查地址和网络' : error instanceof Error ? error.message : String(error); const message = serverAccepted ? `服务器已绑定设备，但本机保存失败：${reason}。请在教师端解绑该设备后重新绑定。` : reason; this.emit({error:message}); throw error; }
    finally { this.emit({busy:false}); }
  }
  private connect() {
    if (!this.state.token || this.disposed) return;
    clearTimeout(this.retry); clearInterval(this.heartbeat);
    const generation = ++this.generation;
    this.validated.clear();
    const base = this.base || window.location.origin;
    const url = new URL(`${base}/ws/device`, window.location.href); url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
    this.emit({connection:'connecting'});
    try {
      const ws = this.socket = new WebSocket(url);
      ws.onopen = () => {
        if (generation !== this.generation) return;
        this.lastFrame = Date.now();
        void this.hello(ws, generation).catch(error => this.emit({error:`准备连接失败：${error instanceof Error ? error.message : String(error)}`}));
      };
      ws.onmessage = event => {
        if (generation !== this.generation) return;
        this.lastFrame = Date.now();
        try { const frame = parseFrame(JSON.parse(String(event.data)) as unknown); if (frame) void this.onFrame(frame, generation).catch(error => this.emit({error:`处理消息失败：${error instanceof Error ? error.message : String(error)}`})); }
        catch { this.emit({error:'收到无法识别的服务器消息'}); }
      };
      ws.onerror = () => { /* close handler schedules retry */ };
      ws.onclose = () => { if (generation !== this.generation) return; clearInterval(this.heartbeat); this.emit({connection:'offline'}); void this.verifyAfterClose(generation); };
    } catch { this.emit({connection:'offline'}); this.scheduleRetry(); }
  }
  private async hello(ws: WebSocket, generation: number) {
    const now = Date.now() + this.serverOffset;
    const expired = new Set(this.state.queue.filter(item => {
      const sent = Date.parse(item.sent_at || '');
      const expires = item.expires_at ? Date.parse(item.expires_at) : (Number.isFinite(sent) ? sent + 600000 : NaN);
      return item.message_id !== this.view.current?.message_id && Number.isFinite(expires) && now >= expires;
    }).map(item => item.message_id));
    if (expired.size) await this.mutate(() => {
      this.state.queue = this.state.queue.filter(item => !expired.has(item.message_id));
      expired.forEach(id => { this.state.ledger[id] = {status:'failed',at:Date.now(),err:'消息已过期'}; });
    });
    if (generation !== this.generation || ws.readyState !== WebSocket.OPEN) return;
    const pendingIDs = this.state.queue.slice(0,4096).map(item => item.message_id);
    ws.send(JSON.stringify({t:'hello',d:{token:this.state.token,last_seq:this.state.lastSeq,client_version:'0.1.0',os:navigator.userAgent.includes('Windows') ? 'windows' : 'browser',pending_ids:pendingIDs}}));
    this.helloPending = pendingIDs;
    this.heartbeat = setInterval(() => {
      if (Date.now() - this.lastFrame > 60000) { ws.close(); return; }
      this.send({t:'ping',d:{}});
    },25000);
  }
  private async verifyAfterClose(generation: number) {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 5000);
    try {
      const response = await this.request('/api/v1/device/me', {headers:{Authorization:`Bearer ${this.state.token}`},signal:controller.signal});
      if (generation !== this.generation || !this.state.token) return;
      if (response.status === 401 || response.status === 403) { await this.unbind(); return; }
    } catch { /* Network errors must not erase an otherwise valid binding. */ }
    finally { clearTimeout(timer); }
    if (generation === this.generation) this.scheduleRetry();
  }
  private scheduleRetry() {
    if (!this.state.token || this.disposed) return;
    const delay = Math.min(30000, 1000 * 2 ** Math.min(this.attempts++, 5)) * (.7 + Math.random() * .6);
    this.retry = setTimeout(() => this.connect(), delay);
  }
  private send(frame: object) { if (this.socket?.readyState === WebSocket.OPEN) this.socket.send(JSON.stringify(frame)); }
  private sendAck(id: string, entry: Ledger) { this.send({t:'ack',d:{message_id:id,status:entry.status,...(entry.err ? {err:entry.err} : {})}}); }
  private async ack(id: string, status: AckStatus, err?: string) {
    await this.mutate(() => { this.state.ledger[id] = {status, at:Date.now(), ...(err ? {err} : {})}; });
    this.sendAck(id, this.state.ledger[id]);
  }
  private async onFrame(frame: Frame, generation: number) {
    const d = frame.d || {};
    switch (frame.t) {
      case 'welcome': {
        const cls = d.class as {name?:unknown} | undefined;
        const serverTime = typeof d.server_time === 'number' ? (d.server_time < 1e12 ? d.server_time * 1000 : d.server_time) : Date.parse(str(d.server_time));
        if (Number.isFinite(serverTime)) this.serverOffset = serverTime - Date.now();
        const recalled = Array.isArray(d.recalled_ids) ? d.recalled_ids.filter((id): id is string => typeof id === 'string') : [];
        const recalledIDs = new Set(recalled);
        recalled.forEach(id => this.suppressed.add(id));
        if (this.view.current && recalledIDs.has(this.view.current.message_id)) this.active?.abort();
        try {
          await this.mutate(() => {
            this.state.className = str(cls?.name, this.state.className); this.state.volume = num(d.volume,this.state.volume);
            this.state.queue = this.state.queue.filter(item => {
              if (!recalledIDs.has(item.message_id)) return true;
              this.state.ledger[item.message_id] = {status:'dismissed',at:Date.now()}; return false;
            });
          });
        } finally { recalled.forEach(id => this.suppressed.delete(id)); }
        if (this.view.current && recalledIDs.has(this.view.current.message_id)) this.emit({current:null,startedAt:0});
        if (generation !== this.generation || this.socket?.readyState !== WebSocket.OPEN) break;
        this.helloPending.forEach(id => { if (!recalledIDs.has(id)) this.validated.add(id); });
        this.attempts = 0; this.emit({connection:'online',error:''});
        // The protocol has no ACK-of-ACK, so replay persisted receipts when reconnecting.
        Object.entries(this.state.ledger).forEach(([id,entry]) => this.sendAck(id,entry));
        void this.startNext(); break;
      }
      case 'notify': {
        const id = str(d.message_id, frame.id); if (!id) throw new Error('通知缺少消息 ID');
        const existing = this.state.ledger[id];
        await this.mutate(() => {
          this.state.lastSeq = Math.max(this.state.lastSeq, num(frame.seq,0));
          if (!this.state.ledger[id]) {
            this.state.queue.push({message_id:id,style:str(d.style,'notice'),text:str(d.text),student_name:str(d.student_name),signature:str(d.signature),class_name:str(d.class_name,this.state.className),audio_url:str(d.audio_url) || undefined,chime:d.chime !== false,repeat:num(d.repeat,1),display_sec:Math.max(1,Math.min(3600,num(d.display_sec,60))),volume:num(d.volume,this.state.volume),sent_at:str(d.sent_at) || undefined,expires_at:str(d.expires_at) || undefined});
            this.state.ledger[id] = {status:'delivered',at:Date.now()};
          }
          const cutoff = Date.now() - 30 * 86400000;
          for (const [key,value] of Object.entries(this.state.ledger)) if (value.at < cutoff && !this.state.queue.some(item => item.message_id === key)) delete this.state.ledger[key];
        });
        if (generation === this.generation) this.validated.add(id);
        const entry = this.state.ledger[id] || existing; if (entry) this.sendAck(id,entry); void this.startNext(); break;
      }
      case 'recall': {
        const id = str(d.message_id); if (!id) break;
        this.suppressed.add(id);
        if (this.view.current?.message_id === id) { this.active?.abort(); }
        try { await this.mutate(() => { this.state.queue = this.state.queue.filter(item => item.message_id !== id); this.state.ledger[id] = {status:'dismissed',at:Date.now()}; }); }
        finally { this.suppressed.delete(id); }
        this.sendAck(id,this.state.ledger[id]);
        if (this.view.current?.message_id === id) this.emit({current:null,startedAt:0});
        void this.startNext(); break;
      }
      case 'config': await this.mutate(() => { this.state.volume = num(d.volume,this.state.volume); }); break;
      case 'unbind': await this.unbind(); break;
      case 'tool': this.setTool(d); break;
      case 'ping': this.send({t:'pong',d:{}}); break;
    }
  }
  private setTool(data: Record<string,unknown>) {
    const params = (data.params || {}) as Record<string,unknown>;
    if (data.name === 'stop') this.emit({tool:null});
    if (data.name === 'countdown') {
      const seconds = Math.max(1,Math.min(86400,num(params.seconds,60)));
      const end = Date.now() + seconds * 1000;
      this.emit({tool:{name:'countdown',seconds,end}});
      setTimeout(() => { const tool = this.view.tool; if (tool?.name === 'countdown' && tool.end === end) void playChime(new AbortController().signal,this.state.volume).catch(error => this.emit({error:`倒计时提示音：${error.message}`})); },seconds*1000);
    }
    if (data.name === 'rollcall') {
      const names = Array.isArray(params.names) ? params.names.filter((name): name is string => typeof name === 'string' && !!name.trim()).map(name => name.trim()) : [];
      if (!names.length) { this.emit({error:'点名名单为空'}); return; }
      const random = new Uint32Array(1); crypto.getRandomValues(random);
      this.emit({tool:{name:'rollcall',names,selected:names[random[0] % names.length],ends:Date.now()+3000}});
    }
  }
  private async startNext() {
    if (this.active || !this.state.queue.length || this.disposed || this.suppressStarts || this.view.connection !== 'online' || this.socket?.readyState !== WebSocket.OPEN || this.suppressed.has(this.state.queue[0].message_id)) return;
    if (!this.validated.has(this.state.queue[0].message_id)) { this.socket.close(); return; }
    const notice = this.state.queue[0]; const controller = this.active = new AbortController();
    const sent = Date.parse(notice.sent_at || '');
    const expires = notice.expires_at ? Date.parse(notice.expires_at) : (Number.isFinite(sent) ? sent + 600000 : NaN);
    if (Number.isFinite(expires) && Date.now() + this.serverOffset >= expires) {
      let saved = false;
      try {
        await this.mutate(() => { this.state.queue = this.state.queue.filter(item => item.message_id !== notice.message_id); this.state.ledger[notice.message_id] = {status:'failed',at:Date.now(),err:'消息已过期'}; });
        saved = true; this.sendAck(notice.message_id,this.state.ledger[notice.message_id]);
      } catch { /* Leave the item for a later durable retry. */ }
      finally { if (this.active === controller) { this.active = undefined; if (saved) void this.startNext(); } }
      return;
    }
    this.emit({current:notice,startedAt:Date.now()});
    const display = sleep(notice.display_sec*1000,controller.signal);
    void display.catch(() => {});
    let retryNext = true;
    try {
      await this.ack(notice.message_id,'playing');
      try {
        await playNotice(notice, url => this.resolveAudio(url), controller.signal);
        if (!controller.signal.aborted) await this.ack(notice.message_id,'played');
      } catch (error) {
        if (controller.signal.aborted) throw error;
        const message = error instanceof Error ? error.message : String(error);
        this.emit({error:`播报失败：${message}`}); await this.ack(notice.message_id,'failed',message);
      }
      await display;
      await this.mutate(() => { this.state.queue = this.state.queue.filter(item => item.message_id !== notice.message_id); });
    } catch (error) { if (!abortError(error)) { retryNext = false; this.emit({error:`无法处理通知：${error instanceof Error ? error.message : String(error)}`}); } }
    finally {
      // Observe rejection when playback was cancelled before its display timer completed.
      void display.catch(() => {});
      if (this.active === controller) { this.active = undefined; if (this.view.current?.message_id === notice.message_id) this.emit({current:null,startedAt:0}); if (retryNext) void this.startNext(); }
    }
  }
  private resolveAudio(url: string): string {
    if (/^https?:\/\//i.test(url)) {
      const parsed = new URL(url); const target = this.state.server ? new URL(this.state.server) : new URL(window.location.origin);
      if (parsed.origin === target.origin) return `${this.base}${parsed.pathname}${parsed.search}`;
      return url;
    }
    return `${this.base}${url.startsWith('/') ? '' : '/'}${url}`;
  }
  async dismiss() {
    const id = this.view.current?.message_id; if (!id) return;
    this.suppressed.add(id); this.active?.abort();
    try { await this.mutate(() => { this.state.queue = this.state.queue.filter(item => item.message_id !== id); this.state.ledger[id] = {status:'dismissed',at:Date.now()}; }); }
    finally { this.suppressed.delete(id); }
    this.sendAck(id,this.state.ledger[id]); void this.startNext();
  }
  async unbind() {
    this.suppressStarts = true;
    this.generation++; this.active?.abort(); clearTimeout(this.retry); clearInterval(this.heartbeat); this.socket?.close();
    try { await this.mutate(() => { const server = this.state.server; this.state = {...fresh(),server}; }); }
    finally { this.suppressStarts = false; }
    this.emit({connection:'offline',current:null,startedAt:0,tool:null,error:'设备已解绑，请输入新的绑定码'});
  }
  closeTool() { this.emit({tool:null}); }
  dispose() { this.disposed = true; this.generation++; this.active?.abort(); clearTimeout(this.retry); clearInterval(this.heartbeat); this.socket?.close(); }
}
