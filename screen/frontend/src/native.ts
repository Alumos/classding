import type {Notice} from '@classding/api/ws';
export interface DisplaySettings {
  selected: string;
  displays: Array<{id:string;name:string;primary:boolean;width:number;height:number}>;
}
export interface ProbeResult {device:{id:string;class_id:string;name?:string};sequence:number;notifications:Array<{t:string;id?:string;seq?:number;d:Notice}>;recalled_ids:string[];}
export interface NativeApp {
  LoadState(): Promise<string>;
  SaveState(value: string): Promise<void>;
  ConfigureServer(server: string): Promise<string>;
  ProbeServer(server: string, token: string, minSequence:number, pendingIds:string[]): Promise<ProbeResult>;
  SetPresentationActive(active: boolean): Promise<void>;
  PrepareAudio(volume: number): Promise<void>;
  RestoreAudio(): Promise<void>;
  SetAutostart(enabled: boolean): Promise<void>;
  GetAutostart(): Promise<boolean>;
  GetTrayError(): Promise<string>;
  GetDisplaySettings(): Promise<DisplaySettings>;
  SetSelectedDisplay(id:string): Promise<void>;
}

declare global {
  interface Window {
    go?: { main?: { App?: NativeApp } };
    runtime?: { EventsOnMultiple(name:string, callback:() => void, maxCallbacks:number): () => void };
  }
}
export const native = (): NativeApp | undefined => window.go?.main?.App;
const KEY = 'classding.screen.state.v1';
export async function loadState(): Promise<string> { return native() ? (await native()!.LoadState()) : (localStorage.getItem(KEY) || ''); }
export async function saveState(value: string): Promise<void> { if (native()) await native()!.SaveState(value); else localStorage.setItem(KEY, value); }
export function normalizeServer(server: string, allowEmpty = false): string {
  if (!server && allowEmpty) return '';
  // Reject paths before URL normalization, which would otherwise hide /a/.. .
  if (!/^https?:\/\/[^/?#\\@]+\/?$/i.test(server)) throw new Error('服务器地址应为 http:// 或 https:// 开头的网址，不包含路径');
  const url = new URL(server);
  if (!['http:','https:'].includes(url.protocol) || url.username || url.password || url.pathname !== '/' || url.search || url.hash) throw new Error('服务器地址无效');
  return url.origin;
}
export async function configureServer(server: string): Promise<string> {
  const normalized = normalizeServer(server, true);
  if (native()) return native()!.ConfigureServer(normalized);
  return normalized;
}
export async function probeServer(server: string, token: string, minSequence:number, pendingIds:string[]): Promise<ProbeResult> {
  const normalized = normalizeServer(server);
  if (native()) return native()!.ProbeServer(normalized, token, minSequence, pendingIds);
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 5000);
  try {
    const response = await fetch(`${normalized}/api/v1/device/migration-check`, {method:'POST',headers:{Authorization:`Bearer ${token}`,'Content-Type':'application/json'},body:JSON.stringify({min_sequence:minSequence,pending_ids:pendingIds}),redirect:'error',signal:controller.signal});
    if (!response.ok) throw new Error(`新服务器身份验证失败（${response.status}）`);
    const result = await response.json() as ProbeResult;
    if (!result?.device?.id || !result.device.class_id) throw new Error('新服务器返回的设备身份不完整');
    return result;
  } finally { clearTimeout(timer); }
}
