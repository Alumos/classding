export type {WSFrame,FrameData,AckStatus,Notice} from './ws';
export type {components,paths} from './types';
let accessToken='';
let baseURL='';
export function setAccessToken(token:string|null){accessToken=token??'';}
export function getAccessToken(){return accessToken;}
export function setBaseURL(value:string){baseURL=value.replace(/\/$/,'');}
export class ApiError extends Error{constructor(public code:string,message:string,public status:number){super(message);this.name='ApiError';}}
let refreshing:Promise<boolean>|null=null;
async function refresh(){try{const response=await fetch(`${baseURL}/api/v1/auth/refresh`,{method:'POST',credentials:'include'});if(!response.ok)return false;const data=await response.json();setAccessToken(data.access_token);return true;}catch{return false;}}
export async function api<T=unknown>(path:string,options:RequestInit={},retry=true):Promise<T>{const headers=new Headers(options.headers);if(accessToken)headers.set('Authorization',`Bearer ${accessToken}`);if(options.body&&!(options.body instanceof FormData))headers.set('Content-Type','application/json');let response:Response;try{response=await fetch(`${baseURL}/api/v1${path.startsWith('/')?path:`/${path}`}`,{...options,headers,credentials:'include'});}catch{throw new ApiError('NETWORK_ERROR','无法连接服务，请确认服务已启动并检查网络。',0);}if(response.status===401&&retry&&accessToken&&!path.startsWith('/auth/')){refreshing??=refresh().finally(()=>{refreshing=null;});if(await refreshing)return api<T>(path,options,false);}if(!response.ok){const body=await response.json().catch(()=>({}));throw new ApiError(body.code??'REQUEST_FAILED',body.message??`请求失败（${response.status}）`,response.status);}if(response.status===204)return undefined as T;return response.json() as Promise<T>;}
