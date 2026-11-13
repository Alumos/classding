import type { Homework } from './homework';
export interface DisplaySettings {
  selected: string;
  displays: Array<{ id: string; name: string; primary: boolean; width: number; height: number }>;
}
export interface HomeworkPreference {
  revision: string;
  visible?: boolean;
  always_on_top?: boolean;
}
export interface HomeworkSettings {
  items: Homework[];
  preferences: Record<string, HomeworkPreference>;
}
export interface NativeApp {
  UpdateStudentCallStatus(bound: boolean, online: boolean, className: string, binding: number): Promise<void>;
  CompleteStudentCall(id: string, success: boolean, message: string): Promise<void>;
  UpdateHomeworks(value: string): Promise<void>;
  GetHomeworkSettings(): Promise<HomeworkSettings>;
  SetHomeworkPreference(id: string, action: string): Promise<void>;
  LoadState(): Promise<string>;
  SaveState(value: string): Promise<void>;
  ConfigureServer(server: string): Promise<string>;
  GetLocalAddresses(): Promise<string[]>;
  SetPresentationActive(active: boolean): Promise<void>;
  PrepareAudio(volume: number): Promise<void>;
  RestoreAudio(): Promise<void>;
  SetAutostart(enabled: boolean): Promise<void>;
  GetAutostart(): Promise<boolean>;
  GetTrayError(): Promise<string>;
  GetDisplaySettings(): Promise<DisplaySettings>;
  SetSelectedDisplay(id: string): Promise<void>;
}

declare global {
  interface Window {
    go?: { main?: { App?: NativeApp } };
    runtime?: {
      EventsOnMultiple(
        name: string,
        callback: (...args: unknown[]) => void,
        maxCallbacks: number,
      ): () => void;
    };
  }
}
export const native = (): NativeApp | undefined =>
  typeof window === 'undefined' ? undefined : window.go?.main?.App;
const KEY = 'classding.screen.state.v1';
export async function loadState(): Promise<string> {
  return native() ? await native()!.LoadState() : localStorage.getItem(KEY) || '';
}
export async function saveState(value: string): Promise<void> {
  if (native()) await native()!.SaveState(value);
  else localStorage.setItem(KEY, value);
}
export function normalizeServer(server: string, allowEmpty = false): string {
  if (!server && allowEmpty) return '';
  // Reject paths before URL normalization, which would otherwise hide /a/.. .
  if (!/^https?:\/\/[^/?#\\@]+\/?$/i.test(server))
    throw new Error('服务器地址应为 http:// 或 https:// 开头的网址，不包含路径');
  const url = new URL(server);
  if (
    !['http:', 'https:'].includes(url.protocol) ||
    url.username ||
    url.password ||
    url.pathname !== '/' ||
    url.search ||
    url.hash
  )
    throw new Error('服务器地址无效');
  return url.origin;
}
export async function configureServer(server: string): Promise<string> {
  const normalized = normalizeServer(server, true);
  if (native()) return native()!.ConfigureServer(normalized);
  return normalized;
}
// LAN IPv4 addresses of the classroom computer; the browser preview has none.
export async function getLocalAddresses(): Promise<string[]> {
  if (!native()) return [];
  const addresses = await native()!.GetLocalAddresses();
  return Array.isArray(addresses) ? addresses.filter((item) => typeof item === 'string').slice(0, 8) : [];
}

// All updates, including unbind cleanup, share one native call queue.
let homeworkWindowUpdates: Promise<void> = Promise.resolve();
export function syncHomeworkWindows(items: Homework[]): Promise<void> {
  const value = JSON.stringify(items);
  const operation = homeworkWindowUpdates.then(async () => {
    if (native()) await native()!.UpdateHomeworks(value);
  });
  homeworkWindowUpdates = operation.catch(() => {});
  return operation;
}
