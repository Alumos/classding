import * as React from 'react';
import {useSiteSettings} from './site';
export {SiteSettingsProvider,SiteFooter,useSiteSettings,type SiteSettings} from './site';
import { Dialog as DialogPrimitive } from '@base-ui/react/dialog';
import { Switch as SwitchPrimitive } from '@base-ui/react/switch';
import { cva, type VariantProps } from 'class-variance-authority';
import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';
import { BellRing, X } from 'lucide-react';
export function cn(...inputs: ClassValue[]) { return twMerge(clsx(inputs)); }
const buttonVariants=cva('cd-button', {variants:{variant:{default:'cd-button-primary',secondary:'cd-button-secondary',ghost:'cd-button-ghost',outline:'cd-button-outline',destructive:'cd-button-destructive'},size:{default:'',sm:'cd-button-sm',icon:'cd-button-icon'}},defaultVariants:{variant:'default',size:'default'}});
export type ButtonProps=React.ButtonHTMLAttributes<HTMLButtonElement>&VariantProps<typeof buttonVariants>;
export const Button=React.forwardRef<HTMLButtonElement,ButtonProps>(({className,variant,size,type='button',...props},ref)=><button ref={ref} type={type} className={cn(buttonVariants({variant,size}),className)} {...props}/>);
Button.displayName='Button';
export const Input=React.forwardRef<HTMLInputElement,React.InputHTMLAttributes<HTMLInputElement>>(({className,...props},ref)=><input ref={ref} className={cn('cd-input',className)} {...props}/>);
Input.displayName='Input';
export const Textarea=React.forwardRef<HTMLTextAreaElement,React.TextareaHTMLAttributes<HTMLTextAreaElement>>(({className,...props},ref)=><textarea ref={ref} className={cn('cd-input cd-textarea',className)} {...props}/>);
Textarea.displayName='Textarea';
export function Card({className,...props}:React.HTMLAttributes<HTMLDivElement>){return <div className={cn('cd-card',className)} {...props}/>;}
export function Badge({className,variant='default',...props}:React.HTMLAttributes<HTMLSpanElement>&{variant?:'default'|'secondary'|'outline'|'success'|'warning'|'destructive'}){return <span className={cn('cd-badge',`cd-badge-${variant}`,className)} {...props}/>;}
export function Dialog({open,onClose,title,description,children,footer,className}: {open:boolean;onClose:()=>void;title:string;description?:string;children:React.ReactNode;footer?:React.ReactNode;className?:string}){return <DialogPrimitive.Root open={open} onOpenChange={value=>{if(!value)onClose();}}><DialogPrimitive.Portal><DialogPrimitive.Backdrop className="cd-dialog-backdrop"/><DialogPrimitive.Popup className={cn('cd-dialog',className)}><div className="cd-dialog-heading"><div><DialogPrimitive.Title className="cd-dialog-title">{title}</DialogPrimitive.Title>{description&&<DialogPrimitive.Description className="cd-dialog-description">{description}</DialogPrimitive.Description>}</div><DialogPrimitive.Close className="cd-button cd-button-icon cd-button-ghost" aria-label="关闭"><X size={20}/></DialogPrimitive.Close></div><div className="cd-dialog-content">{children}</div>{footer&&<div className="cd-dialog-footer">{footer}</div>}</DialogPrimitive.Popup></DialogPrimitive.Portal></DialogPrimitive.Root>;}

export async function copyText(text:string):Promise<boolean>{
  if(typeof navigator!=='undefined'&&typeof window!=='undefined'&&window.isSecureContext&&navigator.clipboard?.writeText){
    try{await navigator.clipboard.writeText(text);return true;}catch{/* Try the selection-based method below. */}
  }
  if(typeof document==='undefined'||!document.body)return false;
  const active=document.activeElement instanceof HTMLElement?document.activeElement:null;
  const field=active instanceof HTMLInputElement||active instanceof HTMLTextAreaElement?active:null;
  const fieldSelection=field?{start:field.selectionStart,end:field.selectionEnd,direction:field.selectionDirection}:null;
  const selection=window.getSelection();
  const ranges=selection?Array.from({length:selection.rangeCount},(_,index)=>selection.getRangeAt(index).cloneRange()):[];
  const temporary=document.createElement('textarea');
  temporary.value=text;
  temporary.readOnly=true;
  temporary.setAttribute('aria-hidden','true');
  temporary.style.cssText='position:fixed;left:-9999px;top:0;opacity:0';
  document.body.appendChild(temporary);
  try{
    temporary.focus({preventScroll:true});
    temporary.select();
    temporary.setSelectionRange(0,temporary.value.length);
    return document.execCommand('copy');
  }catch{return false;}
  finally{
    temporary.remove();
    if(selection){selection.removeAllRanges();for(const range of ranges)selection.addRange(range);}
    if(active){
      try{active.focus({preventScroll:true});}catch{active.focus();}
      if(field&&fieldSelection?.start!==null&&fieldSelection?.start!==undefined&&fieldSelection.end!==null){
        try{field.setSelectionRange(fieldSelection.start,fieldSelection.end,fieldSelection.direction??undefined);}catch{/* Some input types do not support selection. */}
      }
    }
  }
}

export type CopyButtonProps=Omit<ButtonProps,'onClick'>&{text:string;onCopied?:()=>void;dialogTitle?:string};
export function CopyButton({text,onCopied,dialogTitle='手动复制',children,...buttonProps}:CopyButtonProps){
  const [manual,setManual]=React.useState(false);
  async function copy(){if(await copyText(text))onCopied?.();else setManual(true);}
  return <><Button {...buttonProps} onClick={()=>void copy()}>{children??'复制'}</Button><Dialog open={manual} onClose={()=>setManual(false)} title={dialogTitle} description="无法自动复制，请选中以下内容后手动复制。" footer={<Button onClick={()=>setManual(false)}>完成</Button>}><Textarea aria-label="待复制内容" className="cd-copy-text" value={text} readOnly onFocus={event=>event.currentTarget.select()} onClick={event=>event.currentTarget.select()}/></Dialog></>;
}
export function Switch({checked,onCheckedChange,className,...props}:{checked:boolean;onCheckedChange:(checked:boolean)=>void;className?:string;'aria-label'?:string;disabled?:boolean}){return <SwitchPrimitive.Root checked={checked} onCheckedChange={onCheckedChange} className={cn('cd-switch',className)} {...props}><SwitchPrimitive.Thumb className="cd-switch-thumb"/></SwitchPrimitive.Root>;}
export function Brand({compact=false,className}:{compact?:boolean;className?:string}){const {site_name}=useSiteSettings();return <div className={cn('cd-brand',className)}><span className="cd-brand-mark"><BellRing size={23} strokeWidth={2.1}/><i/></span>{!compact&&<span><strong>{site_name}<span className="cd-brand-dot">.</span></strong><small>CLASSDING</small></span>}</div>;}
export interface NoticeScreenProps {text:string;studentName?:string;signature?:string;className?:string;style?:string;progress?:number;compact?:boolean;sentAt?:string;}
export function NoticeScreen({text,studentName,signature='老师',className='教室大屏',style='notice',progress=68,compact=false,sentAt}:NoticeScreenProps){
  const body=React.useRef<HTMLDivElement>(null);
  const paragraph=React.useRef<HTMLParagraphElement>(null);
  const name=React.useRef<HTMLElement>(null);
  React.useLayoutEffect(()=>{
    const container=body.current;
    const content=paragraph.current;
    if(!container||!content)return;
    const fit=()=>{
      const metrics=getComputedStyle(container);
      const height=container.clientHeight-parseFloat(metrics.paddingTop)-parseFloat(metrics.paddingBottom);
      const gap=studentName?parseFloat(metrics.rowGap)||0:0;
      if(height<=0)return;
      const apply=(size:number)=>{
        content.style.fontSize=`${size}px`;
        if(name.current)name.current.style.fontSize=`${size*1.025}px`;
      };
      let low=compact?6:12;
      let high=compact?21:Math.max(24,Math.min(150,innerHeight*.078));
      for(let i=0;i<10;i++){
        const middle=(low+high)/2;
        apply(middle);
        const fits=content.scrollHeight+(name.current?.scrollHeight||0)+gap<=height;
        if(fits)low=middle;else high=middle;
      }
      apply(low);
    };
    const observer=new ResizeObserver(fit);
    observer.observe(container);
    fit();
    return ()=>observer.disconnect();
  },[text,studentName,compact]);
  const label=style==='praise'?'表扬时刻':style==='urgent'?'紧急通知':'班级通知';
  const time=sentAt?new Date(sentAt).toLocaleTimeString('zh-CN',{hour:'2-digit',minute:'2-digit'}):'现在';
  return <div className={cn('cd-notice-screen',`cd-notice-${style}`,compact&&'cd-notice-compact')}>
    <div className="cd-notice-top"><span><BellRing/> {label}</span><span>{className}</span></div>
    <div className="cd-notice-body" ref={body}>{studentName&&<strong ref={name}>{studentName}<span>同学</span></strong>}<p ref={paragraph}>{text||'请带上作业到办公室'}</p></div>
    <div className="cd-notice-footer"><span>{signature}<b>·</b>{time}</span><span className="cd-notice-watermark"><BellRing/> 课堂叮咚</span></div>
    <div className="cd-notice-progress"><i style={{width:`${Math.max(0,Math.min(100,progress))}%`}}/></div>
  </div>;
}
