import { useState } from 'react';
import { Navigate, NavLink, Outlet, Route, Routes, useLocation } from 'react-router';
import { BookOpenCheck, CircleHelp, History, LayoutGrid, LogOut, Radio, Send, UserRound } from 'lucide-react';
import { Avatar, AvatarFallback } from '@classding/ui/components/ui/avatar';
import { Badge } from '@classding/ui/components/ui/badge';
import { Breadcrumb, BreadcrumbItem, BreadcrumbList, BreadcrumbPage, BreadcrumbSeparator } from '@classding/ui/components/ui/breadcrumb';
import { Button } from '@classding/ui/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@classding/ui/components/ui/dialog';
import { Item, ItemContent, ItemDescription, ItemGroup, ItemMedia, ItemTitle } from '@classding/ui/components/ui/item';
import { Separator } from '@classding/ui/components/ui/separator';
import {
  Sidebar, SidebarContent, SidebarFooter, SidebarGroup, SidebarGroupContent, SidebarGroupLabel, SidebarHeader, SidebarInset,
  SidebarMenu, SidebarMenuButton, SidebarMenuItem, SidebarProvider, SidebarRail, SidebarTrigger,
} from '@classding/ui/components/ui/sidebar';
import { Spinner } from '@classding/ui/components/ui/spinner';
import { Brand, SiteFooter } from '@classding/ui/components/classding';
import { cn } from '@classding/ui/lib/utils';
import { useAuth } from './auth';
import { useTeacherSocket } from './hooks';
import { initials } from './components';
import SendPage from './pages/Send';
import ClassesPage from './pages/Classes';
import HistoryPage from './pages/History';
import MePage from './pages/Me';
import LoginPage from './pages/Login';
import HomeworkPage from './pages/Homework';

const links = [
  { to: '/send', icon: Send, label: '发通知', short: '发通知' },
  { to: '/classes', icon: LayoutGrid, label: '我的班级', short: '班级' },
  { to: '/homework', icon: BookOpenCheck, label: '布置作业', short: '作业' },
  { to: '/history', icon: History, label: '发送记录', short: '记录' },
  { to: '/me', icon: UserRound, label: '个人中心', short: '我的' },
];

function HelpDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const steps = [
    { title: '开通班级', text: '在「我的班级」使用运营方提供的激活码创建班级。' },
    { title: '连接大屏', text: '获取 6 位绑定码，在教室大屏端输入。绑定码 10 分钟内有效。' },
    { title: '发送提醒', text: '选择班级，输入内容，点击发送。在记录中查看真实回执。' },
  ];
  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent>
      <DialogHeader><DialogTitle>三步，把提醒送进教室</DialogTitle><DialogDescription>教师端和大屏端连接到同一个服务，即可开始使用。</DialogDescription></DialogHeader>
      <ItemGroup className="gap-2">
        {steps.map((step, index) => <Item key={step.title} variant="outline">
          <ItemMedia className="flex size-8 items-center justify-center rounded-md bg-primary/10 text-sm font-semibold text-primary">{index + 1}</ItemMedia>
          <ItemContent><ItemTitle>{step.title}</ItemTitle><ItemDescription>{step.text}</ItemDescription></ItemContent>
        </Item>)}
      </ItemGroup>
      <p className="flex items-center gap-2 text-sm text-muted-foreground"><Radio className="size-4" />教室离线时，通知将等待连接后补发；超时后自动过期。</p>
    </DialogContent>
  </Dialog>;
}

function Layout() {
  const { user, logout } = useAuth();
  const online = useTeacherSocket();
  const location = useLocation();
  const [help, setHelp] = useState(false);
  const active = links.find(link => location.pathname.startsWith(link.to)) ?? links[0];
  return <SidebarProvider>
    <Sidebar variant="inset" collapsible="icon" className="sidebar">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" render={<NavLink to="/send" />} className="sidebar-brand"><Brand subtitle="教师工作台" /></SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>工作台</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu className="side-nav">
              {links.map(({ to, icon: Icon, label }) => <SidebarMenuItem key={to}>
                <SidebarMenuButton isActive={location.pathname.startsWith(to)} tooltip={label} render={<NavLink to={to} />}><Icon /><span>{label}</span></SidebarMenuButton>
              </SidebarMenuItem>)}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
        <SidebarGroup className="mt-auto">
          <SidebarMenu>
            <SidebarMenuItem><SidebarMenuButton tooltip="帮助与使用指南" onClick={() => setHelp(true)}><CircleHelp /><span>帮助与使用指南</span></SidebarMenuButton></SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter>
        <div className="side-profile flex items-center gap-2 rounded-lg p-1.5 group-data-[collapsible=icon]:p-0">
          <Avatar className="size-8 rounded-lg"><AvatarFallback className="rounded-lg">{initials(user?.display_name)}</AvatarFallback></Avatar>
          <div className="grid min-w-0 flex-1 text-sm leading-tight group-data-[collapsible=icon]:hidden">
            <span className="truncate font-medium">{user?.display_name}</span>
            <span className="truncate text-xs text-muted-foreground">教师 · v{__APP_VERSION__}</span>
          </div>
          <Button variant="ghost" size="icon-sm" onClick={() => void logout()} aria-label="退出登录" title="退出登录" className="group-data-[collapsible=icon]:hidden"><LogOut /></Button>
        </div>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
    <SidebarInset className="workspace min-w-0 pb-[calc(4rem+env(safe-area-inset-bottom))] md:pb-0">
      <header className="sticky top-0 z-10 flex h-14 shrink-0 items-center gap-2 border-b bg-background/95 px-4 backdrop-blur md:rounded-t-xl">
        <SidebarTrigger className="-ml-1 hidden md:inline-flex" />
        <Separator orientation="vertical" className="mr-2 hidden data-vertical:h-4 data-vertical:self-auto md:block" />
        <Brand className="w-auto md:hidden [&>span:last-child]:hidden" />
        <Breadcrumb className="min-w-0">
          <BreadcrumbList className="flex-nowrap">
            <BreadcrumbItem className="hidden sm:inline-flex">工作台</BreadcrumbItem>
            <BreadcrumbSeparator className="hidden sm:inline-flex" />
            <BreadcrumbItem><BreadcrumbPage>{active.label}</BreadcrumbPage></BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
        <div className="ml-auto flex items-center gap-2">
          <Badge variant="outline" className="gap-1.5 whitespace-nowrap">
            <span className={cn('size-1.5 rounded-full', online ? 'bg-emerald-500' : 'animate-pulse bg-amber-500')} />
            {online ? '实时连接已开启' : '正在连接实时通道'}
          </Badge>
          <span className="hidden text-sm text-muted-foreground lg:inline">{new Intl.DateTimeFormat('zh-CN', { month: 'long', day: 'numeric', weekday: 'long' }).format(new Date())}</span>
        </div>
      </header>
      <main className="main-content mx-auto flex w-full max-w-6xl flex-1 flex-col gap-6 p-4 md:p-6"><Outlet /></main>
      <SiteFooter className="workspace-footer" />
    </SidebarInset>
    <nav className="bottom-nav fixed inset-x-0 bottom-0 z-20 grid grid-cols-5 border-t bg-background/95 pb-[env(safe-area-inset-bottom)] backdrop-blur md:hidden" aria-label="教师工作台导航">
      {links.map(({ to, icon: Icon, short }) => <NavLink key={to} to={to} className={({ isActive }) => cn('flex h-16 flex-col items-center justify-center gap-1 text-xs text-muted-foreground', isActive && 'font-medium text-primary')}><Icon className="size-5" /><span>{short}</span></NavLink>)}
    </nav>
    <HelpDialog open={help} onOpenChange={setHelp} />
  </SidebarProvider>;
}

function Protected() {
  const { user, ready } = useAuth();
  if (!ready) return <div className="flex min-h-svh flex-col items-center justify-center gap-4 text-sm text-muted-foreground"><Brand className="w-auto" /><span className="flex items-center gap-2"><Spinner />正在连接你的课堂…</span></div>;
  return user ? <Layout /> : <Navigate to="/login" replace />;
}

export default function App() {
  return <Routes>
    <Route path="/login" element={<LoginPage />} />
    <Route path="/register" element={<LoginPage register />} />
    <Route element={<Protected />}>
      <Route index element={<Navigate to="/send" replace />} />
      <Route path="/send" element={<SendPage />} />
      <Route path="/classes" element={<ClassesPage />} />
      <Route path="/homework" element={<HomeworkPage />} />
      <Route path="/history" element={<HistoryPage />} />
      <Route path="/me" element={<MePage />} />
    </Route>
    <Route path="*" element={<Navigate to="/send" replace />} />
  </Routes>;
}
