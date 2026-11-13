import { useEffect, useRef } from 'react';
import { useMutation, useQueryClient, type InfiniteData } from '@tanstack/react-query';
import { api } from '@classding/api';
import { BellRing, Check, RefreshCw } from 'lucide-react';
import { toast } from 'sonner';
import { Alert, AlertDescription, AlertTitle } from '@classding/ui/components/ui/alert';
import { Badge } from '@classding/ui/components/ui/badge';
import { Button } from '@classding/ui/components/ui/button';
import { Card, CardContent } from '@classding/ui/components/ui/card';
import { Spinner } from '@classding/ui/components/ui/spinner';
import { useAuth } from '../auth';
import { EmptyState, ErrorState, Loading, PageHeader, formatTime } from '../components';
import type { StudentCall, StudentCallPage } from '../models';
import {
  applyStudentCallRead,
  studentCallsKey,
  studentCallsUnavailable,
  useStudentCalls,
} from '../studentCalls';

export default function StudentCallsPage() {
  const { user } = useAuth();
  const userId = user?.id ?? '';
  const currentUser = useRef(userId);
  currentUser.current = userId;
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const cache = useQueryClient();
  const query = useStudentCalls(userId);
  const pages = query.data?.pages ?? [];
  const items = [...new Map(pages.flatMap((page) => page.items).map((item) => [item.id, item])).values()];
  const unread = pages[0]?.unread_count ?? 0;
  const markRead = useMutation({
    mutationFn: ({ id }: { id: string; account: string }) =>
      api<StudentCall>(`/student-calls/${encodeURIComponent(id)}/read`, { method: 'POST' }),
    onSuccess: (call, { account }) => {
      if (!mounted.current || currentUser.current !== account) return;
      cache.setQueryData<InfiniteData<StudentCallPage>>(studentCallsKey(account), (data) =>
        applyStudentCallRead(data, call),
      );
      void cache.invalidateQueries({ queryKey: studentCallsKey(account) });
      toast.success('已标记已读，所有关联老师共享此处理状态');
    },
    onError: (error, { account }) => {
      if (mounted.current && currentUser.current === account) toast.error(error.message);
    },
  });
  const refresh = () => {
    if (!query.isFetching) void query.refetch();
  };
  return (
    <div className="grid min-w-0 gap-6">
      <PageHeader
        title="学生呼叫"
        description="本班学生的呼叫会发送给所有关联老师。打开页面不会自动标记已读。"
      />
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <Badge variant={unread ? 'destructive' : 'secondary'}>{unread} 条未读</Badge>
          <span className="text-muted-foreground">标记已读即表示已处理</span>
        </div>
        <Button variant="outline" size="sm" onClick={refresh} disabled={query.isFetching}>
          {query.isFetching ? <Spinner /> : <RefreshCw />}刷新
        </Button>
      </div>
      {query.isError && query.data && !studentCallsUnavailable(query.error) && (
        <Alert>
          <RefreshCw />
          <AlertTitle>暂时无法同步最新呼叫</AlertTitle>
          <AlertDescription>
            以下保留上次获取的记录和未读数量，连接恢复后会自动重试。也可点击刷新。
          </AlertDescription>
        </Alert>
      )}
      {studentCallsUnavailable(query.error) ? (
        <Alert>
          <BellRing />
          <AlertTitle>当前服务暂未开放学生呼叫</AlertTitle>
          <AlertDescription>请等待服务升级后再试，其他功能不受影响。</AlertDescription>
        </Alert>
      ) : query.isPending ? (
        <Loading />
      ) : query.isError && !query.data ? (
        <ErrorState error={query.error} retry={refresh} />
      ) : items.length === 0 ? (
        <EmptyState
          icon={<BellRing />}
          title="还没有学生呼叫"
          description="学生在教室端确认呼叫后，记录会保存在这里。网页在线时会实时提醒。"
        />
      ) : (
        <ul className="grid min-w-0 gap-3" aria-label="学生呼叫记录">
          {items.map((item) => (
            <li key={item.id}>
              <Card className="student-call-card" data-call-id={item.id}>
                <CardContent className="flex min-w-0 flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                  <div className="grid min-w-0 gap-2">
                    <div className="flex flex-wrap items-center gap-2">
                      <h2 className="break-words text-base font-semibold">{item.class_name}</h2>
                      <Badge variant={item.read_at ? 'secondary' : 'destructive'}>
                        {item.read_at ? '已处理' : '未读'}
                      </Badge>
                    </div>
                    <p className="break-words text-sm text-muted-foreground">
                      {item.device_name || '教室设备'} · 呼叫于 {formatTime(item.created_at)}
                    </p>
                    {item.read_at ? (
                      <p className="break-words text-sm text-muted-foreground">
                        {item.read_by_name || '关联老师'} 已处理 · {formatTime(item.read_at)}
                      </p>
                    ) : (
                      <p className="text-sm text-muted-foreground">任一老师处理后，所有老师同步为已读。</p>
                    )}
                  </div>
                  {!item.read_at && (
                    <Button
                      className="shrink-0 self-start sm:self-auto"
                      onClick={() => markRead.mutate({ id: item.id, account: userId })}
                      disabled={markRead.isPending}
                    >
                      {markRead.isPending && markRead.variables?.id === item.id ? <Spinner /> : <Check />}
                      标记已读
                    </Button>
                  )}
                </CardContent>
              </Card>
            </li>
          ))}
        </ul>
      )}
      {query.hasNextPage && !query.isError && (
        <Button
          variant="outline"
          className="justify-self-center"
          disabled={query.isFetching}
          onClick={() => void query.fetchNextPage()}
        >
          {query.isFetchingNextPage && <Spinner />}加载更早的呼叫
        </Button>
      )}
      <p className="text-xs leading-relaxed text-muted-foreground">
        共享已读不会删除记录。仅在网页打开且联网时实时提醒，不发送系统推送。
      </p>
    </div>
  );
}
