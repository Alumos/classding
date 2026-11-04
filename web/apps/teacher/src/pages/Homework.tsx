import { useState, type FormEvent } from 'react';
import { Link } from 'react-router';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@classding/api';
import { BookOpenCheck, Clock3, GraduationCap, Monitor, Pencil, Plus, Trash2 } from 'lucide-react';
import { toast } from 'sonner';
import { Alert, AlertDescription, AlertTitle } from '@classding/ui/components/ui/alert';
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
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@classding/ui/components/ui/field';
import { Input } from '@classding/ui/components/ui/input';
import { Label } from '@classding/ui/components/ui/label';
import { Spinner } from '@classding/ui/components/ui/spinner';
import { Switch } from '@classding/ui/components/ui/switch';
import { Textarea } from '@classding/ui/components/ui/textarea';
import {
  ConfirmDialog,
  PaginationBar,
  SimpleSelect,
  usePagination,
} from '@classding/ui/components/classding';
import { cn } from '@classding/ui/lib/utils';
import { useAuth } from '../auth';
import { useClasses, useHomeworks } from '../hooks';
import { EmptyState, ErrorState, Loading, PageHeader, formatTime } from '../components';
import type { Class, Homework } from '../models';

type Editor = { kind: 'create'; classId: string } | { kind: 'edit'; item: Homework };
type HomeworkFields = { subject: string; text: string; visible: boolean; always_on_top: boolean };
const lengthOf = (value: string) => Array.from(value).length;

export default function HomeworkPage() {
  const { user } = useAuth();
  const cache = useQueryClient();
  const classes = useClasses();
  const [selectedId, setSelectedId] = useState('');
  const [editor, setEditor] = useState<Editor | null>(null);
  const [deleting, setDeleting] = useState<Homework | null>(null);
  const activeClass = classes.data?.find((item) => item.id === selectedId) ?? classes.data?.[0];
  const query = useHomeworks(activeClass?.id ?? '');
  const items = query.data?.items ?? [];
  const pager = usePagination(items, 6, activeClass?.id);
  const mayManage = (item: Homework) => item.teacher_id === user?.id || activeClass?.role === 'owner';
  const refresh = () => cache.invalidateQueries({ queryKey: ['homeworks'] });
  const updateOption = useMutation({
    mutationFn: ({
      item,
      field,
      value,
    }: {
      item: Homework;
      field: 'visible' | 'always_on_top';
      value: boolean;
    }) =>
      api<Homework>(`/homeworks/${encodeURIComponent(item.id)}`, {
        method: 'PATCH',
        body: JSON.stringify({ [field]: value }),
      }),
    onSuccess: async () => {
      await refresh();
      toast.success('显示设置已保存');
    },
    onError: (error) => toast.error(error.message),
  });
  const remove = useMutation({
    mutationFn: (id: string) =>
      api<{ ok: boolean }>(`/homeworks/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    onSuccess: async () => {
      await refresh();
      setDeleting(null);
      toast.success('作业已删除');
    },
    onError: (error) => toast.error(error.message),
  });
  const create = () => activeClass && setEditor({ kind: 'create', classId: activeClass.id });
  return (
    <>
      <PageHeader
        title="布置作业"
        description="把各科作业放在一起，方便教室里的同学随时查看。"
        actions={
          <Button disabled={!activeClass} onClick={create}>
            <Plus />
            新建作业
          </Button>
        }
      />
      <Alert>
        <Monitor />
        <AlertTitle>教室电脑右下角显示作业</AlertTitle>
        <AlertDescription>
          这里保存的是默认显示和置顶设置，教室电脑也可自行调整。修改保存后，新设置会同步到电脑。
        </AlertDescription>
      </Alert>
      {classes.isLoading ? (
        <Loading />
      ) : classes.error && !classes.data ? (
        <ErrorState error={classes.error} retry={() => void classes.refetch()} />
      ) : !classes.data?.length ? (
        <EmptyState
          icon={<GraduationCap />}
          title="先加入一个班级"
          description="加入或创建班级后，就可以为这间教室布置作业。"
          action={
            <Button nativeButton={false} render={<Link to="/classes" />}>
              前往我的班级
            </Button>
          }
        />
      ) : (
        <>
          {classes.error && <ErrorState error={classes.error} retry={() => void classes.refetch()} />}
          <div className="flex flex-wrap items-end justify-between gap-4">
            <div className="grid w-full gap-2 sm:w-64">
              <Label htmlFor="homework-class-filter">选择班级</Label>
              <SimpleSelect
                id="homework-class-filter"
                label="选择班级"
                value={activeClass?.id ?? ''}
                onValueChange={setSelectedId}
                options={classes.data.map((item) => ({ value: item.id, label: item.name }))}
              />
            </div>
            <p className="text-sm text-muted-foreground">每个班级最多保留 12 条作业</p>
          </div>
          {query.isLoading ? (
            <Loading />
          ) : query.error && !query.data ? (
            <ErrorState error={query.error} retry={() => void query.refetch()} />
          ) : (
            <>
              {query.error && <ErrorState error={query.error} retry={() => void query.refetch()} />}
              <div>
                <h2 className="text-lg font-semibold">班级作业</h2>
                <p className="text-sm text-muted-foreground">
                  {activeClass?.name} · 共 {items.length} 条
                </p>
              </div>
              {!items.length ? (
                <EmptyState
                  icon={<BookOpenCheck />}
                  title="还没有布置作业"
                  description="新建第一条作业，让同学在教室电脑上看到学习任务。"
                  action={
                    <Button onClick={create}>
                      <Plus />
                      新建作业
                    </Button>
                  }
                />
              ) : (
                <>
                  <div className="grid gap-4 md:grid-cols-2">
                    {pager.pageItems.map((item) => {
                      const manageable = mayManage(item);
                      const busy = !manageable || updateOption.isPending || remove.isPending;
                      return (
                        <Card className="homework-card" key={item.id}>
                          <CardHeader>
                            <CardTitle role="heading" aria-level={3} className="flex items-center gap-2">
                              <BookOpenCheck className="size-4 text-primary" />
                              {item.subject}
                            </CardTitle>
                            <CardDescription>{item.teacher_name}</CardDescription>
                            <CardAction>
                              {manageable ? (
                                <div className="flex gap-1">
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    onClick={() => setEditor({ kind: 'edit', item })}
                                  >
                                    <Pencil />
                                    编辑
                                  </Button>
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    className="text-destructive hover:text-destructive"
                                    onClick={() => setDeleting(item)}
                                  >
                                    <Trash2 />
                                    删除
                                  </Button>
                                </div>
                              ) : (
                                <Badge variant="outline">仅查看</Badge>
                              )}
                            </CardAction>
                          </CardHeader>
                          <CardContent className="flex-1">
                            <p className="text-sm leading-relaxed whitespace-pre-wrap break-words">
                              {item.text}
                            </p>
                          </CardContent>
                          <CardFooter className="flex-wrap justify-between gap-3 border-t">
                            <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                              <Clock3 className="size-3.5" />
                              更新于 {formatTime(item.updated_at)}
                            </span>
                            <div className="flex items-center gap-4 text-sm">
                              <label className="flex items-center gap-2">
                                显示
                                <Switch
                                  aria-label={`显示${item.subject}作业`}
                                  checked={item.visible}
                                  disabled={busy}
                                  onCheckedChange={(value) =>
                                    updateOption.mutate({ item, field: 'visible', value })
                                  }
                                />
                              </label>
                              <label className="flex items-center gap-2">
                                置顶
                                <Switch
                                  aria-label={`置顶${item.subject}作业`}
                                  checked={item.always_on_top}
                                  disabled={busy}
                                  onCheckedChange={(value) =>
                                    updateOption.mutate({ item, field: 'always_on_top', value })
                                  }
                                />
                              </label>
                            </div>
                          </CardFooter>
                        </Card>
                      );
                    })}
                  </div>
                  <PaginationBar
                    page={pager.page}
                    pageCount={pager.pageCount}
                    total={pager.total}
                    onPageChange={pager.setPage}
                    className="rounded-xl border px-4"
                  />
                </>
              )}
            </>
          )}
        </>
      )}
      {editor && classes.data && (
        <HomeworkEditor
          key={editor.kind === 'create' ? `new-${editor.classId}` : editor.item.id}
          editor={editor}
          classes={classes.data}
          teacherName={user?.display_name ?? '老师'}
          onClose={() => setEditor(null)}
          onSaved={(classId) => {
            setSelectedId(classId);
            setEditor(null);
          }}
        />
      )}
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
        title="删除这条作业？"
        description={deleting ? `删除后，「${deleting.subject}」作业将从教室电脑移除。` : ''}
        confirmLabel={remove.isPending ? '正在删除…' : '确认删除'}
        pending={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.id)}
      />
    </>
  );
}

function HomeworkEditor({
  editor,
  classes,
  teacherName,
  onClose,
  onSaved,
}: {
  editor: Editor;
  classes: Class[];
  teacherName: string;
  onClose: () => void;
  onSaved: (classId: string) => void;
}) {
  const cache = useQueryClient();
  const item = editor.kind === 'edit' ? editor.item : null;
  const [classId, setClassId] = useState(editor.kind === 'create' ? editor.classId : editor.item.class_id);
  const [subject, setSubject] = useState(item?.subject ?? '');
  const [text, setText] = useState(item?.text ?? '');
  const [visible, setVisible] = useState(item?.visible ?? true);
  const [alwaysOnTop, setAlwaysOnTop] = useState(item?.always_on_top ?? true);
  const subjectLength = lengthOf(subject);
  const textLength = lengthOf(text);
  const valid = !!classId && !!subject.trim() && !!text.trim() && subjectLength <= 24 && textLength <= 300;
  const save = useMutation({
    mutationFn: () => {
      const fields: HomeworkFields = {
        subject: subject.trim(),
        text: text.trim(),
        visible,
        always_on_top: alwaysOnTop,
      };
      return item
        ? api<Homework>(`/homeworks/${encodeURIComponent(item.id)}`, {
            method: 'PATCH',
            body: JSON.stringify(fields),
          })
        : api<Homework>('/homeworks', {
            method: 'POST',
            body: JSON.stringify({ class_id: classId, ...fields }),
          });
    },
    onSuccess: async () => {
      await cache.invalidateQueries({ queryKey: ['homeworks'] });
      toast.success(item ? '作业已保存' : '作业已发布');
      onSaved(classId);
    },
  });
  function submit(event: FormEvent) {
    event.preventDefault();
    if (valid && !save.isPending) save.mutate();
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !save.isPending) onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{item ? '编辑作业' : '新建作业'}</DialogTitle>
          <DialogDescription>
            {item ? '修改内容或默认显示设置。' : '选择一个班级，以你的姓名发布作业。'}
          </DialogDescription>
        </DialogHeader>
        <form id="homework-form" onSubmit={submit}>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="homework-class">班级</FieldLabel>
              {item ? (
                <Input
                  id="homework-class"
                  value={classes.find((c) => c.id === classId)?.name ?? '当前班级'}
                  readOnly
                  className="bg-muted"
                />
              ) : (
                <SimpleSelect
                  id="homework-class"
                  label="班级"
                  value={classId}
                  onValueChange={setClassId}
                  options={classes.map((c) => ({ value: c.id, label: c.name }))}
                />
              )}
            </Field>
            <Field data-invalid={subjectLength > 24 || undefined}>
              <FieldLabel htmlFor="homework-subject">科目</FieldLabel>
              <Input
                id="homework-subject"
                required
                value={subject}
                onChange={(event) => setSubject(event.target.value)}
                placeholder="例如：语文"
                aria-invalid={subjectLength > 24}
                aria-describedby="homework-subject-count"
              />
              <FieldDescription
                id="homework-subject-count"
                className={cn('text-right tabular-nums', subjectLength > 24 && 'text-destructive')}
              >
                {subjectLength} / 24 字
              </FieldDescription>
            </Field>
            <Field data-invalid={textLength > 300 || undefined}>
              <FieldLabel htmlFor="homework-text">作业内容</FieldLabel>
              <Textarea
                id="homework-text"
                required
                value={text}
                onChange={(event) => setText(event.target.value)}
                placeholder="例如：完成课本第 20 页练习"
                className="min-h-28"
                aria-invalid={textLength > 300}
                aria-describedby="homework-text-count"
              />
              <FieldDescription
                id="homework-text-count"
                className={cn('text-right tabular-nums', textLength > 300 && 'text-destructive')}
              >
                {textLength} / 300 字
              </FieldDescription>
            </Field>
            <Field orientation="horizontal">
              <FieldContent>
                <FieldLabel htmlFor="homework-visible">默认显示</FieldLabel>
                <FieldDescription>在教室电脑上显示这条作业</FieldDescription>
              </FieldContent>
              <Switch
                id="homework-visible"
                aria-label="默认显示"
                checked={visible}
                onCheckedChange={setVisible}
              />
            </Field>
            <Field orientation="horizontal">
              <FieldContent>
                <FieldLabel htmlFor="homework-top">默认置顶</FieldLabel>
                <FieldDescription>让作业窗口保持在其他窗口上方</FieldDescription>
              </FieldContent>
              <Switch
                id="homework-top"
                aria-label="默认置顶"
                checked={alwaysOnTop}
                onCheckedChange={setAlwaysOnTop}
              />
            </Field>
            <p className="text-sm text-muted-foreground">发布老师：{item?.teacher_name ?? teacherName}</p>
            {save.error && <FieldError role="alert">{save.error.message}</FieldError>}
          </FieldGroup>
        </form>
        <DialogFooter>
          <Button variant="outline" disabled={save.isPending} onClick={onClose}>
            取消
          </Button>
          <Button type="submit" form="homework-form" disabled={!valid || save.isPending}>
            {save.isPending && <Spinner />}
            {save.isPending ? '正在保存…' : item ? '保存修改' : '发布作业'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
