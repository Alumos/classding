import { useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ApiError } from '@classding/api';
import { toast } from 'sonner';
import { Plus } from 'lucide-react';
import { Avatar, AvatarFallback } from '@classding/ui/components/ui/avatar';
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
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@classding/ui/components/ui/field';
import { Input } from '@classding/ui/components/ui/input';
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from '@classding/ui/components/ui/table';
import {
  ConfirmDialog,
  PageHeader,
  PaginationBar,
  SimpleSelect,
  usePagination,
} from '@classding/ui/components/classding';
import {
  DataTable,
  Panel,
  ResultState,
  SearchInput,
  Toolbar,
  dateTime,
  errorMessage,
  initial,
  json,
  roleName,
  useSession,
  type User,
} from '../lib';

type TeacherForm = {
  username: string;
  display_name: string;
  password: string;
  org_id: string;
  status: 'active' | 'disabled';
};
const emptyTeacherForm: TeacherForm = {
  username: '',
  display_name: '',
  password: '',
  org_id: '',
  status: 'active',
};

export default function UsersPage() {
  const queryClient = useQueryClient();
  const { user: currentUser } = useSession();
  const [search, setSearch] = useState('');
  const [form, setForm] = useState<TeacherForm>(emptyTeacherForm);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<User | null>(null);
  const [deleting, setDeleting] = useState<User | null>(null);
  const [formError, setFormError] = useState('');
  const query = useQuery({ queryKey: ['admin', 'users'], queryFn: () => api<User[]>('/admin/users') });
  const users = (query.data ?? []).filter((user) =>
    `${user.display_name} ${user.username} ${user.org_id ?? ''}`
      .toLowerCase()
      .includes(search.trim().toLowerCase()),
  );
  const pager = usePagination(users, 10, search);
  const save = useMutation({
    mutationFn: ({ user, body }: { user: User | null; body: unknown }) =>
      user
        ? api<User>(`/admin/users/${encodeURIComponent(user.id)}`, json('PATCH', body))
        : api<User>('/admin/users', json('POST', body)),
    onSuccess: (_, { user }) => {
      setFormOpen(false);
      setEditing(null);
      setFormError('');
      void queryClient.invalidateQueries({ queryKey: ['admin', 'users'] });
      toast.success(user ? '教师账号已更新' : '教师账号已创建');
    },
    onError: (error) => setFormError(errorMessage(error)),
  });
  const remove = useMutation({
    mutationFn: (user: User) => api<void>(`/admin/users/${encodeURIComponent(user.id)}`, json('DELETE')),
    onSuccess: () => {
      setDeleting(null);
      setFormError('');
      void queryClient.invalidateQueries({ queryKey: ['admin', 'users'] });
      toast.success('教师账号已删除');
    },
    onError: (error) =>
      setFormError(
        error instanceof ApiError && error.status === 409
          ? '该教师仍负责班级，请先转移或删除其班级。'
          : errorMessage(error),
      ),
  });
  const unlockLogin = useMutation({
    mutationFn: (user: User) => api(`/admin/users/${encodeURIComponent(user.id)}/unlock-login`, json('POST')),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['admin', 'users'] });
      toast.success('已解除登录限制，教师可以重新登录');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });
  function startCreate() {
    setEditing(null);
    setForm(emptyTeacherForm);
    setFormError('');
    setFormOpen(true);
  }
  function startEdit(user: User) {
    setEditing(user);
    setForm({
      username: user.username,
      display_name: user.display_name,
      password: '',
      org_id: user.org_id ?? '',
      status: user.status === 'disabled' ? 'disabled' : 'active',
    });
    setFormError('');
    setFormOpen(true);
  }
  function submit(event: FormEvent) {
    event.preventDefault();
    setFormError('');
    const username = form.username.trim();
    const displayName = form.display_name.trim();
    if (!/^[a-zA-Z0-9._-]{3,32}$/.test(username)) {
      setFormError('用户名须为 3–32 位，支持英文字母、数字、点、下划线和短横线。');
      return;
    }
    if (!displayName || [...displayName].length > 24) {
      setFormError('教师姓名须为 1–24 字。');
      return;
    }
    if (!editing || form.password) {
      const bytes = new TextEncoder().encode(form.password).length;
      if (bytes < 10 || bytes > 128 || !/\p{L}/u.test(form.password) || !/\p{Nd}/u.test(form.password)) {
        setFormError('密码须为 10–128 字节，包含字母和数字。');
        return;
      }
    }
    const body = editing
      ? {
          ...(username !== editing.username ? { username } : {}),
          ...(displayName !== editing.display_name ? { display_name: displayName } : {}),
          ...(form.password ? { password: form.password } : {}),
          ...(form.status !== editing.status ? { status: form.status } : {}),
        }
      : {
          username,
          display_name: displayName,
          password: form.password,
          ...(currentUser.role === 'super_admin' && form.org_id.trim() ? { org_id: form.org_id.trim() } : {}),
        };
    save.mutate({ user: editing, body });
  }
  return (
    <>
      <PageHeader
        title="教师管理"
        description="创建和维护教师账号，管理账号使用状态。"
        actions={
          <Button onClick={startCreate}>
            <Plus />
            添加教师
          </Button>
        }
      />
      <Panel
        title="教师账号"
        description="管理当前权限范围内的教师"
        action={
          <span className="text-sm text-muted-foreground">
            {query.data ? `${query.data.length} 位账号` : ''}
          </span>
        }
      >
        <Toolbar count={query.data ? `共 ${users.length} 条` : undefined}>
          <SearchInput value={search} onChange={setSearch} placeholder="搜索姓名、账号或学校 ID" />
        </Toolbar>
        <ResultState
          loading={query.isLoading}
          error={query.error}
          empty={!users.length}
          emptyText="没有找到教师账号"
        >
          <DataTable>
            <TableHeader>
              <TableRow>
                <TableHead>教师</TableHead>
                <TableHead>账号</TableHead>
                <TableHead>角色</TableHead>
                <TableHead>所属学校 ID</TableHead>
                <TableHead>状态</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {pager.pageItems.map((user) => (
                <TableRow key={user.id}>
                  <TableCell>
                    <div className="person-cell flex items-center gap-3">
                      <Avatar>
                        <AvatarFallback>{initial(user, '师')}</AvatarFallback>
                      </Avatar>
                      <div className="min-w-0">
                        <div className="font-medium">{user.display_name || '—'}</div>
                        <small className="text-xs text-muted-foreground">{user.id}</small>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>{user.username}</TableCell>
                  <TableCell>{roleName[user.role] ?? user.role}</TableCell>
                  <TableCell className="text-muted-foreground">{user.org_id || '—'}</TableCell>
                  <TableCell>
                    <Badge
                      variant={
                        user.status === 'active'
                          ? 'default'
                          : user.status === 'disabled'
                            ? 'destructive'
                            : 'secondary'
                      }
                    >
                      {user.status === 'active' ? '正常' : user.status === 'disabled' ? '已禁用' : '未知'}
                    </Badge>
                    {user.login_locked_until && (
                      <div className="mt-1 text-xs text-destructive">
                        登录限制至 {dateTime(user.login_locked_until)}
                      </div>
                    )}
                  </TableCell>
                  <TableCell className="text-right">
                    {user.role !== 'user' ? (
                      <span className="text-sm text-muted-foreground">
                        {user.id === currentUser.id ? '当前账号' : '管理员'}
                      </span>
                    ) : (
                      <div className="flex justify-end gap-1">
                        <Button variant="ghost" size="sm" onClick={() => startEdit(user)}>
                          编辑
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          disabled={unlockLogin.isPending}
                          onClick={() => unlockLogin.mutate(user)}
                        >
                          解除登录限制
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="text-destructive hover:text-destructive"
                          onClick={() => {
                            setFormError('');
                            setDeleting(user);
                          }}
                        >
                          删除
                        </Button>
                      </div>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </DataTable>
          <PaginationBar
            page={pager.page}
            pageCount={pager.pageCount}
            total={pager.total}
            onPageChange={pager.setPage}
          />
        </ResultState>
      </Panel>
      <Dialog open={formOpen} onOpenChange={setFormOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? '编辑教师账号' : '添加教师'}</DialogTitle>
            <DialogDescription>
              {editing ? '修改账号资料，留空密码则保持原密码。' : '创建后，教师可使用账号和密码登录。'}
            </DialogDescription>
          </DialogHeader>
          <form id="teacher-form" onSubmit={submit}>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="teacher-name">教师姓名</FieldLabel>
                <Input
                  id="teacher-name"
                  required
                  maxLength={48}
                  placeholder="例如：林老师"
                  value={form.display_name}
                  onChange={(e) => setForm({ ...form, display_name: e.target.value })}
                />
                <FieldDescription>1–24 字，作为通知落款</FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="teacher-username">用户名</FieldLabel>
                <Input
                  id="teacher-username"
                  required
                  minLength={3}
                  maxLength={32}
                  autoComplete="off"
                  placeholder="3–32 位字母、数字或 . _ -"
                  value={form.username}
                  onChange={(e) => setForm({ ...form, username: e.target.value })}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="teacher-password">{editing ? '重设密码' : '登录密码'}</FieldLabel>
                <Input
                  id="teacher-password"
                  type="password"
                  required={!editing}
                  autoComplete="new-password"
                  placeholder="至少 10 位，包含字母和数字"
                  value={form.password}
                  onChange={(e) => setForm({ ...form, password: e.target.value })}
                />
                {editing && <FieldDescription>留空则不修改</FieldDescription>}
              </Field>
              {!editing && currentUser.role === 'super_admin' && (
                <Field>
                  <FieldLabel htmlFor="teacher-org">所属学校 ID</FieldLabel>
                  <Input
                    id="teacher-org"
                    placeholder="选填"
                    value={form.org_id}
                    onChange={(e) => setForm({ ...form, org_id: e.target.value })}
                  />
                </Field>
              )}
              {editing && (
                <Field>
                  <FieldLabel htmlFor="teacher-status">账号状态</FieldLabel>
                  <SimpleSelect
                    id="teacher-status"
                    label="账号状态"
                    value={form.status}
                    onValueChange={(value) => setForm({ ...form, status: value as TeacherForm['status'] })}
                    options={[
                      { value: 'active', label: '正常' },
                      { value: 'disabled', label: '已禁用' },
                    ]}
                  />
                </Field>
              )}
              {formError && <FieldError role="alert">{formError}</FieldError>}
            </FieldGroup>
          </form>
          <DialogFooter>
            <Button variant="outline" onClick={() => setFormOpen(false)}>
              取消
            </Button>
            <Button type="submit" form="teacher-form" disabled={save.isPending}>
              {save.isPending ? '正在保存…' : editing ? '保存修改' : '创建账号'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
        title="删除教师账号？"
        description={`确认删除 ${deleting?.display_name || deleting?.username || '该教师'} 的账号。若教师仍负责班级，请先转移或删除班级。`}
        confirmLabel={remove.isPending ? '正在删除…' : '确认删除'}
        pending={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting)}
      >
        {formError && (
          <p className="text-sm text-destructive" role="alert">
            {formError}
          </p>
        )}
      </ConfirmDialog>
    </>
  );
}
