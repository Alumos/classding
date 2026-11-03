import { createContext, lazy, Suspense, useContext, useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { NavLink, Navigate, Route, Routes, useLocation } from 'react-router';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ApiError, onSessionExpired, refreshSession, setAccessToken, type components } from '@classding/api';
import { SiteFooter, Badge, Brand, Button, Card, CopyButton, Dialog, Input } from '@classding/ui';
import { toast } from 'sonner';
import {
  Activity, ArrowDownToLine, ArrowRight, BellRing, BookOpen, Check, ChevronDown,
  ClipboardList, Copy, KeyRound, LayoutDashboard, LoaderCircle, LogOut, Monitor, RefreshCw,
  Search, Settings, ShieldCheck, Upload, Users, XCircle, type LucideIcon,
} from 'lucide-react';
import { binaryRequest } from './binaryRequest';
import { saveDeviceConfigurations } from './deviceConfig';

const ActivityChart = lazy(() => import('./ActivityChart'));

type User = components['schemas']['User'] & { status?: string };
type Classroom = components['schemas']['Class'];
type Device = components['schemas']['Device'] & { last_ip?: string; remote_config?: { server_url: string; revision: string | number } | null };
type Message = components['schemas']['Message'];
type ActivationCode = components['schemas']['ActivationCode'];
type AuthResponse = { access_token: string; user: User };
type Overview = {
  online_devices: number; total_devices: number; today_messages: number; delivery_rate: number;
  avg_delivery_ms: number; total_users: number; total_classes: number;
  recent_messages: Message[]; activity: { date: string; count: number }[];
};
type AuditLog = { id: string; actor: string; action: string; target: string; at: string };
type MessagePage = { items: Message[]; next_cursor: string | null };
type SiteSettings = { site_name: string; icp_number: string; theme: 'green' | 'blue' | 'slate' };

const json = (method: string, body?: unknown): RequestInit => ({
  method,
  headers: { 'Content-Type': 'application/json' },
  ...(body === undefined ? {} : { body: JSON.stringify(body) }),
});
const errorMessage = (error: unknown) => error instanceof Error ? error.message : '无法连接服务，请检查网络或稍后重试。';
const dateTime = (value?: string | null) => {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false });
};
const dateOnly = (value?: string | null) => {
  if (!value) return '长期有效';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString('zh-CN');
};
const localDate = (value?: string | null) => {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return [date.getFullYear(), String(date.getMonth() + 1).padStart(2, '0'), String(date.getDate()).padStart(2, '0')].join('-');
};
const number = (value?: number | null) => typeof value === 'number' && Number.isFinite(value) ? value.toLocaleString('zh-CN') : '—';
const rate = (value?: number | null) => typeof value === 'number' && Number.isFinite(value) ? `${(value * 100).toFixed(1)}%` : '—';
const codeStatus: Record<string, { text: string; variant: 'success' | 'warning' | 'secondary' | 'destructive' }> = {
  active: { text: '已激活', variant: 'success' }, unused: { text: '未使用', variant: 'warning' },
  revoked: { text: '已吊销', variant: 'destructive' }, released: { text: '已释放', variant: 'secondary' },
};
const targetStatus: Record<string, string> = { queued: '待送达', delivered: '已送达', played: '已播放', dismissed: '已关闭', expired: '已过期', failed: '发送失败', playing: '播放中' };
const roleName: Record<string, string> = { super_admin: '运营管理员', school_admin: '学校管理员', user: '教师' };

function StatusBadge({ status }: { status?: string | null }) {
  const entry = status ? codeStatus[status] : undefined;
  return <Badge variant={entry?.variant ?? 'secondary'}><span className="badge-dot" />{entry?.text ?? status ?? '未知'}</Badge>;
}

function ResultState({ loading, error, empty, children, emptyText = '这里暂时没有数据' }: { loading: boolean; error: unknown; empty: boolean; children: ReactNode; emptyText?: string }) {
  if (loading) return <div className="state"><LoaderCircle className="spin" size={22}/><span>正在加载数据…</span></div>;
  if (error) return <div className="state state-error" role="alert"><XCircle size={22}/><strong>数据加载失败</strong><span>{errorMessage(error)}</span></div>;
  if (empty) return <div className="state"><span className="empty-icon"><BookOpen size={22}/></span><strong>{emptyText}</strong><span>有新的数据时会显示在这里</span></div>;
  return <>{children}</>;
}

const SessionContext = createContext<{ user: User; logout: () => void } | null>(null);
function useSession() {
  const session = useContext(SessionContext);
  if (!session) throw new Error('Missing session');
  return session;
}

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
    if (!username.trim() || !password) { setError('请输入账号和密码。'); return; }
    login.mutate();
  }
  return <div className="login-page">
    <div className="login-brand"><Brand/><span className="login-brand-divider"/>管理后台</div>
    <div className="login-layout">
      <section className="login-intro">
        <div className="eyebrow light"><span/> CLASSDING FOR SCHOOLS</div>
        <h1>让每一声叮咚，<br/><em>都安心抵达。</em></h1>
        <p>为每一间教室、每一位老师，<br/>把日常管理照顾得更简单一些。</p>
        <div className="login-art" aria-hidden="true"><div className="art-orbit orbit-one"/><div className="art-orbit orbit-two"/><div className="art-bell"><BellRing size={55} strokeWidth={1.3}/></div><span className="art-spark spark-one">✳</span><span className="art-spark spark-two">✦</span></div>
        <div className="intro-bottom"><span>01 / 管理工作台</span><span>MADE FOR EVERY CLASSROOM</span></div>
      </section>
      <section className="login-form-wrap">
        <div className="login-card">
          <div className="eyebrow"><span/> WELCOME BACK</div>
          <h2>欢迎回来</h2>
          <p className="muted">登录管理后台，继续今天的工作。</p>
          <form onSubmit={submit} className="form-stack">
            <label>管理员账号<Input autoFocus autoComplete="username" placeholder="请输入账号" value={username} onChange={e => setUsername(e.target.value)} /></label>
            <label>密码<Input type="password" autoComplete="current-password" placeholder="请输入密码" value={password} onChange={e => setPassword(e.target.value)} /></label>
            {error && <p className="form-error" role="alert">{error}</p>}
            <Button type="submit" className="login-submit" disabled={login.isPending}>{login.isPending ? '正在登录…' : <>登录管理后台 <ArrowRight size={17}/></>}</Button>
          </form>
          <div className="login-foot"><ShieldCheck size={16}/> 仅限授权管理员访问</div>
        </div>
      </section>
    </div>
    <SiteFooter className="login-copyright"/>
  </div>;
}

const navItems: { to: string; label: string; icon: LucideIcon; superOnly?: boolean }[] = [
  { to: '/', label: '工作概览', icon: LayoutDashboard },
  { to: '/activation-codes', label: '激活码管理', icon: KeyRound, superOnly: true },
  { to: '/users', label: '教师管理', icon: Users },
  { to: '/classes', label: '班级管理', icon: BookOpen },
  { to: '/devices', label: '设备管理', icon: Monitor },
  { to: '/messages', label: '消息审计', icon: BellRing },
  { to: '/audit-logs', label: '操作日志', icon: ClipboardList, superOnly: true },
  { to: '/settings', label: '系统配置', icon: Settings, superOnly: true },
];
function Shell() {
  const { user, logout } = useSession();
  const queryClient = useQueryClient();
  const location = useLocation();
  const current = navItems.find(item => item.to === location.pathname)?.label ?? '管理后台';
  const visible = navItems.filter(item => !item.superOnly || user.role === 'super_admin');
  const mobileNav = useRef<HTMLElement>(null);
  useEffect(() => {
    const revealCurrent = () => {
      const nav = mobileNav.current;
      const active = nav?.querySelector<HTMLElement>('[aria-current="page"]');
      if (!nav || !active || !nav.clientWidth) return;
      const container = nav.getBoundingClientRect();
      const item = active.getBoundingClientRect();
      if (item.left < container.left) nav.scrollLeft += item.left - container.left - 12;
      else if (item.right > container.right) nav.scrollLeft += item.right - container.right + 12;
    };
    revealCurrent();
    window.addEventListener('resize', revealCurrent);
    return () => window.removeEventListener('resize', revealCurrent);
  }, [location.pathname]);
  return <div className="app-layout">
    <aside className="sidebar">
      <div className="sidebar-brand"><Brand/><span>管理后台</span></div>
      <div className="sidebar-caption">WORKSPACE <span>工作空间</span></div>
      <nav className="side-nav" aria-label="管理后台导航">{visible.map(item => <NavLink key={item.to} to={item.to} end={item.to === '/'} className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}><item.icon size={19} strokeWidth={1.8}/><span>{item.label}</span>{item.to === '/' && <span className="nav-arrow">↗</span>}</NavLink>)}</nav>
      <div className="sidebar-spacer"/>
      <div className="sidebar-note"><span className="note-icon"><ShieldCheck size={17}/></span><div><strong>安全管理空间</strong><small>每一次操作都有迹可循</small></div></div>
      <div className="sidebar-user"><span className="avatar">{(user.display_name || user.username || '管').slice(0, 1)}</span><div><strong>{user.display_name || user.username}</strong><small>{roleName[user.role] ?? user.role}</small></div><button onClick={logout} className="icon-button" title="退出登录" aria-label="退出登录"><LogOut size={17}/></button></div>
    </aside>
    <div className="main-column">
      <header className="topbar"><div className="breadcrumbs"><span>管理后台</span><span>/</span><strong>{current}</strong></div><div className="top-actions"><span className="top-date">{new Date().toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric', weekday: 'long' })}</span><button className="top-icon" aria-label="刷新数据" title="刷新数据" onClick={() => void queryClient.invalidateQueries()}><RefreshCw size={18}/></button><span className="top-separator"/><span className="avatar small">{(user.display_name || user.username || '管').slice(0, 1)}</span><span className="top-user">{user.display_name || user.username}<small>{roleName[user.role] ?? user.role}</small></span><ChevronDown size={15} className="top-chevron"/><button className="top-icon mobile-logout" onClick={logout} title="退出登录" aria-label="退出登录"><LogOut size={17}/></button></div></header>
      <nav ref={mobileNav} className="mobile-nav" aria-label="管理后台导航">{visible.map(item => <NavLink key={item.to} to={item.to} end={item.to === '/'} className={({ isActive }) => isActive ? 'active' : ''}><item.icon size={16}/>{item.label}</NavLink>)}</nav>
      <main className="main-content"><Routes>
        <Route path="/" element={<OverviewPage/>}/>
        <Route path="/activation-codes" element={<SuperOnly><CodesPage/></SuperOnly>}/>
        <Route path="/users" element={<UsersPage/>}/>
        <Route path="/classes" element={<ClassesPage/>}/>
        <Route path="/devices" element={<DevicesPage/>}/>
        <Route path="/messages" element={<MessagesPage/>}/>
        <Route path="/audit-logs" element={<SuperOnly><AuditPage/></SuperOnly>}/>
        <Route path="/settings" element={<SuperOnly><SettingsPage/></SuperOnly>}/>
        <Route path="*" element={<Navigate to="/" replace/>}/>
      </Routes></main>
      <SiteFooter className="app-footer"/>
    </div>
  </div>;
}
function SuperOnly({ children }: { children: ReactNode }) {
  const { user } = useSession();
  if (user.role !== 'super_admin') return <div className="permission-state"><ShieldCheck size={32}/><h2>没有访问权限</h2><p>此页面仅对运营管理员开放。</p><NavLink to="/">返回工作概览</NavLink></div>;
  return <>{children}</>;
}
function PageHead({ eyebrow, title, description, action }: { eyebrow: string; title: string; description: string; action?: ReactNode }) {
  return <div className="page-head"><div><div className="eyebrow"><span/>{eyebrow}</div><h1>{title}</h1><p>{description}</p></div>{action && <div className="head-action">{action}</div>}</div>;
}
function Panel({ title, sub, action, children, className = '' }: { title: string; sub?: string; action?: ReactNode; children: ReactNode; className?: string }) {
  return <Card className={`panel ${className}`}><div className="panel-head"><div><h2>{title}</h2>{sub && <p>{sub}</p>}</div>{action}</div>{children}</Card>;
}
function SearchBox({ value, onChange, placeholder }: { value: string; onChange: (value: string) => void; placeholder: string }) {
  return <div className="search-box"><Search size={17}/><Input aria-label={placeholder} placeholder={placeholder} value={value} onChange={e => onChange(e.target.value)}/></div>;
}
function TableScroller({ children }: { children: ReactNode }) {
  const scroll = useRef<HTMLDivElement>(null);
  const [overflowing, setOverflowing] = useState(false);
  useEffect(() => {
    const element = scroll.current;
    if (!element) return;
    const update = () => setOverflowing(element.scrollWidth > element.clientWidth + 1);
    update();
    const observer = new ResizeObserver(update);
    observer.observe(element);
    if (element.firstElementChild) observer.observe(element.firstElementChild);
    return () => observer.disconnect();
  }, [children]);
  return <div><div ref={scroll} className="table-scroll" role="region" aria-label="数据表格" tabIndex={0}>{children}</div>{overflowing && <p className="table-scroll-hint">左右滑动查看更多 <ArrowRight size={14}/></p>}</div>;
}
function StatCard({ label, value, hint, icon: Icon, tone }: { label: string; value: string; hint: string; icon: LucideIcon; tone: string }) {
  return <Card className="stat-card"><div className="stat-top"><span>{label}</span><span className={`stat-icon ${tone}`}><Icon size={19}/></span></div><div className="stat-value">{value}</div><div className="stat-hint">{hint}</div></Card>;
}
function receiptTone(status?: string) {
  return status === 'failed' || status === 'expired' ? 'receipt-bad' : status === 'delivered' || status === 'played' || status === 'dismissed' ? 'receipt-ok' : '';
}
function messageSummary(message: Message) {
  const targets = message.targets ?? [];
  if (!targets.length) return '暂无回执';
  return targets.map(t => targetStatus[t.status] ?? t.status).filter((v, i, a) => a.indexOf(v) === i).join('、');
}
function OverviewPage() {
  const { user } = useSession();
  const query = useQuery({ queryKey: ['admin', 'overview'], queryFn: () => api<Overview>('/admin/overview') });
  const data = query.data;
  return <>
    <PageHead eyebrow="OVERVIEW" title="工作概览" description="从这里，了解每一间教室的连接状态。" />
    <section className="welcome-banner"><div className="welcome-copy"><div className="welcome-kicker"><span/> CLASSDING DASHBOARD</div><h2>你好，{user.display_name || user.username} <span>✳</span></h2><p>今天也一起，让沟通及时抵达。</p><div className="welcome-chip"><span/> {user.role === 'super_admin' ? '全局管理视图' : '学校管理视图'}</div></div><div className="welcome-decoration" aria-hidden="true"><div className="welcome-ring ring-one"/><div className="welcome-ring ring-two"/><span className="welcome-bell"><BellRing size={52} strokeWidth={1.2}/></span><span className="welcome-spark">✦</span></div></section>
    <div className="section-line"><div><h2>数据概览</h2><p>实时掌握站点的运行情况</p></div><span>当前统计 <i/></span></div>
    {query.isError ? <Card className="panel"><ResultState loading={false} error={query.error} empty={false}>{null}</ResultState></Card> : <>
      <div className="stats-grid">
        <StatCard label="在线设备" value={data ? number(data.online_devices) : '—'} hint={data ? `共 ${number(data.total_devices)} 台设备` : '正在获取设备状态'} icon={Monitor} tone="green"/>
        <StatCard label="今日发送" value={data ? number(data.today_messages) : '—'} hint="今天发出的通知" icon={BellRing} tone="peach"/>
        <StatCard label="通知送达率" value={data ? rate(data.delivery_rate) : '—'} hint="按服务端统计口径" icon={Activity} tone="blue"/>
        <StatCard label="平均送达耗时" value={data && Number.isFinite(data.avg_delivery_ms) ? `${number(data.avg_delivery_ms)} ms` : '—'} hint="从发送到送达" icon={Check} tone="yellow"/>
      </div>
      <div className="dashboard-grid">
        <Panel title="通知发送趋势" sub="了解近期教室通知的活跃情况" action={<span className="panel-tag"><i/> 发送数量</span>} className="chart-panel">
          <ResultState loading={query.isLoading} error={null} empty={!data?.activity?.length} emptyText="暂无趋势数据"><Suspense fallback={<div className="state">正在加载图表…</div>}><ActivityChart activity={data?.activity ?? []}/></Suspense></ResultState>
        </Panel>
        <Panel title="连接中的课堂" sub="每个数字，都是一次安心连接" className="connection-panel"><ResultState loading={query.isLoading} error={null} empty={!data}><div className="connection-illustration"><div className="connection-circle"><Monitor size={34} strokeWidth={1.4}/><span><Check size={13}/></span></div><i className="connection-spark one">✳</i><i className="connection-spark two">✦</i></div><div className="connection-value">{number(data?.online_devices)} <small>/ {number(data?.total_devices)}</small></div><p className="connection-caption">台教室设备在线</p><div className="connection-divider"/><div className="connection-meta"><div><strong>{number(data?.total_users)}</strong><span>教师账号</span></div><div><strong>{number(data?.total_classes)}</strong><span>班级总数</span></div></div></ResultState></Panel>
      </div>
      <Panel title="最近通知" sub="查看最新的课堂消息与送达情况" action={<NavLink className="text-link" to="/messages">查看全部 <ArrowRight size={15}/></NavLink>}>
        <ResultState loading={query.isLoading} error={null} empty={!data?.recent_messages?.length} emptyText="还没有通知记录"><MessageTable messages={data?.recent_messages ?? []} compact/></ResultState>
      </Panel>
    </>}
  </>;
}

function downloadCodes(codes: ActivationCode[], filename = 'classding-activation-codes.csv') {
  const fields = ['激活码', '状态', '教师上限', '设备上限', '有效期', '班级 ID', '学校 ID', '批次 ID', '备注', '核销时间'];
  const quote = (value: unknown) => {
    let s = String(value ?? '');
    if (/^[\s]*[=+\-@\t\r]/.test(s)) s = `'${s}`;
    return `"${s.replace(/"/g, '""')}"`;
  };
  const rows = codes.map(code => [code.code, codeStatus[code.status]?.text ?? code.status, code.max_teachers, code.max_devices, code.expires_at ?? '', code.class_id ?? '', code.org_id ?? '', code.batch_id ?? '', code.note ?? '', code.redeemed_at ?? '']);
  const csv = '\ufeff' + [fields, ...rows].map(row => row.map(quote).join(',')).join('\r\n');
  const url = URL.createObjectURL(new Blob([csv], { type: 'text/csv;charset=utf-8' }));
  const anchor = document.createElement('a'); anchor.href = url; anchor.download = filename; document.body.appendChild(anchor); anchor.click(); anchor.remove(); window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}
const defaultCodeForm = { count: '10', max_teachers: '3', max_devices: '1', expires_at: '', org_id: '', note: '' };
function CodesPage() {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState('');
  const [form, setForm] = useState(defaultCodeForm);
  const [formError, setFormError] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [created, setCreated] = useState<ActivationCode[]>([]);
  const [editing, setEditing] = useState<ActivationCode | null>(null);
  const [revoking, setRevoking] = useState<ActivationCode | null>(null);
  const [editForm, setEditForm] = useState({ max_teachers: '', max_devices: '', expires_at: '' });
  const params = new URLSearchParams();
  if (search.trim()) params.set('search', search.trim());
  if (status) params.set('status', status);
  const query = useQuery({ queryKey: ['admin', 'codes', search.trim(), status], queryFn: () => api<ActivationCode[]>(`/admin/activation-codes${params.size ? `?${params}` : ''}`) });
  const create = useMutation({
    mutationFn: (body: unknown) => api<ActivationCode[]>('/admin/activation-codes', json('POST', body)),
    onSuccess: result => { setCreated(result); setCreateOpen(false); setForm(defaultCodeForm); setFormError(''); void queryClient.invalidateQueries({ queryKey: ['admin', 'codes'] }); toast.success(`已生成 ${result.length} 个激活码`); },
    onError: error => setFormError(errorMessage(error)),
  });
  const adjust = useMutation({
    mutationFn: ({ code, body }: { code: string; body: unknown }) => api<ActivationCode>(`/admin/activation-codes/${encodeURIComponent(code)}`, json('PATCH', body)),
    onSuccess: () => { setEditing(null); setRevoking(null); setFormError(''); void queryClient.invalidateQueries({ queryKey: ['admin', 'codes'] }); toast.success('激活码已更新'); },
    onError: error => setFormError(errorMessage(error)),
  });
  function submitCreate(event: FormEvent) {
    event.preventDefault(); setFormError('');
    const count = Number(form.count), teachers = Number(form.max_teachers), devices = Number(form.max_devices);
    if (!Number.isInteger(count) || count < 1 || count > 1000) { setFormError('生成数量须为 1 至 1000 的整数。'); return; }
    if (!Number.isInteger(teachers) || teachers < 1 || !Number.isInteger(devices) || devices < 1) { setFormError('教师和设备上限须为大于 0 的整数。'); return; }
    if (form.expires_at && Number.isNaN(new Date(`${form.expires_at}T23:59:59`).getTime())) { setFormError('请选择有效的到期日期。'); return; }
    create.mutate({ count, max_teachers: teachers, max_devices: devices, ...(form.expires_at ? { expires_at: new Date(`${form.expires_at}T23:59:59`).toISOString() } : {}), ...(form.org_id.trim() ? { org_id: form.org_id.trim() } : {}), ...(form.note.trim() ? { note: form.note.trim() } : {}) });
  }
  function startEdit(code: ActivationCode) {
    setFormError(''); setEditing(code);
    setEditForm({ max_teachers: String(code.max_teachers), max_devices: String(code.max_devices), expires_at: localDate(code.expires_at) });
  }
  function submitEdit(event: FormEvent) {
    event.preventDefault(); setFormError('');
    if (!editing) return;
    const teachers = Number(editForm.max_teachers), devices = Number(editForm.max_devices);
    if (!Number.isInteger(teachers) || teachers < 1 || !Number.isInteger(devices) || devices < 1) { setFormError('教师和设备上限须为大于 0 的整数。'); return; }
    adjust.mutate({ code: editing.code, body: { max_teachers: teachers, max_devices: devices, expires_at: editForm.expires_at ? new Date(`${editForm.expires_at}T23:59:59`).toISOString() : null } });
  }
  return <>
    <PageHead eyebrow="ACTIVATION CODES" title="激活码管理" description="生成和管理授权，让每一间教室顺利开通。" action={<Button onClick={() => { setFormError(''); setCreateOpen(true); }}><span className="plus">＋</span> 生成激活码</Button>}/>
    {created.length > 0 && <div className="created-banner"><div><span className="created-check"><Check size={17}/></span><div><strong>成功生成 {created.length} 个激活码</strong><p>请及时复制或导出本次生成的激活码。</p></div></div><div className="banner-actions"><CopyButton text={created.map(c => c.code).join('\n')} onCopied={() => toast.success('已复制到剪贴板')} variant="outline" size="sm"><Copy size={15}/> 复制全部</CopyButton><Button variant="outline" size="sm" onClick={() => downloadCodes(created, 'classding-new-codes.csv')}><ArrowDownToLine size={15}/> 导出本批</Button><button className="icon-button" aria-label="关闭提示" onClick={() => setCreated([])}>×</button></div></div>}
    <Panel title="全部激活码" sub="查看授权状态，按需调整使用额度" action={<Button variant="outline" size="sm" disabled={!query.data?.length} onClick={() => downloadCodes(query.data ?? [])}><ArrowDownToLine size={15}/> 导出 CSV</Button>}>
      <div className="filter-row"><SearchBox value={search} onChange={setSearch} placeholder="搜索激活码、备注或批次"/><select aria-label="筛选激活码状态" value={status} onChange={e => setStatus(e.target.value)}><option value="">全部状态</option><option value="unused">未使用</option><option value="active">已激活</option><option value="released">已释放</option><option value="revoked">已吊销</option></select><span className="filter-count">{query.data ? `共 ${query.data.length} 条` : ''}</span></div>
      <ResultState loading={query.isLoading} error={query.error} empty={!query.data?.length} emptyText="没有找到激活码"><TableScroller><table><thead><tr><th>激活码</th><th>状态</th><th>教师 / 设备上限</th><th>有效期</th><th>关联班级</th><th>备注</th><th>操作</th></tr></thead><tbody>{query.data?.map(code => <tr key={code.code}><td><div className="code-cell"><strong>{code.code}</strong><CopyButton className="inline-icon" text={code.code} onCopied={() => toast.success('已复制到剪贴板')} variant="ghost" size="icon" aria-label={`复制激活码 ${code.code}`} title="复制"><Copy size={14}/></CopyButton></div><small>{code.batch_id || '—'}</small></td><td><StatusBadge status={code.status}/></td><td>{code.max_teachers} 位 / {code.max_devices} 台</td><td>{dateOnly(code.expires_at)}</td><td>{code.class_id || '—'}</td><td className="truncate-cell" title={code.note ?? ''}>{code.note || '—'}</td><td><div className="row-actions"><button onClick={() => startEdit(code)}>调整</button>{code.status !== 'revoked' && <button className="danger-link" onClick={() => { setFormError(''); setRevoking(code); }}>吊销</button>}</div></td></tr>)}</tbody></table></TableScroller></ResultState>
    </Panel>
    <Dialog open={createOpen} onClose={() => setCreateOpen(false)} title="生成激活码" description="设置授权额度，生成后可复制或导出。" footer={<><Button variant="outline" onClick={() => setCreateOpen(false)}>取消</Button><Button type="submit" form="create-codes" disabled={create.isPending}>{create.isPending ? '正在生成…' : '生成激活码'}</Button></>}>
      <form id="create-codes" onSubmit={submitCreate} className="dialog-form"><label>生成数量<Input type="number" min="1" max="1000" value={form.count} onChange={e => setForm({ ...form, count: e.target.value })}/><small>单次可生成 1–1000 个</small></label><div className="form-grid"><label>教师上限<Input type="number" min="1" value={form.max_teachers} onChange={e => setForm({ ...form, max_teachers: e.target.value })}/></label><label>设备上限<Input type="number" min="1" value={form.max_devices} onChange={e => setForm({ ...form, max_devices: e.target.value })}/></label></div><label>有效期 <small>留空为长期有效</small><Input type="date" value={form.expires_at} onChange={e => setForm({ ...form, expires_at: e.target.value })}/></label><label>所属学校 ID <small>选填</small><Input placeholder="输入学校 ID" value={form.org_id} onChange={e => setForm({ ...form, org_id: e.target.value })}/></label><label>备注 <small>选填</small><Input maxLength={200} placeholder="例如：2026 年秋季采购" value={form.note} onChange={e => setForm({ ...form, note: e.target.value })}/></label>{formError && <p className="form-error" role="alert">{formError}</p>}</form>
    </Dialog>
    <Dialog open={!!editing} onClose={() => setEditing(null)} title="调整激活码" description={editing ? `调整 ${editing.code} 的授权设置。` : undefined} footer={<><Button variant="outline" onClick={() => setEditing(null)}>取消</Button><Button type="submit" form="edit-code" disabled={adjust.isPending}>{adjust.isPending ? '正在保存…' : '保存调整'}</Button></>}>
      <form id="edit-code" onSubmit={submitEdit} className="dialog-form"><div className="form-grid"><label>教师上限<Input type="number" min="1" value={editForm.max_teachers} onChange={e => setEditForm({ ...editForm, max_teachers: e.target.value })}/></label><label>设备上限<Input type="number" min="1" value={editForm.max_devices} onChange={e => setEditForm({ ...editForm, max_devices: e.target.value })}/></label></div><label>有效期 <small>留空为长期有效</small><Input type="date" value={editForm.expires_at} onChange={e => setEditForm({ ...editForm, expires_at: e.target.value })}/></label>{formError && <p className="form-error" role="alert">{formError}</p>}</form>
    </Dialog>
    <Dialog open={!!revoking} onClose={() => setRevoking(null)} title="确认吊销激活码？" description={`吊销后，关联的教室将无法接收新通知。激活码：${revoking?.code ?? ''}`} footer={<><Button variant="outline" onClick={() => setRevoking(null)}>取消</Button><Button variant="destructive" disabled={adjust.isPending} onClick={() => revoking && adjust.mutate({ code: revoking.code, body: { status: 'revoked' } })}>{adjust.isPending ? '正在吊销…' : '确认吊销'}</Button></>}>
      {formError && <p className="form-error" role="alert">{formError}</p>}
    </Dialog>
  </>;
}

type TeacherForm = { username: string; display_name: string; password: string; org_id: string; status: 'active' | 'disabled' };
const emptyTeacherForm: TeacherForm = { username: '', display_name: '', password: '', org_id: '', status: 'active' };

function UsersPage() {
  const queryClient = useQueryClient();
  const { user: currentUser } = useSession();
  const [search, setSearch] = useState('');
  const [form, setForm] = useState<TeacherForm>(emptyTeacherForm);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<User | null>(null);
  const [deleting, setDeleting] = useState<User | null>(null);
  const [formError, setFormError] = useState('');
  const query = useQuery({ queryKey: ['admin', 'users'], queryFn: () => api<User[]>('/admin/users') });
  const users = (query.data ?? []).filter(user => `${user.display_name} ${user.username} ${user.org_id ?? ''}`.toLowerCase().includes(search.trim().toLowerCase()));
  const save = useMutation({
    mutationFn: ({ user, body }: { user: User | null; body: unknown }) => user
      ? api<User>(`/admin/users/${encodeURIComponent(user.id)}`, json('PATCH', body))
      : api<User>('/admin/users', json('POST', body)),
    onSuccess: () => {
      setFormOpen(false); setEditing(null); setFormError('');
      void queryClient.invalidateQueries({ queryKey: ['admin', 'users'] });
      toast.success(editing ? '教师账号已更新' : '教师账号已创建');
    },
    onError: error => setFormError(errorMessage(error)),
  });
  const remove = useMutation({
    mutationFn: (user: User) => api<void>(`/admin/users/${encodeURIComponent(user.id)}`, json('DELETE')),
    onSuccess: () => {
      setDeleting(null); setFormError('');
      void queryClient.invalidateQueries({ queryKey: ['admin', 'users'] });
      toast.success('教师账号已删除');
    },
    onError: error => setFormError(error instanceof ApiError && error.status === 409 ? '该教师仍负责班级，请先转移或删除其班级。' : errorMessage(error)),
  });
  const unlockLogin = useMutation({
    mutationFn: (user: User) => api(`/admin/users/${encodeURIComponent(user.id)}/unlock-login`, json('POST')),
    onSuccess: () => { void queryClient.invalidateQueries({ queryKey: ['admin', 'users'] }); toast.success('已解除登录限制，教师可以重新登录'); },
    onError: error => toast.error(errorMessage(error)),
  });
  function startCreate() {
    setEditing(null); setForm(emptyTeacherForm); setFormError(''); setFormOpen(true);
  }
  function startEdit(user: User) {
    setEditing(user); setForm({ username: user.username, display_name: user.display_name, password: '', org_id: user.org_id ?? '', status: user.status === 'disabled' ? 'disabled' : 'active' }); setFormError(''); setFormOpen(true);
  }
  function submit(event: FormEvent) {
    event.preventDefault(); setFormError('');
    const username = form.username.trim();
    const displayName = form.display_name.trim();
    if (!/^[a-zA-Z0-9._-]{3,32}$/.test(username)) { setFormError('用户名须为 3–32 位，支持英文字母、数字、点、下划线和短横线。'); return; }
    if (!displayName || [...displayName].length > 24) { setFormError('教师姓名须为 1–24 字。'); return; }
    if (!editing || form.password) {
      const bytes = new TextEncoder().encode(form.password).length;
      if (bytes < 10 || bytes > 128 || !/\p{L}/u.test(form.password) || !/\p{Nd}/u.test(form.password)) { setFormError('密码须为 10–128 字节，包含字母和数字。'); return; }
    }
    const body = editing ? {
      ...(username !== editing.username ? { username } : {}),
      ...(displayName !== editing.display_name ? { display_name: displayName } : {}),
      ...(form.password ? { password: form.password } : {}),
      ...(form.status !== editing.status ? { status: form.status } : {}),
    } : {
      username, display_name: displayName, password: form.password,
      ...(currentUser.role === 'super_admin' && form.org_id.trim() ? { org_id: form.org_id.trim() } : {}),
    };
    save.mutate({ user: editing, body });
  }
  return <><PageHead eyebrow="TEACHERS" title="教师管理" description="创建和维护教师账号，管理账号使用状态。" action={<Button onClick={startCreate}><span className="plus">＋</span> 添加教师</Button>}/>
    <Panel title="教师账号" sub="管理当前权限范围内的教师" action={<span className="panel-count">{query.data ? `${query.data.length} 位账号` : '账号列表'}</span>}>
      <div className="filter-row"><SearchBox value={search} onChange={setSearch} placeholder="搜索姓名、账号或学校 ID"/><span className="filter-count">{query.data ? `共 ${users.length} 条` : ''}</span></div>
      <ResultState loading={query.isLoading} error={query.error} empty={!users.length} emptyText="没有找到教师账号"><TableScroller><table><thead><tr><th>教师</th><th>账号</th><th>角色</th><th>所属学校 ID</th><th>状态</th><th>操作</th></tr></thead><tbody>{users.map(user => <tr key={user.id}><td><div className="person-cell"><span className="person-avatar">{(user.display_name || user.username || '师').slice(0, 1)}</span><div><strong>{user.display_name || '—'}</strong><small>{user.id}</small></div></div></td><td>{user.username}</td><td>{roleName[user.role] ?? user.role}</td><td>{user.org_id || '—'}</td><td><Badge variant={user.status === 'active' ? 'success' : user.status === 'disabled' ? 'destructive' : 'secondary'}><span className="badge-dot"/>{user.status === 'active' ? '正常' : user.status === 'disabled' ? '已禁用' : '未知'}</Badge>{user.login_locked_until && <small className="form-error">登录限制至 {dateTime(user.login_locked_until)}</small>}</td><td><div className="row-actions">{user.role !== 'user' ? <span className="subtle">{user.id === currentUser.id ? '当前账号' : '管理员'}</span> : <><button onClick={() => startEdit(user)}>编辑</button><button disabled={unlockLogin.isPending} onClick={() => unlockLogin.mutate(user)}>解除登录限制</button><button className="danger-link" onClick={() => { setFormError(''); setDeleting(user); }}>删除</button></>}</div></td></tr>)}</tbody></table></TableScroller></ResultState>
    </Panel>
    <Dialog open={formOpen} onClose={() => setFormOpen(false)} title={editing ? '编辑教师账号' : '添加教师'} description={editing ? '修改账号资料，留空密码则保持原密码。' : '创建后，教师可使用账号和密码登录。'} footer={<><Button variant="outline" onClick={() => setFormOpen(false)}>取消</Button><Button type="submit" form="teacher-form" disabled={save.isPending}>{save.isPending ? '正在保存…' : editing ? '保存修改' : '创建账号'}</Button></>}>
      <form id="teacher-form" className="dialog-form" onSubmit={submit}>
        <label>教师姓名<Input required maxLength={48} placeholder="例如：林老师" value={form.display_name} onChange={e => setForm({ ...form, display_name: e.target.value })}/><small>1–24 字，作为通知落款</small></label>
        <label>用户名<Input required minLength={3} maxLength={32} autoComplete="off" placeholder="3–32 位字母、数字或 . _ -" value={form.username} onChange={e => setForm({ ...form, username: e.target.value })}/></label>
        <label>{editing ? '重设密码' : '登录密码'}{editing && <small>留空则不修改</small>}<Input type="password" required={!editing} autoComplete="new-password" placeholder="至少 10 位，包含字母和数字" value={form.password} onChange={e => setForm({ ...form, password: e.target.value })}/></label>
        {!editing && currentUser.role === 'super_admin' && <label>所属学校 ID <small>选填</small><Input placeholder="输入学校 ID" value={form.org_id} onChange={e => setForm({ ...form, org_id: e.target.value })}/></label>}
        {editing && <label>账号状态<select className="dialog-select" value={form.status} onChange={e => setForm({ ...form, status: e.target.value as TeacherForm['status'] })}><option value="active">正常</option><option value="disabled">已禁用</option></select></label>}
        {formError && <p className="form-error" role="alert">{formError}</p>}
      </form>
    </Dialog>
    <Dialog open={!!deleting} onClose={() => setDeleting(null)} title="删除教师账号？" description={`确认删除 ${deleting?.display_name || deleting?.username || '该教师'} 的账号。若教师仍负责班级，请先转移或删除班级。`} footer={<><Button variant="outline" onClick={() => setDeleting(null)}>取消</Button><Button variant="destructive" disabled={remove.isPending} onClick={() => deleting && remove.mutate(deleting)}>{remove.isPending ? '正在删除…' : '确认删除'}</Button></>}>
      {formError && <p className="form-error" role="alert">{formError}</p>}
    </Dialog>
  </>;
}

type ClassForm = { name: string; grade: string; owner_id: string; max_teachers: string; max_devices: string };
const emptyClassForm: ClassForm = { name: '', grade: '', owner_id: '', max_teachers: '3', max_devices: '1' };

function ClassesPage() {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState('');
  const [form, setForm] = useState<ClassForm>(emptyClassForm);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<Classroom | null>(null);
  const [deleting, setDeleting] = useState<Classroom | null>(null);
  const [formError, setFormError] = useState('');
  const query = useQuery({ queryKey: ['admin', 'classes'], queryFn: () => api<Classroom[]>('/admin/classes') });
  const users = useQuery({ queryKey: ['admin', 'users'], queryFn: () => api<User[]>('/admin/users') });
  const teachers = (users.data ?? []).filter(user => user.role === 'user' && user.status === 'active' && !!user.org_id && (!editing || user.org_id === editing.org_id));
  const ownerName = (id: string) => {
    const user = users.data?.find(item => item.id === id);
    return user ? `${user.display_name || user.username}（${user.username}）` : id || '—';
  };
  const classes = (query.data ?? []).filter(item => `${item.name} ${item.grade ?? ''} ${item.org_id ?? ''} ${ownerName(item.owner_id)}`.toLowerCase().includes(search.trim().toLowerCase()));
  const save = useMutation({
    mutationFn: ({ item, body }: { item: Classroom | null; body: unknown }) => item
      ? api<Classroom>(`/admin/classes/${encodeURIComponent(item.id)}`, json('PATCH', body))
      : api<Classroom>('/admin/classes', json('POST', body)),
    onSuccess: () => {
      setFormOpen(false); setEditing(null); setFormError('');
      void queryClient.invalidateQueries({ queryKey: ['admin', 'classes'] });
      toast.success(editing ? '班级已更新' : '班级已创建');
    },
    onError: error => setFormError(errorMessage(error)),
  });
  const remove = useMutation({
    mutationFn: (item: Classroom) => api<void>(`/admin/classes/${encodeURIComponent(item.id)}`, json('DELETE')),
    onSuccess: () => {
      setDeleting(null); setFormError('');
      void queryClient.invalidateQueries({ queryKey: ['admin', 'classes'] });
      toast.success('班级已删除');
    },
    onError: error => setFormError(errorMessage(error)),
  });
  function startCreate() {
    setEditing(null); setForm(emptyClassForm); setFormError(''); setFormOpen(true);
  }
  function startEdit(item: Classroom) {
    setEditing(item); setForm({ name: item.name, grade: item.grade ?? '', owner_id: item.owner_id, max_teachers: String(item.max_teachers), max_devices: String(item.max_devices) }); setFormError(''); setFormOpen(true);
  }
  function submit(event: FormEvent) {
    event.preventDefault(); setFormError('');
    const name = form.name.trim(), grade = form.grade.trim();
    const maxTeachers = Number(form.max_teachers), maxDevices = Number(form.max_devices);
    if (!name || [...name].length > 40) { setFormError('班级名称须为 1–40 字。'); return; }
    if ([...grade].length > 40) { setFormError('年级最多 40 字。'); return; }
    if (!teachers.some(user => user.id === form.owner_id)) { setFormError('请选择状态正常的教师作为班级负责人。'); return; }
    if (!Number.isInteger(maxTeachers) || maxTeachers < 1 || maxTeachers > 100 || !Number.isInteger(maxDevices) || maxDevices < 1 || maxDevices > 20) { setFormError('教师上限须为 1–100，设备上限须为 1–20，且均为整数。'); return; }
    save.mutate({ item: editing, body: { name, grade, owner_id: form.owner_id, max_teachers: maxTeachers, max_devices: maxDevices } });
  }
  return <><PageHead eyebrow="CLASSROOMS" title="班级管理" description="创建班级，管理负责人和教室连接额度。" action={<Button onClick={startCreate}><span className="plus">＋</span> 添加班级</Button>}/>
    <Panel title="班级列表" sub="每一间教室都在这里" action={<span className="panel-count">{query.data ? `${query.data.length} 个班级` : '班级列表'}</span>}>
      <div className="filter-row"><SearchBox value={search} onChange={setSearch} placeholder="搜索班级、年级、负责人或学校 ID"/><span className="filter-count">{query.data ? `共 ${classes.length} 条` : ''}</span></div>
      <ResultState loading={query.isLoading} error={query.error} empty={!classes.length} emptyText="没有找到班级"><TableScroller><table><thead><tr><th>班级名称</th><th>年级</th><th>负责人</th><th>所属学校 ID</th><th>教师</th><th>设备</th><th>创建时间</th><th>操作</th></tr></thead><tbody>{classes.map(item => <tr key={item.id}><td><div className="person-cell"><span className="class-avatar"><BookOpen size={17}/></span><div><strong>{item.name}</strong><small>{item.id}</small></div></div></td><td>{item.grade || '—'}</td><td>{ownerName(item.owner_id)}</td><td>{item.org_id || '—'}</td><td>{number(item.teachers_count)} / {number(item.max_teachers)} 位</td><td><span className="online-dot"/> {number(item.online_devices)} 在线 <span className="subtle">/ {number(item.devices_count)} / {number(item.max_devices)} 台</span></td><td>{dateTime(item.created_at)}</td><td><div className="row-actions"><button onClick={() => startEdit(item)}>编辑</button><button className="danger-link" onClick={() => { setFormError(''); setDeleting(item); }}>删除</button></div></td></tr>)}</tbody></table></TableScroller></ResultState>
    </Panel>
    <Dialog open={formOpen} onClose={() => setFormOpen(false)} title={editing ? '编辑班级' : '添加班级'} description={editing ? '调整班级信息和连接额度。' : '创建后即可使用，无需激活码。'} footer={<><Button variant="outline" onClick={() => setFormOpen(false)}>取消</Button><Button type="submit" form="class-form" disabled={save.isPending || users.isLoading || !!users.error || !teachers.length}>{save.isPending ? '正在保存…' : editing ? '保存修改' : '创建班级'}</Button></>}>
      <form id="class-form" className="dialog-form" onSubmit={submit}>
        <label>班级名称<Input required maxLength={80} placeholder="例如：三年级（2）班" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })}/></label>
        <label>年级 <small>选填</small><Input maxLength={80} placeholder="例如：三年级" value={form.grade} onChange={e => setForm({ ...form, grade: e.target.value })}/></label>
        <label>负责人<select className="dialog-select" required value={form.owner_id} onChange={e => setForm({ ...form, owner_id: e.target.value })}><option value="">{users.isLoading ? '正在加载教师…' : '请选择教师'}</option>{editing && !teachers.some(user => user.id === editing.owner_id) && <option value={editing.owner_id} disabled>{users.isLoading ? '正在加载当前负责人…' : '当前负责人不可用，请重新选择'}</option>}{teachers.map(user => <option key={user.id} value={user.id}>{user.display_name || user.username}（{user.username}）{user.org_id ? ` · ${user.org_id}` : ''}</option>)}</select><small>{users.error ? `教师列表加载失败：${errorMessage(users.error)}` : !users.isLoading && !teachers.length ? editing ? '当前学校没有可选教师，请先创建或启用教师账号。' : '请先创建或启用教师账号。' : editing ? '仅可选择本学校状态正常的教师' : '仅可选择状态正常的教师'}</small></label>
        <div className="form-grid"><label>教师上限<Input required type="number" min="1" max="100" step="1" value={form.max_teachers} onChange={e => setForm({ ...form, max_teachers: e.target.value })}/><small>1–100 位</small></label><label>设备上限<Input required type="number" min="1" max="20" step="1" value={form.max_devices} onChange={e => setForm({ ...form, max_devices: e.target.value })}/><small>1–20 台</small></label></div>
        {formError && <p className="form-error" role="alert">{formError}</p>}
      </form>
    </Dialog>
    <Dialog open={!!deleting} onClose={() => setDeleting(null)} title="删除班级？" description={`确认删除班级「${deleting?.name ?? ''}」及其关联信息，此操作无法撤销。`} footer={<><Button variant="outline" onClick={() => setDeleting(null)}>取消</Button><Button variant="destructive" disabled={remove.isPending} onClick={() => deleting && remove.mutate(deleting)}>{remove.isPending ? '正在删除…' : '确认删除'}</Button></>}>
      {formError && <p className="form-error" role="alert">{formError}</p>}
    </Dialog>
  </>;
}
function DevicesPage() {
  const { user } = useSession();
  const queryClient = useQueryClient();
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState('');
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [targets, setTargets] = useState<Device[]>([]);
  const [serverUrl, setServerUrl] = useState('');
  const [failures, setFailures] = useState<{ device: Device; message: string }[]>([]);
  const query = useQuery({ queryKey: ['admin', 'devices'], queryFn: () => api<Device[]>('/admin/devices') });
  const devices = (query.data ?? []).filter(item => `${item.name} ${item.class_id} ${item.os ?? ''} ${item.client_version ?? ''}`.toLowerCase().includes(search.trim().toLowerCase()) && (!status || (status === 'online' ? item.online : !item.online)));
  const selectedDevices = (query.data ?? []).filter(item => selectedIds.has(item.id));
  const allVisibleSelected = devices.length > 0 && devices.every(item => selectedIds.has(item.id));
  const saveConfig = useMutation({
    mutationFn: ({ items, url }: { items: Device[]; url: string }) => saveDeviceConfigurations(items, url),
    onSuccess: ({ errors, saved, url }) => {
      void queryClient.invalidateQueries({ queryKey: ['admin', 'devices'] });
      setSelectedIds(new Set(errors.map(item => item.device.id)));
      if (errors.length) {
        setFailures(errors);
        setTargets(errors.map(item => item.device));
        toast.error(`${saved} 台已保存，${errors.length} 台失败，请检查后重试`);
      } else {
        setTargets([]); setFailures([]);
        toast.success(url ? `${saved} 台设备配置已保存，等待设备接收或上线` : `${saved} 台设备的切换计划已取消`);
      }
    },
  });
  function openConfig(items: Device[]) {
    setTargets(items);
    setServerUrl(items.length === 1 ? items[0].remote_config?.server_url ?? '' : '');
    setFailures([]);
  }
  function toggleVisible() {
    setSelectedIds(previous => {
      const next = new Set(previous);
      for (const device of devices) allVisibleSelected ? next.delete(device.id) : next.add(device.id);
      return next;
    });
  }
  function submitConfig(event: FormEvent) {
    event.preventDefault();
    if (targets.length && !saveConfig.isPending) saveConfig.mutate({ items: targets, url: serverUrl.trim() });
  }
  return <><PageHead eyebrow="DEVICES" title="设备管理" description="了解教室大屏的在线状态和客户端信息。"/>
    <Panel title="设备列表" sub="连接状态以服务端数据为准" action={<span className="panel-count">{query.data ? `${query.data.length} 台设备` : '设备列表'}</span>}>
      <div className="filter-row"><SearchBox value={search} onChange={setSearch} placeholder="搜索设备、班级 ID 或版本"/><select aria-label="筛选设备状态" value={status} onChange={e => setStatus(e.target.value)}><option value="">全部状态</option><option value="online">在线</option><option value="offline">离线</option></select>{user.role === 'super_admin' && <Button variant="outline" size="sm" disabled={!selectedDevices.length} onClick={() => openConfig(selectedDevices)}>配置所选设备（{selectedDevices.length}）</Button>}<span className="filter-count">{query.data ? `共 ${devices.length} 条` : ''}</span></div>
      <ResultState loading={query.isLoading} error={query.error} empty={!devices.length} emptyText="没有找到设备"><TableScroller><table><thead><tr>{user.role === 'super_admin' && <th><input type="checkbox" aria-label="选择全部筛选设备" checked={allVisibleSelected} onChange={toggleVisible}/></th>}<th>设备名称</th><th>连接状态</th><th>班级 ID</th><th>系统 / 版本</th><th>最后心跳</th><th>IP 地址</th>{user.role === 'super_admin' && <><th>服务器切换计划</th><th>操作</th></>}</tr></thead><tbody>{devices.map(item => <tr key={item.id}>{user.role === 'super_admin' && <td><input type="checkbox" aria-label={`选择设备 ${item.name} ${item.id}`} checked={selectedIds.has(item.id)} onChange={event => { const checked = event.target.checked; setSelectedIds(previous => { const next = new Set(previous); checked ? next.add(item.id) : next.delete(item.id); return next; }); }}/></td>}<td><div className="person-cell"><span className="device-avatar"><Monitor size={17}/></span><div><strong>{item.name}</strong><small>{item.id}</small></div></div></td><td><Badge variant={item.online ? 'success' : 'secondary'}><span className="badge-dot"/>{item.online ? '在线' : '离线'}</Badge></td><td>{item.class_id}</td><td>{item.os || '—'} <span className="subtle">/ {item.client_version || '—'}</span></td><td>{dateTime(item.last_seen_at)}</td><td>{item.last_ip || '—'}</td>{user.role === 'super_admin' && <><td>{item.remote_config?.server_url ? <><strong>{item.remote_config.server_url}</strong><small>配置已保存，{item.online ? '等待设备接收' : '等待设备上线'} · 版本 {item.remote_config.revision}</small></> : '未设置'}</td><td><div className="row-actions"><button onClick={() => openConfig([item])}>配置服务器</button></div></td></>}</tr>)}</tbody></table></TableScroller></ResultState>
    </Panel>
    <Dialog open={targets.length > 0} onClose={() => { if (!saveConfig.isPending) { setTargets([]); setFailures([]); } }} title="配置设备服务器" description={`将为 ${targets.length} 台设备保存服务器切换计划。`} footer={<><Button variant="outline" disabled={saveConfig.isPending} onClick={() => { setTargets([]); setFailures([]); }}>取消</Button><Button type="submit" form="device-config-form" disabled={saveConfig.isPending}>{saveConfig.isPending ? '正在保存…' : '保存配置'}</Button></>}>
      <form id="device-config-form" className="dialog-form" onSubmit={submitConfig}>
        <p className="guidance">请先为设备安装支持迁移的新版本客户端，并确保新站在恢复最终备份前无法使用原设备令牌通过验证。<br/><br/>1. 保存新服务器地址，下发切换计划。<br/>2. 暂停在旧站发送消息及其他会修改数据的操作。<br/>3. 从旧站导出最终备份并恢复到新站，确认新站域名和 HTTPS 配置正确。<br/>4. 客户端验证原设备令牌和数据后切换；离线设备会在上线后接收计划。<br/><br/>备份后请勿继续写入旧站，两站不会实时同步。配置已保存不代表设备已经接收或切换。</p>
        <label>服务器地址<Input type="url" placeholder="例如：https://classding.example.com" value={serverUrl} onChange={event => setServerUrl(event.target.value)}/><small>请填写 HTTP 或 HTTPS 站点根地址；留空将取消这些设备的切换计划。</small></label>
        {failures.length > 0 && <div className="form-error" role="alert"><strong>以下设备保存失败，可修改后重试：</strong><ul className="failure-list">{failures.map(({ device, message }) => <li key={device.id}>{device.name}（{device.id}）：{message}</li>)}</ul></div>}
      </form>
    </Dialog>
  </>;
}
function MessageTable({ messages, compact = false, onSelect }: { messages: Message[]; compact?: boolean; onSelect?: (message: Message) => void }) {
  return <TableScroller><table><thead><tr><th>通知内容</th><th>发送人</th><th>目标班级</th><th>发送时间</th><th>回执状态</th>{onSelect && <th>操作</th>}</tr></thead><tbody>{messages.map(item => <tr key={item.id}><td><div className="message-content"><span className={`message-icon ${item.style === 'urgent' ? 'urgent' : item.style === 'praise' ? 'praise' : ''}`}><BellRing size={16}/></span><div><strong>{item.student_name ? `${item.student_name} · ` : ''}{item.text || (item.kind === 'voice' ? '语音通知' : '课堂通知')}</strong>{!compact && <small>{item.id}</small>}</div></div></td><td>{item.signature || item.sender_id || '—'}</td><td>{item.targets?.length ? [...new Set(item.targets.map(t => t.class_name || t.class_id))].join('、') : '—'}</td><td>{dateTime(item.created_at)}</td><td><span className={`receipt ${receiptTone(item.targets?.some(t => t.status === 'failed' || t.status === 'expired') ? 'failed' : item.targets?.length && item.targets.every(t => ['delivered', 'played', 'dismissed'].includes(t.status)) ? 'delivered' : undefined)}`}><i/>{messageSummary(item)}</span></td>{onSelect && <td><div className="row-actions"><button onClick={() => onSelect(item)}>查看回执</button></div></td>}</tr>)}</tbody></table></TableScroller>;
}
function MessagesPage() {
  const [search, setSearch] = useState('');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [selected, setSelected] = useState<Message | null>(null);
  const query = useInfiniteQuery({ queryKey: ['admin', 'messages'], initialPageParam: '', queryFn: ({ pageParam }) => api<MessagePage>(`/admin/messages${pageParam ? `?cursor=${encodeURIComponent(pageParam)}` : ''}`), getNextPageParam: page => page.next_cursor ?? undefined });
  const loadedMessages = query.data?.pages.flatMap(page => page.items ?? []) ?? [];
  const messages = loadedMessages.filter(item => {
    const matches = `${item.text ?? ''} ${item.student_name ?? ''} ${item.signature ?? ''} ${item.targets?.map(t => `${t.class_name} ${t.class_id}`).join(' ') ?? ''}`.toLowerCase().includes(search.trim().toLowerCase());
    const date = localDate(item.created_at);
    return matches && (!from || date >= from) && (!to || date <= to);
  });
  return <><PageHead eyebrow="MESSAGE AUDIT" title="消息审计" description="查看通知内容与回执，及时了解课堂沟通情况。"/>
    <Panel title="消息记录" sub="仅展示当前权限范围内的记录" action={<span className="panel-count">{query.data ? `${loadedMessages.length} 条记录` : '消息记录'}</span>}>
      <div className="filter-row"><SearchBox value={search} onChange={setSearch} placeholder="搜索消息、学生、教师或班级"/><label className="date-filter">从 <input aria-label="开始日期" type="date" value={from} onChange={e => setFrom(e.target.value)}/></label><label className="date-filter">至 <input aria-label="结束日期" type="date" value={to} onChange={e => setTo(e.target.value)}/></label><span className="filter-count">{query.data ? `共 ${messages.length} 条` : ''}</span></div>
      <ResultState loading={query.isLoading} error={query.data ? null : query.error} empty={!messages.length} emptyText="没有找到消息记录"><MessageTable messages={messages} onSelect={setSelected}/></ResultState>
      {query.isFetchNextPageError && <p className="form-error table-note" role="alert">加载更多失败：{errorMessage(query.error)}</p>}
      {query.hasNextPage && <div className="load-more"><Button variant="outline" size="sm" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>{query.isFetchingNextPage ? '正在加载…' : '加载更多'}</Button></div>}
    </Panel>
    <Dialog open={!!selected} onClose={() => setSelected(null)} title="通知回执" description={selected ? `发送于 ${dateTime(selected.created_at)} · ${selected.signature || selected.sender_id}` : undefined} footer={<Button variant="outline" onClick={() => setSelected(null)}>关闭</Button>}>
      {selected && <><div className="message-detail-text">{selected.student_name && <strong>{selected.student_name} · </strong>}{selected.text || (selected.kind === 'voice' ? '语音通知' : '课堂通知')}</div><div className="receipt-list">{selected.targets?.length ? selected.targets.map((target, index) => <div className="receipt-row" key={`${target.class_id}-${target.device_id}-${index}`}><div><strong>{target.class_name || target.class_id}</strong><small>{target.device_name || target.device_id || '未指定设备'}</small></div><div className="receipt-row-meta"><span className={`receipt ${receiptTone(target.status)}`}><i/>{targetStatus[target.status] ?? target.status}</span><small>{target.played_at ? `播放于 ${dateTime(target.played_at)}` : target.delivered_at ? `送达于 ${dateTime(target.delivered_at)}` : ''}</small></div></div>) : <p className="muted">暂无回执</p>}</div></>}
    </Dialog>
  </>;
}
function AuditPage() {
  const [search, setSearch] = useState('');
  const query = useQuery({ queryKey: ['admin', 'audit-logs'], queryFn: () => api<AuditLog[]>('/admin/audit-logs') });
  const logs = (query.data ?? []).filter(item => `${item.actor} ${item.action} ${item.target}`.toLowerCase().includes(search.trim().toLowerCase()));
  return <><PageHead eyebrow="AUDIT LOG" title="操作日志" description="查看后台操作记录，让每一次管理都有迹可循。"/>
    <Panel title="操作记录" sub="记录管理员在后台的操作" action={<span className="panel-count">{query.data ? `${query.data.length} 条记录` : '审计日志'}</span>}>
      <div className="filter-row"><SearchBox value={search} onChange={setSearch} placeholder="搜索操作人、操作或对象"/><span className="filter-count">{query.data ? `共 ${logs.length} 条` : ''}</span></div>
      <ResultState loading={query.isLoading} error={query.error} empty={!logs.length} emptyText="没有找到操作记录"><TableScroller><table><thead><tr><th>操作时间</th><th>操作人</th><th>操作</th><th>操作对象</th><th>记录 ID</th></tr></thead><tbody>{logs.map(item => <tr key={item.id}><td>{dateTime(item.at)}</td><td><strong>{item.actor || '—'}</strong></td><td><Badge variant="secondary">{item.action}</Badge></td><td>{item.target || '—'}</td><td className="subtle">{item.id}</td></tr>)}</tbody></table></TableScroller></ResultState>
    </Panel></>;
}

function SettingsPage() {
  const { logout } = useSession();
  const queryClient = useQueryClient();
  const query = useQuery({ queryKey: ['admin', 'settings'], queryFn: () => api<SiteSettings>('/admin/settings') });
  const [restoreOpen, setRestoreOpen] = useState(false);
  const [restoreFile, setRestoreFile] = useState<File | null>(null);
  const [confirmation, setConfirmation] = useState('');
  const [restoreError, setRestoreError] = useState('');
  const backup = useMutation({
    mutationFn: async () => {
      const response = await binaryRequest('/admin/backup');
      const blob = await response.blob();
      const signature = new Uint8Array(await blob.slice(0, 4).arrayBuffer());
      if (signature.length < 4 || signature[0] !== 0x50 || signature[1] !== 0x4b || !((signature[2] === 0x03 && signature[3] === 0x04) || (signature[2] === 0x05 && signature[3] === 0x06) || (signature[2] === 0x07 && signature[3] === 0x08))) {
        throw new Error('服务端返回的备份文件不是 ZIP 文件。');
      }
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = `classding-backup-${localDate(new Date().toISOString())}.zip`;
      document.body.appendChild(anchor); anchor.click(); anchor.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 1000);
    },
    onSuccess: () => toast.success('备份文件已开始下载'),
    onError: error => toast.error(`备份导出失败：${errorMessage(error)}`),
  });
  const restore = useMutation({
    mutationFn: async (file: File) => {
      const response = await binaryRequest('/admin/restore', {
        method: 'POST',
        headers: { 'Content-Type': 'application/zip', 'X-ClassDing-Restore': 'replace' },
        body: file,
      });
      const result = await response.json().catch(() => null) as { ok?: boolean } | null;
      if (!result?.ok) throw new Error('服务端未确认恢复成功，请检查数据状态后再操作。');
    },
    onSuccess: () => {
      window.dispatchEvent(new Event('classding:site-settings'));
      logout();
      toast.success('全站数据已恢复，请重新登录');
    },
    onError: error => setRestoreError(errorMessage(error)),
  });
  function closeRestore() {
    if (restore.isPending) return;
    setRestoreOpen(false); setRestoreFile(null); setConfirmation(''); setRestoreError('');
  }
  function submitRestore(event: FormEvent) {
    event.preventDefault(); setRestoreError('');
    if (!restoreFile || !/\.zip$/i.test(restoreFile.name)) { setRestoreError('请选择 ZIP 格式的备份文件。'); return; }
    if (restoreFile.size > 256 * 1024 * 1024) { setRestoreError('备份文件不能超过 256 MiB。'); return; }
    if (confirmation !== '覆盖恢复') { setRestoreError('请输入“覆盖恢复”以确认操作。'); return; }
    restore.mutate(restoreFile);
  }
  return <><PageHead eyebrow="SYSTEM SETTINGS" title="系统配置" description="管理站点展示、全站备份与数据恢复。"/>
    <div className="settings-grid">
      <Panel title="站点信息" sub="设置教师端与管理后台的站点名称、备案号和主题。">
        <ResultState loading={query.isLoading} error={query.error} empty={!query.data}>
          {query.data && <SettingsForm initial={query.data} onSaved={settings => {
            queryClient.setQueryData(['admin', 'settings'], settings);
            window.dispatchEvent(new Event('classding:site-settings'));
          }}/>}
        </ResultState>
      </Panel>
      <Panel title="全站备份与恢复" sub="备份包含全站数据和设备身份，请妥善保管文件。">
        <div className="settings-content backup-content">
          <div className="backup-action"><div><h3>导出全站备份</h3><p>下载 ZIP 文件，用于迁移或恢复此站点的数据。</p></div><Button variant="outline" disabled={backup.isPending || restore.isPending} onClick={() => backup.mutate()}><ArrowDownToLine size={17}/>{backup.isPending ? '正在导出…' : '下载备份'}</Button></div>
          <div className="backup-action"><div><h3>导入备份并恢复</h3><p>恢复会覆盖当前全站数据，操作完成后需要重新登录。</p></div><Button variant="destructive" disabled={backup.isPending || restore.isPending} onClick={() => setRestoreOpen(true)}><Upload size={17}/>导入并恢复</Button></div>
        </div>
      </Panel>
    </div>
    <Dialog open={restoreOpen} onClose={closeRestore} title="确认覆盖恢复全站数据？" description="恢复将用备份覆盖当前全站数据，此操作无法撤销。请先下载当前站点备份。" footer={<><Button variant="outline" disabled={restore.isPending} onClick={closeRestore}>取消</Button><Button variant="destructive" type="submit" form="restore-form" disabled={restore.isPending || !restoreFile || confirmation !== '覆盖恢复'}>{restore.isPending ? '正在恢复，请勿关闭页面…' : '确认覆盖恢复'}</Button></>}>
      <form id="restore-form" className="dialog-form" onSubmit={submitRestore}>
        <p className="restore-warning">备份中的账号、消息及设备身份等数据将替换当前数据。恢复成功后，管理员需要重新登录。</p>
        <label>选择备份文件<Input type="file" accept=".zip,application/zip" onChange={event => { setRestoreFile(event.target.files?.[0] ?? null); setRestoreError(''); }}/><small>ZIP 文件最大 256 MiB，解压后最大 512 MiB。</small></label>
        <label>输入“覆盖恢复”确认<Input value={confirmation} onChange={event => setConfirmation(event.target.value)} autoComplete="off"/></label>
        {restoreError && <p className="form-error" role="alert">{restoreError}</p>}
      </form>
    </Dialog>
  </>;
}

function SettingsForm({ initial, onSaved }: { initial: SiteSettings; onSaved: (settings: SiteSettings) => void }) {
  const [form, setForm] = useState(initial);
  const [formError, setFormError] = useState('');
  useEffect(() => setForm(initial), [initial]);
  const save = useMutation({
    mutationFn: (settings: SiteSettings) => api<SiteSettings>('/admin/settings', json('PUT', settings)),
    onSuccess: settings => { setForm(settings); onSaved(settings); toast.success('站点配置已保存'); },
    onError: error => setFormError(errorMessage(error)),
  });
  function submit(event: FormEvent) {
    event.preventDefault(); setFormError('');
    if (!form.site_name.trim()) { setFormError('请输入站点名称。'); return; }
    if (Array.from(form.site_name.trim()).length > 60) { setFormError('站点名称不能超过 60 个字符。'); return; }
    save.mutate({ ...form, site_name: form.site_name.trim(), icp_number: form.icp_number.trim() });
  }
  return <form className="dialog-form settings-content" onSubmit={submit}>
    <label>站点名称<Input required value={form.site_name} onChange={event => setForm({ ...form, site_name: event.target.value })}/><small>最多 60 个字符。</small></label>
    <label>备案号 <small>选填</small><Input value={form.icp_number} placeholder="例如：京ICP备12345678号" onChange={event => setForm({ ...form, icp_number: event.target.value })}/></label>
    <label>主题<select aria-label="主题" className="dialog-select" value={form.theme} onChange={event => setForm({ ...form, theme: event.target.value as SiteSettings['theme'] })}><option value="green">森林绿</option><option value="blue">海洋蓝</option><option value="slate">石板灰</option></select></label>
    {formError && <p className="form-error" role="alert">{formError}</p>}
    <div><Button type="submit" disabled={save.isPending}>{save.isPending ? '正在保存…' : '保存站点配置'}</Button></div>
  </form>;
}

export default function App() {
  const [user, setUser] = useState<User | null>(null);
  const [restoring, setRestoring] = useState(true);
  const [startupError, setStartupError] = useState('');
  const queryClient = useQueryClient();
  useEffect(() => onSessionExpired(() => {
    setUser(null); queryClient.clear(); toast.error('登录已过期或账号已变更，请重新登录');
  }), [queryClient]);
  useEffect(() => {
    let cancelled = false;
    refreshSession<AuthResponse>().then(result => {
      if (cancelled) return;
      if (result.user.role === 'super_admin' || result.user.role === 'school_admin') setUser(result.user);
      else setAccessToken('');
    }).catch(error => { if (!cancelled && error instanceof ApiError && (error.status === 0 || error.status >= 500)) setStartupError(error.message); }).finally(() => { if (!cancelled) setRestoring(false); });
    return () => { cancelled = true; };
  }, []);
  useEffect(() => {
    if (!user) return;
    let cancelled = false;
    const expectedUserId = user.id;
    const timer = window.setInterval(() => {
      refreshSession<AuthResponse>().then(result => {
        if (cancelled) return;
        if (result.user.id !== expectedUserId || (result.user.role !== 'super_admin' && result.user.role !== 'school_admin')) {
          setAccessToken(''); setUser(null); queryClient.clear(); toast.error('登录账号或权限已变更，请重新登录');
          return;
        }
        setUser(result.user);
      }).catch(error => {
        if (!cancelled && error instanceof ApiError && error.status !== 401) toast.error('暂时无法更新登录状态，请检查网络');
      });
    }, 13 * 60 * 1000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, [user?.id, queryClient]);
  function logout() {
    void api('/auth/logout', json('POST')).catch(() => undefined);
    setAccessToken(''); setUser(null); queryClient.clear();
  }
  if (restoring) return <div className="app-loading"><LoaderCircle className="spin" size={26}/><span>正在进入管理后台…</span></div>;
  if (!user) return <Login onLogin={setUser} initialError={startupError}/>;
  return <SessionContext.Provider value={{ user, logout }}><Shell/></SessionContext.Provider>;
}
