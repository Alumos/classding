import type {ReactNode} from 'react';
import {AlertCircle,ArrowRight,LoaderCircle} from 'lucide-react';
import {Button,Card} from '@classding/ui';
export function PageHeading({eyebrow,title,description,action}:{eyebrow?:string;title:string;description:string;action?:ReactNode}){return <div className="page-heading"><div>{eyebrow&&<div className="eyebrow">{eyebrow}</div>}<h1>{title}</h1><p>{description}</p></div>{action}</div>;}
export function ErrorState({error,retry}:{error:unknown;retry?:()=>void}){return <div className="error-state" role="alert"><AlertCircle size={18}/><div><strong>暂时无法获取数据</strong><p>{error instanceof Error?error.message:'请检查网络与服务连接。'}</p></div>{retry&&<Button variant="outline" size="sm" onClick={retry}>重试</Button>}</div>;}
export function Loading(){return <div className="loading-state"><LoaderCircle size={19} className="spin"/>正在获取最新数据…</div>;}
export function EmptyState({icon,title,description,action}:{icon:ReactNode;title:string;description:string;action?:ReactNode}){return <div className="empty-state"><span className="empty-icon">{icon}</span><h3>{title}</h3><p>{description}</p>{action}</div>;}
export function SectionCard({title,description,action,children,className=''}:{title:string;description?:string;action?:ReactNode;children:ReactNode;className?:string}){return <Card className={`section-card ${className}`}><div className="section-heading"><div><h2>{title}</h2>{description&&<p>{description}</p>}</div>{action}</div>{children}</Card>;}
export function ActionLink({children,onClick}:{children:ReactNode;onClick:()=>void}){return <button className="action-link" onClick={onClick}>{children}<ArrowRight size={14}/></button>;}
export function formatTime(value?:string|null){if(!value)return '—';const date=new Date(value);if(Number.isNaN(date.valueOf()))return '—';return date.toLocaleString('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'});}
export const receiptLabels:Record<string,string>={queued:'等待送达',delivered:'已送达',playing:'正在播放',played:'已播放',dismissed:'已关闭',expired:'离线未送达',failed:'投递失败'};
