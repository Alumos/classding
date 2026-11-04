import { useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@classding/api';
import { toast } from 'sonner';
import { BookOpen, Plus } from 'lucide-react';
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
  json,
  number,
  type Classroom,
  type User,
} from '../lib';
import { useSchools } from '../schools';

type ClassForm = { name: string; grade: string; owner_id: string; max_teachers: string; max_devices: string };
const emptyClassForm: ClassForm = { name: '', grade: '', owner_id: '', max_teachers: '3', max_devices: '1' };

export default function ClassesPage() {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState('');
  const [form, setForm] = useState<ClassForm>(emptyClassForm);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<Classroom | null>(null);
  const [deleting, setDeleting] = useState<Classroom | null>(null);
  const [formError, setFormError] = useState('');
  const query = useQuery({
    queryKey: ['admin', 'classes'],
    queryFn: () => api<Classroom[]>('/admin/classes'),
  });
  const users = useQuery({ queryKey: ['admin', 'users'], queryFn: () => api<User[]>('/admin/users') });
  const teachers = (users.data ?? []).filter(
    (user) =>
      user.role === 'user' &&
      user.status === 'active' &&
      !!user.org_id &&
      (!editing || user.org_id === editing.org_id),
  );
  const { schoolName } = useSchools();
  const ownerName = (id: string) => {
    const user = users.data?.find((item) => item.id === id);
    return user ? `${user.display_name || user.username}（${user.username}）` : id || '—';
  };
  const classes = (query.data ?? []).filter((item) =>
    `${item.name} ${item.grade ?? ''} ${schoolName(item.org_id)} ${ownerName(item.owner_id)}`
      .toLowerCase()
      .includes(search.trim().toLowerCase()),
  );
  const pager = usePagination(classes, 10, search);
  const ownerOptions = [
    ...(editing && !teachers.some((user) => user.id === editing.owner_id)
      ? [
          {
            value: editing.owner_id,
            label: users.isLoading ? '正在加载当前负责人…' : '当前负责人不可用，请重新选择',
          },
        ]
      : []),
    ...teachers.map((user) => ({
      value: user.id,
      label: `${user.display_name || user.username}（${user.username}） · ${schoolName(user.org_id)}`,
    })),
  ];
  const save = useMutation({
    mutationFn: ({ item, body }: { item: Classroom | null; body: unknown }) =>
      item
        ? api<Classroom>(`/admin/classes/${encodeURIComponent(item.id)}`, json('PATCH', body))
        : api<Classroom>('/admin/classes', json('POST', body)),
    onSuccess: (_, { item }) => {
      setFormOpen(false);
      setEditing(null);
      setFormError('');
      void queryClient.invalidateQueries({ queryKey: ['admin', 'classes'] });
      toast.success(item ? '班级已更新' : '班级已创建');
    },
    onError: (error) => setFormError(errorMessage(error)),
  });
  const remove = useMutation({
    mutationFn: (item: Classroom) =>
      api<void>(`/admin/classes/${encodeURIComponent(item.id)}`, json('DELETE')),
    onSuccess: () => {
      setDeleting(null);
      setFormError('');
      void queryClient.invalidateQueries({ queryKey: ['admin', 'classes'] });
      toast.success('班级已删除');
    },
    onError: (error) => setFormError(errorMessage(error)),
  });
  function startCreate() {
    setEditing(null);
    setForm(emptyClassForm);
    setFormError('');
    setFormOpen(true);
  }
  function startEdit(item: Classroom) {
    setEditing(item);
    setForm({
      name: item.name,
      grade: item.grade ?? '',
      owner_id: item.owner_id,
      max_teachers: String(item.max_teachers),
      max_devices: String(item.max_devices),
    });
    setFormError('');
    setFormOpen(true);
  }
  function submit(event: FormEvent) {
    event.preventDefault();
    setFormError('');
    const name = form.name.trim(),
      grade = form.grade.trim();
    const maxTeachers = Number(form.max_teachers),
      maxDevices = Number(form.max_devices);
    if (!name || [...name].length > 40) {
      setFormError('班级名称须为 1–40 字。');
      return;
    }
    if ([...grade].length > 40) {
      setFormError('年级最多 40 字。');
      return;
    }
    if (!teachers.some((user) => user.id === form.owner_id)) {
      setFormError('请选择状态正常的教师作为班级负责人。');
      return;
    }
    if (
      !Number.isInteger(maxTeachers) ||
      maxTeachers < 1 ||
      maxTeachers > 100 ||
      !Number.isInteger(maxDevices) ||
      maxDevices < 1 ||
      maxDevices > 20
    ) {
      setFormError('教师上限须为 1–100，设备上限须为 1–20，且均为整数。');
      return;
    }
    save.mutate({
      item: editing,
      body: { name, grade, owner_id: form.owner_id, max_teachers: maxTeachers, max_devices: maxDevices },
    });
  }
  const ownerHint = users.error
    ? `教师列表加载失败：${errorMessage(users.error)}`
    : !users.isLoading && !teachers.length
      ? editing
        ? '当前学校没有可选教师，请先创建或启用教师账号。'
        : '请先创建或启用教师账号。'
      : editing
        ? '仅可选择本学校状态正常的教师'
        : '仅可选择状态正常的教师';
  return (
    <>
      <PageHeader
        title="班级管理"
        description="创建班级，管理负责人和教室连接额度。"
        actions={
          <Button onClick={startCreate}>
            <Plus />
            添加班级
          </Button>
        }
      />
      <Panel
        title="班级列表"
        description="每一间教室都在这里"
        action={
          <span className="text-sm text-muted-foreground">
            {query.data ? `${query.data.length} 个班级` : ''}
          </span>
        }
      >
        <Toolbar count={query.data ? `共 ${classes.length} 条` : undefined}>
          <SearchInput value={search} onChange={setSearch} placeholder="搜索班级、年级、负责人或学校" />
        </Toolbar>
        <ResultState
          loading={query.isLoading}
          error={query.error}
          empty={!classes.length}
          emptyText="没有找到班级"
        >
          <DataTable>
            <TableHeader>
              <TableRow>
                <TableHead>班级名称</TableHead>
                <TableHead>年级</TableHead>
                <TableHead>负责人</TableHead>
                <TableHead>所属学校</TableHead>
                <TableHead>教师</TableHead>
                <TableHead>设备</TableHead>
                <TableHead>创建时间</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {pager.pageItems.map((item) => (
                <TableRow key={item.id}>
                  <TableCell>
                    <div className="person-cell flex items-center gap-3">
                      <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted">
                        <BookOpen className="size-4" />
                      </span>
                      <div className="min-w-0">
                        <div className="font-medium">{item.name}</div>
                        <small className="text-xs text-muted-foreground">{item.id}</small>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>{item.grade || '—'}</TableCell>
                  <TableCell>{ownerName(item.owner_id)}</TableCell>
                  <TableCell className="text-muted-foreground">{schoolName(item.org_id)}</TableCell>
                  <TableCell>
                    {number(item.teachers_count)} / {number(item.max_teachers)} 位
                  </TableCell>
                  <TableCell>
                    <Badge variant={item.online_devices ? 'default' : 'secondary'}>
                      {number(item.online_devices)} 在线
                    </Badge>{' '}
                    <span className="text-xs text-muted-foreground">
                      {number(item.devices_count)} / {number(item.max_devices)} 台
                    </span>
                  </TableCell>
                  <TableCell className="text-muted-foreground">{dateTime(item.created_at)}</TableCell>
                  <TableCell className="text-right">
                    <div className="flex justify-end gap-1">
                      <Button variant="ghost" size="sm" onClick={() => startEdit(item)}>
                        编辑
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive hover:text-destructive"
                        onClick={() => {
                          setFormError('');
                          setDeleting(item);
                        }}
                      >
                        删除
                      </Button>
                    </div>
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
            <DialogTitle>{editing ? '编辑班级' : '添加班级'}</DialogTitle>
            <DialogDescription>
              {editing ? '调整班级信息和连接额度。' : '创建后即可使用，无需激活码。'}
            </DialogDescription>
          </DialogHeader>
          <form id="class-form" onSubmit={submit}>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="class-name">班级名称</FieldLabel>
                <Input
                  id="class-name"
                  required
                  maxLength={80}
                  placeholder="例如：三年级（2）班"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="class-grade">年级</FieldLabel>
                <Input
                  id="class-grade"
                  maxLength={80}
                  placeholder="选填，例如：三年级"
                  value={form.grade}
                  onChange={(e) => setForm({ ...form, grade: e.target.value })}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="class-owner">负责人</FieldLabel>
                <SimpleSelect
                  id="class-owner"
                  label="负责人"
                  placeholder={users.isLoading ? '正在加载教师…' : '请选择教师'}
                  value={form.owner_id}
                  onValueChange={(value) => setForm({ ...form, owner_id: value })}
                  options={ownerOptions}
                />
                <FieldDescription>{ownerHint}</FieldDescription>
              </Field>
              <div className="grid grid-cols-2 gap-4">
                <Field>
                  <FieldLabel htmlFor="class-teachers">教师上限</FieldLabel>
                  <Input
                    id="class-teachers"
                    required
                    type="number"
                    min="1"
                    max="100"
                    step="1"
                    value={form.max_teachers}
                    onChange={(e) => setForm({ ...form, max_teachers: e.target.value })}
                  />
                  <FieldDescription>1–100 位</FieldDescription>
                </Field>
                <Field>
                  <FieldLabel htmlFor="class-devices">设备上限</FieldLabel>
                  <Input
                    id="class-devices"
                    required
                    type="number"
                    min="1"
                    max="20"
                    step="1"
                    value={form.max_devices}
                    onChange={(e) => setForm({ ...form, max_devices: e.target.value })}
                  />
                  <FieldDescription>1–20 台</FieldDescription>
                </Field>
              </div>
              {formError && <FieldError role="alert">{formError}</FieldError>}
            </FieldGroup>
          </form>
          <DialogFooter>
            <Button variant="outline" onClick={() => setFormOpen(false)}>
              取消
            </Button>
            <Button
              type="submit"
              form="class-form"
              disabled={save.isPending || users.isLoading || !!users.error || !teachers.length}
            >
              {save.isPending ? '正在保存…' : editing ? '保存修改' : '创建班级'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
        title="删除班级？"
        description={`确认删除班级「${deleting?.name ?? ''}」及其关联信息，此操作无法撤销。`}
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
