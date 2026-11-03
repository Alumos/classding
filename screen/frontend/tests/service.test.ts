import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';

const mocks = vi.hoisted(() => ({
  disk: '',
  play: vi.fn((_notice:unknown, _url:unknown, signal:AbortSignal) => new Promise<void>((_resolve,reject) => {
    if (signal.aborted) { reject(new DOMException('取消','AbortError')); return; }
    signal.addEventListener('abort',() => reject(new DOMException('取消','AbortError')),{once:true});
  })),
}));
vi.mock('../src/native', () => ({
  loadState: async () => mocks.disk,
  saveState: async (value:string) => { mocks.disk = value; },
  configureServer: async (server:string) => server,
}));
vi.mock('../src/audio', () => ({playNotice:mocks.play,playChime:vi.fn(async () => {})}));
import {ScreenService} from '../src/service';

class Socket {
  static OPEN = 1;
  static instances: Socket[] = [];
  readyState = 1;
  sent: Array<{t:string;d:Record<string,unknown>}> = [];
  onopen?: () => void;
  onmessage?: (event:{data:string}) => void;
  onclose?: () => void;
  onerror?: () => void;
  constructor(public url:string) { Socket.instances.push(this); }
  send(text:string) { this.sent.push(JSON.parse(text)); }
  open() { this.onopen?.(); }
  frame(value:object) { this.onmessage?.({data:JSON.stringify(value)}); }
  close() { this.readyState = 3; this.onclose?.(); }
}
const welcome = {t:'welcome',d:{class:{id:'c1',name:'三年二班'},volume:70}};
const notice = {t:'notify',seq:42,d:{message_id:'m1',text:'请到办公室',style:'notice',class_name:'三年二班',chime:true,repeat:1,display_sec:60,volume:70}};
let services:ScreenService[] = [];
function makeService() { const service = new ScreenService(); services.push(service); return service; }
function socket() { return Socket.instances.at(-1)!; }

beforeEach(() => {
  mocks.disk = ''; mocks.play.mockClear(); Socket.instances = []; services = [];
  vi.stubGlobal('window',{location:{origin:'http://localhost:5175',href:'http://localhost:5175/'}});
  vi.stubGlobal('navigator',{userAgent:'test browser'});
  vi.stubGlobal('WebSocket',Socket);
});
afterEach(() => { services.forEach(service=>service.dispose()); vi.unstubAllGlobals(); });

describe('大屏消息可靠性', () => {
  it('忽略格式无效的消息封装，并继续接收合法通知', async () => {
    mocks.disk = JSON.stringify({version:1,server:'',token:'device-token',deviceName:'教室',className:'三年二班',volume:70,lastSeq:0,queue:[],ledger:{}});
    const service = makeService(); await service.init(); socket().open();
    for (const value of [null,[],42,{t:[],d:{}}]) socket().onmessage?.({data:JSON.stringify(value)});
    await new Promise(resolve => setTimeout(resolve,0));
    expect(service.getSnapshot().error).toBe('');
    socket().frame(welcome); socket().frame(notice);
    await vi.waitFor(() => expect(mocks.play).toHaveBeenCalledTimes(1));
  });

  it('成功绑定后保存设备令牌并通过 hello 鉴权', async () => {
    const fetchMock = vi.fn(async () => ({ok:true,status:200,json:async () => ({device_token:'secret',device:{name:'一号屏'},class:{id:'c1',name:'三年二班'},volume:70})}));
    vi.stubGlobal('fetch',fetchMock);
    const service = makeService(); await service.init(); await service.bind('123456','一号屏','');
    expect(JSON.parse(mocks.disk).token).toBe('secret');
    expect(JSON.parse(fetchMock.mock.calls[0][1].body).code).toBe('123456');
    socket().open();
    expect(socket().sent[0]).toMatchObject({t:'hello',d:{token:'secret',last_seq:0}});
  });

  it('重复通知只播一次，重启后按持久队列恢复并上报最后序号', async () => {
    mocks.disk = JSON.stringify({version:1,server:'',token:'device-token',deviceName:'教室',className:'三年二班',volume:70,lastSeq:0,queue:[],ledger:{}});
    const first = makeService(); await first.init(); socket().open(); socket().frame(welcome); socket().frame(notice); socket().frame(notice);
    await vi.waitFor(() => expect(mocks.play).toHaveBeenCalledTimes(1));
    await vi.waitFor(() => expect(JSON.parse(mocks.disk).ledger.m1.status).toBe('playing'));
    expect(JSON.parse(mocks.disk).queue).toHaveLength(1);
    expect(JSON.parse(mocks.disk).lastSeq).toBe(42);
    expect(socket().sent.some(frame=>frame.t==='ack' && frame.d.message_id==='m1')).toBe(true);
    first.dispose();
    const second = makeService(); await second.init(); socket().open();
    expect(socket().sent[0]).toMatchObject({t:'hello',d:{token:'device-token',last_seq:42}});
    expect(mocks.play).toHaveBeenCalledTimes(1);
    socket().frame(welcome);
    await vi.waitFor(() => expect(mocks.play).toHaveBeenCalledTimes(2));
  });

  it('重连时向服务器提交待播 ID，并在播放前处理离线撤回', async () => {
    mocks.disk = JSON.stringify({version:1,server:'',token:'device-token',deviceName:'教室',className:'三年二班',volume:70,lastSeq:42,queue:[notice.d],ledger:{m1:{status:'delivered',at:Date.now()}}});
    const service = makeService(); await service.init(); socket().open();
    expect(socket().sent[0]).toMatchObject({t:'hello',d:{pending_ids:['m1']}});
    socket().frame({t:'welcome',d:{class:{name:'三年二班'},volume:70,recalled_ids:['m1']}});
    await vi.waitFor(() => expect(JSON.parse(mocks.disk).ledger.m1.status).toBe('dismissed'));
    expect(JSON.parse(mocks.disk).queue).toHaveLength(0);
    expect(mocks.play).not.toHaveBeenCalled();
  });

  it('断线后暂停下一条，重连确认后不播离线撤回的消息', async () => {
    const second = {...notice.d,message_id:'m2'};
    mocks.disk = JSON.stringify({version:1,server:'',token:'device-token',deviceName:'教室',className:'三年二班',volume:70,lastSeq:42,queue:[notice.d,second],ledger:{m1:{status:'delivered',at:Date.now()},m2:{status:'delivered',at:Date.now()}}});
    vi.stubGlobal('fetch',vi.fn(async () => ({ok:true,status:200})));
    const service = makeService(); await service.init(); socket().open(); socket().frame(welcome);
    await vi.waitFor(() => expect(mocks.play).toHaveBeenCalledTimes(1));
    socket().close(); await service.dismiss();
    expect(mocks.play).toHaveBeenCalledTimes(1);
    await vi.waitFor(() => expect(Socket.instances).toHaveLength(2),{timeout:2000});
    socket().open(); socket().frame({t:'welcome',d:{class:{name:'三年二班'},volume:70,recalled_ids:['m2']}});
    await vi.waitFor(() => expect(JSON.parse(mocks.disk).ledger.m2.status).toBe('dismissed'));
    expect(mocks.play).toHaveBeenCalledTimes(1);
  });

  it('恢复队列时不播过期消息，并报告失败', async () => {
    const expired = {...notice.d,sent_at:new Date(Date.now()-11*60*1000).toISOString()};
    mocks.disk = JSON.stringify({version:1,server:'',token:'device-token',deviceName:'教室',className:'三年二班',volume:70,lastSeq:42,queue:[expired],ledger:{m1:{status:'delivered',at:Date.now()}}});
    const service = makeService(); await service.init(); socket().open(); socket().frame(welcome);
    await vi.waitFor(() => expect(JSON.parse(mocks.disk).ledger.m1.status).toBe('failed'));
    expect(JSON.parse(mocks.disk).queue).toHaveLength(0);
    expect(mocks.play).not.toHaveBeenCalled();
    expect(socket().sent.some(frame=>frame.t==='ack' && frame.d.status==='failed')).toBe(true);
  });

  it('发送 hello 前清理大量过期消息，避免超过服务器的待播 ID 上限', async () => {
    const past = new Date(Date.now()-1000).toISOString();
    const expired = Array.from({length:4097},(_,i) => ({...notice.d,message_id:`old${i}`,expires_at:past}));
    const valid = {...notice.d,message_id:'valid',expires_at:new Date(Date.now()+60000).toISOString()};
    const ledger = Object.fromEntries([...expired,valid].map(item => [item.message_id,{status:'delivered',at:Date.now()}]));
    mocks.disk = JSON.stringify({version:1,server:'',token:'device-token',deviceName:'教室',className:'三年二班',volume:70,lastSeq:42,queue:[...expired,valid],ledger});
    const service = makeService(); await service.init(); socket().open();
    await vi.waitFor(() => expect(socket().sent[0]).toMatchObject({t:'hello',d:{pending_ids:['valid']}}));
    expect(JSON.parse(mocks.disk).ledger.old0.status).toBe('failed');
  });

  it('超出单次校验上限时分批重连，未经服务器确认的消息不会播放', async () => {
    const expires_at = new Date(Date.now()+60000).toISOString();
    const queue = Array.from({length:4097},(_,i) => ({...notice.d,message_id:`m${i}`,expires_at}));
    const ledger = Object.fromEntries(queue.map(item => [item.message_id,{status:'delivered',at:Date.now()}]));
    mocks.disk = JSON.stringify({version:1,server:'',token:'device-token',deviceName:'教室',className:'三年二班',volume:70,lastSeq:42,queue,ledger});
    vi.stubGlobal('fetch',vi.fn(async () => ({ok:true,status:200})));
    const service = makeService(); await service.init(); socket().open();
    expect((socket().sent[0].d.pending_ids as string[])).toHaveLength(4096);
    socket().frame({t:'welcome',d:{class:{name:'三年二班'},volume:70,recalled_ids:queue.slice(0,4096).map(item=>item.message_id)}});
    await vi.waitFor(() => expect(Socket.instances).toHaveLength(2),{timeout:4000});
    expect(mocks.play).not.toHaveBeenCalled();
    socket().open();
    expect(socket().sent[0]).toMatchObject({t:'hello',d:{pending_ids:['m4096']}});
    socket().frame(welcome);
    await vi.waitFor(() => expect(mocks.play).toHaveBeenCalledTimes(1));
  });

  it('撤回当前通知后移出队列，并对重发保持去重', async () => {
    mocks.disk = JSON.stringify({version:1,server:'',token:'device-token',deviceName:'教室',className:'三年二班',volume:70,lastSeq:0,queue:[],ledger:{}});
    const service = makeService(); await service.init(); socket().open(); socket().frame(welcome); socket().frame(notice);
    await vi.waitFor(() => expect(mocks.play).toHaveBeenCalledTimes(1));
    socket().frame({t:'recall',d:{message_id:'m1'}});
    await vi.waitFor(() => expect(JSON.parse(mocks.disk).ledger.m1.status).toBe('dismissed'));
    socket().frame(notice);
    await vi.waitFor(() => expect(socket().sent.some(frame=>frame.t==='ack' && frame.d.status==='dismissed')).toBe(true));
    expect(JSON.parse(mocks.disk).queue).toHaveLength(0);
    expect(mocks.play).toHaveBeenCalledTimes(1);
  });
});
