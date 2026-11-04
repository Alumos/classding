import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import type { components } from '@classding/api';
import { ArrowRight, Inbox, Search } from 'lucide-react';
import { Badge } from '@classding/ui/components/ui/badge';
import { Card, CardAction, CardDescription, CardHeader, CardTitle } from '@classding/ui/components/ui/card';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@classding/ui/components/ui/input-group';
import { Table } from '@classding/ui/components/ui/table';
import { EmptyState, ErrorState, LoadingState } from '@classding/ui/components/classding';
import { cn } from '@classding/ui/lib/utils';

type Schemas = components['schemas'];
export type User = Schemas['User'];
export type Classroom = Schemas['Class'];
export type Device = Schemas['Device'];
export type Message = Schemas['Message'];
export type ActivationCode = Schemas['ActivationCode'];
export type AuthResponse = Schemas['AuthResponse'];
export type Overview = Schemas['Overview'];
export type AuditLog = Schemas['AuditLog'];
export type MessagePage = Schemas['MessageList'];
export type SiteSettings = Schemas['SiteSettings'];
export type School = Schemas['School'];

// api() adds the JSON content type to any non-form body.
export const json = (method: string, body?: unknown): RequestInit => ({
  method,
  ...(body === undefined ? {} : { body: JSON.stringify(body) }),
});
export const errorMessage = (error: unknown) =>
  error instanceof Error ? error.message : '无法连接服务，请检查网络或稍后重试。';
export const dateTime = (value?: string | null) => {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? value
    : date.toLocaleString('zh-CN', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        hour12: false,
      });
};
export const dateOnly = (value?: string | null) => {
  if (!value) return '长期有效';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString('zh-CN');
};
export const localDate = (value?: string | null) => {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return [
    date.getFullYear(),
    String(date.getMonth() + 1).padStart(2, '0'),
    String(date.getDate()).padStart(2, '0'),
  ].join('-');
};
export const number = (value?: number | null) =>
  typeof value === 'number' && Number.isFinite(value) ? value.toLocaleString('zh-CN') : '—';
export const rate = (value?: number | null) =>
  typeof value === 'number' && Number.isFinite(value) ? `${(value * 100).toFixed(1)}%` : '—';
export const roleName: Record<string, string> = {
  super_admin: '运营管理员',
  school_admin: '学校管理员',
  user: '教师',
};
export const initial = (user: Pick<User, 'display_name' | 'username'>, fallback = '管') =>
  (user.display_name || user.username || fallback).slice(0, 1);

const codeStatus: Record<
  string,
  { text: string; variant: 'default' | 'secondary' | 'outline' | 'destructive' }
> = {
  active: { text: '已激活', variant: 'default' },
  unused: { text: '未使用', variant: 'outline' },
  revoked: { text: '已吊销', variant: 'destructive' },
  released: { text: '已释放', variant: 'secondary' },
};
export const codeStatusText = (status: string) => codeStatus[status]?.text ?? status;
export function StatusBadge({ status }: { status?: string | null }) {
  const entry = status ? codeStatus[status] : undefined;
  return <Badge variant={entry?.variant ?? 'secondary'}>{entry?.text ?? status ?? '未知'}</Badge>;
}

const SessionContext = createContext<{
  user: User;
  logout: () => void;
  updateUser: (user: User) => void;
} | null>(null);
export const SessionProvider = SessionContext.Provider;
export function useSession() {
  const session = useContext(SessionContext);
  if (!session) throw new Error('Missing session');
  return session;
}

/** Shows loading, error or empty states before rendering list content. */
export function ResultState({
  loading,
  error,
  empty,
  children,
  emptyText = '这里暂时没有数据',
}: {
  loading: boolean;
  error: unknown;
  empty: boolean;
  children: ReactNode;
  emptyText?: string;
}) {
  if (loading) return <LoadingState label="正在加载数据…" />;
  if (error)
    return (
      <div className="px-6 pb-6">
        <ErrorState error={error} title="数据加载失败" />
      </div>
    );
  if (empty)
    return (
      <div className="px-6 pb-6">
        <EmptyState icon={<Inbox />} title={emptyText} description="有新的数据时会显示在这里" />
      </div>
    );
  return <>{children}</>;
}

/** Card with a header row, used by every list page. */
export function Panel({
  title,
  description,
  action,
  children,
  className,
}: {
  title: string;
  description?: string;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <Card className={cn('gap-0 overflow-hidden py-0', className)}>
      <CardHeader className="border-b py-5">
        <CardTitle role="heading" aria-level={2}>
          {title}
        </CardTitle>
        {description && <CardDescription>{description}</CardDescription>}
        {action && <CardAction>{action}</CardAction>}
      </CardHeader>
      {children}
    </Card>
  );
}

/** Horizontally scrollable, keyboard-focusable table with an overflow hint. */
export function DataTable({ children }: { children: ReactNode }) {
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
  return (
    <div>
      <Table
        className="[&_td]:px-4 [&_td:first-child]:pl-6 [&_td:last-child]:pr-6 [&_th]:px-4 [&_th:first-child]:pl-6 [&_th:last-child]:pr-6 [&_td]:py-3 [&_th]:h-11 [&_th]:bg-muted/50"
        containerProps={{
          ref: scroll,
          role: 'region',
          'aria-label': '数据表格',
          tabIndex: 0,
          className: 'focus-visible:outline-2 focus-visible:outline-ring',
        }}
      >
        {children}
      </Table>
      {overflowing && (
        <p className="flex items-center justify-end gap-1 border-t px-6 py-2 text-xs text-muted-foreground">
          左右滑动查看更多 <ArrowRight className="size-3" />
        </p>
      )}
    </div>
  );
}

/** Search and filter controls above a table. */
export function Toolbar({ children, count }: { children: ReactNode; count?: string }) {
  return (
    <div className="flex flex-wrap items-center gap-2 border-b px-6 py-3">
      {children}
      {count && <span className="ml-auto text-sm whitespace-nowrap text-muted-foreground">{count}</span>}
    </div>
  );
}

export function SearchInput({
  value,
  onChange,
  placeholder,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
}) {
  return (
    <InputGroup className="w-full sm:w-72">
      <InputGroupInput
        aria-label={placeholder}
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
      <InputGroupAddon>
        <Search />
      </InputGroupAddon>
    </InputGroup>
  );
}
