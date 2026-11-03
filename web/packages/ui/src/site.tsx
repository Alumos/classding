import * as React from 'react';

export interface SiteSettings { site_name: string; icp_number: string; theme: 'green' | 'blue' | 'slate' }
const defaults: SiteSettings = {site_name:'课堂叮咚',icp_number:'',theme:'green'};
const SiteContext = React.createContext<SiteSettings>(defaults);

export function SiteSettingsProvider({children}: {children:React.ReactNode}) {
  const [settings,setSettings] = React.useState(defaults);
  React.useEffect(()=>{
    let active=true;
    let controller:AbortController|undefined;
    const load=async()=>{
      controller?.abort();
      controller=new AbortController();
      try {
        const response=await fetch('/api/v1/config',{signal:controller.signal,cache:'no-store'});
        if(!response.ok)return;
        const data=await response.json() as Partial<SiteSettings>;
        if(active)setSettings({
          site_name:typeof data.site_name==='string'&&data.site_name.trim()?data.site_name:defaults.site_name,
          icp_number:typeof data.icp_number==='string'?data.icp_number:'',
          theme:data.theme==='blue'||data.theme==='slate'?data.theme:'green',
        });
      }catch{/* Keep the current appearance when temporarily offline. */}
    };
    void load();
    const reload=()=>void load();
    window.addEventListener('classding:site-settings',reload);
    window.addEventListener('focus',reload);
    return ()=>{active=false;controller?.abort();window.removeEventListener('classding:site-settings',reload);window.removeEventListener('focus',reload);};
  },[]);
  React.useEffect(()=>{
    document.title=settings.site_name;
    document.documentElement.dataset.cdTheme=settings.theme;
  },[settings]);
  return <SiteContext.Provider value={settings}>{children}</SiteContext.Provider>;
}

export function useSiteSettings(){return React.useContext(SiteContext);}

export function SiteFooter({className=''}:{className?:string}){
  const {site_name,icp_number}=useSiteSettings();
  return <footer className={`cd-site-footer ${className}`}><span>© {site_name}</span>{icp_number&&<a href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer">{icp_number}</a>}</footer>;
}
