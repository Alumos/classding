import {afterEach,beforeEach,describe,expect,it,vi} from 'vitest';
import type {Homework} from '../src/homework';

const mocks=vi.hoisted(()=>({
  disk:'',
  failSave:false,
  saves:0,
  saveGate:null as null|((value:string)=>Promise<void>),
  configure:vi.fn(async (server:string)=>server),
}));
vi.mock('../src/native',async importOriginal=>({
  ...await importOriginal<typeof import('../src/native')>(),
  loadState:async()=>mocks.disk,
  saveState:async(value:string)=>{mocks.saves++;if(mocks.failSave)throw new Error('磁盘不可写');if(mocks.saveGate)await mocks.saveGate(value);mocks.disk=value;},
  configureServer:mocks.configure,
}));
vi.mock('../src/audio',()=>({playNotice:vi.fn(),playChime:vi.fn()}));
import {ScreenService} from '../src/service';

class Socket {
  static OPEN=1;
  static instances:Socket[]=[];
  readyState=1;
  onopen?:()=>void;
  onmessage?:(event:{data:string})=>void;
  onclose?:()=>void;
  onerror?:()=>void;
  constructor(public url:string){Socket.instances.push(this);}
  send(_text:string){}
  open(){this.onopen?.();}
  frame(value:object){this.onmessage?.({data:JSON.stringify(value)});}
  close(){if(this.readyState===3)return;this.readyState=3;this.onclose?.();}
}

const card=(text:string):Homework=>({id:'h1',class_id:'c1',subject:'数学',text,teacher_id:'t1',teacher_name:'林老师',visible:true,always_on_top:true,created_at:'2026-11-01T00:00:00Z',updated_at:'2026-11-01T00:00:00Z'});
const saved=(homeworks:Homework[]=[card('旧作业')],homeworkRevision=1)=>({version:1,server:'https://old.example',token:'secret',deviceId:'d1',classId:'c1',deviceName:'教室',className:'三年二班',volume:70,lastSeq:0,sourceSequence:0,queue:[],ledger:{},homeworks,homeworkRevision});
const welcome=(items:Homework[],revision:number)=>({t:'welcome',d:{class:{id:'c1',name:'三年二班'},device:{id:'d1',class_id:'c1',name:'教室'},volume:70,homeworks:items,homework_revision:revision}});
const update=(items:Homework[],revision:number)=>({t:'homework',d:{items,homework_revision:revision}});
const plan={server_url:'https://new.example',revision:'migration-1'};
const socket=()=>Socket.instances.at(-1)!;
let services:ScreenService[]=[];
const makeService=()=>{const service=new ScreenService();services.push(service);return service;};

beforeEach(()=>{
  mocks.disk=JSON.stringify(saved());mocks.failSave=false;mocks.saves=0;mocks.saveGate=null;mocks.configure.mockClear();Socket.instances=[];services=[];
  vi.stubGlobal('window',{location:{origin:'http://localhost:5175',href:'http://localhost:5175/'}});
  vi.stubGlobal('navigator',{userAgent:'test browser'});
  vi.stubGlobal('WebSocket',Socket);
  vi.stubGlobal('fetch',vi.fn(async()=>({ok:true,status:200,json:async()=>({items:[card('旧作业')],homework_revision:1})})));
});
afterEach(()=>{services.forEach(service=>service.dispose());vi.useRealTimers();vi.unstubAllGlobals();});

describe('大屏作业同步',()=>{
  it('接收欢迎快照和后续更新，拒绝更旧或无效的快照',async()=>{
    const service=makeService();await service.init();socket().open();
    socket().frame(welcome([card('欢迎快照')],2));
    await vi.waitFor(()=>expect(service.getSnapshot().homeworks[0].text).toBe('欢迎快照'));
    socket().frame(update([card('最新作业')],3));
    await vi.waitFor(()=>expect(service.getSnapshot().homeworks[0].text).toBe('最新作业'));
    const saves=mocks.saves;
    socket().frame(update([card('过期作业')],2));
    socket().frame(update([{...card('其他班级'),class_id:'c2'}],4));
    await vi.waitFor(()=>expect(mocks.saves).toBeGreaterThanOrEqual(saves+2));
    expect(service.getSnapshot().homeworks).toEqual([card('最新作业')]);
    expect(JSON.parse(mocks.disk)).toMatchObject({homeworks:[card('最新作业')],homeworkRevision:3});
  });

  it('保存失败时保留此前的作业和版本',async()=>{
    const service=makeService();await service.init();socket().open();
    mocks.failSave=true;
    socket().frame(update([card('无法保存的新作业')],2));
    await vi.waitFor(()=>expect(service.getSnapshot().error).toContain('磁盘不可写'));
    expect(service.getSnapshot().homeworks).toEqual([card('旧作业')]);
    expect(JSON.parse(mocks.disk)).toMatchObject({homeworks:[card('旧作业')],homeworkRevision:1});
  });

  it('解绑时清空缓存作业与版本，重启后不会恢复',async()=>{
    const service=makeService();await service.init();
    await service.unbind();
    expect(service.getSnapshot()).toMatchObject({bound:false,homeworks:[]});
    expect(JSON.parse(mocks.disk)).toMatchObject({token:'',homeworks:[],homeworkRevision:0});
    const restarted=makeService();await restarted.init();
    expect(restarted.getSnapshot()).toMatchObject({bound:false,homeworks:[]});
  });

  it('较晚到达的旧轮询结果不会覆盖实时新作业',async()=>{
    vi.useFakeTimers();
    let complete!:(value:unknown)=>void;
    const fetchMock=vi.fn(()=>new Promise(resolve=>{complete=resolve;}));
    vi.stubGlobal('fetch',fetchMock);
    const service=makeService();await service.init();socket().open();socket().frame(welcome([card('欢迎快照')],2));
    await vi.advanceTimersByTimeAsync(0);
    await vi.advanceTimersByTimeAsync(30000);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    socket().frame(update([card('实时新作业')],3));
    await vi.advanceTimersByTimeAsync(0);
    complete({ok:true,status:200,json:async()=>({items:[card('过期轮询')],homework_revision:2})});
    await vi.advanceTimersByTimeAsync(0);
    expect(service.getSnapshot().homeworks).toEqual([card('实时新作业')]);
    expect(JSON.parse(mocks.disk).homeworkRevision).toBe(3);
  });
});
