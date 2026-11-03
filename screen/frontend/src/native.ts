export interface DisplaySettings {
  selected: string;
  displays: Array<{id:string;name:string;primary:boolean;width:number;height:number}>;
}
export interface NativeApp {
  LoadState(): Promise<string>;
  SaveState(value: string): Promise<void>;
  ConfigureServer(server: string): Promise<string>;
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
export async function configureServer(server: string): Promise<string> {
  if (native()) return native()!.ConfigureServer(server);
  return server.replace(/\/$/, '');
}
