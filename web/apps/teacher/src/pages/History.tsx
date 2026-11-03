import { useState } from 'react';
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@classding/api';
import { NoticeScreen } from '@classding/ui';
import { ArrowDown, AudioLines, CheckCheck, History, Info, Monitor, RotateCcw, Send, Sparkles, Undo2, Zap } from 'lucide-react';
import { toast } from 'sonner';
import { Alert, AlertDescription } from '@classding/ui/components/ui/alert';
import { Badge } from '@classding/ui/components/ui/badge';
import { Button } from '@classding/ui/components/ui/button';
import { Card } from '@classding/ui/components/ui/card';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@classding/ui/components/ui/dialog';
import { Item, ItemContent, ItemDescription, ItemGroup, ItemMedia, ItemTitle } from '@classding/ui/components/ui/item';
import { Spinner } from '@classding/ui/components/ui/spinner';
import { ConfirmDialog, SimpleSelect } from '@classding/ui/components/classding';
import { cn } from '@classding/ui/lib/utils';
import { useAuth } from '../auth';
import { useClasses } from '../hooks';
import { EmptyState, ErrorState, Loading, PageHeader, formatTime, receiptLabels } from '../components';
import type { Message } from '../models';

const receiptVariant = (status: string) => status === 'played' ? 'default' : status === 'failed' || status === 'expired' ? 'destructive' : status === 'queued' ? 'outline' : 'secondary';

export default function HistoryPage() {
  const cache = useQueryClient();
  const { user } = useAuth();
  const classes = useClasses();
  const [filter, setFilter] = useState('all');
  const [detail, setDetail] = useState<Message | null>(null);
  const [confirm, setConfirm] = useState<Message | null>(null);
  const classId = filter === 'all' ? '' : filter;
  const query = useInfiniteQuery({
    queryKey: ['messages', classId], initialPageParam: '',
    queryFn: ({ pageParam }) => { const search = new URLSearchParams(); if (classId) search.set('class_id', classId); if (pageParam) search.set('cursor', pageParam); return api<{ items: Message[]; next_cursor: string | null }>(`/messages?${search}`); },
    getNextPageParam: last => last.next_cursor ?? undefined,
  });
  const items = query.data?.pages.flatMap(page => page.items) ?? [];
  const resend = useMutation({ mutationFn: (id: string) => api<Message>(`/messages/${id}/resend`, { method: 'POST' }), onSuccess: () => { toast.success('已重新发送，正在投递'); void cache.invalidateQueries({ queryKey: ['messages'] }); }, onError: e => toast.error(e.message) });
  const recall = useMutation({ mutationFn: (id: string) => api<Message>(`/messages/${id}/recall`, { method: 'POST' }), onSuccess: () => { toast.success('撤回指令已发送'); void cache.invalidateQueries({ queryKey: ['messages'] }); setConfirm(null); setDetail(null); }, onError: e => toast.error(e.message) });
  return <>
    <PageHeader title="发送记录" description="每一声叮咚，都有迹可循。查看送达状态，及时照顾每一次提醒。" actions={<SimpleSelect label="按班级筛选" value={filter} onValueChange={setFilter} options={[{ value: 'all', label: '全部班级' }, ...(classes.data ?? []).map(c => ({ value: c.id, label: c.name }))]} className="w-full sm:w-52" />} />
    <Alert><Info /><AlertDescription>回执随大屏状态实时更新。离线消息将在大屏恢复连接后补发，默认 10 分钟过期。</AlertDescription></Alert>
    {query.isLoading ? <Loading /> : query.error ? <ErrorState error={query.error} retry={() => void query.refetch()} /> : !items.length
      ? <EmptyState icon={<History />} title="第一声叮咚，还在路上" description="发送第一条通知后，你会在这里看到它的回音。" />
      : <Card className="gap-0 divide-y py-0">
        {items.map(message => {
          const canRecall = message.sender_id === user?.id || classes.data?.some(c => c.role === 'owner' && message.targets?.some(t => t.class_id === c.id)) || user?.role !== 'user';
          const targets = message.targets ?? [];
          const Icon = message.kind === 'voice' ? AudioLines : message.style === 'praise' ? Sparkles : message.style === 'urgent' ? Zap : Send;
          return <div className="history-row grid gap-3 p-4 md:grid-cols-[minmax(0,1fr)_14rem_auto] md:items-center md:px-6" key={message.id}>
            <button type="button" className="flex min-w-0 items-start gap-3 text-left" onClick={() => setDetail(message)}>
              <span className={cn('flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted', message.style === 'urgent' && 'bg-destructive/10 text-destructive', message.style === 'praise' && 'bg-amber-500/10 text-amber-600 dark:text-amber-400')}><Icon className="size-4" /></span>
              <span className="grid min-w-0 gap-1">
                <span className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                  <Badge variant={message.recalled_at ? 'secondary' : message.style === 'urgent' ? 'destructive' : 'outline'}>{message.recalled_at ? '已撤回' : message.style === 'praise' ? '表扬' : message.style === 'urgent' ? '紧急' : '通知'}</Badge>
                  {message.signature}<span>·</span><time>{formatTime(message.created_at)}</time>
                </span>
                <strong className="line-clamp-2 font-medium break-all">{message.student_name && <span className="text-primary">{message.student_name}同学，</span>}{message.text || '老师的原声通知'}</strong>
                <span className="text-xs text-muted-foreground">{message.kind === 'voice' ? '原声播报' : `朗读 ${message.repeat} 遍`} · 显示 {message.display_sec} 秒</span>
              </span>
            </button>
            <div className="flex flex-wrap gap-1.5 md:justify-start">
              {targets.length ? targets.slice(0, 3).map((target, index) => <Badge key={`${target.device_id}-${index}`} variant={receiptVariant(target.status)} className="max-w-full">
                {target.status === 'played' && <CheckCheck />}<span className="truncate">{target.class_name}</span>· {receiptLabels[target.status] ?? target.status}
              </Badge>) : <Badge variant="outline">等待大屏上线</Badge>}
              {targets.length > 3 && <Button variant="link" size="xs" className="px-0" onClick={() => setDetail(message)}>查看全部 {targets.length} 台设备</Button>}
            </div>
            <div className="flex gap-1 md:justify-end">
              <Button variant="ghost" size="sm" disabled={resend.isPending || !!message.recalled_at} onClick={() => resend.mutate(message.id)} title="使用相同内容重新发送"><RotateCcw />重发</Button>
              {canRecall && !message.recalled_at && <Button variant="ghost" size="sm" onClick={() => setConfirm(message)}><Undo2 />撤回</Button>}
            </div>
          </div>;
        })}
        {query.hasNextPage && <div className="flex justify-center p-4"><Button variant="outline" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>{query.isFetchingNextPage ? <Spinner /> : <ArrowDown />}加载更早的记录</Button></div>}
      </Card>}
    <Dialog open={!!detail} onOpenChange={open => { if (!open) setDetail(null); }}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader><DialogTitle>通知详情</DialogTitle><DialogDescription>{detail ? `发送于 ${formatTime(detail.created_at)}` : ''}</DialogDescription></DialogHeader>
        {detail && <div className="grid min-w-0 gap-4">
          <div className="overflow-hidden rounded-lg border"><NoticeScreen compact text={detail.text} studentName={detail.student_name} signature={detail.signature} style={detail.style} className={detail.targets?.[0]?.class_name ?? '教室大屏'} /></div>
          <section className="grid gap-2 rounded-lg bg-muted p-4" aria-label="通知完整内容">
            <h3 className="text-sm font-medium">通知内容</h3>
            {detail.student_name && <p className="font-medium">{detail.student_name}同学</p>}
            <p className="leading-relaxed whitespace-pre-wrap break-words">{detail.text || '老师的原声通知'}</p>
            <p className="text-sm text-muted-foreground">{detail.signature}</p>
          </section>
          <ItemGroup className="gap-2">{detail.targets?.map((t, i) => <Item key={`${t.device_id}-${i}`} variant="outline" size="sm">
            <ItemMedia variant="icon"><Monitor /></ItemMedia>
            <ItemContent><ItemTitle>{t.class_name}</ItemTitle><ItemDescription>{t.device_name || '教室大屏'}</ItemDescription></ItemContent>
            <Badge variant={receiptVariant(t.status)}>{receiptLabels[t.status] ?? t.status}</Badge>
          </Item>)}</ItemGroup>
        </div>}
      </DialogContent>
    </Dialog>
    <ConfirmDialog open={!!confirm} onOpenChange={open => { if (!open) setConfirm(null); }} title="撤回这条通知？" description="未播放的通知将停止投递，正在显示的通知将关闭；已经朗读的内容无法收回。" cancelLabel="暂不撤回" confirmLabel={recall.isPending ? '正在撤回…' : '确认撤回'} pending={recall.isPending} onConfirm={() => confirm && recall.mutate(confirm.id)} />
  </>;
}
