export type {WSFrame,FrameData,AckStatus,Notice} from './ws';
export type {components,paths} from './types';

let accessToken='';
let activeUserId='';
let sessionVersion=0;
let baseURL='';
let authScope:''|'admin'|'teacher'='';
export function setAuthScope(scope:'admin'|'teacher'){authScope=scope;}
const expiredListeners=new Set<()=>void>();

// The token subject is used only to keep an existing UI session from silently
// switching accounts when another tab replaces the refresh cookie.
function tokenSubject(token:string):string{
  try{
    const payload=token.split('.')[1].replace(/-/g,'+').replace(/_/g,'/');
    const value=JSON.parse(atob(payload.padEnd(Math.ceil(payload.length/4)*4,'=')));
    return typeof value.sub==='string'?value.sub:'';
  }catch{return '';}
}

export function setAccessToken(token:string|null,userId?:string){
  accessToken=token??'';
  activeUserId=accessToken?(userId??tokenSubject(accessToken)):'';
  sessionVersion++;
}
export function getAccessToken(){return accessToken;}
export function setBaseURL(value:string){baseURL=value.replace(/\/$/,'');}
export class ApiError extends Error{constructor(public code:string,message:string,public status:number){super(message);this.name='ApiError';}}
export function onSessionExpired(listener:()=>void){expiredListeners.add(listener);return()=>{expiredListeners.delete(listener);};}

type RefreshResult={access_token:string;user:{id:string}};
let refreshing:Promise<RefreshResult>|null=null;
function expireSession(){
  setAccessToken(null);
  for(const listener of expiredListeners)listener();
}

// All callers share the same in-flight request because refresh cookies rotate.
// Clear the promise on either outcome so a later login can start a new session.
export function refreshSession<T extends RefreshResult=RefreshResult>():Promise<T>{
  if(refreshing)return refreshing as Promise<T>;
  const version=sessionVersion;
  const expectedUser=activeUserId;
  const request=(async()=>{
    let response:Response;
    try{response=await fetch(`${baseURL}/api/v1/auth/refresh`,{method:'POST',credentials:'include',headers:authScope?{'X-ClassDing-Auth-Scope':authScope}:{}});}
    catch{throw new ApiError('NETWORK_ERROR','无法连接服务，请确认服务已启动并检查网络。',0);}
    if(!response.ok){
      const body=await response.json().catch(()=>({}));
      if(response.status===401&&expectedUser&&sessionVersion===version)expireSession();
      throw new ApiError(body.code??'REQUEST_FAILED',body.message??`请求失败（${response.status}）`,response.status);
    }
    const data=await response.json() as RefreshResult;
    if(sessionVersion!==version)throw new ApiError('SESSION_CHANGED','登录状态已变化，请重新登录。',401);
    if(expectedUser&&data.user?.id!==expectedUser){
      expireSession();
      throw new ApiError('SESSION_CHANGED','登录账号已变化，请重新登录。',401);
    }
    accessToken=data.access_token;
    activeUserId=data.user?.id??tokenSubject(data.access_token);
    return data;
  })();
  refreshing=request.finally(()=>{refreshing=null;});
  return refreshing as Promise<T>;
}

export async function api<T=unknown>(path:string,options:RequestInit={},retry=true):Promise<T>{
  const headers=new Headers(options.headers);
  if(authScope)headers.set('X-ClassDing-Auth-Scope',authScope);
  const requestToken=accessToken;
  const version=sessionVersion;
  if(requestToken)headers.set('Authorization',`Bearer ${requestToken}`);
  if(options.body&&!(options.body instanceof FormData))headers.set('Content-Type','application/json');
  let response:Response;
  try{response=await fetch(`${baseURL}/api/v1${path.startsWith('/')?path:`/${path}`}`,{...options,headers,credentials:'include'});}
  catch{throw new ApiError('NETWORK_ERROR','无法连接服务，请确认服务已启动并检查网络。',0);}
  if(response.status===401&&retry&&requestToken&&!path.startsWith('/auth/')&&version===sessionVersion){
    if(requestToken===accessToken)await refreshSession();
    return api<T>(path,options,false);
  }
  if(!response.ok){const body=await response.json().catch(()=>({}));throw new ApiError(body.code??'REQUEST_FAILED',body.message??`请求失败（${response.status}）`,response.status);}
  if(response.status===204)return undefined as T;
  return response.json() as Promise<T>;
}
