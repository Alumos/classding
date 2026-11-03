import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const auth = vi.hoisted(() => ({ token: 'old-token', refresh: vi.fn() }));
vi.mock('@classding/api', () => ({
  ApiError: class ApiError extends Error {
    constructor(public code: string, message: string, public status: number) { super(message); }
  },
  getAccessToken: () => auth.token,
  refreshSession: auth.refresh,
}));

import { binaryRequest } from './binaryRequest';

describe('管理员二进制请求', () => {
  beforeEach(() => { auth.token = 'old-token'; auth.refresh.mockReset(); });
  afterEach(() => vi.unstubAllGlobals());

  it('以管理员会话下载 ZIP，而不设置 JSON 请求头', async () => {
    const zip = new Uint8Array([0x50, 0x4b, 0x03, 0x04]);
    const fetchMock = vi.fn().mockResolvedValue(new Response(zip, { headers: { 'Content-Type': 'application/zip' } }));
    vi.stubGlobal('fetch', fetchMock);
    const response = await binaryRequest('/admin/backup');
    expect(new Uint8Array(await response.arrayBuffer())).toEqual(zip);
    const [path, options] = fetchMock.mock.calls[0];
    expect(path).toBe('/api/v1/admin/backup');
    expect(options.headers.get('Authorization')).toBe('Bearer old-token');
    expect(options.headers.get('X-ClassDing-Auth-Scope')).toBe('admin');
    expect(options.headers.get('Content-Type')).toBeNull();
  });

  it('以原始 ZIP 上传，令牌过期后安全刷新并重试', async () => {
    const body = new Blob(['backup'], { type: 'application/zip' });
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response('', { status: 401 })).mockResolvedValueOnce(new Response('{"ok":true}'));
    vi.stubGlobal('fetch', fetchMock);
    auth.refresh.mockImplementation(async () => { auth.token = 'new-token'; return { access_token: auth.token }; });

    const response = await binaryRequest('/admin/restore', { method: 'POST', headers: { 'Content-Type': 'application/zip', 'X-ClassDing-Restore': 'replace' }, body });
    expect(await response.json()).toEqual({ ok: true });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    for (const [path, options] of fetchMock.mock.calls) {
      expect(path).toBe('/api/v1/admin/restore');
      expect(options.body).toBe(body);
      expect(options.credentials).toBe('include');
      expect(options.headers.get('Content-Type')).toBe('application/zip');
      expect(options.headers.get('X-ClassDing-Restore')).toBe('replace');
      expect(options.headers.get('X-ClassDing-Auth-Scope')).toBe('admin');
    }
    expect(fetchMock.mock.calls[1][1].headers.get('Authorization')).toBe('Bearer new-token');
  });

  it('会话发生变化时不会用其他账号重放请求', async () => {
    const fetchMock = vi.fn().mockImplementation(async () => { auth.token = 'different-account'; return new Response('', { status: 401 }); });
    vi.stubGlobal('fetch', fetchMock);
    await expect(binaryRequest('/admin/backup')).rejects.toMatchObject({ code: 'SESSION_CHANGED', status: 401 });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(auth.refresh).not.toHaveBeenCalled();
  });

  it('刷新期间退出登录时不会继续发送请求', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response('', { status: 401 }));
    vi.stubGlobal('fetch', fetchMock);
    auth.refresh.mockImplementation(async () => { auth.token = ''; return { access_token: 'new-token' }; });
    await expect(binaryRequest('/admin/restore', { method: 'POST' })).rejects.toMatchObject({ code: 'SESSION_CHANGED', status: 401 });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('保留服务端错误信息', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{"code":"INVALID_BACKUP","message":"备份文件无效"}', { status: 400 })));
    await expect(binaryRequest('/admin/restore')).rejects.toMatchObject({ code: 'INVALID_BACKUP', message: '备份文件无效', status: 400 });
  });
});
