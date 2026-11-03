import {useEffect,useRef,useState} from 'react';
import {Link} from 'react-router';
import {useMutation,useQueryClient} from '@tanstack/react-query';
import {api} from '@classding/api';
import {Badge,Button,Card,Dialog,Input,NoticeScreen,Select,Switch,Textarea,cn} from '@classding/ui';
import {ArrowRight,AudioLines,BellRing,Check,CheckCheck,ChevronDown,ChevronRight,Eye,Keyboard,LayoutGrid,LoaderCircle,Mic,Monitor,Plus,Send,Settings2,ShieldCheck,Sparkles,UserRound,Volume2,Zap} from 'lucide-react';
import {toast} from 'sonner';
import {useAuth} from '../auth';
import {useClasses,useRecorder} from '../hooks';
import {EmptyState,ErrorState,Loading,PageHeading} from '../components';
import type {Message} from '../models';

const styles=[
  {value:'notice',label:'日常通知',icon:<BellRing size={17}/>},
  {value:'praise',label:'表扬一下',icon:<Sparkles size={17}/>},
  {value:'urgent',label:'紧急提醒',icon:<Zap size={17}/>},
];
const durations=[
  {value:'15',label:'15 秒'},{value:'30',label:'30 秒'},{value:'60',label:'1 分钟'},
  {value:'120',label:'2 分钟'},{value:'180',label:'3 分钟'},
  {value:'300',label:'5 分钟'},{value:'600',label:'10 分钟'},
];

export default function SendPage(){
  const {user}=useAuth();
  const cache=useQueryClient();
  const classes=useClasses();
  const [selected,setSelected]=useState<string[]>([]);
  const initialized=useRef(false);
  const [mode,setMode]=useState<'text'|'voice'>('text');
  const [text,setText]=useState('');
  const [student,setStudent]=useState('');
  const signature=user?.display_name||user?.username||'老师';
  const [style,setStyle]=useState('notice');
  const [duration,setDuration]=useState('60');
  const [repeat,setRepeat]=useState('2');
  const [chime,setChime]=useState(false);
  const [settings,setSettings]=useState(false);
  const [preview,setPreview]=useState(false);
  const [audio,setAudio]=useState<{audio_id:string;url:string}|null>(null);
  const [uploading,setUploading]=useState(false);
  const [lastSent,setLastSent]=useState<Message|null>(null);
  const charCount=Array.from(text).length;
  useEffect(()=>{
    if(classes.data?.length&&!initialized.current){initialized.current=true;setSelected([classes.data[0].id]);}
  },[classes.data]);
  const selectedClasses=(classes.data??[]).filter(item=>selected.includes(item.id));
  const onlineCount=(classes.data??[]).reduce((sum,item)=>sum+(item.online_devices??0),0);
  async function upload(blob:Blob){
    setUploading(true);
    try{
      const body=new FormData();body.append('file',blob,`recording.${blob.type.includes('mp4')?'m4a':'webm'}`);
      const result=await api<{audio_id:string;url:string;transcript?:string}>('/audio',{method:'POST',body});
      setAudio(result);if(result.transcript)setText(result.transcript);
      toast.success('录音已上传，可发送老师原声');
    }catch(error){toast.error(error instanceof Error?error.message:'录音上传失败');}
    finally{setUploading(false);}
  }
  const recorder=useRecorder(blob=>void upload(blob));
  const sent=useMutation({
    mutationFn:()=>api<Message>('/messages',{method:'POST',body:JSON.stringify({
      class_ids:selectedClasses.map(item=>item.id),kind:mode==='voice'?'voice':'text',text,
      student_name:student.trim(),signature,style,display_sec:Number(duration),repeat:Number(repeat),
      chime:style==='urgent'&&chime,play_mode:mode==='voice'?'voice':'tts',
      ...(mode==='voice'&&audio?{audio_id:audio.audio_id}:{}),
    })}),
    onSuccess:message=>{setLastSent(message);toast.success('通知已提交，正在投递到教室');void cache.invalidateQueries({queryKey:['messages']});},
    onError:error=>toast.error(error.message),
  });
  function toggle(id:string){setSelected(value=>value.includes(id)?value.filter(item=>item!==id):[...value,id]);}
  const canSend=selectedClasses.length>0&&charCount<=200&&(mode==='voice'?!!audio:!!text.trim())&&!recorder.recording&&!uploading;
  return <>
    <PageHeading eyebrow="A LITTLE DING, A LOT OF CONNECTION" title="把提醒，送进教室 ✦" description="选好班级，写下想说的话，剩下的交给叮咚。" action={<div className="online-pill"><span className="live-dot"/><strong>{onlineCount}</strong> 块大屏在线<Monitor size={15}/></div>}/>
    <div className="send-grid">
      <div className="composer-column">
        <Card className="class-selector-card">
          <div className="section-heading"><h2><span className="step-number">01</span>选择接收班级</h2><button className="text-button" onClick={()=>setSelected(selectedClasses.length===(classes.data?.length??0)?[]:(classes.data??[]).map(item=>item.id))} disabled={!classes.data?.length}>{selectedClasses.length===(classes.data?.length??0)&&classes.data?.length?'取消全选':'全选班级'}</button></div>
          {classes.isLoading?<Loading/>:classes.error?<ErrorState error={classes.error} retry={()=>void classes.refetch()}/>:classes.data?.length?<div className="class-options">{classes.data.map((item,index)=>{
            const active=selected.includes(item.id);
            return <button key={item.id} className={cn('class-option',active&&'selected')} onClick={()=>toggle(item.id)} aria-pressed={active}><span className={`class-option-icon class-color-${index%3}`}><LayoutGrid size={19} strokeWidth={1.7}/></span><strong>{item.name}</strong><span className="class-option-online"><i className={item.online_devices?'online':'offline'}/>{item.online_devices?'大屏在线':'大屏离线'}</span><span className="class-check">{active&&<Check size={12} strokeWidth={3}/>}</span></button>;
          })}</div>:<EmptyState icon={<LayoutGrid size={24}/>} title="你的第一间教室，等你连接" description="使用激活码创建班级，或输入协同码加入现有班级。" action={<Link className="cd-button cd-button-secondary" to="/classes"><Plus size={16}/>添加班级</Link>}/>}
          <div className="class-selector-footer"><ShieldCheck size={14}/><span>只有你所在班级的大屏，会收到这条通知。</span></div>
        </Card>
        <Card className="compose-card">
          <div className="section-heading"><h2><span className="step-number">02</span>写一条通知</h2><div className="compose-tabs"><button className={mode==='text'?'active':''} onClick={()=>setMode('text')}><Keyboard size={15}/>文字</button><button className={mode==='voice'?'active':''} onClick={()=>setMode('voice')}><Mic size={15}/>语音</button></div></div>
          {mode==='text'?<div className="message-textarea-wrap"><Textarea aria-label="通知内容" value={text} onChange={event=>{if(Array.from(event.target.value).length<=200)setText(event.target.value);}} placeholder="写下你的提醒，例如：请带上数学作业，到办公室找我。" className="message-textarea"/><span className={cn('character-count',charCount>180&&'near-limit')}>{charCount}<i>/ 200</i></span></div>:<div className="voice-area"><span className={cn('voice-orb',recorder.recording&&'recording')}><AudioLines size={32}/></span><h3>{uploading?'正在上传录音…':recorder.recording?'正在录音，再点一次结束':audio?'原声已准备好':'用熟悉的声音，传达提醒'}</h3><p>最长 30 秒 · 不进行语音转文字</p><Button disabled={uploading} variant={recorder.recording?'destructive':'secondary'} onClick={()=>recorder.recording?recorder.stop():void recorder.start()}>{uploading?<LoaderCircle className="spin" size={16}/>:<Mic size={16}/>}{recorder.recording?'结束录音':audio?'重新录音':'点击开始录音'}</Button>{audio&&<audio controls src={audio.url} className="voice-audio"/>}{recorder.error&&<div className="form-error">{recorder.error}</div>}</div>}
          <div className="student-signature-fields">
            <label htmlFor="student-name"><span>提醒哪位同学<small>选填</small></span><Input id="student-name" value={student} onChange={event=>setStudent(event.target.value)} maxLength={24} placeholder="例如：李沐阳"/><p>姓名会在大屏上醒目显示</p></label>
            <label htmlFor="teacher-signature"><span>老师落款</span><Input id="teacher-signature" aria-label="老师落款" value={signature} readOnly aria-readonly="true"/><p><UserRound size={14}/>使用当前登录教师的姓名</p></label>
          </div>
          <div className="notice-options-grid">
            <div className="notice-option-field"><label htmlFor="notice-style">通知样式</label><Select id="notice-style" label="通知样式" value={style} options={styles} onValueChange={value=>{setStyle(value);setChime(false);}}/></div>
            <div className="notice-option-field"><label htmlFor="notice-duration">通知显示时长</label><Select id="notice-duration" label="通知显示时长" value={duration} options={durations} onValueChange={setDuration}/></div>
          </div>
          <p className="notice-duration-hint">学生可点击“已读通知，关闭消息”提前关闭。</p>
          {style==='urgent'?<div className="notice-sound-row"><label htmlFor="urgent-chime"><strong>紧急事项提示音</strong><span>需要时手动开启，播放叮咚声提醒学生。</span></label><Switch id="urgent-chime" checked={chime} onCheckedChange={setChime} aria-label="紧急事项提示音"/></div>:<div className="notice-sound-hint"><BellRing size={16}/><span>提示音默认关闭，紧急提醒可手动开启。</span></div>}
          <div className="send-settings"><button className="settings-toggle" onClick={()=>setSettings(!settings)} aria-expanded={settings} aria-controls="speech-settings"><span><Settings2 size={16}/>语音播报设置</span><span>朗读 {repeat} 遍<ChevronDown size={16} className={settings?'rotated':''}/></span></button>{settings&&<div id="speech-settings" className="speech-settings-panel"><div className="notice-option-field"><label htmlFor="speech-repeat">朗读次数</label><Select id="speech-repeat" label="朗读次数" value={repeat} options={['1','2','3'].map(value=>({value,label:`${value} 遍`}))} onValueChange={setRepeat}/></div><p>语音朗读与叮咚提示音分别设置。</p></div>}</div>
          <div className="send-action"><Button className="send-submit" disabled={!canSend||sent.isPending} onClick={()=>sent.mutate()}>{sent.isPending?<LoaderCircle className="spin" size={17}/>:<Send size={17}/>}{sent.isPending?'正在发送…':`发送到 ${selectedClasses.length} 个班级`}<ArrowRight size={17}/></Button><span><ShieldCheck size={14}/>发送后可在记录中查看回执或撤回</span></div>
          {lastSent&&<div className="last-sent-status" role="status"><CheckCheck size={16}/><span>最近一条通知已提交，{lastSent.targets?.filter(target=>target.status==='played').length??0} 台设备已播放。</span><Link to="/history">查看回执<ChevronRight size={14}/></Link></div>}
        </Card>
      </div>
      <aside className="preview-column">
        <div className="preview-heading"><h2><Eye size={17}/>教室里的样子</h2><span>实时预览</span></div>
        <div className="screen-monitor"><div className="monitor-top"><span className="monitor-camera"/><span>CLASSROOM DISPLAY</span></div><NoticeScreen compact text={text} studentName={student} signature={signature} className={selectedClasses[0]?.name??'你的班级'} style={style}/><div className="monitor-bottom"><span/></div></div><div className="monitor-stand"/>
        <div className="preview-caption"><span className="live-dot"/>内容随你输入实时更新<button onClick={()=>setPreview(true)}>放大预览<ArrowRight size={14}/></button></div>
        <div className="playback-flow">{style==='urgent'&&chime&&<><span><span className="flow-icon"><BellRing size={16}/></span>叮咚提示</span><ChevronRight size={14}/></>}<span><span className="flow-icon"><Monitor size={16}/></span>大屏显示</span><ChevronRight size={14}/><span><span className="flow-icon"><Volume2 size={16}/></span>语音播报</span></div>
        <div className="little-tip"><Sparkles size={20}/><div><h3>一个小贴士</h3><p>填写学生姓名，大屏会自动高亮显示。后排的同学，也能一眼看到自己的名字。</p></div></div>
        <div className="how-it-works"><h3>每条消息，都有回音</h3><p><span><Check size={14}/></span><strong>已送达</strong>消息已到达教室大屏</p><p><span><Volume2 size={14}/></span><strong>已播放</strong>大屏已完成语音播报</p><p><span><Monitor size={14}/></span><strong>离线等待</strong>网络恢复后自动补发</p></div>
        <Link className="connect-link" to="/classes"><Monitor size={16}/>还没连接教室大屏？<ArrowRight size={16}/></Link>
      </aside>
    </div>
    <Dialog open={preview} onClose={()=>setPreview(false)} title="教室大屏预览" description="通知会按当前内容和样式显示。" className="notice-preview-dialog"><NoticeScreen compact text={text} studentName={student} signature={signature} className={selectedClasses[0]?.name??'你的班级'} style={style}/></Dialog>
  </>;
}
