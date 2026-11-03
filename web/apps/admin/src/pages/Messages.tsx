import { useState } from 'react';
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@classding/api';
import { BellRing } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@classding/ui/components/ui/badge';
import { Button } from '@classding/ui/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@classding/ui/components/ui/dialog';
import { Input } from '@classding/ui/components/ui/input';
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from '@classding/ui/components/ui/table';
import { ConfirmDialog, PageHeader, PaginationBar, usePagination } from '@classding/ui/components/classding';
import { cn } from '@classding/ui/lib/utils';
import {
  DataTable,
  Panel,
  ResultState,
  SearchInput,
  Toolbar,
  dateTime,
  errorMessage,
  json,
  localDate,
  type Message,
  type MessagePage,
} from '../lib';
import { deliveryLabel, deliverySummary, deliveryTone } from '@classding/ui/lib/delivery';

const styleTone: Record<string, string> = {
  urgent: 'bg-destructive/10 text-destructive',
  praise: 'bg-amber-500/10 text-amber-600 dark:text-amber-400',
};

export function MessageTable({
  messages,
  compact = false,
  onSelect,
  onDelete,
}: {
  messages: Message[];
  compact?: boolean;
  onSelect?: (message: Message) => void;
  onDelete?: (message: Message) => void;
}) {
  return (
    <DataTable>
      <TableHeader>
        <TableRow>
          <TableHead>通知内容</TableHead>
          <TableHead>发送人</TableHead>
          <TableHead>目标班级</TableHead>
          <TableHead>发送时间</TableHead>
          <TableHead>回执状态</TableHead>
          {onSelect && <TableHead className="text-right">操作</TableHead>}
        </TableRow>
      </TableHeader>
      <TableBody>
        {messages.map((item) => {
          const summary = deliverySummary((item.targets ?? []).map((t) => t.status));
          return (
            <TableRow key={item.id}>
              <TableCell>
                <div className="flex items-center gap-3">
                  <span
                    className={cn(
                      'flex size-8 shrink-0 items-center justify-center rounded-md bg-muted',
                      styleTone[item.style ?? ''],
                    )}
                  >
                    <BellRing className="size-4" />
                  </span>
                  <div className="min-w-0">
                    <div className="max-w-64 truncate font-medium">
                      {item.student_name ? `${item.student_name} · ` : ''}
                      {item.text || (item.kind === 'voice' ? '语音通知' : '课堂通知')}
                    </div>
                    {!compact && <div className="text-xs text-muted-foreground">{item.id}</div>}
                  </div>
                </div>
              </TableCell>
              <TableCell>{item.signature || item.sender_id || '—'}</TableCell>
              <TableCell>
                {item.targets?.length
                  ? [...new Set(item.targets.map((t) => t.class_name || t.class_id))].join('、')
                  : '—'}
              </TableCell>
              <TableCell className="text-muted-foreground">{dateTime(item.created_at)}</TableCell>
              <TableCell>
                <Badge variant={summary.tone}>{summary.text}</Badge>
              </TableCell>
              {onSelect && (
                <TableCell className="text-right">
                  <div className="flex justify-end gap-1">
                    <Button variant="ghost" size="sm" onClick={() => onSelect(item)}>
                      查看回执
                    </Button>
                    {onDelete && (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive hover:text-destructive"
                        onClick={() => onDelete(item)}
                      >
                        删除
                      </Button>
                    )}
                  </div>
                </TableCell>
              )}
            </TableRow>
          );
        })}
      </TableBody>
    </DataTable>
  );
}

export default function MessagesPage() {
  const [search, setSearch] = useState('');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [selected, setSelected] = useState<Message | null>(null);
  const [deleting, setDeleting] = useState<Message | null>(null);
  const queryClient = useQueryClient();
  const query = useInfiniteQuery({
    queryKey: ['admin', 'messages'],
    initialPageParam: '',
    queryFn: ({ pageParam }) =>
      api<MessagePage>(`/admin/messages${pageParam ? `?cursor=${encodeURIComponent(pageParam)}` : ''}`),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
  });
  const loaded = query.data?.pages.flatMap((page) => page.items ?? []) ?? [];
  const messages = loaded.filter((item) => {
    const matches =
      `${item.text ?? ''} ${item.student_name ?? ''} ${item.signature ?? ''} ${item.targets?.map((t) => `${t.class_name} ${t.class_id}`).join(' ') ?? ''}`
        .toLowerCase()
        .includes(search.trim().toLowerCase());
    const date = localDate(item.created_at);
    return matches && (!from || date >= from) && (!to || date <= to);
  });
  const pager = usePagination(messages, 10, `${search}|${from}|${to}`);
  const remove = useMutation({
    mutationFn: (message: Message) =>
      api(`/admin/messages/${encodeURIComponent(message.id)}`, json('DELETE')),
    onSuccess: () => {
      setDeleting(null);
      void queryClient.invalidateQueries({ queryKey: ['admin', 'messages'] });
      void queryClient.invalidateQueries({ queryKey: ['admin', 'overview'] });
      toast.success('消息记录已删除');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });
  return (
    <>
      <PageHeader title="消息审计" description="查看通知内容与回执，及时了解课堂沟通情况。" />
      <Panel
        title="消息记录"
        description="仅展示当前权限范围内的记录"
        action={
          <span className="text-sm text-muted-foreground">{query.data ? `${loaded.length} 条记录` : ''}</span>
        }
      >
        <Toolbar count={query.data ? `共 ${messages.length} 条` : undefined}>
          <SearchInput value={search} onChange={setSearch} placeholder="搜索消息、学生、教师或班级" />
          <label className="flex items-center gap-2 text-sm text-muted-foreground">
            从
            <Input
              aria-label="开始日期"
              type="date"
              className="w-40"
              value={from}
              onChange={(e) => setFrom(e.target.value)}
            />
          </label>
          <label className="flex items-center gap-2 text-sm text-muted-foreground">
            至
            <Input
              aria-label="结束日期"
              type="date"
              className="w-40"
              value={to}
              onChange={(e) => setTo(e.target.value)}
            />
          </label>
        </Toolbar>
        <ResultState
          loading={query.isLoading}
          error={query.data ? null : query.error}
          empty={!messages.length && !query.hasNextPage}
          emptyText="没有找到消息记录"
        >
          <MessageTable messages={pager.pageItems} onSelect={setSelected} onDelete={setDeleting} />
          <PaginationBar
            page={pager.page}
            pageCount={pager.pageCount}
            total={pager.total}
            onPageChange={pager.setPage}
            hasMore={!!query.hasNextPage}
            loadingMore={query.isFetchingNextPage}
            onLoadMore={() => query.fetchNextPage()}
          />
        </ResultState>
        {query.isFetchNextPageError && (
          <p className="px-6 py-3 text-sm text-destructive" role="alert">
            加载更多失败：{errorMessage(query.error)}
          </p>
        )}
      </Panel>
      <Dialog
        open={!!selected}
        onOpenChange={(open) => {
          if (!open) setSelected(null);
        }}
      >
        <DialogContent className="sm:max-w-lg" showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>通知回执</DialogTitle>
            {selected && (
              <DialogDescription>
                发送于 {dateTime(selected.created_at)} · {selected.signature || selected.sender_id}
              </DialogDescription>
            )}
          </DialogHeader>
          {selected && (
            <div className="grid gap-4">
              <p className="rounded-lg bg-muted p-4 text-sm leading-relaxed whitespace-pre-wrap break-words">
                {selected.student_name && <strong>{selected.student_name} · </strong>}
                {selected.text || (selected.kind === 'voice' ? '语音通知' : '课堂通知')}
              </p>
              <div className="divide-y rounded-lg border">
                {selected.targets?.length ? (
                  selected.targets.map((target, index) => (
                    <div
                      className="flex items-center justify-between gap-4 p-3 text-sm"
                      key={`${target.class_id}-${target.device_id}-${index}`}
                    >
                      <div className="min-w-0">
                        <div className="font-medium">{target.class_name || target.class_id}</div>
                        <div className="truncate text-xs text-muted-foreground">
                          {target.device_name || target.device_id || '未指定设备'}
                        </div>
                      </div>
                      <div className="text-right">
                        <Badge variant={deliveryTone(target.status)}>{deliveryLabel(target.status)}</Badge>
                        <div className="mt-1 text-xs text-muted-foreground">
                          {target.played_at
                            ? `播放于 ${dateTime(target.played_at)}`
                            : target.delivered_at
                              ? `送达于 ${dateTime(target.delivered_at)}`
                              : ''}
                        </div>
                      </div>
                    </div>
                  ))
                ) : (
                  <p className="p-3 text-sm text-muted-foreground">暂无回执</p>
                )}
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setSelected(null)}>
              关闭
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
        title="删除这条消息记录？"
        description="删除后无法恢复。尚未播放的通知会同时从教室大屏撤回。"
        confirmLabel={remove.isPending ? '正在删除…' : '确认删除'}
        pending={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting)}
      />
    </>
  );
}
