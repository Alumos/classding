import { useState, type ReactNode } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@classding/api';
import {
  BookOpen,
  ChevronRight,
  Download,
  FileText,
  KeyRound,
  LogOut,
  Monitor,
  PenLine,
  ShieldCheck,
  UserPlus,
} from 'lucide-react';
import { toast } from 'sonner';
import { Avatar, AvatarFallback } from '@classding/ui/components/ui/avatar';
import { Badge } from '@classding/ui/components/ui/badge';
import { Button } from '@classding/ui/components/ui/button';
import {
  Card,
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
  ItemSeparator,
  ItemTitle,
} from '@classding/ui/components/ui/item';
import { Separator } from '@classding/ui/components/ui/separator';
import { Brand } from '@classding/ui/components/classding';
import { useAuth } from '../auth';
import { useClasses } from '../hooks';
import { PageHeader, initials } from '../components';
import type { Class, User } from '../models';
import { APP_VERSION } from '@classding/ui/version';

type Info = 'install' | 'privacy' | 'terms';
const infoTitles: Record<Info, string> = {
  install: '连接教室大屏',
  privacy: '隐私与数据说明',
  terms: '关于课堂叮咚',
};

function InfoContent({ info }: { info: Info }) {
  const section = (title: string, children: ReactNode) => (
    <section className="grid gap-1">
      <h3 className="font-medium">{title}</h3>
      <p className="text-sm leading-relaxed text-muted-foreground">{children}</p>
    </section>
  );
  if (info === 'install')
    return (
      <div className="grid gap-4">
        {section('1. 开通班级', '向学校管理员获取激活码，在「我的班级」中新建班级。')}
        {section(
          '2. 安装大屏端',
          '大屏端面向 Windows 10 / 11。优先安装到不会被还原的磁盘，并请学校管理员将软件加入还原保护例外。',
        )}
        {section(
          '3. 绑定教室',
          '在班级设置中获取 6 位绑定码，在大屏端输入。绑定码 10 分钟内有效，仅使用一次。',
        )}
        {section(
          '4. 测试播报',
          '发送一条测试通知，确认屏幕、语音、音量和回执正常。Windows 的音量恢复、置顶覆盖与还原环境需在学校实际设备上验证。',
        )}
      </div>
    );
  if (info === 'privacy')
    return (
      <div className="grid gap-4">
        <p className="text-sm text-muted-foreground">
          这是开发版本的数据处理说明，部署方须在正式上线前补充真实主体、联系方式和适用的隐私政策。
        </p>
        {section(
          '收集与使用',
          '服务保存教师账号、班级关系、通知内容及播放回执，用于完成消息投递和权限管理。学生姓名仅在老师主动填写时随通知发送，默认不建立花名册。',
        )}
        {section(
          '录音与留存',
          '录音在老师授权麦克风后上传到部署方服务器。默认关闭语音识别，不发送到外部语音厂商。消息默认保留 90 天，录音默认保留 7 天，由服务器定期清理。',
        )}
        {section(
          '查询与删除',
          '教师可查看和撤回通知。账号注销及进一步的数据删除申请请联系实际部署方；本开发版本尚未提供自助注销入口。',
        )}
      </div>
    );
  return (
    <div className="grid gap-4">
      <Brand />
      <p className="text-sm leading-relaxed text-muted-foreground">
        让每一声提醒，恰好被听见。课堂叮咚通过教师端、服务端与教室大屏连接课堂，支持多位老师、多间教室和通知回执。
      </p>
      <p className="text-sm text-muted-foreground">
        当前版本为 {APP_VERSION}。生产部署与 Windows 实机验收请参阅项目交付说明。
      </p>
    </div>
  );
}

export default function MePage() {
  const { user, logout, updateUser } = useAuth();
  const classes = useClasses();
  const cache = useQueryClient();
  const [name, setName] = useState(user?.display_name ?? '');
  const [edit, setEdit] = useState(false);
  const [code, setCode] = useState('');
  const [passwordOpen, setPasswordOpen] = useState(false);
  const [current, setCurrent] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [info, setInfo] = useState<Info | null>(null);
  const release = useQuery({
    queryKey: ['client-latest'],
    queryFn: () => api<{ version: string; url: string; sha256: string; notes: string }>('/client/latest'),
    retry: false,
  });
  const update = useMutation({
    mutationFn: () =>
      api<User>('/me', { method: 'PATCH', body: JSON.stringify({ display_name: name.trim() }) }),
    onSuccess: (u) => {
      updateUser(u);
      setEdit(false);
      toast.success('老师落款已更新');
    },
    onError: (e) => toast.error(e.message),
  });
  const join = useMutation({
    mutationFn: () =>
      api<Class>('/classes/join', {
        method: 'POST',
        body: JSON.stringify({ code: code.trim().toUpperCase() }),
      }),
    onSuccess: () => {
      setCode('');
      void cache.invalidateQueries({ queryKey: ['classes'] });
      toast.success('已加入班级，去发送你的第一条提醒吧');
    },
    onError: (e) => toast.error(e.message),
  });
  const change = useMutation({
    mutationFn: () =>
      api('/me/password', {
        method: 'PUT',
        body: JSON.stringify({ current_password: current, new_password: password }),
      }),
    onSuccess: () => {
      setPasswordOpen(false);
      setCurrent('');
      setPassword('');
      setConfirmPassword('');
      toast.success('密码已更新，请重新登录');
      void logout();
    },
    onError: (e) => toast.error(e.message),
  });
  function download() {
    if (!release.data?.url) return;
    const url = new URL(release.data.url, location.origin);
    if (['https:', 'http:'].includes(url.protocol)) location.assign(url.href);
  }
  const links: { icon: typeof KeyRound; title: string; description: string; onClick: () => void }[] = [
    {
      icon: KeyRound,
      title: '修改登录密码',
      description: '定期更新密码，保护你的课堂账号',
      onClick: () => setPasswordOpen(true),
    },
    {
      icon: BookOpen,
      title: '使用与安装指南',
      description: '第一次连接大屏？从这里开始',
      onClick: () => setInfo('install'),
    },
    {
      icon: ShieldCheck,
      title: '隐私说明',
      description: '了解课堂消息与个人信息如何处理',
      onClick: () => setInfo('privacy'),
    },
    {
      icon: FileText,
      title: '产品使用说明',
      description: `课堂叮咚 v${APP_VERSION}`,
      onClick: () => setInfo('terms'),
    },
  ];
  return (
    <>
      <PageHeader
        title={`你好，${user?.display_name || '老师'}`}
        description="把沟通交给叮咚，把更多时间留给课堂。"
      />
      <div className="grid items-start gap-6 lg:grid-cols-[20rem_minmax(0,1fr)]">
        <Card>
          <CardContent className="grid justify-items-center gap-3 text-center">
            <Avatar className="size-16">
              <AvatarFallback className="text-xl">{initials(user?.display_name)}</AvatarFallback>
            </Avatar>
            <div>
              <p className="text-lg font-semibold">{user?.display_name}</p>
              <p className="text-sm text-muted-foreground">@{user?.username}</p>
            </div>
            <Badge variant="secondary">叮咚教师</Badge>
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setName(user?.display_name ?? '');
                setEdit(true);
              }}
            >
              <PenLine />
              修改落款
            </Button>
            <Separator className="my-1" />
            <div className="grid w-full grid-cols-2 text-sm">
              <div>
                <p className="text-xl font-semibold tabular-nums">
                  {classes.data?.filter((c) => c.role === 'owner').length ?? 0}
                </p>
                <p className="text-muted-foreground">个创建班级</p>
              </div>
              <div>
                <p className="text-xl font-semibold tabular-nums">
                  {classes.data?.filter((c) => c.role === 'collaborator').length ?? 0}
                </p>
                <p className="text-muted-foreground">个协同班级</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <div className="grid gap-6">
          <Card>
            <CardHeader>
              <CardTitle role="heading" aria-level={2} className="flex items-center gap-2">
                <UserPlus className="size-4" />
                和同事，一起照顾班级
              </CardTitle>
              <CardDescription>获得班主任的协同码后，在这里加入班级。协同码一次有效。</CardDescription>
            </CardHeader>
            <CardContent>
              <form
                className="flex gap-2"
                onSubmit={(e) => {
                  e.preventDefault();
                  join.mutate();
                }}
              >
                <Input
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                  placeholder="输入班级协同码"
                  aria-label="班级协同码"
                  maxLength={32}
                  required
                  className="font-mono uppercase"
                />
                <Button type="submit" disabled={!code.trim() || join.isPending}>
                  加入班级
                </Button>
              </form>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle role="heading" aria-level={2} className="flex items-center gap-2">
                <Monitor className="size-4" />
                让教室大屏，听见你的提醒
              </CardTitle>
              <CardDescription>安装课堂叮咚大屏端（Windows 10 / 11），输入绑定码即可连接。</CardDescription>
            </CardHeader>
            <CardFooter className="flex-wrap gap-2">
              <Button disabled={!release.data?.url} onClick={download}>
                <Download />
                {release.data?.url ? '下载大屏客户端' : '安装包尚未发布'}
              </Button>
              <Button variant="ghost" onClick={() => setInfo('install')}>
                安装说明
                <ChevronRight />
              </Button>
              {release.data?.url && (
                <span className="text-xs text-muted-foreground">
                  v{release.data.version} · 下载后校验 SHA-256
                </span>
              )}
            </CardFooter>
          </Card>
          <Card className="gap-0 py-0">
            <CardHeader className="border-b py-4">
              <CardTitle role="heading" aria-level={2}>
                账号与帮助
              </CardTitle>
            </CardHeader>
            <ItemGroup>
              {links.map((link, index) => (
                <div key={link.title}>
                  {index > 0 && <ItemSeparator />}
                  <Item
                    render={<button type="button" onClick={link.onClick} />}
                    className="w-full rounded-none px-6 text-left"
                  >
                    <ItemMedia variant="icon">
                      <link.icon />
                    </ItemMedia>
                    <ItemContent>
                      <ItemTitle>{link.title}</ItemTitle>
                      <ItemDescription>{link.description}</ItemDescription>
                    </ItemContent>
                    <ItemActions>
                      <ChevronRight className="size-4 text-muted-foreground" />
                    </ItemActions>
                  </Item>
                </div>
              ))}
            </ItemGroup>
          </Card>
          <Button variant="outline" className="justify-self-start" onClick={() => void logout()}>
            <LogOut />
            退出当前账号
          </Button>
        </div>
      </div>
      <Dialog open={edit} onOpenChange={setEdit}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>老师怎么称呼</DialogTitle>
            <DialogDescription>这个名字会作为通知落款显示在教室大屏上。</DialogDescription>
          </DialogHeader>
          <form
            id="signature-form"
            onSubmit={(e) => {
              e.preventDefault();
              update.mutate();
            }}
          >
            <Field>
              <FieldLabel htmlFor="me-signature">默认落款</FieldLabel>
              <Input
                id="me-signature"
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
                maxLength={24}
              />
            </Field>
          </form>
          <DialogFooter>
            <Button type="submit" form="signature-form" disabled={!name.trim() || update.isPending}>
              保存落款
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={passwordOpen} onOpenChange={setPasswordOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>修改登录密码</DialogTitle>
            <DialogDescription>更新后需要重新登录。</DialogDescription>
          </DialogHeader>
          <form
            id="password-form"
            onSubmit={(e) => {
              e.preventDefault();
              if (password !== confirmPassword) {
                toast.error('两次输入的新密码不一致');
                return;
              }
              change.mutate();
            }}
          >
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="current-password">当前密码</FieldLabel>
                <Input
                  id="current-password"
                  required
                  type="password"
                  value={current}
                  onChange={(e) => setCurrent(e.target.value)}
                  autoComplete="current-password"
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="new-password">新密码</FieldLabel>
                <Input
                  id="new-password"
                  required
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  minLength={10}
                  autoComplete="new-password"
                />
                <FieldDescription>至少 10 位，包含字母和数字</FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="confirm-password">再次输入新密码</FieldLabel>
                <Input
                  id="confirm-password"
                  required
                  type="password"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  minLength={10}
                  autoComplete="new-password"
                />
              </Field>
            </FieldGroup>
          </form>
          <DialogFooter>
            <Button type="submit" form="password-form" disabled={change.isPending}>
              更新密码
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog
        open={!!info}
        onOpenChange={(open) => {
          if (!open) setInfo(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{info ? infoTitles[info] : ''}</DialogTitle>
          </DialogHeader>
          {info && <InfoContent info={info} />}
        </DialogContent>
      </Dialog>
    </>
  );
}
