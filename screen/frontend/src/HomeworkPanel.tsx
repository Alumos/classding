import {useCallback,useEffect,useState} from 'react';
import {BookOpenCheck,Eye,EyeOff,Pin,RotateCcw} from 'lucide-react';
import {Button,Card,Switch} from '@classding/ui';
import type {Homework} from './homework';
import {native,type HomeworkSettings} from './native';

export function HomeworkPanel({items}:{items:Homework[]}){
  const [settings,setSettings]=useState<HomeworkSettings>({items:[],preferences:{}});
  const [error,setError]=useState('');
  const [busy,setBusy]=useState(false);
  const isNative=!!native();
  const refresh=useCallback(()=>{if(native())void native()!.GetHomeworkSettings().then(setSettings).catch(e=>setError(String(e)));},[]);
  useEffect(()=>{refresh();},[items,refresh]);
  useEffect(()=>{
    const unsub=window.runtime?.EventsOnMultiple('homework:settings',refresh,-1);
    window.addEventListener('classding:homework-applied',refresh);
    const errors=window.runtime?.EventsOnMultiple('homework:error',value=>setError(String(value)),-1);
    return()=>{unsub?.();errors?.();window.removeEventListener('classding:homework-applied',refresh);};
  },[refresh]);
  async function change(id:string,action:string){
    setBusy(true);setError('');
    try{await native()?.SetHomeworkPreference(id,action);refresh();}
    catch(e){setError(`保存作业显示设置失败：${String(e)}`);}
    finally{setBusy(false);}
  }
  return <Card className="screen-homework-panel">
    <div className="screen-homework-heading"><div><h2><BookOpenCheck size={22}/>班级作业</h2><p>{isNative?'作业显示在桌面右侧、任务栏上方。关闭主窗口后仍可查看。':'浏览器仅预览作业；桌面悬浮窗请使用 Windows 客户端。'}</p></div>{isNative&&items.length>0&&<div className="screen-homework-bulk"><Button variant="outline" disabled={busy} onClick={()=>void change('*','show')}><Eye size={16}/>显示全部</Button><Button variant="outline" disabled={busy} onClick={()=>void change('*','hide')}><EyeOff size={16}/>隐藏全部</Button></div>}</div>
    {!items.length?<p className="screen-homework-empty">老师发布的各科作业会在这里出现。</p>:<div className="screen-homework-list">{items.map(item=>{
      const saved=settings.preferences[item.id];
      const pref=saved?.revision===item.updated_at?saved:undefined;
      const visible=pref?.visible??item.visible;
      const top=pref?.always_on_top??item.always_on_top;
      return <article className="screen-homework-item" key={item.id}>
        <header><h3>{item.subject}</h3><span>{item.teacher_name}</span></header>
        <p>{item.text}</p>
        <div className="screen-homework-options"><label><Eye size={17}/>显示<Switch aria-label={`显示${item.subject}作业`} checked={visible} disabled={!isNative||busy} onCheckedChange={value=>void change(item.id,value?'show':'hide')}/></label><label><Pin size={17}/>置顶<Switch aria-label={`置顶${item.subject}作业`} checked={top} disabled={!isNative||busy} onCheckedChange={value=>void change(item.id,value?'pin':'unpin')}/></label>{isNative&&pref&&<Button variant="ghost" size="sm" disabled={busy} onClick={()=>void change(item.id,'default')}><RotateCcw size={15}/>跟随后台设置</Button>}</div>
      </article>;
    })}</div>}
    {isNative&&items.length>0&&<p className="screen-homework-hint">电脑上的调整会保存在本机。老师修改并保存这条作业后，将应用后台的新设置；长作业可在悬浮窗内滚动阅读。</p>}
    {error&&<div className="inline-error" role="alert">{error}</div>}
  </Card>;
}
