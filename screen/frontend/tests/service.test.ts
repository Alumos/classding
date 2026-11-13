import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  disk: '',
  configure: vi.fn(async (server: string) => server),
  saveGate: null as null | ((value: string) => Promise<void>),
  play: vi.fn(
    (_notice: unknown, _url: unknown, signal: AbortSignal) =>
      new Promise<void>((_resolve, reject) => {
        if (signal.aborted) {
          reject(new DOMException('取消', 'AbortError'));
          return;
        }
        signal.addEventListener('abort', () => reject(new DOMException('取消', 'AbortError')), {
          once: true,
        });
      }),
  ),
}));
vi.mock('../src/native', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../src/native')>()),
  loadState: async () => mocks.disk,
  saveState: async (value: string) => {
    if (mocks.saveGate) await mocks.saveGate(value);
    mocks.disk = value;
  },
  configureServer: mocks.configure,
}));
vi.mock('../src/audio', () => ({ playNotice: mocks.play, playChime: vi.fn(async () => {}) }));
import { ScreenService } from '../src/service';

class Socket {
  static OPEN = 1;
  static instances: Socket[] = [];
  readyState = 1;
  sent: Array<{ t: string; d: Record<string, unknown> }> = [];
  onopen?: () => void;
  onmessage?: (event: { data: string }) => void;
  onclose?: () => void;
  onerror?: () => void;
  constructor(public url: string) {
    Socket.instances.push(this);
  }
  send(text: string) {
    this.sent.push(JSON.parse(text));
  }
  open() {
    this.onopen?.();
  }
  frame(value: object) {
    this.onmessage?.({ data: JSON.stringify(value) });
  }
  close() {
    this.readyState = 3;
    this.onclose?.();
  }
}
const welcome = { t: 'welcome', d: { class: { id: 'c1', name: '三年二班' }, volume: 70 } };
const notice = {
  t: 'notify',
  seq: 42,
  d: {
    message_id: 'm1',
    text: '请到办公室',
    style: 'notice',
    class_name: '三年二班',
    chime: true,
    repeat: 1,
    display_sec: 60,
    volume: 70,
  },
};
let services: ScreenService[] = [];
function makeService() {
  const service = new ScreenService();
  services.push(service);
  return service;
}
function socket() {
  return Socket.instances.at(-1)!;
}

beforeEach(() => {
  mocks.disk = '';
  mocks.saveGate = null;
  mocks.play.mockClear();
  mocks.configure.mockClear();
  Socket.instances = [];
  services = [];
  vi.stubGlobal('window', { location: { origin: 'http://localhost:5175', href: 'http://localhost:5175/' } });
  vi.stubGlobal('navigator', { userAgent: 'test browser' });
  vi.stubGlobal('WebSocket', Socket);
});
afterEach(() => {
  services.forEach((service) => service.dispose());
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('学生呼叫', () => {
  const result = { id: 'call1', class_id: 'c1', device_id: 'd1', created_at: '2026-11-13T03:00:00Z' };
  async function online() {
    mocks.disk = JSON.stringify({
      version: 1,
      token: 'student-call-test-token',
      deviceId: 'd1',
      classId: 'c1',
      deviceName: '教室',
      className: '三年二班',
      volume: 70,
      lastSeq: 0,
      queue: [],
      ledger: {},
    });
    const service = makeService();
    await service.init();
    socket().frame(welcome);
    await vi.waitFor(() => expect(service.getSnapshot().connection).toBe('online'));
    return service;
  }
  function response(data: unknown, status = 200, retry?: string) {
    return {
      ok: status < 400,
      status,
      json: async () => data,
      headers: new Headers(retry ? { 'Retry-After': retry } : {}),
    } as Response;
  }
  it('未绑定和离线不发送请求', async () => {
    const request = vi.fn();
    vi.stubGlobal('fetch', request);
    const service = makeService();
    await service.init();
    await expect(service.sendStudentCall()).rejects.toThrow('未绑定');
    const bound = await online();
    socket().close();
    await expect(bound.sendStudentCall()).rejects.toThrow('连接中断');
    expect(request.mock.calls.filter(([path]) => String(path).includes('student-calls'))).toHaveLength(0);
  });
  it('复用设备认证并阻止发送中重复，成功必须收到有效记录', async () => {
    const service = await online();
    let resolve!: (value: Response) => void;
    const request = vi.fn(
      (_path: string, _init?: RequestInit) =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    );
    vi.stubGlobal('fetch', request);
    const sending = service.sendStudentCall('native-id');
    await expect(service.sendStudentCall()).rejects.toThrow('正在发送');
    expect(request).toHaveBeenCalledWith(
      '/api/v1/device/student-calls',
      expect.objectContaining({
        method: 'POST',
        headers: { Authorization: 'Bearer student-call-test-token', 'Content-Type': 'application/json' },
        body: JSON.stringify({ request_id: 'native-id' }),
      }),
    );
    resolve(response(result));
    await sending;
    expect(service.getSnapshot()).toMatchObject({
      studentCallSending: false,
      studentCallMessage: expect.stringContaining('已发送'),
    });
  });
  it('不确定失败保留幂等编号，跨备用入口重试不创建新请求', async () => {
    const service = await online();
    const request = vi
      .fn()
      .mockRejectedValueOnce(new TypeError('offline'))
      .mockResolvedValue(response(result));
    vi.stubGlobal('fetch', request);
    await expect(service.sendStudentCall('original-id')).rejects.toThrow('未确认');
    await service.sendStudentCall('another-native-id');
    expect(request.mock.calls.map(([, init]) => JSON.parse(init.body).request_id)).toEqual([
      'original-id',
      'original-id',
    ]);
  });
  it('限流显示等待秒数，不假成功；无效成功响应可重试', async () => {
    const service = await online();
    const request = vi
      .fn()
      .mockResolvedValueOnce(response({}, 429, '27'))
      .mockResolvedValueOnce(response({}))
      .mockResolvedValue(response(result));
    vi.stubGlobal('fetch', request);
    await expect(service.sendStudentCall('rate-id')).rejects.toThrow('27 秒');
    await expect(service.sendStudentCall('new-id')).rejects.toThrow('未确认有效呼叫');
    await service.sendStudentCall('ignore-id');
    expect(request.mock.calls.map(([, init]) => JSON.parse(init.body).request_id)).toEqual([
      'rate-id',
      'new-id',
      'new-id',
    ]);
  });
  it('解绑中止请求并拒绝旧成功写回', async () => {
    const service = await online();
    let resolve!: (value: Response) => void;
    let signal: AbortSignal | undefined;
    vi.stubGlobal(
      'fetch',
      vi.fn((_path: string, init: RequestInit) => {
        signal = init.signal!;
        return new Promise<Response>((done) => {
          resolve = done;
        });
      }),
    );
    const pending = service.sendStudentCall('old-binding');
    await service.unbind();
    expect(signal?.aborted).toBe(true);
    resolve(response(result));
    await expect(pending).rejects.toThrow('绑定已变化');
    expect(service.getSnapshot()).toMatchObject({
      bound: false,
      studentCallSending: false,
      studentCallMessage: '',
    });
  });
  it('401 清除失效绑定，403 明确显示授权不可用', async () => {
    const service = await online();
    const request = vi.fn().mockResolvedValueOnce(response({}, 403)).mockResolvedValue(response({}, 401));
    vi.stubGlobal('fetch', request);
    await expect(service.sendStudentCall('license-id')).rejects.toThrow('授权不可用');
    await expect(service.sendStudentCall()).rejects.toThrow('已失效');
    expect(service.getSnapshot().bound).toBe(false);
  });
});

describe('大屏消息可靠性', () => {
  it('忽略格式无效的消息封装，并继续接收合法通知', async () => {
    mocks.disk = JSON.stringify({
      version: 1,
      server: '',
      token: 'device-token',
      deviceName: '教室',
      className: '三年二班',
      volume: 70,
      lastSeq: 0,
      queue: [],
      ledger: {},
    });
    const service = makeService();
    await service.init();
    socket().open();
    for (const value of [null, [], 42, { t: [], d: {} }])
      socket().onmessage?.({ data: JSON.stringify(value) });
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(service.getSnapshot().error).toBe('');
    socket().frame(welcome);
    socket().frame(notice);
    await vi.waitFor(() => expect(mocks.play).toHaveBeenCalledTimes(1));
  });

  it('成功绑定后保存设备令牌并通过 hello 鉴权', async () => {
    const fetchMock = vi.fn(async () => ({
      ok: true,
      status: 200,
      json: async () => ({
        device_token: 'secret',
        device: { id: 'd1', class_id: 'c1', name: '三年二班智慧黑板' },
        class: { id: 'c1', name: '三年二班' },
        volume: 70,
      }),
    }));
    vi.stubGlobal('fetch', fetchMock);
    const service = makeService();
    await service.init();
    await service.bind('123456', '', '');
    expect(JSON.parse(mocks.disk).token).toBe('secret');
    expect(JSON.parse(fetchMock.mock.calls[0][1].body).code).toBe('123456');
    expect(JSON.parse(fetchMock.mock.calls[0][1].body).name).toBe('');
    expect(JSON.parse(mocks.disk)).toMatchObject({
      deviceId: 'd1',
      classId: 'c1',
      deviceName: '三年二班智慧黑板',
    });
    socket().open();
    expect(socket().sent[0]).toMatchObject({ t: 'hello', d: { token: 'secret', last_seq: 0 } });
  });

  it('重复通知只播一次，重启后按持久队列恢复并上报最后序号', async () => {
    mocks.disk = JSON.stringify({
      version: 1,
      server: '',
      token: 'device-token',
      deviceName: '教室',
      className: '三年二班',
      volume: 70,
      lastSeq: 0,
      queue: [],
      ledger: {},
    });
    const first = makeService();
    await first.init();
    socket().open();
    socket().frame(welcome);
    socket().frame(notice);
    socket().frame(notice);
    await vi.waitFor(() => expect(mocks.play).toHaveBeenCalledTimes(1));
    await vi.waitFor(() => expect(JSON.parse(mocks.disk).ledger.m1.status).toBe('playing'));
    expect(JSON.parse(mocks.disk).queue).toHaveLength(1);
    expect(JSON.parse(mocks.disk).lastSeq).toBe(42);
    expect(socket().sent.some((frame) => frame.t === 'ack' && frame.d.message_id === 'm1')).toBe(true);
    first.dispose();
    const second = makeService();
    await second.init();
    socket().open();
    expect(socket().sent[0]).toMatchObject({ t: 'hello', d: { token: 'device-token', last_seq: 42 } });
    expect(mocks.play).toHaveBeenCalledTimes(1);
    socket().frame(welcome);
    await vi.waitFor(() => expect(mocks.play).toHaveBeenCalledTimes(2));
  });

  it('重连时向服务器提交待播 ID，并在播放前处理离线撤回', async () => {
    mocks.disk = JSON.stringify({
      version: 1,
      server: '',
      token: 'device-token',
      deviceName: '教室',
      className: '三年二班',
      volume: 70,
      lastSeq: 42,
      queue: [notice.d],
      ledger: { m1: { status: 'delivered', at: Date.now() } },
    });
    const service = makeService();
    await service.init();
    socket().open();
    expect(socket().sent[0]).toMatchObject({ t: 'hello', d: { pending_ids: ['m1'] } });
    socket().frame({ t: 'welcome', d: { class: { name: '三年二班' }, volume: 70, recalled_ids: ['m1'] } });
    await vi.waitFor(() => expect(JSON.parse(mocks.disk).ledger.m1.status).toBe('dismissed'));
    expect(JSON.parse(mocks.disk).queue).toHaveLength(0);
    expect(mocks.play).not.toHaveBeenCalled();
  });

  it('断线后暂停下一条，重连确认后不播离线撤回的消息', async () => {
    const second = { ...notice.d, message_id: 'm2' };
    mocks.disk = JSON.stringify({
      version: 1,
      server: '',
      token: 'device-token',
      deviceName: '教室',
      className: '三年二班',
      volume: 70,
      lastSeq: 42,
      queue: [notice.d, second],
      ledger: { m1: { status: 'delivered', at: Date.now() }, m2: { status: 'delivered', at: Date.now() } },
    });
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ ok: true, status: 200 })),
    );
    const service = makeService();
    await service.init();
    socket().open();
    socket().frame(welcome);
    await vi.waitFor(() => expect(mocks.play).toHaveBeenCalledTimes(1));
    socket().close();
    await service.dismiss();
    expect(mocks.play).toHaveBeenCalledTimes(1);
    await vi.waitFor(() => expect(Socket.instances).toHaveLength(2), { timeout: 2000 });
    socket().open();
    socket().frame({ t: 'welcome', d: { class: { name: '三年二班' }, volume: 70, recalled_ids: ['m2'] } });
    await vi.waitFor(() => expect(JSON.parse(mocks.disk).ledger.m2.status).toBe('dismissed'));
    expect(mocks.play).toHaveBeenCalledTimes(1);
  });

  it('恢复队列时不播过期消息，并报告失败', async () => {
    const expired = { ...notice.d, sent_at: new Date(Date.now() - 11 * 60 * 1000).toISOString() };
    mocks.disk = JSON.stringify({
      version: 1,
      server: '',
      token: 'device-token',
      deviceName: '教室',
      className: '三年二班',
      volume: 70,
      lastSeq: 42,
      queue: [expired],
      ledger: { m1: { status: 'delivered', at: Date.now() } },
    });
    const service = makeService();
    await service.init();
    socket().open();
    socket().frame(welcome);
    await vi.waitFor(() => expect(JSON.parse(mocks.disk).ledger.m1.status).toBe('failed'));
    expect(JSON.parse(mocks.disk).queue).toHaveLength(0);
    expect(mocks.play).not.toHaveBeenCalled();
    expect(socket().sent.some((frame) => frame.t === 'ack' && frame.d.status === 'failed')).toBe(true);
  });

  it('发送 hello 前清理大量过期消息，避免超过服务器的待播 ID 上限', async () => {
    const past = new Date(Date.now() - 1000).toISOString();
    const expired = Array.from({ length: 4097 }, (_, i) => ({
      ...notice.d,
      message_id: `old${i}`,
      expires_at: past,
    }));
    const valid = {
      ...notice.d,
      message_id: 'valid',
      expires_at: new Date(Date.now() + 60000).toISOString(),
    };
    const ledger = Object.fromEntries(
      [...expired, valid].map((item) => [item.message_id, { status: 'delivered', at: Date.now() }]),
    );
    mocks.disk = JSON.stringify({
      version: 1,
      server: '',
      token: 'device-token',
      deviceName: '教室',
      className: '三年二班',
      volume: 70,
      lastSeq: 42,
      queue: [...expired, valid],
      ledger,
    });
    const service = makeService();
    await service.init();
    socket().open();
    await vi.waitFor(() =>
      expect(socket().sent[0]).toMatchObject({ t: 'hello', d: { pending_ids: ['valid'] } }),
    );
    expect(JSON.parse(mocks.disk).ledger.old0.status).toBe('failed');
  });

  it('超出单次校验上限时分批重连，未经服务器确认的消息不会播放', async () => {
    const expires_at = new Date(Date.now() + 60000).toISOString();
    const queue = Array.from({ length: 4097 }, (_, i) => ({ ...notice.d, message_id: `m${i}`, expires_at }));
    const ledger = Object.fromEntries(
      queue.map((item) => [item.message_id, { status: 'delivered', at: Date.now() }]),
    );
    mocks.disk = JSON.stringify({
      version: 1,
      server: '',
      token: 'device-token',
      deviceName: '教室',
      className: '三年二班',
      volume: 70,
      lastSeq: 42,
      queue,
      ledger,
    });
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ ok: true, status: 200 })),
    );
    const service = makeService();
    await service.init();
    socket().open();
    expect(socket().sent[0].d.pending_ids as string[]).toHaveLength(4096);
    socket().frame({
      t: 'welcome',
      d: {
        class: { name: '三年二班' },
        volume: 70,
        recalled_ids: queue.slice(0, 4096).map((item) => item.message_id),
      },
    });
    await vi.waitFor(() => expect(Socket.instances).toHaveLength(2), { timeout: 4000 });
    expect(mocks.play).not.toHaveBeenCalled();
    socket().open();
    expect(socket().sent[0]).toMatchObject({ t: 'hello', d: { pending_ids: ['m4096'] } });
    socket().frame(welcome);
    await vi.waitFor(() => expect(mocks.play).toHaveBeenCalledTimes(1));
  });

  it('撤回当前通知后移出队列，并对重发保持去重', async () => {
    mocks.disk = JSON.stringify({
      version: 1,
      server: '',
      token: 'device-token',
      deviceName: '教室',
      className: '三年二班',
      volume: 70,
      lastSeq: 0,
      queue: [],
      ledger: {},
    });
    const service = makeService();
    await service.init();
    socket().open();
    socket().frame(welcome);
    socket().frame(notice);
    await vi.waitFor(() => expect(mocks.play).toHaveBeenCalledTimes(1));
    socket().frame({ t: 'recall', d: { message_id: 'm1' } });
    await vi.waitFor(() => expect(JSON.parse(mocks.disk).ledger.m1.status).toBe('dismissed'));
    socket().frame(notice);
    await vi.waitFor(() =>
      expect(socket().sent.some((frame) => frame.t === 'ack' && frame.d.status === 'dismissed')).toBe(true),
    );
    expect(JSON.parse(mocks.disk).queue).toHaveLength(0);
    expect(mocks.play).toHaveBeenCalledTimes(1);
  });

  it('手动关闭时保存 dismissed 并发送回执，显示时长限制为 15 至 600 秒', async () => {
    mocks.disk = JSON.stringify({
      version: 1,
      server: '',
      token: 'device-token',
      deviceName: '教室',
      className: '三年二班',
      volume: 70,
      lastSeq: 0,
      queue: [],
      ledger: {},
    });
    const service = makeService();
    await service.init();
    socket().open();
    socket().frame(welcome);
    socket().frame({ ...notice, d: { ...notice.d, display_sec: 1 } });
    await vi.waitFor(() => expect(service.getSnapshot().current?.display_sec).toBe(15));
    socket().frame({ ...notice, seq: 43, d: { ...notice.d, message_id: 'm2', display_sec: 1000 } });
    await vi.waitFor(() => expect(JSON.parse(mocks.disk).queue).toHaveLength(2));
    expect(JSON.parse(mocks.disk).queue[1].display_sec).toBe(600);
    await service.dismiss();
    expect(JSON.parse(mocks.disk).ledger.m1.status).toBe('dismissed');
    expect(
      socket().sent.some(
        (frame) => frame.t === 'ack' && frame.d.message_id === 'm1' && frame.d.status === 'dismissed',
      ),
    ).toBe(true);
  });

  it('展示时限到达时中止未结束的语音并清除当前通知', async () => {
    vi.useFakeTimers();
    mocks.disk = JSON.stringify({
      version: 1,
      server: '',
      token: 'device-token',
      deviceName: '教室',
      className: '三年二班',
      volume: 70,
      lastSeq: 42,
      queue: [{ ...notice.d, display_sec: 15 }],
      ledger: { m1: { status: 'delivered', at: Date.now() } },
    });
    const service = makeService();
    await service.init();
    socket().open();
    socket().frame(welcome);
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

describe('网络恢复', () => {
  const bound = () =>
    JSON.stringify({
      version: 1,
      token: 'device-token',
      deviceId: 'd1',
      classId: 'c1',
      deviceName: '教室',
      className: '三年二班',
      volume: 70,
      lastSeq: 0,
      queue: [],
      ledger: {},
    });
  it('连接迟迟无响应时放弃并重试，不会一直卡在“正在连接”', async () => {
    vi.useFakeTimers();
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ ok: false, status: 503 }) as Response),
    );
    mocks.disk = bound();
    const service = makeService();
    await service.init();
    const stalled = socket();
    await vi.advanceTimersByTimeAsync(9999);
    expect(stalled.readyState).toBe(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(stalled.readyState).toBe(3);
    expect(service.getSnapshot().connection).toBe('offline');
  });
  it('系统报告网络恢复后立即重连，不等待退避', async () => {
    const listeners: Record<string, () => void> = {};
    vi.stubGlobal('window', {
      location: { origin: 'http://localhost:5175', href: 'http://localhost:5175/' },
      addEventListener: (type: string, listener: () => void) => (listeners[type] = listener),
      removeEventListener: (type: string) => delete listeners[type],
    });
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ ok: false, status: 503 }) as Response),
    );
    mocks.disk = bound();
    const service = makeService();
    await service.init();
    socket().close();
    const before = Socket.instances.length;
    listeners.online();
    expect(Socket.instances.length).toBe(before + 1);
    socket().frame(welcome);
    await vi.waitFor(() => expect(service.getSnapshot().connection).toBe('online'));
    listeners.online();
    expect(Socket.instances.length).toBe(before + 1);
    service.dispose();
    expect(listeners.online).toBeUndefined();
  });
});

describe('切换服务器', () => {
  const plan = { server_url: 'https://new.example', revision: 'r1' };
  const saved = () => ({
    version: 1,
    server: 'https://old.example',
    token: 'device-token',
    deviceId: 'd1',
    classId: 'c1',
    deviceName: '教室',
    className: '三年二班',
    volume: 70,
    lastSeq: 42,
    queue: [],
    ledger: {},
  });
  const sendPlan = () => socket().frame({ t: 'config', d: { remote_config: plan } });
  const disk = () => JSON.parse(mocks.disk);

  it('hello 上报当前服务器地址、内网地址和已应用的配置版本', async () => {
    mocks.disk = JSON.stringify({ ...saved(), appliedRevision: 'r0' });
    const service = makeService();
    await service.init();
    socket().open();
    await vi.waitFor(() =>
      expect(socket().sent[0]).toMatchObject({
        t: 'hello',
        d: { server_url: 'https://old.example', lan_ips: [], applied_revision: 'r0' },
      }),
    );
  });

  it('收到新地址后直接切换，新服务器接受设备后清除回退记录', async () => {
    mocks.disk = JSON.stringify(saved());
    const service = makeService();
    await service.init();
    socket().open();
    socket().frame(welcome);
    sendPlan();
    await vi.waitFor(() => expect(disk().server).toBe('https://new.example'));
    expect(disk()).toMatchObject({
      token: 'device-token',
      appliedRevision: 'r1',
      fallback: { server: 'https://old.example', revision: 'r1' },
    });
    expect(disk().remoteConfig).toBeUndefined();
    expect(mocks.configure.mock.calls.map((call) => call[0])).toEqual([
      'https://old.example',
      'https://new.example',
    ]);
    const next = socket();
    expect(Socket.instances).toHaveLength(2);
    next.open();
    next.frame(welcome);
    await vi.waitFor(() => expect(disk().fallback).toBeUndefined());
    expect(service.getSnapshot().connection).toBe('online');
  });

  it('新服务器不认可本设备时回到原服务器，不会解绑', async () => {
    mocks.disk = JSON.stringify(saved());
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ status: 401, ok: false })),
    );
    const service = makeService();
    await service.init();
    socket().open();
    socket().frame(welcome);
    sendPlan();
    await vi.waitFor(() => expect(disk().server).toBe('https://new.example'));
    socket().close();
    await vi.waitFor(() => expect(disk().server).toBe('https://old.example'));
    expect(disk()).toMatchObject({ token: 'device-token', appliedRevision: 'r1' });
    expect(disk().fallback).toBeUndefined();
    expect(service.getSnapshot()).toMatchObject({ bound: true });
    expect(service.getSnapshot().error).toContain('已回到原服务器');
  });

  it('回退后同一版本的地址不会再次切换', async () => {
    mocks.disk = JSON.stringify({ ...saved(), appliedRevision: 'r1' });
    const service = makeService();
    await service.init();
    socket().open();
    socket().frame(welcome);
    sendPlan();
    await new Promise((resolve) => setTimeout(resolve, 30));
    expect(disk().server).toBe('https://old.example');
    expect(mocks.configure).toHaveBeenCalledTimes(1);
    expect(service.getSnapshot().bound).toBe(true);
  });

  it('新服务器长时间无法连接时回到原服务器', async () => {
    mocks.disk = JSON.stringify({
      ...saved(),
      server: 'https://new.example',
      appliedRevision: 'r1',
      fallback: { server: 'https://old.example', revision: 'r1', until: Date.now() - 1 },
    });
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new TypeError('offline');
      }),
    );
    const service = makeService();
    await service.init();
    socket().close();
    await vi.waitFor(() => expect(disk().server).toBe('https://old.example'));
    expect(disk().fallback).toBeUndefined();
    expect(service.getSnapshot().bound).toBe(true);
  });

  it('播放通知期间等待播放结束后再切换', async () => {
    mocks.disk = JSON.stringify(saved());
    const service = makeService();
    await service.init();
    socket().open();
    socket().frame(welcome);
    socket().frame(notice);
    await vi.waitFor(() => expect(service.getSnapshot().current?.message_id).toBe('m1'));
    sendPlan();
    await new Promise((resolve) => setTimeout(resolve, 30));
    expect(disk().server).toBe('https://old.example');
    expect(disk().remoteConfig).toEqual(plan);
    await service.dismiss();
    await vi.waitFor(() => expect(disk().server).toBe('https://new.example'), { timeout: 7000 });
  });
});
