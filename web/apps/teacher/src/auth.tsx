import {createContext,useContext,useEffect,useRef,useState,type ReactNode} from 'react';
import {api,onSessionExpired,refreshSession,setAccessToken} from '@classding/api';
import {useQueryClient} from '@tanstack/react-query';
import {toast} from 'sonner';
import type {AuthResponse,User} from './models';

const Context=createContext<{user:User|null;ready:boolean;authenticate:(response:AuthResponse)=>void;logout:()=>Promise<void>;updateUser:(u:User)=>void}>(null!);

export function AuthProvider({children}:{children:ReactNode}){
  const [user,setUser]=useState<User|null>(null);
  const [ready,setReady]=useState(false);
  const revision=useRef(0);
  const cache=useQueryClient();

  useEffect(()=>onSessionExpired(()=>{
    revision.current++;
    setUser(null);
    cache.clear();
    toast.error('登录状态已变化，请重新登录');
  }),[cache]);

  useEffect(()=>{
    let alive=true;
    const current=revision.current;
    refreshSession<AuthResponse>().then(data=>{
      if(alive&&current===revision.current)setUser(data.user);
    }).catch(()=>{}).finally(()=>{
      if(alive&&current===revision.current)setReady(true);
    });
    return()=>{alive=false;};
  },[]);

  function authenticate(response:AuthResponse){
    revision.current++;
    setAccessToken(response.access_token,response.user.id);
    setUser(response.user);
    cache.clear();
    setReady(true);
  }
  async function logout(){
    const request=api('/auth/logout',{method:'POST'});
    revision.current++;
    setAccessToken(null);
    setUser(null);
    setReady(true);
    cache.clear();
    try{await request;}catch{/* Local sign-out should still complete if the server is unavailable. */}
  }
  return <Context.Provider value={{user,ready,authenticate,logout,updateUser:setUser}}>{children}</Context.Provider>;
}
export function useAuth(){return useContext(Context);}
