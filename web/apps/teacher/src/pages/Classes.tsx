import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@classding/api';
import {
  ArrowRight,
  Clock3,
  Copy,
  GraduationCap,
  KeyRound,
  LayoutGrid,
  Link2,
  Monitor,
  Plus,
  RefreshCw,
  Settings2,
  Timer,
  Trash2,
  UserPlus,
  Users,
  Volume2,
} from 'lucide-react';
import { toast } from 'sonner';
import { Alert, AlertDescription } from '@classding/ui/components/ui/alert';
import { Avatar, AvatarFallback } from '@classding/ui/components/ui/avatar';
import { Badge } from '@classding/ui/components/ui/badge';
import { Button } from '@classding/ui/components/ui/button';
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@classding/ui/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@classding/ui/components/ui/dialog';
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@classding/ui/components/ui/field';
import { Input } from '@classding/ui/components/ui/input';
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemMedia,
  ItemTitle,
} from '@classding/ui/components/ui/item';
import { Separator } from '@classding/ui/components/ui/separator';
import { Spinner } from '@classding/ui/components/ui/spinner';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@classding/ui/components/ui/tabs';
import { Textarea } from '@classding/ui/components/ui/textarea';
import { ConfirmDialog, CopyButton, SimpleSelect } from '@classding/ui/components/classding';
import { useAuth } from '../auth';
import { useClasses, useNow } from '../hooks';
import { EmptyState, ErrorState, Loading, PageHeader, formatTime, initials } from '../components';
import type { BindCode, Class, Device, Member } from '../models';

export default function ClassesPage() {
  const query = useClasses();
  const [create, setCreate] = useState(false);
  const [join, setJoin] = useState(false);
  const [selected, setSelected] = useState<{ item: Class; tab: string } | null>(null);
  const online = query.data?.reduce((n, c) => n + (c.online_devices ?? 0), 0);
  return (
    <>
      <PageHeader
        title="我的班级"
        description="把老师与教室连在一起，让每一次沟通都有回应。"
        actions={
          <>
            <Button variant="outline" onClick={() => setJoin(true)}>
              <UserPlus />
              加入班级
            </Button>
            <Button onClick={() => setCreate(true)}>
              <Plus />
              新建班级
            </Button>
          </>
        }
      />
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3">
        <Card size="sm">
          <CardHeader>
            <CardDescription>任教班级</CardDescription>
            <CardTitle className="text-2xl tabular-nums">{query.data?.length ?? '—'}</CardTitle>
          </CardHeader>
        </Card>
        <Card size="sm">
          <CardHeader>
            <CardDescription>大屏在线</CardDescription>
            <CardTitle className="text-2xl tabular-nums">{online ?? '—'}</CardTitle>
          </CardHeader>
        </Card>
        <Card size="sm" className="col-span-2 sm:col-span-1">
          <CardHeader>
            <CardDescription>协同</CardDescription>
            <CardTitle className="text-sm font-normal text-muted-foreground">
              与任课老师，一起照顾每一间教室。
            </CardTitle>
          </CardHeader>
        </Card>
      </div>
      {query.isLoading ? (
        <Loading />
      ) : query.error ? (
        <ErrorState error={query.error} retry={() => void query.refetch()} />
      ) : !query.data?.length ? (
        <EmptyState
          icon={<GraduationCap />}
          title="从连接第一间教室开始"
          description="用激活码开通一个班级，或向班主任获取协同码加入。"
          action={
            <Button onClick={() => setCreate(true)}>
              <Plus />
              添加我的第一个班级
            </Button>
          }
        />
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {query.data.map((item) => (
            <Card key={item.id} className="class-management-card">
              <CardHeader>
                <span className="mb-2 flex size-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
                  <LayoutGrid className="size-5" />
                </span>
                <CardTitle role="heading" aria-level={2} className="text-lg">
                  {item.name}
                </CardTitle>
                <CardDescription>
                  {item.grade || '专属班级空间'} · {item.teachers_count ?? 1} / {item.max_teachers} 位老师
                </CardDescription>
                <CardAction>
                  <Badge variant={item.role === 'owner' ? 'default' : 'secondary'}>
                    {item.role === 'owner' ? '班级创建者' : '协同老师'}
                  </Badge>
                </CardAction>
              </CardHeader>
              <CardContent>
                <div className="flex items-center gap-2 rounded-lg bg-muted px-3 py-2 text-sm">
                  <span
                    className={
                      item.online_devices
                        ? 'size-2 rounded-full bg-emerald-500'
                        : 'size-2 rounded-full bg-muted-foreground/40'
                    }
                  />
                  <span className="font-medium">
                    {item.online_devices ? '教室大屏在线' : '教室大屏未连接'}
                  </span>
                  <span className="ml-auto text-muted-foreground tabular-nums">
                    {item.devices_count ?? 0} / {item.max_devices} 台
                  </span>
                </div>
              </CardContent>
              <CardFooter className="gap-2">
                <Button
                  variant="outline"
                  className="flex-1"
                  onClick={() => setSelected({ item, tab: 'settings' })}
                >
                  <Settings2 />
                  班级设置
                </Button>
                <Button
                  variant="secondary"
                  className="flex-1"
                  onClick={() => setSelected({ item, tab: item.role === 'owner' ? 'devices' : 'members' })}
                >
                  {item.role === 'owner' ? <Link2 /> : <Users />}
                  {item.role === 'owner' ? '连接大屏' : '查看成员'}
                  <ArrowRight />
                </Button>
              </CardFooter>
            </Card>
          ))}
          <button
            type="button"
            onClick={() => setCreate(true)}
            className="flex min-h-48 flex-col items-center justify-center gap-2 rounded-xl border border-dashed p-6 text-center text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          >
            <span className="flex size-10 items-center justify-center rounded-lg bg-muted">
              <Plus className="size-5" />
            </span>
            <span className="font-medium text-foreground">连接一间新教室</span>
            <span className="text-sm">使用激活码开通班级大屏</span>
          </button>
        </div>
      )}
      <ClassForm open={create} onOpenChange={setCreate} />
      <JoinForm open={join} onOpenChange={setJoin} />
      {selected && (
        <ClassDetails classItem={selected.item} initialTab={selected.tab} onClose={() => setSelected(null)} />
      )}
    </>
  );
}

function ClassForm({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const [name, setName] = useState('');
  const [code, setCode] = useState('');
  const cache = useQueryClient();
  const mutation = useMutation({
    mutationFn: () =>
      api<Class>('/classes', {
        method: 'POST',
        body: JSON.stringify({ name: name.trim(), activation_code: code.trim().toUpperCase() }),
      }),
    onSuccess: () => {
      toast.success('班级已创建，接下来连接教室大屏');
      void cache.invalidateQueries({ queryKey: ['classes'] });
      onOpenChange(false);
      setName('');
      setCode('');
    },
    onError: (e) => toast.error(e.message),
  });
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>开通一间新教室</DialogTitle>
          <DialogDescription>一个激活码对应一个班级，名额与有效期由运营方设置。</DialogDescription>
        </DialogHeader>
        <form
          id="class-create"
          onSubmit={(e) => {
            e.preventDefault();
            mutation.mutate();
          }}
        >
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="new-class-name">班级名称</FieldLabel>
              <Input
                id="new-class-name"
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
                maxLength={40}
                placeholder="例如：三年级（2）班"
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="new-class-code">班级激活码</FieldLabel>
              <Input
                id="new-class-code"
                required
                value={code}
                onChange={(e) => setCode(e.target.value)}
                placeholder="XXXX-XXXX"
                maxLength={20}
                autoCapitalize="characters"
                className="font-mono uppercase"
              />
              <FieldDescription className="flex items-center gap-1.5">
                <KeyRound className="size-3.5" />
                激活码请向学校管理员或运营方获取。
              </FieldDescription>
            </Field>
          </FieldGroup>
        </form>
        <DialogFooter>
          <Button
            type="submit"
            form="class-create"
            disabled={!name.trim() || !code.trim() || mutation.isPending}
          >
            {mutation.isPending ? <Spinner /> : <Plus />}创建班级
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function JoinForm({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const [code, setCode] = useState('');
  const cache = useQueryClient();
  const join = useMutation({
    mutationFn: () =>
      api<Class>('/classes/join', {
        method: 'POST',
        body: JSON.stringify({ code: code.trim().toUpperCase() }),
      }),
    onSuccess: () => {
      void cache.invalidateQueries({ queryKey: ['classes'] });
      toast.success('已加入班级，一起开始照顾这间教室');
      onOpenChange(false);
      setCode('');
    },
    onError: (e) => toast.error(e.message),
  });
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>和同事一起管理班级</DialogTitle>
          <DialogDescription>输入班级创建者分享给你的协同码。协同码一次有效，有效期 7 天。</DialogDescription>
        </DialogHeader>
        <form
          id="class-join"
          onSubmit={(e) => {
            e.preventDefault();
            join.mutate();
          }}
        >
          <Field>
            <FieldLabel htmlFor="join-code">班级协同码</FieldLabel>
            <Input
              id="join-code"
              required
              value={code}
              onChange={(e) => setCode(e.target.value)}
              placeholder="输入协同码"
              maxLength={32}
              autoCapitalize="characters"
              className="font-mono uppercase"
            />
          </Field>
        </form>
        <DialogFooter>
          <Button type="submit" form="class-join" disabled={!code.trim() || join.isPending}>
            <UserPlus />
            加入班级
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type Confirm = { title: string; description: string; run: () => Promise<void> | void };

function ClassDetails({
  classItem: c,
  initialTab,
  onClose,
}: {
  classItem: Class;
  initialTab: string;
  onClose: () => void;
}) {
  const cache = useQueryClient();
  const { user } = useAuth();
  const owner = c.role === 'owner' || user?.role === 'school_admin' || user?.role === 'super_admin';
  const [tab, setTab] = useState(initialTab);
  const [bind, setBind] = useState<BindCode | null>(null);
  const [invite, setInvite] = useState<{ code: string; expires_at: string } | null>(null);
  const [volume, setVolume] = useState(c.volume);
  const [name, setName] = useState(c.name);
  const [confirm, setConfirm] = useState<Confirm | null>(null);
  const [pending, setPending] = useState(false);
  const [seconds, setSeconds] = useState('300');
  const [names, setNames] = useState('');
  const [toolPending, setToolPending] = useState(false);
  const [transfer, setTransfer] = useState('');
  const now = useNow();
  const devices = useQuery({
    queryKey: ['devices', c.id],
    queryFn: () => api<Device[]>(`/classes/${c.id}/devices`),
  });
  const members = useQuery({
    queryKey: ['members', c.id],
    queryFn: () => api<Member[]>(`/classes/${c.id}/members`),
  });
  const update = useMutation({
    mutationFn: () =>
      api<Class>(`/classes/${c.id}`, {
        method: 'PATCH',
        body: JSON.stringify({ name: name.trim(), volume }),
      }),
    onSuccess: () => {
      void cache.invalidateQueries({ queryKey: ['classes'] });
      toast.success('班级设置已保存');
    },
    onError: (e) => toast.error(e.message),
  });
  const bindMutation = useMutation({
    mutationFn: () => api<BindCode>(`/classes/${c.id}/bind-code`, { method: 'POST' }),
    onSuccess: setBind,
    onError: (e) => toast.error(e.message),
  });
  const inviteMutation = useMutation({
    mutationFn: () =>
      api<{ code: string; expires_at: string }>(`/classes/${c.id}/invites`, { method: 'POST' }),
    onSuccess: setInvite,
    onError: (e) => toast.error(e.message),
  });
  async function action(path: string, method = 'POST', body?: unknown) {
    setPending(true);
    try {
      await api(path, { method, ...(body ? { body: JSON.stringify(body) } : {}) });
      toast.success('操作已完成');
      void cache.invalidateQueries({ queryKey: ['classes'] });
      void cache.invalidateQueries({ queryKey: ['devices', c.id] });
      void cache.invalidateQueries({ queryKey: ['members', c.id] });
      setConfirm(null);
      return true;
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '操作失败');
      return false;
    } finally {
      setPending(false);
    }
  }
  async function tool(type: string, params?: unknown) {
    setToolPending(true);
    try {
      await api(`/classes/${c.id}/tools/${type}`, { method: 'POST', body: JSON.stringify(params ?? {}) });
      toast.success('课堂工具指令已发送');
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '指令发送失败');
    } finally {
      setToolPending(false);
    }
  }
  const remaining = bind ? Math.max(0, Math.floor((new Date(bind.expires_at).valueOf() - now) / 1000)) : 0;
  const collaborators = members.data?.filter((m) => m.role !== 'owner') ?? [];
  return (
    <>
      <Dialog
        open
        onOpenChange={(open) => {
          if (!open) onClose();
        }}
      >
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{c.name}</DialogTitle>
            <DialogDescription>管理这间教室的连接、同事和课堂工具。</DialogDescription>
          </DialogHeader>
          <Tabs value={tab} onValueChange={(value) => setTab(String(value))} className="min-w-0">
            <TabsList className="w-full">
              <TabsTrigger value="devices">
                <Monitor />
                教室大屏
              </TabsTrigger>
              <TabsTrigger value="members">
                <Users />
                协同老师
              </TabsTrigger>
              <TabsTrigger value="tools">
                <Timer />
                课堂工具
              </TabsTrigger>
              <TabsTrigger value="settings">
                <Settings2 />
                班级设置
              </TabsTrigger>
            </TabsList>
            <TabsContent value="devices" className="grid gap-4 pt-2">
              {owner && (
                <div className="grid justify-items-center gap-3 rounded-lg border bg-muted/50 p-5 text-center">
                  <p className="text-sm text-muted-foreground">在教室大屏端输入 6 位绑定码</p>
                  {bind ? (
                    <>
                      <div className="bind-digits flex gap-2 font-mono text-2xl font-semibold">
                        {bind.code.split('').map((digit, i) => (
                          <span
                            key={i}
                            className="flex size-11 items-center justify-center rounded-md border bg-background"
                          >
                            {digit}
                          </span>
                        ))}
                      </div>
                      <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
                        <Clock3 className="size-3.5" />
                        {remaining
                          ? `${Math.floor(remaining / 60)} 分 ${String(remaining % 60).padStart(2, '0')} 秒后过期`
                          : '绑定码已过期，请重新获取'}
                      </p>
                    </>
                  ) : (
                    <p className="text-sm">绑定一次，之后自动接收通知。</p>
                  )}
                  <Button
                    variant="secondary"
                    onClick={() => bindMutation.mutate()}
                    disabled={bindMutation.isPending}
                  >
                    <RefreshCw />
                    {bind ? '重新获取绑定码' : '获取绑定码'}
                  </Button>
                </div>
              )}
              <div className="flex items-center justify-between">
                <h3 className="text-sm font-medium">已连接设备</h3>
                <Badge variant="secondary">
                  {devices.data?.length ?? 0} / {c.max_devices}
                </Badge>
              </div>
              {devices.isLoading ? (
                <Loading />
              ) : devices.error ? (
                <ErrorState error={devices.error} />
              ) : !devices.data?.length ? (
                <EmptyState
                  icon={<Monitor />}
                  title="大屏还没上线"
                  description="在大屏端输入绑定码后，设备会显示在这里。"
                />
              ) : (
                <ItemGroup className="gap-2">
                  {devices.data.map((device) => (
                    <Item key={device.id} variant="outline">
                      <ItemMedia variant="icon">
                        <Monitor />
                      </ItemMedia>
                      <ItemContent>
                        <ItemTitle>
                          {device.name}
                          <Badge variant={device.online ? 'default' : 'secondary'}>
                            {device.online ? '在线' : '离线'}
                          </Badge>
                        </ItemTitle>
                        <ItemDescription>
                          {device.os} · {device.client_version ? `v${device.client_version}` : '版本未知'} ·
                          最近连接：{formatTime(device.last_seen_at)}
                        </ItemDescription>
                      </ItemContent>
                      {owner && (
                        <ItemActions>
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            aria-label={`解绑${device.name}`}
                            onClick={() =>
                              setConfirm({
                                title: '解绑这块大屏？',
                                description: '解绑后设备将停止接收通知，需要输入新绑定码才能重新连接。',
                                run: () => void action(`/devices/${device.id}`, 'DELETE'),
                              })
                            }
                          >
                            <Trash2 />
                          </Button>
                        </ItemActions>
                      )}
                    </Item>
                  ))}
                </ItemGroup>
              )}
            </TabsContent>
            <TabsContent value="members" className="grid gap-4 pt-2">
              {owner && (
                <div className="grid gap-3 rounded-lg border p-4">
                  <div className="flex flex-wrap items-center gap-3">
                    <div className="min-w-0 flex-1">
                      <p className="font-medium">邀请任课老师，一起用叮咚</p>
                      <p className="text-sm text-muted-foreground">
                        教师席位：{members.data?.length ?? 0} / {c.max_teachers} 位
                      </p>
                    </div>
                    <Button
                      variant="secondary"
                      disabled={inviteMutation.isPending}
                      onClick={() => inviteMutation.mutate()}
                    >
                      生成协同码
                    </Button>
                  </div>
                  {invite && (
                    <div className="invite-result flex flex-wrap items-center gap-2 rounded-md bg-muted p-3">
                      <code className="font-mono text-lg font-semibold tracking-wider">{invite.code}</code>
                      <CopyButton
                        text={invite.code}
                        size="sm"
                        variant="outline"
                        onCopied={() => toast.success('协同码已复制')}
                      >
                        <Copy />
                        复制
                      </CopyButton>
                      <p className="w-full text-xs text-muted-foreground">
                        有效期至 {formatTime(invite.expires_at)} · 仅可使用一次
                      </p>
                    </div>
                  )}
                </div>
              )}
              {members.isLoading ? (
                <Loading />
              ) : members.error ? (
                <ErrorState error={members.error} />
              ) : (
                <ItemGroup className="gap-2">
                  {members.data?.map((member) => (
                    <Item key={member.user_id} variant="outline">
                      <ItemMedia>
                        <Avatar>
                          <AvatarFallback>{initials(member.display_name)}</AvatarFallback>
                        </Avatar>
                      </ItemMedia>
                      <ItemContent>
                        <ItemTitle>
                          {member.display_name}
                          {member.user_id === user?.id ? '（你）' : ''}
                        </ItemTitle>
                        <ItemDescription>{member.username}</ItemDescription>
                      </ItemContent>
                      <ItemActions>
                        <Badge variant={member.role === 'owner' ? 'default' : 'secondary'}>
                          {member.role === 'owner' ? '创建者' : '协同老师'}
                        </Badge>
                        {owner && member.role !== 'owner' && (
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            aria-label={`移除${member.display_name}`}
                            onClick={() =>
                              setConfirm({
                                title: '移除这位协同老师？',
                                description: '移除后将释放教师席位，对方将无法再向本班发送通知。',
                                run: () =>
                                  void action(`/classes/${c.id}/members/${member.user_id}`, 'DELETE'),
                              })
                            }
                          >
                            <Trash2 />
                          </Button>
                        )}
                      </ItemActions>
                    </Item>
                  ))}
                </ItemGroup>
              )}
            </TabsContent>
            <TabsContent value="tools" className="grid gap-4 pt-2">
              <Alert>
                <Monitor />
                <AlertDescription>工具将在该班已连接的大屏上运行。</AlertDescription>
              </Alert>
              <div className="grid gap-4 sm:grid-cols-2">
                <form
                  className="grid content-start gap-3 rounded-lg border p-4"
                  onSubmit={(e) => {
                    e.preventDefault();
                    void tool('countdown', { seconds: Number(seconds) });
                  }}
                >
                  <p className="flex items-center gap-2 font-medium">
                    <Timer className="size-4" />
                    课堂倒计时
                  </p>
                  <Field>
                    <FieldLabel htmlFor="tool-seconds">倒计时时长</FieldLabel>
                    <SimpleSelect
                      id="tool-seconds"
                      label="倒计时时长"
                      value={seconds}
                      onValueChange={setSeconds}
                      options={[60, 180, 300, 600, 900, 1800].map((sec) => ({
                        value: String(sec),
                        label: `${sec / 60} 分钟`,
                      }))}
                    />
                  </Field>
                  <Button variant="secondary" type="submit" disabled={toolPending}>
                    开始倒计时
                  </Button>
                </form>
                <form
                  className="grid content-start gap-3 rounded-lg border p-4"
                  onSubmit={(e) => {
                    e.preventDefault();
                    void tool('rollcall', {
                      names: names
                        .split(/[\n,，、]/)
                        .map((n) => n.trim())
                        .filter(Boolean),
                    });
                  }}
                >
                  <p className="flex items-center gap-2 font-medium">
                    <Users className="size-4" />
                    随机点名
                  </p>
                  <Field>
                    <FieldLabel htmlFor="tool-names">本次参与的同学</FieldLabel>
                    <Textarea
                      id="tool-names"
                      value={names}
                      onChange={(e) => setNames(e.target.value)}
                      placeholder="输入姓名，每行一位；仅用于本次点名。"
                      maxLength={2000}
                      className="min-h-20"
                    />
                  </Field>
                  <Button variant="secondary" type="submit" disabled={!names.trim() || toolPending}>
                    开始随机点名
                  </Button>
                </form>
              </div>
              <Button variant="outline" disabled={toolPending} onClick={() => void tool('stop')}>
                停止大屏工具
              </Button>
            </TabsContent>
            <TabsContent value="settings" className="grid gap-4 pt-2">
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="detail-name">班级名称</FieldLabel>
                  <Input
                    id="detail-name"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    maxLength={40}
                    disabled={!owner}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="detail-volume">
                    播报音量 <span className="ml-auto font-mono text-muted-foreground">{volume}%</span>
                  </FieldLabel>
                  <input
                    id="detail-volume"
                    type="range"
                    min={0}
                    max={100}
                    value={volume}
                    onChange={(e) => setVolume(Number(e.target.value))}
                    disabled={!owner}
                    className="w-full accent-primary"
                  />
                  <FieldDescription className="flex items-center gap-1.5">
                    <Volume2 className="size-3.5" />
                    大屏播报后会恢复原来的系统音量。
                  </FieldDescription>
                </Field>
              </FieldGroup>
              {owner ? (
                <>
                  <Button
                    className="justify-self-start"
                    disabled={update.isPending || !name.trim()}
                    onClick={() => update.mutate()}
                  >
                    保存班级设置
                  </Button>
                  <Separator />
                  <div className="grid gap-3 rounded-lg border p-4">
                    <div>
                      <p className="font-medium">转让班级</p>
                      <p className="text-sm text-muted-foreground">
                        转让后，对方成为创建者，你成为协同老师。
                      </p>
                    </div>
                    <div className="flex flex-col gap-2 sm:flex-row">
                      <SimpleSelect
                        label="选择一位协同老师"
                        placeholder={collaborators.length ? '选择一位协同老师' : '暂无协同老师'}
                        value={transfer}
                        onValueChange={setTransfer}
                        options={collaborators.map((m) => ({ value: m.user_id, label: m.display_name }))}
                        disabled={!collaborators.length}
                      />
                      <Button
                        variant="outline"
                        disabled={!transfer}
                        onClick={() =>
                          setConfirm({
                            title: '转让班级创建者？',
                            description: '对方将获得成员管理、设备绑定、班级删除等权限。',
                            run: async () => {
                              if (await action(`/classes/${c.id}/transfer`, 'POST', { user_id: transfer }))
                                onClose();
                            },
                          })
                        }
                      >
                        确认转让
                      </Button>
                    </div>
                  </div>
                  <div className="grid gap-3 rounded-lg border border-destructive/30 p-4">
                    <div>
                      <p className="font-medium text-destructive">删除班级</p>
                      <p className="text-sm text-muted-foreground">
                        所有设备将被解绑，激活码释放；是否可重新使用由运营方决定。
                      </p>
                    </div>
                    <Button
                      variant="destructive"
                      className="justify-self-start"
                      onClick={() =>
                        setConfirm({
                          title: '确认删除这个班级？',
                          description: '所有设备和协同关系都会解除。请确认这是你要删除的班级。',
                          run: async () => {
                            if (await action(`/classes/${c.id}`, 'DELETE')) onClose();
                          },
                        })
                      }
                    >
                      删除班级
                    </Button>
                  </div>
                </>
              ) : (
                <Button
                  variant="destructive"
                  className="justify-self-start"
                  onClick={() =>
                    setConfirm({
                      title: '退出这个班级？',
                      description: '退出后将无法再向这个班级发送通知。',
                      run: async () => {
                        if (await action(`/classes/${c.id}/leave`)) onClose();
                      },
                    })
                  }
                >
                  退出班级
                </Button>
              )}
            </TabsContent>
          </Tabs>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={!!confirm}
        onOpenChange={(open) => {
          if (!open) setConfirm(null);
        }}
        title={confirm?.title ?? ''}
        description={confirm?.description}
        confirmLabel="确认操作"
        pending={pending}
        onConfirm={() => void confirm?.run()}
      />
    </>
  );
}
