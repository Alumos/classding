import React, {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {createRoot} from 'react-dom/client';
import {Badge, Brand, Button, Card, Input, NoticeScreen, Switch} from '@classding/ui';
import {ArrowRight, AudioLines, BellRing, Check, ChevronDown, CircleHelp, Clock3, Link2, Monitor, Radio, Settings2, ShieldCheck, Volume2, Wifi, WifiOff, X} from 'lucide-react';
import {ScreenService} from './service';
import {native, type DisplaySettings} from './native';
import {playNotice, unlockAudio} from './audio';
import '@classding/ui/styles.css';
import './styles.css';

const service = new ScreenService();
void service.init();

function App() {
  const state = useSyncExternalStore(service.subscribe,service.getSnapshot);
  const [code,setCode] = useState('');
  const [deviceName,setDeviceName] = useState('');
  const [server,setServer] = useState('');
  const [advanced,setAdvanced] = useState(false);
  const [focused,setFocused] = useState(false);
  const [soundTesting,setSoundTesting] = useState(false);
  const [localError,setLocalError] = useState('');
  const [trayError,setTrayError] = useState('');
  const [autostart,setAutostart] = useState(false);
  const [displaySettings,setDisplaySettings] = useState<DisplaySettings | null>(null);
  const [now,setNow] = useState(Date.now());
  const codeRef = useRef<HTMLInputElement>(null);
  const testController = useRef<AbortController | null>(null);
  const isNative = !!native();
  useEffect(() => { if (state.ready) { setServer(state.server); setDeviceName(state.deviceName); } },[state.ready]);
  useEffect(() => { if (state.current) testController.current?.abort(); },[state.current]);
  useEffect(() => { if (isNative) void native()!.GetAutostart().then(setAutostart).catch(() => {}); },[isNative]);
  useEffect(() => {
    if (!isNative) return;
    const refresh = () => { void native()!.GetTrayError().then(error => { if (error) setTrayError(`后台待命不可用：${error}`); }).catch(() => {}); };
    const unsubscribe = window.runtime?.EventsOnMultiple('tray:unavailable',refresh,-1);
    refresh(); window.addEventListener('focus',refresh);
    return () => { unsubscribe?.(); window.removeEventListener('focus',refresh); };
  },[isNative]);
  useEffect(() => {
    if (!isNative) return;
    const refresh = () => { void native()!.GetDisplaySettings().then(setDisplaySettings).catch(() => {}); };
    refresh(); window.addEventListener('focus',refresh);
    return () => window.removeEventListener('focus',refresh);
  },[isNative]);
  useEffect(() => {
    if (!state.current && !state.tool) return;
    setNow(Date.now());
    const timer = setInterval(() => setNow(Date.now()),150);
    return () => clearInterval(timer);
  },[state.current,state.tool]);
  useEffect(() => { if (isNative) void native()!.SetPresentationActive(!!state.current || !!state.tool).catch(error => setLocalError(`全屏显示失败：${String(error)}`)); },[isNative,!!state.current,!!state.tool]);
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { if (state.current) void service.dismiss(); else if (state.tool) service.closeTool(); }
    };
    window.addEventListener('keydown',onKey); return () => window.removeEventListener('keydown',onKey);
  },[state.current,state.tool]);
  const bind = async (event: React.FormEvent) => {
    event.preventDefault(); setLocalError('');
    void unlockAudio();
    const address = server.trim().replace(/\/$/,'');
    if (isNative && !address) { setAdvanced(true); setLocalError('请先填写课堂叮咚服务器地址'); return; }
    if (address) { try { const url = new URL(address); if (!['http:','https:'].includes(url.protocol) || url.pathname !== '/' || url.search || url.hash || url.username || url.password) throw new Error(); } catch { setAdvanced(true); setLocalError('服务器地址应为 http:// 或 https:// 开头的网址，不包含路径'); return; } }
    try { await service.bind(code,deviceName,address); setCode(''); } catch { /* Message is shown in state. */ }
  };
  const testSound = async () => {
    setSoundTesting(true); setLocalError('');
    const controller = new AbortController(); testController.current = controller;
    try { await unlockAudio(); await playNotice({text:'叮咚，课堂叮咚已准备就绪。',chime:true,repeat:1,volume:state.volume},url=>url,controller.signal); }
    catch (error) { if (!controller.signal.aborted) setLocalError(`测试播报失败：${error instanceof Error ? error.message : String(error)}`); }
    finally { if (testController.current === controller) testController.current = null; setSoundTesting(false); }
  };
  const toggleStart = async (checked:boolean) => {
    try { await native()?.SetAutostart(checked); setAutostart(checked); setLocalError(''); }
    catch (error) { setLocalError(`设置开机启动失败：${error instanceof Error ? error.message : String(error)}`); }
  };
  const selectDisplay = async (id:string) => {
    try { await native()?.SetSelectedDisplay(id); setDisplaySettings(current => current ? {...current,selected:id} : current); setLocalError(''); }
    catch (error) { setLocalError(`选择显示器失败：${error instanceof Error ? error.message : String(error)}`); }
  };
  const error = localError || trayError || state.error;
  const current = state.current;
  const progress = current ? Math.max(0,100-(now-state.startedAt)/(current.display_sec*1000)*100) : 0;
  const tool = state.tool;
  if (!state.ready) return <div className="screen-loading"><Brand/><span>正在准备大屏…</span></div>;
  return <>
    <main className="shell">
      <header className="topbar"><Brand/><div className="topbar-right"><span className="desktop-label"><Monitor size={15}/> 大屏客户端</span><span className="version">V 0.1.3</span></div></header>
      {!state.bound ? <div className="bind-layout">
        <section className="bind-intro"><div className="eyebrow"><span className="eyebrow-dot"/> 让每一句话，都被听见</div><h1>连接班级，<br/><em>让课堂有回应。</em></h1><p>老师发来消息时，课堂叮咚会在大屏上展示并为同学们朗读。</p><div className="intro-steps"><div><span>01</span>老师在手机端生成绑定码</div><div><span>02</span>在此输入 6 位数字完成连接</div><div><span>03</span>保持运行，随时接收班级消息</div></div><div className="intro-decoration"><BellRing size={150} strokeWidth={.7}/></div></section>
        <section className="bind-side"><Card className="binding-card"><div className="card-icon"><Link2 size={25}/></div><div className="card-heading"><span className="overline">READY TO CONNECT</span><h2>绑定到班级</h2><p>打开教师端「班级」，获取 6 位绑定码</p></div>
          <form onSubmit={bind}><label className="input-label" htmlFor="bind-code">班级绑定码</label><div className={`code-boxes ${focused ? 'is-focused' : ''}`} onClick={()=>codeRef.current?.focus()}>
            {Array.from({length:6},(_,index)=><span key={index} className={index === code.length && focused ? 'active' : ''}>{code[index] || ''}</span>)}
            <input ref={codeRef} id="bind-code" value={code} onChange={event=>setCode(event.target.value.replace(/\D/g,'').slice(0,6))} onFocus={()=>setFocused(true)} onBlur={()=>setFocused(false)} autoComplete="one-time-code" inputMode="numeric" aria-label="6 位班级绑定码"/>
          </div>
          <button type="button" className="advanced-toggle" onClick={()=>setAdvanced(!advanced)}><Settings2 size={15}/> 设备与服务器设置 <ChevronDown className={advanced ? 'rotated' : ''} size={15}/></button>
          {(advanced || (isNative && !server)) && <div className="advanced-fields"><label htmlFor="device-name">设备名称</label><Input id="device-name" value={deviceName} placeholder="留空则使用班级名 + 智慧黑板" maxLength={40} onChange={event=>setDeviceName(event.target.value)}/><label htmlFor="server">服务器地址 {isNative && <span>（必填）</span>}</label><Input id="server" value={server} placeholder={isNative ? 'https://你的服务器地址' : '留空使用当前服务'} onChange={event=>setServer(event.target.value)}/><small>{isNative ? '请向管理员获取服务地址。' : '本地预览留空将连接本机服务。'}</small></div>}
          {error && <div className="inline-error" role="alert">{error}</div>}
          <Button className="bind-button" disabled={code.length !== 6 || state.busy} type="submit">{state.busy ? '正在连接…' : '连接班级'} <ArrowRight size={17}/></Button></form>
          <div className="binding-foot"><ShieldCheck size={15}/> 绑定码仅可使用一次，请妥善保管</div></Card><p className="help-text"><CircleHelp size={15}/> 需要帮助？请联系你的学校管理员</p></section>
      </div> : <div className="dashboard"><div className="dashboard-heading"><div><div className="eyebrow"><span className="eyebrow-dot"/> CLASSDING SCREEN</div><h1>一切准备就绪<span>.</span></h1><p>保持课堂叮咚运行，班级消息会自动出现在这里。</p></div><Badge variant={state.connection === 'online' ? 'success' : 'warning'}>{state.connection === 'online' ? <Wifi size={13}/> : <WifiOff size={13}/>} {state.connection === 'online' ? '连接正常' : state.connection === 'connecting' ? '正在连接' : '连接中断 · 自动重试'}</Badge></div>
        <div className="dashboard-grid"><Card className="class-card"><div className="class-card-top"><span className="class-icon"><Monitor size={22}/></span><Badge variant="success"><span className="status-dot"/> 已绑定</Badge></div><span className="section-label">当前班级</span><h2>{state.className}</h2><p>{state.deviceName}</p><div className="class-divider"/><div className="class-summary"><div><Radio size={18}/><span>消息接收<strong>{state.connection === 'online' ? '在线待命' : '等待连接'}</strong></span></div><div><Volume2 size={18}/><span>班级音量<strong>{state.volume}%</strong></span></div></div></Card>
          <div className="dashboard-side"><Card className="ready-card"><div className="ready-visual"><span className="sound-orbit orbit-one"/><span className="sound-orbit orbit-two"/><span className="ready-bell"><BellRing size={37}/></span></div><div><h3>声音准备好了吗？</h3><p>可以播放一段提示音和语音，确认教室能够听清。</p><Button variant="secondary" onClick={()=>void testSound()} disabled={soundTesting}><AudioLines size={16}/>{soundTesting ? '正在测试…' : '测试播报'}</Button></div></Card>
            <Card className="settings-card"><div className="setting-row"><span className="setting-icon"><Clock3 size={18}/></span><div><strong>开机启动</strong><p>{isNative ? '电脑开机后在后台自动守候消息' : '仅 Windows 客户端支持'}</p></div><Switch checked={autostart} onCheckedChange={checked=>void toggleStart(checked)} disabled={!isNative} aria-label="开机启动"/></div>{displaySettings && displaySettings.displays.length > 0 && <div className="setting-row"><span className="setting-icon"><Monitor size={18}/></span><div><strong>通知显示器</strong><p>通知与课堂工具出现的位置</p></div><select className="display-select" aria-label="通知显示器" value={displaySettings.selected} onChange={event=>void selectDisplay(event.target.value)}><option value="">跟随当前窗口</option>{displaySettings.selected && !displaySettings.displays.some(display=>display.id===displaySettings.selected) && <option value={displaySettings.selected}>所选显示器已断开</option>}{displaySettings.displays.map(display=><option key={display.id} value={display.id}>{display.name}</option>)}</select></div>}<div className="setting-row"><span className="setting-icon"><ShieldCheck size={18}/></span><div><strong>设备管理</strong><p>如需解绑，请在教师端班级设置中操作</p></div><Check size={17} className="muted-check"/></div></Card></div></div>
        {error && <div className="inline-error dashboard-error" role="alert">{error}</div>}<div className="dashboard-note"><span><span className="status-dot"/> {state.connection === 'online' ? '正在守候班级消息' : '正在尝试恢复连接'}</span><span>课堂叮咚 · 让沟通更简单</span></div></div>}
      <footer className="page-footer"><span>© 课堂叮咚 ClassDing</span><span>为每一间教室而设计 <span className="footer-heart">✳</span></span></footer>
    </main>
    {current && <div className="presentation" data-style={current.style}><NoticeScreen text={current.text || '老师发来一条语音消息'} studentName={current.student_name} signature={current.signature || '老师'} className={current.class_name || state.className} style={current.style} progress={progress} sentAt={current.sent_at}/><div className="presentation-controls">{state.pending > 0 && <span>还有 {state.pending} 条</span>}<button type="button" className="dismiss-notice" onClick={()=>void service.dismiss()}><Check size={22}/>已读通知，关闭消息</button></div></div>}
    {!current && tool && <div className="tool-screen"><header><Brand/><button type="button" onClick={()=>service.closeTool()} aria-label="关闭课堂工具"><X size={24}/></button></header>{tool.name === 'countdown' ? <div className="tool-content"><span className="tool-label"><Clock3 size={19}/> 课堂倒计时</span><div className="countdown-time">{formatTime(Math.max(0,Math.ceil((tool.end-now)/1000)))}</div><p>{now >= tool.end ? '时间到！' : '专注的每一分钟，都在闪闪发光'}</p></div> : <div className="tool-content"><span className="tool-label"><BellRing size={19}/> 幸运点名</span><div className="rollcall-name">{now < tool.ends ? tool.names[Math.floor(now / 120) % tool.names.length] : tool.selected}</div><p>{now < tool.ends ? '正在抽取一位同学…' : '就是你啦！'}</p></div>}<footer>{state.className} · 课堂叮咚</footer></div>}
  </>;
}
function formatTime(total:number) { const hours = Math.floor(total/3600); const minutes = Math.floor(total%3600/60); const seconds = total%60; return [hours ? String(hours).padStart(2,'0') : '',String(minutes).padStart(2,'0'),String(seconds).padStart(2,'0')].filter(Boolean).join(':'); }

createRoot(document.getElementById('root')!).render(<App/>);
