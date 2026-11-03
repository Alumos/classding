import {createContext,useContext,useEffect,useState,type ReactNode} from 'react';
import {api,setAccessToken} from '@classding/api';
import {useQueryClient} from '@tanstack/react-query';
import type {AuthResponse,User} from './models';
const Context=createContext<{user:User|null;ready:boolean;authenticate:(response:AuthResponse)=>void;logout:()=>Promise<void>;updateUser:(u:User)=>void}>(null!);
let initialRefresh:Promise<AuthResponse>|null=null;
export function AuthProvider({children}:{children:ReactNode}){const [user,setUser]=useState<User|null>(null);const [ready,setReady]=useState(false);const cache=useQueryClient();useEffect(()=>{let alive=true;initialRefresh??=api<AuthResponse>('/auth/refresh',{method:'POST'});initialRefresh.then(data=>{if(alive){setAccessToken(data.access_token);setUser(data.user);}}).catch(()=>{}).finally(()=>{if(alive)setReady(true);});return()=>{alive=false;};},[]);function authenticate(response:AuthResponse){setAccessToken(response.access_token);setUser(response.user);cache.clear();setReady(true);}async function logout(){try{await api('/auth/logout',{method:'POST'});}finally{setAccessToken('');setUser(null);initialRefresh=null;cache.clear();}}return <Context.Provider value={{user,ready,authenticate,logout,updateUser:setUser}}>{children}</Context.Provider>;}
export function useAuth(){return useContext(Context);}
