import { lazy, Suspense } from 'react';
import { NavLink } from 'react-router';
import { useQuery } from '@tanstack/react-query';
import { api } from '@classding/api';
import { Activity, ArrowRight, BellRing, Gauge, Monitor, School, Users } from 'lucide-react';
import { Button } from '@classding/ui/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@classding/ui/components/ui/card';
import { ErrorState, LoadingState, PageHeader } from '@classding/ui/components/classding';
import { Panel, ResultState, number, rate, useSession, type Overview } from '../lib';
import { MessageTable } from './Messages';

const ActivityChart = lazy(() => import('../ActivityChart'));

function StatCard({
  label,
  value,
  hint,
  icon: Icon,
}: {
  label: string;
  value: string;
  hint: string;
  icon: typeof Monitor;
}) {
  return (
    <Card className="gap-2">
      <CardHeader className="flex flex-row items-center justify-between">
        <CardDescription>{label}</CardDescription>
        <Icon className="size-4 text-muted-foreground" />
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-semibold tabular-nums">{value}</div>
        <p className="mt-1 text-xs text-muted-foreground">{hint}</p>
      </CardContent>
    </Card>
  );
}

export default function OverviewPage() {
  const { user } = useSession();
  const query = useQuery({
    queryKey: ['admin', 'overview'],
    queryFn: () => api<Overview>('/admin/overview'),
  });
  const data = query.data;
  return (
    <>
      <PageHeader
        title="工作概览"
        description={`你好，${user.display_name || user.username}。这里汇总了${user.role === 'super_admin' ? '全站' : '本校'}教室的连接与通知情况。`}
      />
      {query.isError ? (
        <ErrorState error={query.error} title="数据加载失败" retry={() => void query.refetch()} />
      ) : (
        <>
          <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
            <StatCard
              label="在线设备"
              value={data ? number(data.online_devices) : '—'}
              hint={data ? `共 ${number(data.total_devices)} 台设备` : '正在获取设备状态'}
              icon={Monitor}
            />
            <StatCard
              label="今日发送"
              value={data ? number(data.today_messages) : '—'}
              hint="今天发出的通知"
              icon={BellRing}
            />
            <StatCard
              label="通知送达率"
              value={data ? rate(data.delivery_rate) : '—'}
              hint="按服务端统计口径"
              icon={Activity}
            />
            <StatCard
              label="平均送达耗时"
              value={
                data && Number.isFinite(data.avg_delivery_ms) ? `${number(data.avg_delivery_ms)} ms` : '—'
              }
              hint="从发送到送达"
              icon={Gauge}
            />
          </div>
          <div className="grid gap-4 lg:grid-cols-3">
            <Card className="lg:col-span-2">
              <CardHeader>
                <CardTitle>通知发送趋势</CardTitle>
                <CardDescription>近期教室通知的发送数量</CardDescription>
              </CardHeader>
              <CardContent>
                <ResultState
                  loading={query.isLoading}
                  error={null}
                  empty={!data?.activity?.length}
                  emptyText="暂无趋势数据"
                >
                  <Suspense fallback={<LoadingState label="正在加载图表…" />}>
                    <ActivityChart activity={data?.activity ?? []} />
                  </Suspense>
                </ResultState>
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>站点规模</CardTitle>
                <CardDescription>当前权限范围内的账号与班级</CardDescription>
              </CardHeader>
              <CardContent className="grid gap-3">
                {[
                  {
                    icon: Monitor,
                    label: '教室设备在线',
                    value: `${number(data?.online_devices)} / ${number(data?.total_devices)}`,
                  },
                  { icon: Users, label: '教师账号', value: number(data?.total_users) },
                  { icon: School, label: '班级总数', value: number(data?.total_classes) },
                ].map((item) => (
                  <div key={item.label} className="flex items-center gap-3 rounded-lg border p-3">
                    <span className="flex size-9 items-center justify-center rounded-md bg-muted">
                      <item.icon className="size-4" />
                    </span>
                    <span className="text-sm text-muted-foreground">{item.label}</span>
                    <span className="ml-auto font-semibold tabular-nums">{item.value}</span>
                  </div>
                ))}
              </CardContent>
            </Card>
          </div>
          <Panel
            title="最近通知"
            description="最新的课堂消息与送达情况"
            action={
              <Button variant="ghost" size="sm" nativeButton={false} render={<NavLink to="/messages" />}>
                查看全部
                <ArrowRight />
              </Button>
            }
          >
            <ResultState
              loading={query.isLoading}
              error={null}
              empty={!data?.recent_messages?.length}
              emptyText="还没有通知记录"
            >
              <MessageTable messages={data?.recent_messages ?? []} compact />
            </ResultState>
          </Panel>
        </>
      )}
    </>
  );
}
