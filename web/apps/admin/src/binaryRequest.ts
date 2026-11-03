import { ApiError, getAccessToken, refreshSession } from '@classding/api';

/** Send a ZIP request without the JSON content-type imposed by the shared helper. */
export async function binaryRequest(path: string, options: RequestInit = {}): Promise<Response> {
  const request = async (token: string) => {
    const headers = new Headers(options.headers);
    headers.set('X-ClassDing-Auth-Scope', 'admin');
    if (token) headers.set('Authorization', `Bearer ${token}`);
    try {
      return await fetch(`/api/v1${path}`, { ...options, headers, credentials: 'include' });
    } catch {
      throw new ApiError('NETWORK_ERROR', '无法连接服务，请确认服务已启动并检查网络。', 0);
    }
  };

  const token = getAccessToken();
  let response = await request(token);
  if (response.status === 401 && token) {
    // Do not replay a request under another account if the session changed.
    if (getAccessToken() !== token)
      throw new ApiError('SESSION_CHANGED', '登录状态已变化，请重新操作。', 401);
    const refreshed = await refreshSession();
    if (getAccessToken() !== refreshed.access_token)
      throw new ApiError('SESSION_CHANGED', '登录状态已变化，请重新操作。', 401);
    response = await request(refreshed.access_token);
  }
  if (!response.ok) {
    const body = (await response.json().catch(() => ({}))) as { code?: string; message?: string };
    throw new ApiError(
      body.code ?? 'REQUEST_FAILED',
      body.message ?? `请求失败（${response.status}）`,
      response.status,
    );
  }
  return response;
}
