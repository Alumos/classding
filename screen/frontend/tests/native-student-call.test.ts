import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  disk: JSON.stringify({
    version: 1,
    token: 'student-call-bridge-token',
    deviceId: 'd1',
    classId: 'c1',
    deviceName: '教室',
    className: '三年二班',
    server: 'http://localhost:18080',
    volume: 70,
    lastSeq: 0,
    queue: [],
    ledger: {},
  }),
  configure: vi.fn(async (server: string) => server),
}));

vi.mock('../src/native', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../src/native')>()),
  loadState: async () => mocks.disk,
  saveState: async (value: string) => {
    mocks.disk = value;
  },
  configureServer: mocks.configure,
}));

import { ScreenService } from '../src/service';

class Socket {
  static OPEN = 1;
  static instances: Socket[] = [];
  readyState = 1;
  onopen?: () => void;
  onmessage?: (event: { data: string }) => void;
  onclose?: () => void;
  onerror?: () => void;
  constructor(public url: string) {
    Socket.instances.push(this);
  }
  send(_text: string) {}
  close() {
    this.onclose?.();
  }
  frame(value: object) {
    this.onmessage?.({ data: JSON.stringify(value) });
  }
}

const welcome = { t: 'welcome', d: { class: { id: 'c1', name: '三年二班' }, volume: 70 } };
let services: ScreenService[] = [];
let eventHandler: ((...args: unknown[]) => void) | undefined;
let eventOff: ReturnType<typeof vi.fn>;
let app: {
  UpdateStudentCallStatus: ReturnType<typeof vi.fn>;
  CompleteStudentCall: ReturnType<typeof vi.fn>;
};

beforeEach(() => {
  mocks.disk = JSON.stringify({
    version: 1,
    token: 'student-call-bridge-token',
    deviceId: 'd1',
    classId: 'c1',
    deviceName: '教室',
    className: '三年二班',
    server: 'http://localhost:18080',
    volume: 70,
    lastSeq: 0,
    queue: [],
    ledger: {},
  });
  mocks.configure.mockClear();
  Socket.instances = [];
  services = [];
  eventHandler = undefined;
  eventOff = vi.fn();
  app = {
    UpdateStudentCallStatus: vi.fn(async () => {}),
    CompleteStudentCall: vi.fn(async () => {}),
  };
  vi.stubGlobal('window', {
    location: { origin: 'http://localhost:5175', href: 'http://localhost:5175/' },
    go: { main: { App: app } },
    runtime: {
      EventsOnMultiple: vi.fn((_name: string, callback: (...args: unknown[]) => void) => {
        eventHandler = callback;
        return eventOff;
      }),
    },
  });
  vi.stubGlobal('WebSocket', Socket);
});

afterEach(() => {
  services.forEach((service) => service.dispose());
  vi.unstubAllGlobals();
});

async function online() {
  const service = new ScreenService();
  services.push(service);
  await service.init();
  Socket.instances.at(-1)!.frame(welcome);
  await vi.waitFor(() => expect(service.getSnapshot().connection).toBe('online'));
  await vi.waitFor(() => expect(app.UpdateStudentCallStatus).toHaveBeenCalled());
  return service;
}

function response(data: unknown, status = 200) {
  return { ok: status < 400, status, json: async () => data, headers: new Headers() } as Response;
}

describe('学生呼叫原生桥接', () => {
  it('注册事件、拒绝旧绑定 epoch，并回传发送结果', async () => {
    const service = await online();
    expect(eventHandler).toBeDefined();
    expect(eventOff).not.toHaveBeenCalled();

    const request = vi.fn().mockResolvedValue({
      ...response({ id: 'call1', class_id: 'c1', device_id: 'd1', created_at: '2026-11-13T03:00:00Z' }),
    });
    vi.stubGlobal('fetch', request);

    eventHandler!('stale-id', 99);
    await Promise.resolve();
    expect(request).not.toHaveBeenCalled();
    expect(app.CompleteStudentCall).not.toHaveBeenCalled();

    eventHandler!('native-call-id', 0);
    await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(1));
    await vi.waitFor(() =>
      expect(app.CompleteStudentCall).toHaveBeenCalledWith(
        'native-call-id',
        true,
        '呼叫已发送，本班老师打开网页即可收到提醒',
      ),
    );
    expect(JSON.parse(request.mock.calls[0][1].body).request_id).toBe('native-call-id');
  });

  it('解绑使旧epoch事件失效，dispose移除监听', async () => {
    const service = await online();
    const request = vi.fn();
    vi.stubGlobal('fetch', request);
    const callback = eventHandler!;
    await service.unbind();
    callback('old-binding', 0);
    await Promise.resolve();
    expect(request).not.toHaveBeenCalled();
    expect(eventOff).not.toHaveBeenCalled();
    service.dispose();
    expect(eventOff).toHaveBeenCalledTimes(1);
    callback('after-dispose', 0);
    await Promise.resolve();
    expect(request).not.toHaveBeenCalled();
  });
});
