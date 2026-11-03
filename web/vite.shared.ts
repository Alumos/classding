import { env } from 'node:process';
import type { ProxyOptions } from 'vite';

// Every app forwards the API and the realtime channel to the local Go server
// during development; CLASSDING_API_URL points them somewhere else.
export function apiProxy(
  target = env.CLASSDING_API_URL || 'http://127.0.0.1:8080',
): Record<string, ProxyOptions> {
  return {
    '/api': { target, changeOrigin: false },
    '/ws': { target: target.replace(/^http/, 'ws'), ws: true, changeOrigin: false },
  };
}
