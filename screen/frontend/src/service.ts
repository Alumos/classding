import type { components, paths } from '@classding/api/types';
import type { AckStatus, Notice, WSFrame } from '@classding/api/ws';
import { configureServer, getLocalAddresses, loadState, normalizeServer, saveState, native } from './native';
import { readHomeworks, type Homework } from './homework';
import { playChime, playNotice } from './audio';
import { APP_VERSION } from '@classding/ui/version';

export type { AckStatus, Notice } from '@classding/api/ws';
export type Connection = 'connecting' | 'online' | 'offline';
interface Ledger {
  status: AckStatus;
  at: number;
  err?: string;
}
interface RemoteConfig {
  server_url: string;
  revision: string;
}
// The previous server stays on record until the new one accepts this device.
interface Fallback {
  server: string;
  revision: string;
  until: number;
}
const FALLBACK_MS = 10 * 60 * 1000;
const MESSAGE_TTL_MS = 10 * 60 * 1000;
const LEDGER_RETENTION_MS = 30 * 24 * 60 * 60 * 1000;
const REQUEST_TIMEOUT_MS = 5000;
const HEARTBEAT_MS = 25000;
const SILENCE_MS = 60000;
// A socket that neither opens nor receives its welcome in time is abandoned and retried.
const CONNECT_TIMEOUT_MS = 10000;
const MAX_PENDING = 4096;
const CLIENT_OS = navigator.userAgent.includes('Windows') ? 'windows' : 'browser';
interface StoredState {
  homeworks: Homework[];
  homeworkRevision: number;
  version: 1;
  server: string;
  token: string;
  deviceName: string;
  className: string;
  volume: number;
  lastSeq: number;
  queue: Notice[];
  ledger: Record<string, Ledger>;
  deviceId: string;
  classId: string;
  remoteConfig?: RemoteConfig;
  appliedRevision: string;
  fallback?: Fallback;
}
export type Tool =
  | { name: 'countdown'; end: number; seconds: number }
  | { name: 'rollcall'; names: string[]; selected: string; ends: number }
  | null;
export interface Snapshot {
  studentCallSending: boolean;
  studentCallMessage: string;
  homeworks: Homework[];
  ready: boolean;
  busy: boolean;
  error: string;
  connection: Connection;
  bound: boolean;
  className: string;
  deviceName: string;
  server: string;
  volume: number;
  pending: number;
  current: Notice | null;
  startedAt: number;
  tool: Tool;
}
type Frame = Pick<WSFrame, 't' | 'id' | 'seq'> & { d?: Record<string, unknown> };
const frameTypes = [
  'hello',
  'ack',
  'welcome',
  'notify',
  'recall',
  'tool',
  'config',
  'unbind',
  'ping',
  'pong',
  'receipt',
  'presence',
  'homework',
] as const satisfies readonly WSFrame['t'][];
const parseFrame = (input: unknown): Frame | null => {
  if (typeof input !== 'object' || input === null || Array.isArray(input)) return null;
  const value = input as Record<string, unknown>;
  const t = frameTypes.find((type) => type === value.t);
  if (!t) return null;
  const d =
    typeof value.d === 'object' && value.d !== null && !Array.isArray(value.d)
      ? (value.d as Record<string, unknown>)
      : undefined;
  return {
    t,
    id: typeof value.id === 'string' ? value.id : undefined,
    seq: typeof value.seq === 'number' ? value.seq : undefined,
    d,
  };
};
const fresh = (): StoredState => ({
  homeworks: [],
  homeworkRevision: 0,
  version: 1,
  server: '',
  token: '',
  deviceName: '',
  className: '',
  volume: 80,
  lastSeq: 0,
  queue: [],
  ledger: {},
  deviceId: '',
  classId: '',
  appliedRevision: '',
});
const sleep = (ms: number, signal: AbortSignal) =>
  new Promise<void>((resolve, reject) => {
    if (signal.aborted) {
      reject(new DOMException('已取消', 'AbortError'));
      return;
    }
    const cancel = () => {
      clearTimeout(timer);
      reject(new DOMException('已取消', 'AbortError'));
    };
    const timer = setTimeout(() => {
      signal.removeEventListener('abort', cancel);
      resolve();
    }, ms);
    signal.addEventListener('abort', cancel, { once: true });
  });
const num = (value: unknown, fallback: number) =>
  typeof value === 'number' && Number.isFinite(value) ? value : fallback;
const str = (value: unknown, fallback = '') => (typeof value === 'string' ? value : fallback);
const errorText = (error: unknown) => (error instanceof Error ? error.message : String(error));
const abortError = (error: unknown) => error instanceof DOMException && error.name === 'AbortError';
const duration = (value: unknown) => Math.max(15, Math.min(600, num(value, 60)));
// Notices saved before expires_at existed fall back to the server's ten-minute lifetime.
const expiresAt = (notice: Notice) => {
  if (notice.expires_at) return Date.parse(notice.expires_at);
  const sent = Date.parse(notice.sent_at || '');
  return Number.isFinite(sent) ? sent + MESSAGE_TTL_MS : NaN;
};
const remoteConfig = (value: unknown): RemoteConfig | undefined => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
  const candidate = value as Record<string, unknown>;
  if (
    typeof candidate.server_url !== 'string' ||
    typeof candidate.revision !== 'string' ||
    !candidate.revision.trim()
  )
    return undefined;
  try {
    return { server_url: normalizeServer(candidate.server_url), revision: candidate.revision };
  } catch {
    return undefined;
  }
};

export class ScreenService {
  private state = fresh();
  private view: Snapshot = {
    studentCallSending: false,
    studentCallMessage: '',
    homeworks: [],
    ready: false,
    busy: false,
    error: '',
    connection: 'offline',
    bound: false,
    className: '',
    deviceName: '',
    server: '',
    volume: 80,
    pending: 0,
    current: null,
    startedAt: 0,
    tool: null,
  };
  private listeners = new Set<() => void>();
  private ops: Promise<void> = Promise.resolve();
  private socket?: WebSocket;
  private retry?: ReturnType<typeof setTimeout>;
  private homeworkTimer?: ReturnType<typeof setInterval>;
  private homeworkPolling = false;
  private heartbeat?: ReturnType<typeof setInterval>;
  private switchTimer?: ReturnType<typeof setTimeout>;
  private lanIPs: string[] = [];
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
  private bindingEpoch = 0;
  private callController?: AbortController;
  private callID = '';
  private connectTimer?: ReturnType<typeof setTimeout>;
  // Reconnect at once when Windows reports the network is back, instead of waiting out the backoff.
  private readonly onNetworkBack = () => {
    if (this.view.connection === 'online' || !this.state.token || this.disposed) return;
    this.attempts = 0;
    this.connect();
  };
  private callOff?: () => void;
  private callNativeQueue: Promise<void> = Promise.resolve();
  private callNativeKey = '';
  subscribe = (fn: () => void) => {
    this.listeners.add(fn);
    return () => {
      this.listeners.delete(fn);
    };
  };
  getSnapshot = () => this.view;
  private emit(patch: Partial<Snapshot> = {}) {
    const current = patch.current === undefined ? this.view.current : patch.current;
    this.view = {
      ...this.view,
      ...patch,
      bound: !!this.state.token,
      className: this.state.className,
      deviceName: this.state.deviceName,
      homeworks: this.state.homeworks,
      server: this.state.server,
      volume: this.state.volume,
      pending: Math.max(0, this.state.queue.length - (current ? 1 : 0)),
    };
    this.listeners.forEach((fn) => fn());
    this.syncStudentCall();
  }
  private mutate(change: () => void): Promise<void> {
    const operation = this.ops.then(async () => {
      const before = structuredClone(this.state);
      try {
        change();
        await saveState(JSON.stringify(this.state));
        this.emit();
      } catch (error) {
        this.state = before;
        this.emit({ error: `无法保存设备状态：${errorText(error)}` });
        this.socket?.close();
        throw error;
      }
    });
    this.ops = operation.catch(() => {});
    return operation;
  }
  private async setBase(server: string) {
    this.base = await configureServer(server);
  }
  private async request(path: string, init?: RequestInit): Promise<Response> {
    return fetch(`${this.base}${path}`, init);
  }
  // An authenticated device request that gives up after REQUEST_TIMEOUT_MS.
  private async deviceRequest(path: string): Promise<Response> {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);
    try {
      return await this.request(path, {
        headers: { Authorization: `Bearer ${this.state.token}` },
        signal: controller.signal,
      });
    } finally {
      clearTimeout(timer);
    }
  }
  async init() {
    window.addEventListener?.('online', this.onNetworkBack);
    this.callOff = window.runtime?.EventsOnMultiple(
      'student-call:send',
      (id, binding) => {
        if (typeof id !== 'string' || binding !== this.bindingEpoch) return;
        void this.sendStudentCall(id)
          .then(
            () => native()?.CompleteStudentCall(id, true, '呼叫已发送，本班老师打开网页即可收到提醒'),
            (error) => native()?.CompleteStudentCall(id, false, errorText(error)),
          )
          .catch(() => {});
      },
      -1,
    );
    try {
      const raw = await loadState();
      if (raw) {
        const saved = JSON.parse(raw) as Partial<StoredState>;
        if (
          saved.version !== 1 ||
          typeof saved.token !== 'string' ||
          !Array.isArray(saved.queue) ||
          typeof saved.ledger !== 'object' ||
          !saved.ledger
        )
          throw new Error('本地数据格式无法识别');
        this.state = { ...fresh(), ...saved } as StoredState;
        // A completed message must never be spoken again after a restart.
        this.state.queue = this.state.queue
          .filter(
            (item) => !['played', 'dismissed', 'failed'].includes(this.state.ledger[item.message_id]?.status),
          )
          .map((item) => ({ ...item, display_sec: duration(item.display_sec) }));
        this.state.homeworks = readHomeworks(this.state.homeworks, this.state.classId) ?? [];
        this.state.homeworkRevision = Math.max(0, num(this.state.homeworkRevision, 0));
        this.state.remoteConfig = remoteConfig(this.state.remoteConfig);
        if (
          !this.state.fallback ||
          typeof this.state.fallback.server !== 'string' ||
          typeof this.state.fallback.until !== 'number'
        )
          this.state.fallback = undefined;
      }
      await this.setBase(this.state.server);
      this.emit({ ready: true });
      if (this.state.token) {
        this.startHomeworkPolling();
        this.connect();
        this.scheduleServerSwitch();
      }
    } catch (error) {
      this.emit({
        ready: true,
        error: `初始化失败：${errorText(error)}`,
      });
    }
  }
  async bind(code: string, deviceName: string, server: string) {
    if (!/^\d{6}$/.test(code)) throw new Error('请输入 6 位数字绑定码');
    this.emit({ busy: true, error: '' });
    let serverAccepted = false;
    try {
      server = normalizeServer(server, true);
      await this.setBase(server);
      const payload: paths['/device/bind']['post']['requestBody']['content']['application/json'] = {
        code,
        name: deviceName.trim(),
        os: CLIENT_OS,
        client_version: APP_VERSION,
      };
      const response = await this.request('/api/v1/device/bind', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      const data = (await response.json().catch(() => ({}))) as Partial<
        components['schemas']['DeviceBinding'] & components['schemas']['Error']
      >;
      if (!response.ok) throw new Error(data.message || `绑定失败（${response.status}）`);
      if (
        !data.device_token ||
        !data.class?.name ||
        !data.class.id ||
        !data.device?.id ||
        data.device.class_id !== data.class.id
      )
        throw new Error('服务器返回的绑定信息不完整');
      serverAccepted = true;
      this.resetStudentCall();
      await this.mutate(() => {
        this.state = {
          ...fresh(),
          server,
          token: data.device_token!,
          deviceId: data.device!.id,
          classId: data.class!.id,
          deviceName: data.device!.name,
          className: data.class!.name,
          volume: num(data.volume, 80),
        };
        this.applyHomeworkSnapshot(data.homeworks, data.homework_revision);
      });
      this.startHomeworkPolling();
      this.attempts = 0;
      this.connect();
    } catch (error) {
      const reason = error instanceof TypeError ? '无法连接服务器，请检查地址和网络' : errorText(error);
      const message = serverAccepted
        ? `服务器已绑定设备，但本机保存失败：${reason}。请在教师端解绑该设备后重新绑定。`
        : reason;
      this.emit({ error: message });
      throw error;
    } finally {
      this.emit({ busy: false });
    }
  }
  private connect() {
    if (!this.state.token || this.disposed) return;
    clearTimeout(this.retry);
    clearTimeout(this.connectTimer);
    clearInterval(this.heartbeat);
    const generation = ++this.generation;
    this.validated.clear();
    // Refreshed on every connection; the next hello reports a changed school network address.
    void getLocalAddresses()
      .then((addresses) => {
        this.lanIPs = addresses;
      })
      .catch(() => {});
    const base = this.base || window.location.origin;
    const url = new URL(`${base}/ws/device`, window.location.href);
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
    this.emit({ connection: 'connecting' });
    try {
      const ws = (this.socket = new WebSocket(url));
      this.connectTimer = setTimeout(() => {
        if (generation === this.generation && this.view.connection !== 'online') ws.close();
      }, CONNECT_TIMEOUT_MS);
      ws.onopen = () => {
        if (generation !== this.generation) return;
        this.lastFrame = Date.now();
        void this.hello(ws, generation).catch((error) =>
          this.emit({ error: `准备连接失败：${errorText(error)}` }),
        );
      };
      ws.onmessage = (event) => {
        if (generation !== this.generation) return;
        this.lastFrame = Date.now();
        try {
          const frame = parseFrame(JSON.parse(String(event.data)) as unknown);
          if (frame)
            void this.onFrame(frame, generation).catch((error) =>
              this.emit({ error: `处理消息失败：${errorText(error)}` }),
            );
        } catch {
          this.emit({ error: '收到无法识别的服务器消息' });
        }
      };
      ws.onerror = () => {
        /* close handler schedules retry */
      };
      ws.onclose = () => {
        if (generation !== this.generation) return;
        clearTimeout(this.connectTimer);
        clearInterval(this.heartbeat);
        this.emit({ connection: 'offline' });
        void this.verifyAfterClose(generation);
      };
    } catch {
      this.emit({ connection: 'offline' });
      this.scheduleRetry();
    }
  }
  private async hello(ws: WebSocket, generation: number) {
    const now = Date.now() + this.serverOffset;
    const expired = new Set(
      this.state.queue
        .filter((item) => {
          const expires = expiresAt(item);
          return (
            item.message_id !== this.view.current?.message_id && Number.isFinite(expires) && now >= expires
          );
        })
        .map((item) => item.message_id),
    );
    if (expired.size)
      await this.mutate(() => {
        expired.forEach((id) => this.settle(id, 'failed', '消息已过期'));
      });
    if (generation !== this.generation || ws.readyState !== WebSocket.OPEN) return;
    const pendingIDs = this.state.queue.slice(0, MAX_PENDING).map((item) => item.message_id);
    ws.send(
      JSON.stringify({
        t: 'hello',
        d: {
          token: this.state.token,
          last_seq: this.state.lastSeq,
          client_version: APP_VERSION,
          os: CLIENT_OS,
          pending_ids: pendingIDs,
          lan_ips: this.lanIPs,
          server_url: this.state.server || window.location.origin,
          applied_revision: this.state.appliedRevision,
        },
      }),
    );
    this.helloPending = pendingIDs;
    this.heartbeat = setInterval(() => {
      if (Date.now() - this.lastFrame > SILENCE_MS) {
        ws.close();
        return;
      }
      this.send({ t: 'ping', d: {} });
    }, HEARTBEAT_MS);
  }
  private async verifyAfterClose(generation: number) {
    try {
      const response = await this.deviceRequest('/api/v1/device/me');
      if (generation !== this.generation || !this.state.token) return;
      if (response.status === 401 || response.status === 403) {
        await this.ops;
        if (generation !== this.generation || !this.state.token) return;
        // A new server that does not know this device sends it back instead of unbinding it.
        if (this.state.fallback) {
          await this.revertServer('不认可本设备');
          return;
        }
        await this.unbind();
        return;
      } else if (response.ok && typeof response.json === 'function') {
        const data = (await response.json()) as {
          device?: { id?: unknown; class_id?: unknown; name?: unknown; remote_config?: unknown };
          class?: { id?: unknown; name?: unknown };
          sequence?: unknown;
          remote_config?: unknown;
          homeworks?: unknown;
          homework_revision?: unknown;
        };
        if (generation !== this.generation) return;
        await this.mutate(() => {
          if (generation !== this.generation) return;
          const id = str(data.device?.id);
          const classId = str(data.device?.class_id, str(data.class?.id));
          if (
            (!this.state.deviceId || this.state.deviceId === id) &&
            (!this.state.classId || this.state.classId === classId)
          ) {
            this.state.deviceId ||= id;
            this.state.classId ||= classId;
            this.state.deviceName = str(data.device?.name, this.state.deviceName);
            this.state.className = str(data.class?.name, this.state.className);
            this.applyHomeworkSnapshot(data.homeworks, data.homework_revision);
            if (data.device && Object.hasOwn(data.device, 'remote_config'))
              this.setRemoteConfig(data.device.remote_config);
            else if (Object.hasOwn(data, 'remote_config')) this.setRemoteConfig(data.remote_config);
          }
        });
        if (this.state.remoteConfig) {
          this.scheduleServerSwitch();
          return;
        }
      }
    } catch {
      /* Network errors must not erase an otherwise valid binding. */
    }
    if (generation === this.generation) this.scheduleRetry();
  }
  private applyHomeworkSnapshot(items: unknown, revision: unknown) {
    if (
      typeof revision !== 'number' ||
      !Number.isSafeInteger(revision) ||
      revision < 0 ||
      revision < this.state.homeworkRevision
    )
      return;
    const parsed = readHomeworks(items, this.state.classId);
    if (parsed === null) return;
    this.state.homeworks = parsed;
    this.state.homeworkRevision = revision;
  }
  private startHomeworkPolling() {
    clearInterval(this.homeworkTimer);
    this.homeworkTimer = setInterval(() => {
      void this.refreshHomeworks();
    }, 30000);
  }
  private async refreshHomeworks(force = false) {
    if (
      this.homeworkPolling ||
      this.disposed ||
      !this.state.token ||
      (!force && this.view.connection !== 'online')
    )
      return;
    this.homeworkPolling = true;
    const generation = this.generation;
    try {
      const response = await this.deviceRequest('/api/v1/device/homeworks');
      if (!response.ok) return;
      const data = (await response.json()) as { items?: unknown; homework_revision?: unknown };
      if (generation !== this.generation || this.disposed) return;
      await this.mutate(() => {
        if (generation === this.generation) this.applyHomeworkSnapshot(data.items, data.homework_revision);
      });
    } catch {
      /* The live connection and cached assignments remain usable during network failures. */
    } finally {
      this.homeworkPolling = false;
    }
  }
  private scheduleRetry() {
    if (!this.state.token || this.disposed) return;
    if (this.state.fallback && Date.now() > this.state.fallback.until) {
      void this.revertServer('长时间无法连接');
      return;
    }
    const delay = Math.min(30000, 1000 * 2 ** Math.min(this.attempts++, 5)) * (0.7 + Math.random() * 0.6);
    this.retry = setTimeout(() => this.connect(), delay);
  }
  private setRemoteConfig(value: unknown) {
    if (value === null) {
      this.state.remoteConfig = undefined;
      return;
    }
    const plan = remoteConfig(value);
    if (!plan) return;
    if (plan.revision === this.state.appliedRevision || plan.server_url === this.state.server) {
      if (plan.server_url === this.state.server) this.state.appliedRevision = plan.revision;
      this.state.remoteConfig = undefined;
    } else this.state.remoteConfig = plan;
  }
  private scheduleServerSwitch(delay = 0) {
    clearTimeout(this.switchTimer);
    if (this.disposed || !this.state.token || !this.state.remoteConfig) return;
    this.switchTimer = setTimeout(() => {
      void this.applyRemoteServer();
    }, delay);
  }
  // A new server address from the administrator is applied directly; the
  // device leaves the old server and keeps it only as a fallback.
  private async applyRemoteServer() {
    const plan = this.state.remoteConfig;
    if (!plan || this.disposed) return;
    // Let an active presentation finish; switching never interrupts a student reading.
    if (this.active) {
      this.scheduleServerSwitch(5000);
      return;
    }
    await this.ops;
    if (this.disposed || this.active || this.state.remoteConfig !== plan) return;
    const previous = this.state.server || window.location.origin;
    try {
      await this.mutate(() => {
        this.state.fallback = { server: previous, revision: plan.revision, until: Date.now() + FALLBACK_MS };
        this.state.server = plan.server_url;
        this.state.remoteConfig = undefined;
        this.state.appliedRevision = plan.revision;
      });
      this.base = await configureServer(plan.server_url);
    } catch (error) {
      this.emit({ error: `切换服务器失败：${errorText(error)}` });
      return;
    }
    this.reconnect();
  }
  private async revertServer(reason: string) {
    const fallback = this.state.fallback;
    if (!fallback || this.disposed) return;
    clearTimeout(this.retry);
    try {
      await this.mutate(() => {
        this.state.server = fallback.server;
        this.state.fallback = undefined;
      });
      this.base = await configureServer(fallback.server);
    } catch {
      /* Keep retrying with the persisted server. */
    }
    this.emit({ error: `新服务器${reason}，已回到原服务器` });
    this.reconnect();
  }
  // Abandons the current socket and connects again right away.
  private reconnect() {
    this.generation++;
    this.socket?.close();
    this.attempts = 0;
    this.connect();
  }
  // Drops the notice from the queue and records how it ended. Call inside mutate.
  private settle(id: string, status: AckStatus, err?: string) {
    this.state.queue = this.state.queue.filter((item) => item.message_id !== id);
    this.state.ledger[id] = { status, at: Date.now(), ...(err ? { err } : {}) };
  }
  private send(frame: object) {
    if (this.socket?.readyState === WebSocket.OPEN) this.socket.send(JSON.stringify(frame));
  }
  private sendAck(id: string, entry: Ledger) {
    this.send({
      t: 'ack',
      d: { message_id: id, status: entry.status, ...(entry.err ? { err: entry.err } : {}) },
    });
  }
  private async ack(id: string, status: AckStatus, err?: string) {
    await this.mutate(() => {
      this.state.ledger[id] = { status, at: Date.now(), ...(err ? { err } : {}) };
    });
    this.sendAck(id, this.state.ledger[id]);
  }
  private async onFrame(frame: Frame, generation: number) {
    if (generation !== this.generation) return;
    const d = frame.d || {};
    switch (frame.t) {
      case 'welcome': {
        const cls = d.class as { id?: unknown; name?: unknown } | undefined;
        const device = d.device as { id?: unknown; class_id?: unknown; name?: unknown } | undefined;
        const serverTime =
          typeof d.server_time === 'number'
            ? d.server_time < 1e12
              ? d.server_time * 1000
              : d.server_time
            : Date.parse(str(d.server_time));
        if (Number.isFinite(serverTime)) this.serverOffset = serverTime - Date.now();
        const recalled = Array.isArray(d.recalled_ids)
          ? d.recalled_ids.filter((id): id is string => typeof id === 'string')
          : [];
        const recalledIDs = new Set(recalled);
        recalled.forEach((id) => this.suppressed.add(id));
        if (this.view.current && recalledIDs.has(this.view.current.message_id)) this.active?.abort();
        try {
          await this.mutate(() => {
            if (generation !== this.generation) return;
            // The server accepted this device, so the switch is complete.
            this.state.fallback = undefined;
            this.applyHomeworkSnapshot(d.homeworks, d.homework_revision);
            this.state.className = str(cls?.name, this.state.className);
            this.state.volume = num(d.volume, this.state.volume);
            const id = str(device?.id);
            const classId = str(device?.class_id, str(cls?.id));
            if (
              (!this.state.deviceId || this.state.deviceId === id) &&
              (!this.state.classId || this.state.classId === classId)
            ) {
              this.state.deviceId ||= id;
              this.state.classId ||= classId;
              this.state.deviceName = str(device?.name, this.state.deviceName);
            }
            if (Object.hasOwn(d, 'remote_config')) this.setRemoteConfig(d.remote_config);
            this.state.queue
              .filter((item) => recalledIDs.has(item.message_id))
              .forEach((item) => this.settle(item.message_id, 'dismissed'));
          });
        } finally {
          recalled.forEach((id) => this.suppressed.delete(id));
        }
        this.scheduleServerSwitch();
        if (this.view.current && recalledIDs.has(this.view.current.message_id))
          this.emit({ current: null, startedAt: 0 });
        if (generation !== this.generation || this.socket?.readyState !== WebSocket.OPEN) break;
        this.helloPending.forEach((id) => {
          if (!recalledIDs.has(id)) this.validated.add(id);
        });
        this.attempts = 0;
        this.emit({ connection: 'online', error: '' });
        // The protocol has no ACK-of-ACK, so replay persisted receipts when reconnecting.
        Object.entries(this.state.ledger).forEach(([id, entry]) => this.sendAck(id, entry));
        void this.startNext();
        break;
      }
      case 'notify': {
        const id = str(d.message_id, frame.id);
        if (!id) throw new Error('通知缺少消息 ID');
        const existing = this.state.ledger[id];
        await this.mutate(() => {
          if (generation !== this.generation) return;
          this.state.lastSeq = Math.max(this.state.lastSeq, num(frame.seq, 0));
          if (!this.state.ledger[id]) {
            this.state.queue.push({
              message_id: id,
              style: str(d.style, 'notice'),
              text: str(d.text),
              student_name: str(d.student_name),
              signature: str(d.signature),
              class_name: str(d.class_name, this.state.className),
              audio_url: str(d.audio_url) || undefined,
              chime: d.style === 'urgent' && d.chime === true,
              repeat: num(d.repeat, 1),
              display_sec: duration(d.display_sec),
              volume: num(d.volume, this.state.volume),
              sent_at: str(d.sent_at) || undefined,
              expires_at: str(d.expires_at) || undefined,
            });
            this.state.ledger[id] = { status: 'delivered', at: Date.now() };
          }
          const cutoff = Date.now() - LEDGER_RETENTION_MS;
          for (const [key, value] of Object.entries(this.state.ledger))
            if (value.at < cutoff && !this.state.queue.some((item) => item.message_id === key))
              delete this.state.ledger[key];
        });
        if (generation === this.generation) this.validated.add(id);
        const entry = this.state.ledger[id] || existing;
        if (entry) this.sendAck(id, entry);
        void this.startNext();
        break;
      }
      case 'recall': {
        const id = str(d.message_id);
        if (!id) break;
        this.suppressed.add(id);
        if (this.view.current?.message_id === id) {
          this.active?.abort();
        }
        try {
          await this.mutate(() => this.settle(id, 'dismissed'));
        } finally {
          this.suppressed.delete(id);
        }
        this.sendAck(id, this.state.ledger[id]);
        if (this.view.current?.message_id === id) this.emit({ current: null, startedAt: 0 });
        void this.startNext();
        break;
      }
      case 'homework':
        await this.mutate(() => {
          if (generation === this.generation) this.applyHomeworkSnapshot(d.items, d.homework_revision);
        });
        break;
      case 'config':
        await this.mutate(() => {
          if (generation !== this.generation) return;
          this.state.volume = num(d.volume, this.state.volume);
          if (Object.hasOwn(d, 'remote_config')) this.setRemoteConfig(d.remote_config);
        });
        if (Object.hasOwn(d, 'remote_config')) this.scheduleServerSwitch();
        break;
      case 'unbind':
        await this.unbind();
        break;
      case 'tool':
        this.setTool(d);
        break;
      case 'ping':
        this.send({ t: 'pong', d: {} });
        break;
    }
  }
  private setTool(data: Record<string, unknown>) {
    const params = (data.params || {}) as Record<string, unknown>;
    if (data.name === 'stop') this.emit({ tool: null });
    if (data.name === 'countdown') {
      const seconds = Math.max(1, Math.min(86400, num(params.seconds, 60)));
      const end = Date.now() + seconds * 1000;
      this.emit({ tool: { name: 'countdown', seconds, end } });
      setTimeout(() => {
        const tool = this.view.tool;
        if (tool?.name === 'countdown' && tool.end === end)
          void playChime(new AbortController().signal, this.state.volume).catch((error) =>
            this.emit({ error: `倒计时提示音：${error.message}` }),
          );
      }, seconds * 1000);
    }
    if (data.name === 'rollcall') {
      const names = Array.isArray(params.names)
        ? params.names
            .filter((name): name is string => typeof name === 'string' && !!name.trim())
            .map((name) => name.trim())
        : [];
      if (!names.length) {
        this.emit({ error: '点名名单为空' });
        return;
      }
      const random = new Uint32Array(1);
      crypto.getRandomValues(random);
      this.emit({
        tool: { name: 'rollcall', names, selected: names[random[0] % names.length], ends: Date.now() + 3000 },
      });
    }
  }
  private async startNext() {
    if (
      this.active ||
      !this.state.queue.length ||
      this.disposed ||
      this.suppressStarts ||
      this.view.connection !== 'online' ||
      this.socket?.readyState !== WebSocket.OPEN ||
      this.suppressed.has(this.state.queue[0].message_id)
    )
      return;
    if (!this.validated.has(this.state.queue[0].message_id)) {
      this.socket.close();
      return;
    }
    const notice = this.state.queue[0];
    const controller = (this.active = new AbortController());
    const expires = expiresAt(notice);
    if (Number.isFinite(expires) && Date.now() + this.serverOffset >= expires) {
      let saved = false;
      try {
        await this.mutate(() => this.settle(notice.message_id, 'failed', '消息已过期'));
        saved = true;
        this.sendAck(notice.message_id, this.state.ledger[notice.message_id]);
      } catch {
        /* Leave the item for a later durable retry. */
      } finally {
        if (this.active === controller) {
          this.active = undefined;
          if (saved) void this.startNext();
        }
      }
      return;
    }
    this.emit({ current: notice, startedAt: Date.now() });
    const display = sleep(notice.display_sec * 1000, controller.signal);
    void display.catch(() => {});
    let retryNext = true;
    try {
      const playback = (async () => {
        await this.ack(notice.message_id, 'playing');
        if (controller.signal.aborted) return;
        try {
          await playNotice(notice, (url) => this.resolveAudio(url), controller.signal);
          if (!controller.signal.aborted) await this.ack(notice.message_id, 'played');
        } catch (error) {
          if (controller.signal.aborted) return;
          const message = errorText(error);
          this.emit({ error: `播报失败：${message}` });
          await this.ack(notice.message_id, 'failed', message);
        }
      })();
      // The teacher's display timer also bounds long or repeated speech.
      void playback.catch(() => {});
      await Promise.race([display, playback.then(() => display)]);
      controller.abort();
      await playback;
      await this.mutate(() => {
        this.state.queue = this.state.queue.filter((item) => item.message_id !== notice.message_id);
      });
    } catch (error) {
      if (!abortError(error)) {
        retryNext = false;
        this.emit({ error: `无法处理通知：${errorText(error)}` });
      }
    } finally {
      // Observe rejection when playback was cancelled before its display timer completed.
      void display.catch(() => {});
      if (this.active === controller) {
        this.active = undefined;
        if (this.view.current?.message_id === notice.message_id) this.emit({ current: null, startedAt: 0 });
        if (this.state.remoteConfig) this.scheduleServerSwitch();
        else if (retryNext) void this.startNext();
      }
    }
  }
  private resolveAudio(url: string): string {
    if (/^https?:\/\//i.test(url)) {
      const parsed = new URL(url);
      const target = this.state.server ? new URL(this.state.server) : new URL(window.location.origin);
      if (parsed.origin === target.origin) return `${this.base}${parsed.pathname}${parsed.search}`;
      return url;
    }
    return `${this.base}${url.startsWith('/') ? '' : '/'}${url}`;
  }
  private syncStudentCall() {
    if (this.disposed || !native()) return;
    const { bound, connection, className } = this.view;
    const online = connection === 'online';
    const epoch = this.bindingEpoch;
    const key = JSON.stringify([bound, online, className, epoch]);
    if (key === this.callNativeKey) return;
    this.callNativeKey = key;
    this.callNativeQueue = this.callNativeQueue
      .catch(() => {})
      .then(async () => {
        if (!this.disposed) await native()?.UpdateStudentCallStatus(bound, online, className, epoch);
      })
      .catch((error) => {
        this.emit({ error: `学生呼叫入口更新失败：${errorText(error)}` });
      });
  }
  private resetStudentCall() {
    this.bindingEpoch++;
    this.callController?.abort();
    this.callController = undefined;
    this.callID = '';
    this.emit({ studentCallSending: false, studentCallMessage: '' });
  }
  // Retry an uncertain request with its original id, never queue calls offline.
  async sendStudentCall(requestID?: string): Promise<void> {
    if (this.disposed || !this.state.token) throw new Error('设备未绑定，请重新绑定后呼叫');
    if (this.view.studentCallSending) throw new Error('呼叫正在发送，请勿重复点击');
    if (this.view.connection !== 'online') throw new Error('连接中断，呼叫未发送。请恢复网络后重试');
    const id = this.callID || requestID || crypto.randomUUID();
    if (!/^[A-Za-z0-9_.:-]{1,128}$/.test(id)) throw new Error('呼叫编号无效');
    this.callID = id;
    const epoch = this.bindingEpoch;
    const controller = new AbortController();
    this.callController = controller;
    this.emit({ studentCallSending: true, studentCallMessage: '正在发送，请稍候…' });
    const timeout = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);
    try {
      const response = await this.request('/api/v1/device/student-calls', {
        method: 'POST',
        headers: { Authorization: `Bearer ${this.state.token}`, 'Content-Type': 'application/json' },
        body: JSON.stringify({ request_id: id }),
        signal: controller.signal,
      });
      const data = (await response.json().catch(() => ({}))) as Record<string, unknown>;
      if (epoch !== this.bindingEpoch || this.disposed) throw new Error('设备绑定已变化，呼叫结果不再适用');
      if (!response.ok) {
        if (response.status === 401) {
          await this.unbind();
          throw new Error('设备已失效或被解绑，请重新绑定');
        }
        if (response.status === 403) throw new Error('设备授权不可用，请联系学校管理员');
        if (response.status === 429) {
          this.callID = '';
          const seconds = response.headers?.get('Retry-After');
          throw new Error(
            seconds && /^\d+$/.test(seconds)
              ? '本班刚刚发起过呼叫，请在 ' + seconds + ' 秒后重试'
              : '本班刚刚发起过呼叫，请稍后重试',
          );
        }
        if (response.status === 409) this.callID = '';
        throw new Error(
          typeof data.message === 'string' ? data.message : '发送失败（' + response.status + '），请重试',
        );
      }
      if (
        typeof data.id !== 'string' ||
        !data.id ||
        data.class_id !== this.state.classId ||
        data.device_id !== this.state.deviceId ||
        typeof data.created_at !== 'string' ||
        !Number.isFinite(Date.parse(data.created_at))
      )
        throw new Error('服务器未确认有效呼叫，结果未知，请重试（不会重复创建同一呼叫）');
      this.callID = '';
      this.emit({ studentCallMessage: '呼叫已发送，本班老师打开网页即可收到提醒' });
    } catch (error) {
      const message =
        abortError(error) || error instanceof TypeError
          ? '网络异常，发送结果未确认，请重试（不会重复创建同一呼叫）'
          : errorText(error);
      if (epoch === this.bindingEpoch && !this.disposed) this.emit({ studentCallMessage: message });
      throw new Error(message);
    } finally {
      clearTimeout(timeout);
      if (this.callController === controller) this.callController = undefined;
      if (epoch === this.bindingEpoch && !this.disposed) this.emit({ studentCallSending: false });
    }
  }
  async dismiss() {
    const id = this.view.current?.message_id;
    if (!id) return;
    this.suppressed.add(id);
    this.active?.abort();
    try {
      await this.mutate(() => this.settle(id, 'dismissed'));
    } finally {
      this.suppressed.delete(id);
    }
    this.sendAck(id, this.state.ledger[id]);
    void this.startNext();
  }
  // Stops playback, timers and the socket; the next connect starts fresh.
  private halt() {
    this.resetStudentCall();
    this.generation++;
    this.active?.abort();
    clearTimeout(this.retry);
    clearInterval(this.heartbeat);
    clearInterval(this.homeworkTimer);
    clearTimeout(this.switchTimer);
    this.socket?.close();
  }
  async unbind() {
    this.suppressStarts = true;
    this.halt();
    try {
      await this.mutate(() => {
        const server = this.state.server;
        this.state = { ...fresh(), server };
      });
    } finally {
      this.suppressStarts = false;
    }
    this.emit({
      connection: 'offline',
      current: null,
      startedAt: 0,
      tool: null,
      error: '设备已解绑，请输入新的绑定码',
    });
  }
  closeTool() {
    this.emit({ tool: null });
  }
  dispose() {
    this.disposed = true;
    window.removeEventListener?.('online', this.onNetworkBack);
    clearTimeout(this.connectTimer);
    this.callOff?.();
    this.callOff = undefined;
    this.halt();
  }
}
