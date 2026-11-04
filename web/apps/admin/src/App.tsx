import { useEffect, useState, type FormEvent, type ReactNode } from 'react';
import { NavLink, Navigate, Route, Routes, useLocation } from 'react-router';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api, ApiError, onSessionExpired, refreshSession, setAccessToken } from '@classding/api';
import { toast } from 'sonner';
import {
  BellRing,
  BookOpen,
  ClipboardList,
  KeyRound,
  LayoutDashboard,
  LogOut,
  Monitor,
  RefreshCw,
  School,
  Settings,
  ShieldCheck,
  Users,
  type LucideIcon,
} from 'lucide-react';
import { Avatar, AvatarFallback } from '@classding/ui/components/ui/avatar';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@classding/ui/components/ui/breadcrumb';
import { Button } from '@classding/ui/components/ui/button';
import { Card, CardContent } from '@classding/ui/components/ui/card';
import { Field, FieldError, FieldGroup, FieldLabel } from '@classding/ui/components/ui/field';
import { Input } from '@classding/ui/components/ui/input';
import { Separator } from '@classding/ui/components/ui/separator';
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarRail,
  SidebarTrigger,
} from '@classding/ui/components/ui/sidebar';
import { Spinner } from '@classding/ui/components/ui/spinner';
import { Brand, SiteFooter, UiScaleMenu, applyUiScale } from '@classding/ui/components/classding';
import { cn } from '@classding/ui/lib/utils';
import {
  SessionProvider,
  errorMessage,
  initial,
  json,
  roleName,
  useSession,
  type AuthResponse,
  type User,
} from './lib';
import OverviewPage from './pages/Overview';
import CodesPage from './pages/Codes';
import UsersPage from './pages/Users';
import ClassesPage from './pages/Classes';
import DevicesPage from './pages/Devices';
import MessagesPage from './pages/Messages';
import AuditPage from './pages/Audit';
import SettingsPage from './pages/Settings';
import SchoolsPage from './pages/Schools';

function Login({ onLogin, initialError }: { onLogin: (user: User) => void; initialError: string }) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState(initialError);
  const login = useMutation({
    mutationFn: () => api<AuthResponse>('/auth/login', json('POST', { username: username.trim(), password })),
    onSuccess: (result) => {
      if (result.user.role !== 'super_admin' && result.user.role !== 'school_admin') {
        setAccessToken('');
        setError('此账号没有管理后台权限，请使用管理员账号登录。');
        return;
      }
      setAccessToken(result.access_token, result.user.id);
      onLogin(result.user);
    },
    onError: (err) => setError(errorMessage(err)),
  });
  function submit(event: FormEvent) {
    event.preventDefault();
    setError('');
    if (!username.trim() || !password) {
      setError('请输入账号和密码。');
      return;
    }
    login.mutate();
  }
  return (
    <div className="flex min-h-svh flex-col bg-muted">
      <div className="flex flex-1 items-center justify-center p-4 md:p-10">
        <div className="w-full max-w-sm md:max-w-4xl">
          <Card className="overflow-hidden p-0">
            <CardContent className="grid p-0 md:grid-cols-2">
              <form onSubmit={submit} className="flex flex-col gap-6 p-6 md:p-10">
                <Brand subtitle="管理后台" className="login-brand" />
                <div className="space-y-1">
                  <h1 className="text-2xl font-semibold tracking-tight">欢迎回来</h1>
                  <p className="text-sm text-muted-foreground">登录管理后台，继续今天的工作。</p>
                </div>
                <FieldGroup>
                  <Field>
                    <FieldLabel htmlFor="admin-username">管理员账号</FieldLabel>
                    <Input
                      id="admin-username"
                      autoFocus
                      autoComplete="username"
                      placeholder="请输入账号"
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="admin-password">密码</FieldLabel>
                    <Input
                      id="admin-password"
                      type="password"
                      autoComplete="current-password"
                      placeholder="请输入密码"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                    />
                  </Field>
                  {error && <FieldError role="alert">{error}</FieldError>}
                  <Button type="submit" size="lg" className="w-full" disabled={login.isPending}>
                    {login.isPending ? (
                      <>
                        <Spinner />
                        正在登录…
                      </>
                    ) : (
                      '登录管理后台'
                    )}
                  </Button>
                </FieldGroup>
                <p className="flex items-center justify-center gap-1.5 text-xs text-muted-foreground">
                  <ShieldCheck className="size-3.5" />
                  仅限授权管理员访问
                </p>
              </form>
              <div className="relative hidden flex-col justify-between overflow-hidden bg-primary p-10 text-primary-foreground md:flex">
                <div className="absolute -top-24 -right-24 size-72 rounded-full bg-primary-foreground/10" />
                <div className="absolute -bottom-32 -left-16 size-80 rounded-full bg-primary-foreground/5" />
                <span className="relative flex size-12 items-center justify-center rounded-xl bg-primary-foreground/15">
                  <BellRing className="size-6" />
                </span>
                <div className="relative space-y-3">
                  <p className="text-2xl leading-snug font-semibold">
                    让每一声叮咚，
                    <br />
                    都安心抵达。
                  </p>
                  <p className="text-sm text-primary-foreground/80">
                    为每一间教室、每一位老师，把日常管理照顾得更简单一些。
                  </p>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
      <SiteFooter className="login-copyright" />
    </div>
  );
}

const navItems: { to: string; label: string; icon: LucideIcon; superOnly?: boolean }[] = [
  { to: '/', label: '工作概览', icon: LayoutDashboard },
  { to: '/schools', label: '学校管理', icon: School, superOnly: true },
  { to: '/activation-codes', label: '激活码管理', icon: KeyRound, superOnly: true },
  { to: '/users', label: '教师管理', icon: Users },
  { to: '/classes', label: '班级管理', icon: BookOpen },
  { to: '/devices', label: '设备管理', icon: Monitor },
  { to: '/messages', label: '消息审计', icon: BellRing },
  { to: '/audit-logs', label: '操作日志', icon: ClipboardList, superOnly: true },
  { to: '/settings', label: '系统配置', icon: Settings, superOnly: true },
];

function AppSidebar({ items }: { items: typeof navItems }) {
  const { user, logout } = useSession();
  const location = useLocation();
  return (
    <Sidebar variant="inset" collapsible="icon" className="sidebar">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" render={<NavLink to="/" />} className="sidebar-brand">
              <Brand subtitle="管理后台" />
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>工作空间</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu className="side-nav">
              {items.map((item) => {
                const active =
                  item.to === '/' ? location.pathname === '/' : location.pathname.startsWith(item.to);
                return (
                  <SidebarMenuItem key={item.to}>
                    <SidebarMenuButton
                      isActive={active}
                      tooltip={item.label}
                      render={<NavLink to={item.to} end={item.to === '/'} />}
                    >
                      <item.icon />
                      <span>{item.label}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                );
              })}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter>
        <div className="sidebar-user flex items-center gap-2 rounded-lg p-1.5 group-data-[collapsible=icon]:p-0">
          <Avatar className="size-8 rounded-lg">
            <AvatarFallback className="rounded-lg">{initial(user)}</AvatarFallback>
          </Avatar>
          <div className="grid min-w-0 flex-1 text-left text-sm leading-tight group-data-[collapsible=icon]:hidden">
            <span className="truncate font-medium">{user.display_name || user.username}</span>
            <span className="truncate text-xs text-muted-foreground">{roleName[user.role] ?? user.role}</span>
          </div>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={logout}
            title="退出登录"
            aria-label="退出登录"
            className="group-data-[collapsible=icon]:hidden"
          >
            <LogOut />
          </Button>
        </div>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}

function Shell() {
  const { user, logout, updateUser } = useSession();
  const queryClient = useQueryClient();
  const location = useLocation();
  const visible = navItems.filter((item) => !item.superOnly || user.role === 'super_admin');
  const current = navItems.find((item) => item.to === location.pathname)?.label ?? '管理后台';
  useEffect(() => {
    document
      .querySelector('.mobile-nav [aria-current="page"]')
      ?.scrollIntoView({ block: 'nearest', inline: 'nearest' });
  }, [location.pathname]);
  return (
    <SidebarProvider>
      <AppSidebar items={visible} />
      <SidebarInset className="main-column min-w-0">
        <header className="sticky top-0 z-10 flex h-14 shrink-0 items-center gap-2 border-b bg-background/95 px-4 backdrop-blur md:rounded-t-xl">
          <SidebarTrigger className="-ml-1 hidden md:inline-flex" />
          <Separator
            orientation="vertical"
            className="mr-2 hidden data-vertical:h-4 data-vertical:self-auto md:block"
          />
          <Breadcrumb className="min-w-0">
            <BreadcrumbList className="flex-nowrap">
              <BreadcrumbItem className="hidden sm:inline-flex">管理后台</BreadcrumbItem>
              <BreadcrumbSeparator className="hidden sm:inline-flex" />
              <BreadcrumbItem>
                <BreadcrumbPage className="truncate">{current}</BreadcrumbPage>
              </BreadcrumbItem>
            </BreadcrumbList>
          </Breadcrumb>
          <div className="ml-auto flex items-center gap-1">
            <span className="mr-2 hidden text-sm text-muted-foreground lg:inline">
              {new Date().toLocaleDateString('zh-CN', {
                year: 'numeric',
                month: 'long',
                day: 'numeric',
                weekday: 'long',
              })}
            </span>
            <UiScaleMenu
              value={user.ui_scale}
              onChange={(scale) => {
                applyUiScale(scale);
                const previous = user;
                updateUser({ ...user, ui_scale: scale });
                api<User>('/me', json('PATCH', { ui_scale: scale }))
                  .then(updateUser)
                  .catch((error) => {
                    applyUiScale(previous.ui_scale);
                    updateUser(previous);
                    toast.error(`字号保存失败：${errorMessage(error)}`);
                  });
              }}
            />
            <Button
              variant="ghost"
              size="icon"
              aria-label="刷新数据"
              title="刷新数据"
              onClick={() => void queryClient.invalidateQueries()}
            >
              <RefreshCw />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="md:hidden"
              onClick={logout}
              title="退出登录"
              aria-label="退出登录"
            >
              <LogOut />
            </Button>
          </div>
        </header>
        <nav
          className="mobile-nav no-scrollbar flex gap-1 overflow-x-auto border-b px-3 py-2 md:hidden"
          aria-label="管理后台导航"
        >
          {visible.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === '/'}
              className={({ isActive }) =>
                cn(
                  'flex shrink-0 items-center gap-1.5 rounded-md px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground',
                  isActive && 'bg-muted font-medium text-foreground',
                )
              }
            >
              <item.icon className="size-4" />
              {item.label}
            </NavLink>
          ))}
        </nav>
        <main className="main-content mx-auto flex w-full max-w-6xl flex-1 flex-col gap-6 p-4 md:p-6">
          <Routes>
            <Route path="/" element={<OverviewPage />} />
            <Route
              path="/activation-codes"
              element={
                <SuperOnly>
                  <CodesPage />
                </SuperOnly>
              }
            />
            <Route
              path="/schools"
              element={
                <SuperOnly>
                  <SchoolsPage />
                </SuperOnly>
              }
            />
            <Route path="/users" element={<UsersPage />} />
            <Route path="/classes" element={<ClassesPage />} />
            <Route path="/devices" element={<DevicesPage />} />
            <Route path="/messages" element={<MessagesPage />} />
            <Route
              path="/audit-logs"
              element={
                <SuperOnly>
                  <AuditPage />
                </SuperOnly>
              }
            />
            <Route
              path="/settings"
              element={
                <SuperOnly>
                  <SettingsPage />
                </SuperOnly>
              }
            />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </main>
        <SiteFooter className="app-footer" />
      </SidebarInset>
    </SidebarProvider>
  );
}

function SuperOnly({ children }: { children: ReactNode }) {
  const { user } = useSession();
  if (user.role !== 'super_admin')
    return (
      <div className="flex min-h-[50vh] flex-col items-center justify-center gap-3 text-center">
        <ShieldCheck className="size-8 text-muted-foreground" />
        <h2 className="text-lg font-semibold">没有访问权限</h2>
        <p className="text-sm text-muted-foreground">此页面仅对运营管理员开放。</p>
        <Button variant="outline" nativeButton={false} render={<NavLink to="/" />}>
          返回工作概览
        </Button>
      </div>
    );
  return <>{children}</>;
}

export default function App() {
  const [user, setUser] = useState<User | null>(null);
  const [restoring, setRestoring] = useState(true);
  const [startupError, setStartupError] = useState('');
  const queryClient = useQueryClient();
  useEffect(
    () =>
      onSessionExpired(() => {
        setUser(null);
        queryClient.clear();
        toast.error('登录已过期或账号已变更，请重新登录');
      }),
    [queryClient],
  );
  useEffect(() => {
    let cancelled = false;
    refreshSession<AuthResponse>()
      .then((result) => {
        if (cancelled) return;
        if (result.user.role === 'super_admin' || result.user.role === 'school_admin') setUser(result.user);
        else setAccessToken('');
      })
      .catch((error) => {
        if (!cancelled && error instanceof ApiError && (error.status === 0 || error.status >= 500))
          setStartupError(error.message);
      })
      .finally(() => {
        if (!cancelled) setRestoring(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);
  useEffect(() => {
    if (!user) return;
    let cancelled = false;
    const expectedUserId = user.id;
    const timer = window.setInterval(
      () => {
        refreshSession<AuthResponse>()
          .then((result) => {
            if (cancelled) return;
            if (
              result.user.id !== expectedUserId ||
              (result.user.role !== 'super_admin' && result.user.role !== 'school_admin')
            ) {
              setAccessToken('');
              setUser(null);
              queryClient.clear();
              toast.error('登录账号或权限已变更，请重新登录');
              return;
            }
            setUser(result.user);
          })
          .catch((error) => {
            if (!cancelled && error instanceof ApiError && error.status !== 401)
              toast.error('暂时无法更新登录状态，请检查网络');
          });
      },
      13 * 60 * 1000,
    );
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [user?.id, queryClient]);
  function logout() {
    void api('/auth/logout', json('POST')).catch(() => undefined);
    setAccessToken('');
    setUser(null);
    queryClient.clear();
  }
  // Each administrator's last chosen text size follows them to any browser.
  useEffect(() => {
    applyUiScale(user?.ui_scale);
  }, [user?.ui_scale]);
  if (restoring)
    return (
      <div className="flex min-h-svh items-center justify-center gap-2 text-sm text-muted-foreground">
        <Spinner />
        正在进入管理后台…
      </div>
    );
  if (!user) return <Login onLogin={setUser} initialError={startupError} />;
  return (
    <SessionProvider value={{ user, logout, updateUser: setUser }}>
      <Shell />
    </SessionProvider>
  );
}
