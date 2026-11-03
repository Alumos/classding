import { useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@classding/api';
import { toast } from 'sonner';
import { CircleCheck, Copy, Download, Plus, X } from 'lucide-react';
import { Alert, AlertAction, AlertDescription, AlertTitle } from '@classding/ui/components/ui/alert';
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
  CopyButton,
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
  StatusBadge,
  Toolbar,
  codeStatusText,
  dateOnly,
  errorMessage,
  json,
  localDate,
  type ActivationCode,
} from '../lib';

function downloadCodes(codes: ActivationCode[], filename = 'classding-activation-codes.csv') {
  const fields = [
    '激活码',
    '状态',
    '教师上限',
    '设备上限',
    '有效期',
    '班级 ID',
    '学校 ID',
    '批次 ID',
    '备注',
    '核销时间',
  ];
  const quote = (value: unknown) => {
    let s = String(value ?? '');
    // Prevent spreadsheet formula injection.
    if (/^[\s]*[=+\-@\t\r]/.test(s)) s = `'${s}`;
    return `"${s.replace(/"/g, '""')}"`;
  };
  const rows = codes.map((code) => [
    code.code,
    codeStatusText(code.status),
    code.max_teachers,
    code.max_devices,
    code.expires_at ?? '',
    code.class_id ?? '',
    code.org_id ?? '',
    code.batch_id ?? '',
    code.note ?? '',
    code.redeemed_at ?? '',
  ]);
  const csv = '﻿' + [fields, ...rows].map((row) => row.map(quote).join(',')).join('\r\n');
  const url = URL.createObjectURL(new Blob([csv], { type: 'text/csv;charset=utf-8' }));
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}

const defaultCodeForm = {
  count: '10',
  max_teachers: '3',
  max_devices: '1',
  expires_at: '',
  org_id: '',
  note: '',
};
const statusOptions = [
  { value: 'all', label: '全部状态' },
  { value: 'unused', label: '未使用' },
  { value: 'active', label: '已激活' },
  { value: 'released', label: '已释放' },
  { value: 'revoked', label: '已吊销' },
];
const endOfDay = (value: string) => new Date(`${value}T23:59:59`);

export default function CodesPage() {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState('all');
  const [form, setForm] = useState(defaultCodeForm);
  const [formError, setFormError] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [created, setCreated] = useState<ActivationCode[]>([]);
  const [editing, setEditing] = useState<ActivationCode | null>(null);
  const [revoking, setRevoking] = useState<ActivationCode | null>(null);
  const [editForm, setEditForm] = useState({ max_teachers: '', max_devices: '', expires_at: '' });
  const params = new URLSearchParams();
  if (search.trim()) params.set('search', search.trim());
  if (status !== 'all') params.set('status', status);
  const query = useQuery({
    queryKey: ['admin', 'codes', search.trim(), status],
    queryFn: () => api<ActivationCode[]>(`/admin/activation-codes${params.size ? `?${params}` : ''}`),
  });
  const pager = usePagination(query.data ?? [], 10, `${search}|${status}`);
  const create = useMutation({
    mutationFn: (body: unknown) => api<ActivationCode[]>('/admin/activation-codes', json('POST', body)),
    onSuccess: (result) => {
      setCreated(result);
      setCreateOpen(false);
      setForm(defaultCodeForm);
      setFormError('');
      void queryClient.invalidateQueries({ queryKey: ['admin', 'codes'] });
      toast.success(`已生成 ${result.length} 个激活码`);
    },
    onError: (error) => setFormError(errorMessage(error)),
  });
  const adjust = useMutation({
    mutationFn: ({ code, body }: { code: string; body: unknown }) =>
      api<ActivationCode>(`/admin/activation-codes/${encodeURIComponent(code)}`, json('PATCH', body)),
    onSuccess: () => {
      setEditing(null);
      setRevoking(null);
      setFormError('');
      void queryClient.invalidateQueries({ queryKey: ['admin', 'codes'] });
      toast.success('激活码已更新');
    },
    onError: (error) => setFormError(errorMessage(error)),
  });
  function submitCreate(event: FormEvent) {
    event.preventDefault();
    setFormError('');
    const count = Number(form.count),
      teachers = Number(form.max_teachers),
      devices = Number(form.max_devices);
    if (!Number.isInteger(count) || count < 1 || count > 1000) {
      setFormError('生成数量须为 1 至 1000 的整数。');
      return;
    }
    if (!Number.isInteger(teachers) || teachers < 1 || !Number.isInteger(devices) || devices < 1) {
      setFormError('教师和设备上限须为大于 0 的整数。');
      return;
    }
    if (form.expires_at && Number.isNaN(endOfDay(form.expires_at).getTime())) {
      setFormError('请选择有效的到期日期。');
      return;
    }
    create.mutate({
      count,
      max_teachers: teachers,
      max_devices: devices,
      ...(form.expires_at ? { expires_at: endOfDay(form.expires_at).toISOString() } : {}),
      ...(form.org_id.trim() ? { org_id: form.org_id.trim() } : {}),
      ...(form.note.trim() ? { note: form.note.trim() } : {}),
    });
  }
  function startEdit(code: ActivationCode) {
    setFormError('');
    setEditing(code);
    setEditForm({
      max_teachers: String(code.max_teachers),
      max_devices: String(code.max_devices),
      expires_at: localDate(code.expires_at),
    });
  }
  function submitEdit(event: FormEvent) {
    event.preventDefault();
    setFormError('');
    if (!editing) return;
    const teachers = Number(editForm.max_teachers),
      devices = Number(editForm.max_devices);
    if (!Number.isInteger(teachers) || teachers < 1 || !Number.isInteger(devices) || devices < 1) {
      setFormError('教师和设备上限须为大于 0 的整数。');
      return;
    }
    adjust.mutate({
      code: editing.code,
      body: {
        max_teachers: teachers,
        max_devices: devices,
        expires_at: editForm.expires_at ? endOfDay(editForm.expires_at).toISOString() : null,
      },
    });
  }
  return (
    <>
      <PageHeader
        title="激活码管理"
        description="生成和管理授权，让每一间教室顺利开通。"
        actions={
          <Button
            onClick={() => {
              setFormError('');
              setCreateOpen(true);
            }}
          >
            <Plus />
            生成激活码
          </Button>
        }
      />
      {created.length > 0 && (
        <Alert>
          <CircleCheck />
          <AlertTitle>成功生成 {created.length} 个激活码</AlertTitle>
          <AlertDescription>
            <p>请及时复制或导出本次生成的激活码。</p>
            <div className="mt-2 flex flex-wrap gap-2">
              <CopyButton
                text={created.map((c) => c.code).join('\n')}
                onCopied={() => toast.success('已复制到剪贴板')}
                variant="outline"
                size="sm"
              >
                <Copy />
                复制全部
              </CopyButton>
              <Button
                variant="outline"
                size="sm"
                onClick={() => downloadCodes(created, 'classding-new-codes.csv')}
              >
                <Download />
                导出本批
              </Button>
            </div>
          </AlertDescription>
          <AlertAction>
            <Button variant="ghost" size="icon-sm" aria-label="关闭提示" onClick={() => setCreated([])}>
              <X />
            </Button>
          </AlertAction>
        </Alert>
      )}
      <Panel
        title="全部激活码"
        description="查看授权状态，按需调整使用额度"
        action={
          <Button
            variant="outline"
            size="sm"
            disabled={!query.data?.length}
            onClick={() => downloadCodes(query.data ?? [])}
          >
            <Download />
            导出 CSV
          </Button>
        }
      >
        <Toolbar count={query.data ? `共 ${query.data.length} 条` : undefined}>
          <SearchInput value={search} onChange={setSearch} placeholder="搜索激活码、备注或批次" />
          <SimpleSelect
            label="筛选激活码状态"
            value={status}
            onValueChange={setStatus}
            options={statusOptions}
            className="w-full sm:w-36"
          />
        </Toolbar>
        <ResultState
          loading={query.isLoading}
          error={query.error}
          empty={!query.data?.length}
          emptyText="没有找到激活码"
        >
          <DataTable>
            <TableHeader>
              <TableRow>
                <TableHead>激活码</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>教师 / 设备上限</TableHead>
                <TableHead>有效期</TableHead>
                <TableHead>关联班级</TableHead>
                <TableHead>备注</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {pager.pageItems.map((code) => (
                <TableRow key={code.code}>
                  <TableCell>
                    <div className="flex items-center gap-1">
                      <span className="font-mono font-medium tracking-wide">{code.code}</span>
                      <CopyButton
                        text={code.code}
                        onCopied={() => toast.success('已复制到剪贴板')}
                        variant="ghost"
                        size="icon-xs"
                        aria-label={`复制激活码 ${code.code}`}
                        title="复制"
                      >
                        <Copy />
                      </CopyButton>
                    </div>
                    <div className="text-xs text-muted-foreground">{code.batch_id || '—'}</div>
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={code.status} />
                  </TableCell>
                  <TableCell>
                    {code.max_teachers} 位 / {code.max_devices} 台
                  </TableCell>
                  <TableCell>{dateOnly(code.expires_at)}</TableCell>
                  <TableCell className="text-muted-foreground">{code.class_id || '—'}</TableCell>
                  <TableCell className="max-w-48 truncate" title={code.note ?? ''}>
                    {code.note || '—'}
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex justify-end gap-1">
                      <Button variant="ghost" size="sm" onClick={() => startEdit(code)}>
                        调整
                      </Button>
                      {code.status !== 'revoked' && (
                        <Button
                          variant="ghost"
                          size="sm"
                          className="text-destructive hover:text-destructive"
                          onClick={() => {
                            setFormError('');
                            setRevoking(code);
                          }}
                        >
                          吊销
                        </Button>
                      )}
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
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>生成激活码</DialogTitle>
            <DialogDescription>设置授权额度，生成后可复制或导出。</DialogDescription>
          </DialogHeader>
          <form id="create-codes" onSubmit={submitCreate}>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="code-count">生成数量</FieldLabel>
                <Input
                  id="code-count"
                  type="number"
                  min="1"
                  max="1000"
                  value={form.count}
                  onChange={(e) => setForm({ ...form, count: e.target.value })}
                />
                <FieldDescription>单次可生成 1–1000 个</FieldDescription>
              </Field>
              <div className="grid grid-cols-2 gap-4">
                <Field>
                  <FieldLabel htmlFor="code-teachers">教师上限</FieldLabel>
                  <Input
                    id="code-teachers"
                    type="number"
                    min="1"
                    value={form.max_teachers}
                    onChange={(e) => setForm({ ...form, max_teachers: e.target.value })}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="code-devices">设备上限</FieldLabel>
                  <Input
                    id="code-devices"
                    type="number"
                    min="1"
                    value={form.max_devices}
                    onChange={(e) => setForm({ ...form, max_devices: e.target.value })}
                  />
                </Field>
              </div>
              <Field>
                <FieldLabel htmlFor="code-expires">有效期</FieldLabel>
                <Input
                  id="code-expires"
                  type="date"
                  value={form.expires_at}
                  onChange={(e) => setForm({ ...form, expires_at: e.target.value })}
                />
                <FieldDescription>留空为长期有效</FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="code-org">所属学校 ID</FieldLabel>
                <Input
                  id="code-org"
                  placeholder="选填"
                  value={form.org_id}
                  onChange={(e) => setForm({ ...form, org_id: e.target.value })}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="code-note">备注</FieldLabel>
                <Input
                  id="code-note"
                  maxLength={200}
                  placeholder="例如：2026 年秋季采购"
                  value={form.note}
                  onChange={(e) => setForm({ ...form, note: e.target.value })}
                />
              </Field>
              {formError && <FieldError role="alert">{formError}</FieldError>}
            </FieldGroup>
          </form>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              取消
            </Button>
            <Button type="submit" form="create-codes" disabled={create.isPending}>
              {create.isPending ? '正在生成…' : '生成激活码'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog
        open={!!editing}
        onOpenChange={(open) => {
          if (!open) setEditing(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>调整激活码</DialogTitle>
            {editing && <DialogDescription>调整 {editing.code} 的授权设置。</DialogDescription>}
          </DialogHeader>
          <form id="edit-code" onSubmit={submitEdit}>
            <FieldGroup>
              <div className="grid grid-cols-2 gap-4">
                <Field>
                  <FieldLabel htmlFor="edit-teachers">教师上限</FieldLabel>
                  <Input
                    id="edit-teachers"
                    type="number"
                    min="1"
                    value={editForm.max_teachers}
                    onChange={(e) => setEditForm({ ...editForm, max_teachers: e.target.value })}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="edit-devices">设备上限</FieldLabel>
                  <Input
                    id="edit-devices"
                    type="number"
                    min="1"
                    value={editForm.max_devices}
                    onChange={(e) => setEditForm({ ...editForm, max_devices: e.target.value })}
                  />
                </Field>
              </div>
              <Field>
                <FieldLabel htmlFor="edit-expires">有效期</FieldLabel>
                <Input
                  id="edit-expires"
                  type="date"
                  value={editForm.expires_at}
                  onChange={(e) => setEditForm({ ...editForm, expires_at: e.target.value })}
                />
                <FieldDescription>留空为长期有效</FieldDescription>
              </Field>
              {formError && <FieldError role="alert">{formError}</FieldError>}
            </FieldGroup>
          </form>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditing(null)}>
              取消
            </Button>
            <Button type="submit" form="edit-code" disabled={adjust.isPending}>
              {adjust.isPending ? '正在保存…' : '保存调整'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={!!revoking}
        onOpenChange={(open) => {
          if (!open) setRevoking(null);
        }}
        title="确认吊销激活码？"
        description={`吊销后，关联的教室将无法接收新通知。激活码：${revoking?.code ?? ''}`}
        confirmLabel={adjust.isPending ? '正在吊销…' : '确认吊销'}
        pending={adjust.isPending}
        onConfirm={() => revoking && adjust.mutate({ code: revoking.code, body: { status: 'revoked' } })}
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
