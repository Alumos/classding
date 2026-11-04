import { useState, type FormEvent } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@classding/api';
import { toast } from 'sonner';
import { Plus } from 'lucide-react';
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
import { ConfirmDialog, PageHeader, PaginationBar, usePagination } from '@classding/ui/components/classding';
import {
  DataTable,
  Panel,
  ResultState,
  SearchInput,
  Toolbar,
  dateTime,
  errorMessage,
  json,
  type School,
} from '../lib';
import { useSchools } from '../schools';

export default function SchoolsPage() {
  const queryClient = useQueryClient();
  const { query, schools } = useSchools();
  const [search, setSearch] = useState('');
  const [editing, setEditing] = useState<School | null>(null);
  const [formOpen, setFormOpen] = useState(false);
  const [name, setName] = useState('');
  const [formError, setFormError] = useState('');
  const [deleting, setDeleting] = useState<School | null>(null);
  const visible = schools.filter((school) => school.name.toLowerCase().includes(search.trim().toLowerCase()));
  const pager = usePagination(visible, 10, search);
  const refresh = () => void queryClient.invalidateQueries({ queryKey: ['admin'] });

  const save = useMutation({
    mutationFn: (value: string) =>
      editing
        ? api<School>(`/admin/schools/${encodeURIComponent(editing.id)}`, json('PATCH', { name: value }))
        : api<School>('/admin/schools', json('POST', { name: value })),
    onSuccess: () => {
      toast.success(editing ? '学校名称已更新' : '学校已创建');
      setFormOpen(false);
      refresh();
    },
    onError: (error) => setFormError(errorMessage(error)),
  });
  const remove = useMutation({
    mutationFn: (school: School) =>
      api(`/admin/schools/${encodeURIComponent(school.id)}`, { method: 'DELETE' }),
    onSuccess: () => {
      toast.success('学校已删除');
      setDeleting(null);
      refresh();
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  function open(school: School | null) {
    setEditing(school);
    setName(school?.name ?? '');
    setFormError('');
    setFormOpen(true);
  }
  function submit(event: FormEvent) {
    event.preventDefault();
    const value = name.trim();
    if (!value || [...value].length > 40) {
      setFormError('学校名称须为 1–40 字。');
      return;
    }
    save.mutate(value);
  }

  return (
    <>
      <PageHeader
        title="学校管理"
        description="为每所学校建档，教师、班级和激活码都按学校归属。"
        actions={
          <Button onClick={() => open(null)}>
            <Plus />
            新建学校
          </Button>
        }
      />
      <Panel title="全部学校" description="自行注册的教师不属于任何学校，兑换某校的激活码后会自动加入该校">
        <Toolbar count={query.data ? `共 ${visible.length} 所` : undefined}>
          <SearchInput value={search} onChange={setSearch} placeholder="搜索学校名称" />
        </Toolbar>
        <ResultState
          loading={query.isLoading}
          error={query.error}
          empty={!visible.length}
          emptyText="还没有学校"
        >
          <DataTable>
            <TableHeader>
              <TableRow>
                <TableHead>学校</TableHead>
                <TableHead>教师</TableHead>
                <TableHead>班级</TableHead>
                <TableHead>激活码</TableHead>
                <TableHead>创建时间</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {pager.pageItems.map((school) => {
                const used = school.teachers_count + school.classes_count + school.codes_count > 0;
                return (
                  <TableRow key={school.id}>
                    <TableCell>
                      <div className="font-medium">{school.name}</div>
                      <small className="text-xs text-muted-foreground">{school.id}</small>
                    </TableCell>
                    <TableCell>{school.teachers_count} 位</TableCell>
                    <TableCell>{school.classes_count} 个</TableCell>
                    <TableCell>{school.codes_count} 个</TableCell>
                    <TableCell className="text-muted-foreground">{dateTime(school.created_at)}</TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        <Button variant="ghost" size="sm" onClick={() => open(school)}>
                          改名
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="text-destructive hover:text-destructive"
                          disabled={used}
                          title={used ? '请先移出该校的教师、班级和激活码' : undefined}
                          onClick={() => setDeleting(school)}
                        >
                          删除
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
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
            <DialogTitle>{editing ? '修改学校名称' : '新建学校'}</DialogTitle>
            <DialogDescription>
              {editing ? '名称会同步显示在教师、班级和激活码中。' : '创建后即可为该校生成激活码、添加教师。'}
            </DialogDescription>
          </DialogHeader>
          <form id="school-form" onSubmit={submit}>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="school-name">学校名称</FieldLabel>
                <Input
                  id="school-name"
                  maxLength={40}
                  placeholder="例如：阳光实验小学"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                />
                <FieldDescription>最多 40 字，不能与其他学校重名</FieldDescription>
              </Field>
              {formError && <FieldError role="alert">{formError}</FieldError>}
            </FieldGroup>
          </form>
          <DialogFooter>
            <Button variant="outline" onClick={() => setFormOpen(false)}>
              取消
            </Button>
            <Button type="submit" form="school-form" disabled={save.isPending}>
              {save.isPending ? '正在保存…' : editing ? '保存名称' : '创建学校'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(value) => {
          if (!value) setDeleting(null);
        }}
        title="确认删除学校？"
        description={`将删除「${deleting?.name ?? ''}」。该校已没有教师、班级和激活码。`}
        confirmLabel={remove.isPending ? '正在删除…' : '确认删除'}
        pending={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting)}
      />
    </>
  );
}
