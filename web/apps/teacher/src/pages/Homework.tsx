import {useState,type FormEvent} from 'react';
import {useMutation,useQueryClient} from '@tanstack/react-query';
import {api} from '@classding/api';
import {Button,Card,Dialog,Input,Select,Switch,Textarea} from '@classding/ui';
import {BookOpenCheck,Clock3,Edit3,GraduationCap,LoaderCircle,Monitor,Plus,Trash2} from 'lucide-react';
import {Link} from 'react-router';
import {toast} from 'sonner';
import {useAuth} from '../auth';
import {useClasses,useHomeworks} from '../hooks';
import {EmptyState,ErrorState,Loading,PageHeading,formatTime} from '../components';
import type {Class,Homework} from '../models';

type Editor={kind:'create';classId:string}|{kind:'edit';item:Homework};
type HomeworkFields={subject:string;text:string;visible:boolean;always_on_top:boolean};
const lengthOf=(value:string)=>Array.from(value).length;

export default function HomeworkPage(){
  const {user}=useAuth();
  const cache=useQueryClient();
  const classes=useClasses();
  const [selectedId,setSelectedId]=useState('');
  const [editor,setEditor]=useState<Editor|null>(null);
  const [deleting,setDeleting]=useState<Homework|null>(null);
  const activeClass=classes.data?.find(item=>item.id===selectedId)??classes.data?.[0];
  const query=useHomeworks(activeClass?.id??'');
  const items=query.data?.items??[];
  const mayManage=(item:Homework)=>item.teacher_id===user?.id||activeClass?.role==='owner'||user?.role==='school_admin'||user?.role==='super_admin';
  const refresh=()=>cache.invalidateQueries({queryKey:['homeworks']});
  const updateOption=useMutation({
    mutationFn:({item,field,value}:{item:Homework;field:'visible'|'always_on_top';value:boolean})=>api<Homework>(`/homeworks/${encodeURIComponent(item.id)}`,{method:'PATCH',body:JSON.stringify({[field]:value})}),
    onSuccess:async()=>{await refresh();toast.success('显示设置已保存');},
    onError:error=>toast.error(error.message),
  });
  const remove=useMutation({
    mutationFn:(id:string)=>api<{ok:boolean}>(`/homeworks/${encodeURIComponent(id)}`,{method:'DELETE'}),
    onSuccess:async()=>{await refresh();setDeleting(null);toast.success('作业已删除');},
    onError:error=>toast.error(error.message),
  });

  return <div className="homework-page">
    <PageHeading eyebrow="HOMEWORK, IN VIEW" title="布置作业" description="把各科作业放在一起，方便教室里的同学随时查看。" action={<Button className="homework-create-button" disabled={!activeClass} onClick={()=>activeClass&&setEditor({kind:'create',classId:activeClass.id})}><Plus size={18}/>新建作业</Button>}/>
    <div className="homework-intro"><span className="homework-intro-icon"><Monitor size={20}/></span><div><strong>教室电脑右下角显示作业</strong><p>这里保存的是默认显示和置顶设置，教室电脑也可自行调整。修改保存后，新设置会同步到电脑。</p></div></div>
    {classes.isLoading?<Loading/>:classes.error&&!classes.data?<ErrorState error={classes.error} retry={()=>void classes.refetch()}/>:!classes.data?.length?<Card><EmptyState icon={<GraduationCap size={30}/>} title="先加入一个班级" description="加入或创建班级后，就可以为这间教室布置作业。" action={<Link className="cd-button cd-button-primary" to="/classes">前往我的班级</Link>}/></Card>:<>
      {classes.error&&<ErrorState error={classes.error} retry={()=>void classes.refetch()}/>}
      <div className="homework-toolbar"><div className="homework-class-filter"><label htmlFor="homework-class-filter">选择班级</label><Select id="homework-class-filter" label="选择班级" value={activeClass?.id??''} onValueChange={setSelectedId} options={classes.data.map(item=>({value:item.id,label:item.name}))}/></div><p>每个班级最多保留 12 条作业</p></div>
      {query.isLoading?<Loading/>:query.error&&!query.data?<ErrorState error={query.error} retry={()=>void query.refetch()}/>:<>
        {query.error&&<ErrorState error={query.error} retry={()=>void query.refetch()}/>}
        <div className="homework-list-heading"><div><h2>班级作业</h2><p>{activeClass?.name} · 共 {items.length} 条</p></div></div>
        {!items.length?<Card><EmptyState icon={<BookOpenCheck size={30}/>} title="还没有布置作业" description="新建第一条作业，让同学在教室电脑上看到学习任务。" action={<Button onClick={()=>activeClass&&setEditor({kind:'create',classId:activeClass.id})}><Plus size={17}/>新建作业</Button>}/></Card>:<div className="homework-list">{items.map(item=><Card className="homework-card" key={item.id}>
          <div className="homework-card-header"><div className="homework-subject"><span className="homework-book-icon"><BookOpenCheck size={20}/></span><div><h3>{item.subject}</h3><p>{item.teacher_name}</p></div></div><div className="homework-card-actions">{mayManage(item)?<><Button variant="outline" size="sm" onClick={()=>setEditor({kind:'edit',item})}><Edit3 size={15}/>编辑</Button><Button variant="ghost" size="sm" onClick={()=>setDeleting(item)}><Trash2 size={15}/>删除</Button></>:<span className="homework-readonly">仅查看</span>}</div></div>
          <p className="homework-text">{item.text}</p>
          <div className="homework-card-footer"><span className="homework-time"><Clock3 size={15}/>更新于 {formatTime(item.updated_at)}</span><div className="homework-options"><label><span>显示</span><Switch aria-label={`显示${item.subject}作业`} checked={item.visible} disabled={!mayManage(item)||updateOption.isPending||remove.isPending} onCheckedChange={value=>updateOption.mutate({item,field:'visible',value})}/></label><label><span>置顶</span><Switch aria-label={`置顶${item.subject}作业`} checked={item.always_on_top} disabled={!mayManage(item)||updateOption.isPending||remove.isPending} onCheckedChange={value=>updateOption.mutate({item,field:'always_on_top',value})}/></label></div></div>
        </Card>)}</div>}
      </>}
    </>}
    {editor&&classes.data&&<HomeworkEditor key={editor.kind==='create'?`new-${editor.classId}`:editor.item.id} editor={editor} classes={classes.data} teacherName={user?.display_name??'老师'} onClose={()=>setEditor(null)} onSaved={classId=>{setSelectedId(classId);setEditor(null);}}/>}
    <Dialog open={!!deleting} onClose={()=>{if(!remove.isPending)setDeleting(null);}} title="删除这条作业？" description={deleting?`删除后，「${deleting.subject}」作业将从教室电脑移除。`:''} footer={<><Button variant="outline" disabled={remove.isPending} onClick={()=>setDeleting(null)}>取消</Button><Button variant="destructive" disabled={remove.isPending} onClick={()=>deleting&&remove.mutate(deleting.id)}>{remove.isPending?'正在删除…':'确认删除'}</Button></>}><></></Dialog>
  </div>;
}

function HomeworkEditor({editor,classes,teacherName,onClose,onSaved}:{editor:Editor;classes:Class[];teacherName:string;onClose:()=>void;onSaved:(classId:string)=>void}){
  const cache=useQueryClient();
  const item=editor.kind==='edit'?editor.item:null;
  const [classId,setClassId]=useState(editor.kind==='create'?editor.classId:editor.item.class_id);
  const [subject,setSubject]=useState(item?.subject??'');
  const [text,setText]=useState(item?.text??'');
  const [visible,setVisible]=useState(item?.visible??true);
  const [alwaysOnTop,setAlwaysOnTop]=useState(item?.always_on_top??true);
  const subjectLength=lengthOf(subject);
  const textLength=lengthOf(text);
  const valid=!!classId&&!!subject.trim()&&!!text.trim()&&subjectLength<=24&&textLength<=300;
  const save=useMutation({
    mutationFn:()=>{
      const fields:HomeworkFields={subject:subject.trim(),text:text.trim(),visible,always_on_top:alwaysOnTop};
      return item?api<Homework>(`/homeworks/${encodeURIComponent(item.id)}`,{method:'PATCH',body:JSON.stringify(fields)}):api<Homework>('/homeworks',{method:'POST',body:JSON.stringify({class_id:classId,...fields})});
    },
    onSuccess:async()=>{await cache.invalidateQueries({queryKey:['homeworks']});toast.success(item?'作业已保存':'作业已发布');onSaved(classId);},
  });
  function submit(event:FormEvent){event.preventDefault();if(valid&&!save.isPending)save.mutate();}
  return <Dialog open onClose={()=>{if(!save.isPending)onClose();}} title={item?'编辑作业':'新建作业'} description={item?'修改内容或默认显示设置。':'选择一个班级，以你的姓名发布作业。'} className="homework-dialog">
    <form className="homework-form" onSubmit={submit}>
      <div className="homework-form-field"><label htmlFor="homework-class">班级</label>{item?<div className="homework-class-value">{classes.find(c=>c.id===classId)?.name??'当前班级'}</div>:<Select id="homework-class" label="班级" value={classId} onValueChange={setClassId} options={classes.map(c=>({value:c.id,label:c.name}))}/>}</div>
      <div className="homework-form-field"><label htmlFor="homework-subject">科目</label><Input id="homework-subject" required value={subject} onChange={event=>setSubject(event.target.value)} placeholder="例如：语文" aria-describedby="homework-subject-count"/><span id="homework-subject-count" className={subjectLength>24?'homework-count is-over':'homework-count'}>{subjectLength} / 24 字</span></div>
      <div className="homework-form-field"><label htmlFor="homework-text">作业内容</label><Textarea id="homework-text" required value={text} onChange={event=>setText(event.target.value)} placeholder="例如：完成课本第 20 页练习" aria-describedby="homework-text-count"/><span id="homework-text-count" className={textLength>300?'homework-count is-over':'homework-count'}>{textLength} / 300 字</span></div>
      <div className="homework-form-options"><label><span><strong>默认显示</strong><small>在教室电脑上显示这条作业</small></span><Switch aria-label="默认显示" checked={visible} onCheckedChange={setVisible}/></label><label><span><strong>默认置顶</strong><small>让作业窗口保持在其他窗口上方</small></span><Switch aria-label="默认置顶" checked={alwaysOnTop} onCheckedChange={setAlwaysOnTop}/></label></div>
      <p className="homework-author">发布老师：{item?.teacher_name??teacherName}</p>
      {save.error&&<div className="homework-form-error" role="alert">{save.error.message}</div>}
      <div className="homework-form-actions"><Button variant="outline" disabled={save.isPending} onClick={onClose}>取消</Button><Button type="submit" disabled={!valid||save.isPending}>{save.isPending?<LoaderCircle size={17} className="spin"/>:null}{save.isPending?'正在保存…':item?'保存修改':'发布作业'}</Button></div>
    </form>
  </Dialog>;
}
