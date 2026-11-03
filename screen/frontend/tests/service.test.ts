import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';

const mocks = vi.hoisted(() => ({
  disk: '',
  probe: vi.fn(),
  configure: vi.fn(async (server:string) => server),
  saveGate: null as null | ((value:string) => Promise<void>),
  play: vi.fn((_notice:unknown, _url:unknown, signal:AbortSignal) => new Promise<void>((_resolve,reject) => {
    if (signal.aborted) { reject(new DOMException('取消','AbortError')); return; }
    signal.addEventListener('abort',() => reject(new DOMException('取消','AbortError')),{once:true});
  })),
}));
vi.mock('../src/native', async (importOriginal) => ({
  ...await importOriginal<typeof import('../src/native')>(),
  loadState: async () => mocks.disk,
  saveState: async (value:string) => { if (mocks.saveGate) await mocks.saveGate(value); mocks.disk = value; },
  configureServer: mocks.configure,
  probeServer: mocks.probe,
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
  mocks.disk = ''; mocks.saveGate = null; mocks.play.mockClear(); mocks.configure.mockClear(); mocks.probe.mockReset(); Socket.instances = []; services = [];
  vi.stubGlobal('window',{location:{origin:'http://localhost:5175',href:'http://localhost:5175/'}});
  vi.stubGlobal('navigator',{userAgent:'test browser'});
  vi.stubGlobal('WebSocket',Socket);
});
afterEach(() => { services.forEach(service=>service.dispose()); vi.useRealTimers(); vi.unstubAllGlobals(); });

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
    const fetchMock = vi.fn(async () => ({ok:true,status:200,json:async () => ({device_token:'secret',device:{id:'d1',class_id:'c1',name:'三年二班智慧黑板'},class:{id:'c1',name:'三年二班'},volume:70})}));
    vi.stubGlobal('fetch',fetchMock);
    const service = makeService(); await service.init(); await service.bind('123456','','');
    expect(JSON.parse(mocks.disk).token).toBe('secret');
    expect(JSON.parse(fetchMock.mock.calls[0][1].body).code).toBe('123456');
    expect(JSON.parse(fetchMock.mock.calls[0][1].body).name).toBe('');
    expect(JSON.parse(mocks.disk)).toMatchObject({deviceId:'d1',classId:'c1',deviceName:'三年二班智慧黑板'});
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

  it('手动关闭时保存 dismissed 并发送回执，显示时长限制为 15 至 600 秒', async () => {
    mocks.disk = JSON.stringify({version:1,server:'',token:'device-token',deviceName:'教室',className:'三年二班',volume:70,lastSeq:0,queue:[],ledger:{}});
    const service = makeService(); await service.init(); socket().open(); socket().frame(welcome);
    socket().frame({...notice,d:{...notice.d,display_sec:1}});
    await vi.waitFor(() => expect(service.getSnapshot().current?.display_sec).toBe(15));
    socket().frame({...notice,seq:43,d:{...notice.d,message_id:'m2',display_sec:1000}});
    await vi.waitFor(() => expect(JSON.parse(mocks.disk).queue).toHaveLength(2));
    expect(JSON.parse(mocks.disk).queue[1].display_sec).toBe(600);
    await service.dismiss();
    expect(JSON.parse(mocks.disk).ledger.m1.status).toBe('dismissed');
    expect(socket().sent.some(frame => frame.t === 'ack' && frame.d.message_id === 'm1' && frame.d.status === 'dismissed')).toBe(true);
  });

  it('展示时限到达时中止未结束的语音并清除当前通知', async () => {
    vi.useFakeTimers();
    mocks.disk = JSON.stringify({version:1,server:'',token:'device-token',deviceName:'教室',className:'三年二班',volume:70,lastSeq:42,queue:[{...notice.d,display_sec:15}],ledger:{m1:{status:'delivered',at:Date.now()}}});
    const service = makeService(); await service.init(); socket().open(); socket().frame(welcome);
    await vi.advanceTimersByTimeAsync(0);
    expect(mocks.play).toHaveBeenCalledTimes(1);
    const signal = mocks.play.mock.calls[0][2];
    expect(service.getSnapshot().current?.message_id).toBe('m1');
    await vi.advanceTimersByTimeAsync(14999);
    expect(signal.aborted).toBe(false);
    expect(service.getSnapshot().current?.message_id).toBe('m1');
    await vi.advanceTimersByTimeAsync(1);
    expect(signal.aborted).toBe(true);
    expect(service.getSnapshot().current).toBeNull();
    expect(JSON.parse(mocks.disk).queue).toEqual([]);
  });
});

describe('远程迁移', () => {
  const plan = {server_url:'https://new.example',revision:'r1'};
  const saved = () => ({version:1,server:'https://old.example',token:'device-token',deviceId:'d1',classId:'c1',deviceName:'教室',className:'三年二班',volume:70,lastSeq:42,queue:[notice.d],ledger:{m1:{status:'delivered',at:Date.now()}}});
  const sendPlan = () => socket().frame({t:'config',d:{remote_config:plan}});
  const probeResult = () => ({device:{id:'d1',class_id:'c1'},sequence:42,notifications:[notice],recalled_ids:[]});

  it('新站未上线时保留旧连接和身份，旧站 401 也不会解绑', async () => {
    mocks.disk = JSON.stringify(saved()); mocks.probe.mockRejectedValue(new Error('offline'));
    vi.stubGlobal('fetch',vi.fn(async () => ({status:401,ok:false})));
    const service = makeService(); await service.init(); socket().open(); sendPlan();
    await vi.waitFor(() => expect(mocks.probe).toHaveBeenCalledWith(plan.server_url,'device-token',42,['m1']));
    expect(JSON.parse(mocks.disk).remoteConfig).toEqual(plan);
    expect(mocks.configure.mock.calls.map(call => call[0])).toEqual(['https://old.example']);
    socket().close();
    await new Promise(resolve => setTimeout(resolve,30));
    expect(JSON.parse(mocks.disk)).toMatchObject({token:'device-token',server:'https://old.example',remoteConfig:plan});
    expect(service.getSnapshot().bound).toBe(true);
  });

  it('重启后立即重试持久化计划并迁移，同时保留队列、回执与序号', async () => {
    mocks.disk = JSON.stringify({...saved(),remoteConfig:plan});
    mocks.probe.mockResolvedValue(probeResult());
    const service = makeService(); await service.init();
    await vi.waitFor(() => expect(JSON.parse(mocks.disk).server).toBe('https://new.example'));
    expect(JSON.parse(mocks.disk)).toMatchObject({token:'device-token',deviceId:'d1',classId:'c1',lastSeq:42,queue:[notice.d],ledger:{m1:{status:'delivered'}},appliedRevision:'r1'});
    expect(JSON.parse(mocks.disk).remoteConfig).toBeUndefined();
    expect(mocks.configure.mock.calls.map(call => call[0])).toEqual(['https://old.example','https://new.example']);
    socket().open();
    expect(socket().sent[0]).toMatchObject({t:'hello',d:{token:'device-token',last_seq:42,pending_ids:['m1']}});
    socket().frame({t:'welcome',d:{class:{id:'c1',name:'三年二班'},device:{id:'d1',class_id:'c1',name:'教室'},remote_config:plan}});
    await new Promise(resolve => setTimeout(resolve,30));
    expect(mocks.probe).toHaveBeenCalledTimes(1);
  });

  it('新站设备或班级身份不符时不切换', async () => {
    mocks.disk = JSON.stringify(saved()); mocks.probe.mockResolvedValue({...probeResult(),device:{id:'other',class_id:'c1'}});
    const service = makeService(); await service.init(); socket().open(); sendPlan();
    await vi.waitFor(() => expect(service.getSnapshot().error).toContain('身份不匹配'));
    expect(JSON.parse(mocks.disk)).toMatchObject({server:'https://old.example',token:'device-token',remoteConfig:plan});
    expect(mocks.configure).toHaveBeenCalledTimes(1);
  });

  it('取消计划后，即使已在途的探测返回成功也不迁移', async () => {
    let finish!: (value:unknown) => void;
    mocks.disk = JSON.stringify(saved()); mocks.probe.mockImplementation(() => new Promise(resolve => { finish = resolve; }));
    const service = makeService(); await service.init(); socket().open(); sendPlan();
    await vi.waitFor(() => expect(mocks.probe).toHaveBeenCalledTimes(1));
    socket().frame({t:'config',d:{remote_config:null}});
    await vi.waitFor(() => expect(JSON.parse(mocks.disk).remoteConfig).toBeUndefined());
    finish(probeResult());
    await new Promise(resolve => setTimeout(resolve,30));
    expect(JSON.parse(mocks.disk)).toMatchObject({server:'https://old.example',token:'device-token'});
    expect(mocks.configure).toHaveBeenCalledTimes(1);
  });

  it('迁移时先持久化新站签名，再播放待播录音', async () => {
    const oldURL = '/api/v1/audio/a?signature=old';
    const newURL = '/api/v1/audio/a?signature=new';
    mocks.disk = JSON.stringify({...saved(),queue:[{...notice.d,audio_url:oldURL}],remoteConfig:plan});
    mocks.probe.mockResolvedValue({...probeResult(),notifications:[{...notice,d:{...notice.d,audio_url:newURL}}]});
    const service = makeService(); await service.init();
    await vi.waitFor(() => expect(JSON.parse(mocks.disk)).toMatchObject({server:plan.server_url,queue:[{message_id:'m1',audio_url:newURL}]}));
    expect(mocks.play).not.toHaveBeenCalled();
    socket().open(); socket().frame(welcome);
    await vi.waitFor(() => expect(mocks.play).toHaveBeenCalledTimes(1));
    expect(mocks.play.mock.calls[0][0]).toMatchObject({message_id:'m1',audio_url:newURL});
    const resolveAudio = mocks.play.mock.calls[0][1] as (url:string)=>string;
    expect(resolveAudio(newURL)).toBe(`https://new.example${newURL}`);
  });

  it('新站序号落后时继续使用旧站并保留迁移计划', async () => {
    mocks.disk = JSON.stringify({...saved(),remoteConfig:plan});
    mocks.probe.mockResolvedValue({...probeResult(),sequence:41});
    const service = makeService(); await service.init();
    await vi.waitFor(() => expect(service.getSnapshot().error).toContain('未恢复完整数据'));
    expect(mocks.probe).toHaveBeenCalledWith(plan.server_url,'device-token',42,['m1']);
    expect(JSON.parse(mocks.disk)).toMatchObject({server:'https://old.example',remoteConfig:plan});
    expect(mocks.configure.mock.calls.map(call=>call[0])).toEqual(['https://old.example']);
  });

  it('使用 welcome 的源站序号作为迁移检查下限', async () => {
    mocks.disk = JSON.stringify({...saved(),queue:[],ledger:{}});
    mocks.probe.mockRejectedValue(new Error('offline'));
    const service = makeService(); await service.init(); socket().open();
    socket().frame({t:'welcome',d:{...welcome.d,sequence:50}});
    await vi.waitFor(() => expect(JSON.parse(mocks.disk).sourceSequence).toBe(50));
    sendPlan();
    await vi.waitFor(() => expect(mocks.probe).toHaveBeenCalledWith(plan.server_url,'device-token',50,[]));
  });

  it('迁移检查期间收到新通知时不切换，检查期间也不开始播放', async () => {
    let finish!: (value:unknown) => void;
    mocks.disk = JSON.stringify(saved()); mocks.probe.mockImplementation(() => new Promise(resolve => { finish = resolve; }));
    const service = makeService(); await service.init(); socket().open(); sendPlan();
    await vi.waitFor(() => expect(mocks.probe).toHaveBeenCalledTimes(1));
    socket().frame(welcome);
    socket().frame({...notice,seq:43,d:{...notice.d,message_id:'m2'}});
    await vi.waitFor(() => expect(JSON.parse(mocks.disk)).toMatchObject({sourceSequence:43,queue:[{message_id:'m1'},{message_id:'m2'}]}));
    expect(mocks.play).not.toHaveBeenCalled();
    finish(probeResult());
    await new Promise(resolve => setTimeout(resolve,30));
    expect(JSON.parse(mocks.disk)).toMatchObject({server:'https://old.example',remoteConfig:plan,sourceSequence:43});
    expect(mocks.configure.mock.calls.map(call=>call[0])).toEqual(['https://old.example']);
  });

  it('迁移检查返回待播消息已撤回时，从队列移除并记录撤回', async () => {
    mocks.disk = JSON.stringify({...saved(),remoteConfig:plan});
    mocks.probe.mockResolvedValue({...probeResult(),notifications:[],recalled_ids:['m1']});
    const service = makeService(); await service.init();
    await vi.waitFor(() => expect(JSON.parse(mocks.disk)).toMatchObject({server:plan.server_url,queue:[],ledger:{m1:{status:'dismissed'}}}));
    socket().open(); socket().frame(welcome);
    await new Promise(resolve => setTimeout(resolve,30));
    expect(mocks.play).not.toHaveBeenCalled();
  });

  it('新站没有返回待播消息或撤回结果时不切换', async () => {
    mocks.disk = JSON.stringify({...saved(),remoteConfig:plan});
    mocks.probe.mockResolvedValue({...probeResult(),notifications:[]});
    const service = makeService(); await service.init();
    await vi.waitFor(() => expect(mocks.probe).toHaveBeenCalledTimes(1));
    await new Promise(resolve => setTimeout(resolve,30));
    expect(JSON.parse(mocks.disk)).toMatchObject({server:'https://old.example',remoteConfig:plan,queue:[{message_id:'m1'}]});
    expect(mocks.configure.mock.calls.map(call=>call[0])).toEqual(['https://old.example']);
  });

  it('播放期间和待播消息超过单次检查上限时不探测', async () => {
    mocks.disk = JSON.stringify(saved());
    const service = makeService(); await service.init(); socket().open(); socket().frame(welcome);
    await vi.waitFor(() => expect(mocks.play).toHaveBeenCalledTimes(1));
    sendPlan();
    await vi.waitFor(() => expect(JSON.parse(mocks.disk).remoteConfig).toEqual(plan));
    await new Promise(resolve => setTimeout(resolve,30));
    expect(mocks.probe).not.toHaveBeenCalled();
    service.dispose();

    const queue = Array.from({length:4097},(_,i) => ({...notice.d,message_id:`m${i}`}));
    const ledger = Object.fromEntries(queue.map(item=>[item.message_id,{status:'delivered',at:Date.now()}]));
    mocks.disk = JSON.stringify({...saved(),queue,ledger,remoteConfig:plan});
    const second = makeService(); await second.init();
    await new Promise(resolve => setTimeout(resolve,30));
    expect(mocks.probe).not.toHaveBeenCalled();
  });

  it('断线后读取设备内嵌的最新迁移计划及源站序号', async () => {
    const updated = {server_url:'https://next.example',revision:'r2'};
    mocks.disk = JSON.stringify({...saved(),remoteConfig:plan}); mocks.probe.mockRejectedValue(new Error('offline'));
    vi.stubGlobal('fetch',vi.fn(async () => ({status:200,ok:true,json:async () => ({device:{id:'d1',class_id:'c1',remote_config:updated},class:{id:'c1',name:'三年二班'},sequence:48})})));
    const service = makeService(); await service.init(); socket().open();
    await vi.waitFor(() => expect(mocks.probe).toHaveBeenCalledTimes(1));
    socket().close();
    await vi.waitFor(() => expect(JSON.parse(mocks.disk)).toMatchObject({server:'https://old.example',remoteConfig:updated,sourceSequence:48}));
    expect(service.getSnapshot().bound).toBe(true);
  });

  it('源站明确撤销设备后，在途迁移检查成功也不能恢复绑定', async () => {
    let finish!: (value:unknown) => void;
    mocks.disk = JSON.stringify({...saved(),remoteConfig:plan});
    mocks.probe.mockImplementation(() => new Promise(resolve => { finish = resolve; }));
    vi.stubGlobal('fetch',vi.fn(async () => ({status:401,ok:false,json:async () => ({code:'DEVICE_REVOKED'})})));
    const service = makeService(); await service.init(); socket().open();
    await vi.waitFor(() => expect(mocks.probe).toHaveBeenCalledTimes(1));
    socket().close();
    await vi.waitFor(() => expect(service.getSnapshot().bound).toBe(false));
    expect(JSON.parse(mocks.disk).token).toBe('');
    finish(probeResult());
    await new Promise(resolve => setTimeout(resolve,30));
    expect(service.getSnapshot().bound).toBe(false);
    expect(JSON.parse(mocks.disk)).toMatchObject({server:'https://old.example',token:''});
    expect(mocks.configure.mock.calls.map(call=>call[0])).toEqual(['https://old.example']);
  });

  it('迁移结果保存期间收到取消时回滚旧站并保留取消', async () => {
    let release!: () => void;
    let blocked = false;
    const gate = new Promise<void>(resolve => { release = resolve; });
    mocks.saveGate = async value => { if (JSON.parse(value).server === plan.server_url) { blocked = true; await gate; } };
    mocks.disk = JSON.stringify(saved()); mocks.probe.mockResolvedValue(probeResult());
    vi.stubGlobal('fetch',vi.fn(async () => ({status:200,ok:true})));
    const service = makeService(); await service.init(); socket().open(); sendPlan();
    await vi.waitFor(() => expect(blocked).toBe(true));
    socket().frame({t:'config',d:{remote_config:null}});
    release();
    await vi.waitFor(() => {
      const data = JSON.parse(mocks.disk);
      expect(data.server).toBe('https://old.example');
      expect(data.remoteConfig).toBeUndefined();
    });
    await new Promise(resolve => setTimeout(resolve,30));
    expect(JSON.parse(mocks.disk)).toMatchObject({server:'https://old.example',token:'device-token'});
    expect(JSON.parse(mocks.disk).remoteConfig).toBeUndefined();
    expect(Socket.instances.every(ws=>String(ws.url).startsWith('wss://old.example/'))).toBe(true);
  });
});
