import { useEffect, useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@classding/api';
import { toast } from 'sonner';
import { Download, TriangleAlert, Upload } from 'lucide-react';
import { Alert, AlertDescription } from '@classding/ui/components/ui/alert';
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
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@classding/ui/components/ui/field';
import { Input } from '@classding/ui/components/ui/input';
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemSeparator,
  ItemTitle,
} from '@classding/ui/components/ui/item';
import { ErrorState, LoadingState, PageHeader, SimpleSelect } from '@classding/ui/components/classding';
import { binaryRequest } from '../binaryRequest';
import { errorMessage, json, localDate, useSession, type SiteSettings } from '../lib';

const swatch = (color: string) => <span className="size-3 rounded-full" style={{ background: color }} />;
const themes = [
  { value: 'green', label: '森林绿', icon: swatch('oklch(0.48 0.1 163)') },
  { value: 'blue', label: '海洋蓝', icon: swatch('oklch(0.5 0.15 258)') },
  { value: 'slate', label: '石板灰', icon: swatch('oklch(0.4 0.04 257)') },
];

function SettingsForm({
  initial,
  onSaved,
}: {
  initial: SiteSettings;
  onSaved: (settings: SiteSettings) => void;
}) {
  const [form, setForm] = useState(initial);
  const [formError, setFormError] = useState('');
  useEffect(() => setForm(initial), [initial]);
  const save = useMutation({
    mutationFn: (settings: SiteSettings) => api<SiteSettings>('/admin/settings', json('PUT', settings)),
    onSuccess: (settings) => {
      setForm(settings);
      onSaved(settings);
      toast.success('站点配置已保存');
    },
    onError: (error) => setFormError(errorMessage(error)),
  });
  function submit(event: FormEvent) {
    event.preventDefault();
    setFormError('');
    if (!form.site_name.trim()) {
      setFormError('请输入站点名称。');
      return;
    }
    if (Array.from(form.site_name.trim()).length > 60) {
      setFormError('站点名称不能超过 60 个字符。');
      return;
    }
    save.mutate({ ...form, site_name: form.site_name.trim(), icp_number: form.icp_number.trim() });
  }
  return (
    <form onSubmit={submit} className="flex flex-1 flex-col">
      <CardContent className="flex-1">
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="site-name">站点名称</FieldLabel>
            <Input
              id="site-name"
              required
              value={form.site_name}
              onChange={(event) => setForm({ ...form, site_name: event.target.value })}
            />
            <FieldDescription>最多 60 个字符，显示在两个后台的侧栏和页脚。</FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="site-icp">备案号</FieldLabel>
            <Input
              id="site-icp"
              value={form.icp_number}
              placeholder="选填，例如：京ICP备12345678号"
              onChange={(event) => setForm({ ...form, icp_number: event.target.value })}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="site-theme">主题</FieldLabel>
            <SimpleSelect
              id="site-theme"
              label="主题"
              value={form.theme}
              onValueChange={(value) => setForm({ ...form, theme: value as SiteSettings['theme'] })}
              options={themes}
            />
          </Field>
          {formError && <FieldError role="alert">{formError}</FieldError>}
        </FieldGroup>
      </CardContent>
      <CardFooter className="border-t">
        <Button type="submit" disabled={save.isPending}>
          {save.isPending ? '正在保存…' : '保存站点配置'}
        </Button>
      </CardFooter>
    </form>
  );
}

export default function SettingsPage() {
  const { logout } = useSession();
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: ['admin', 'settings'],
    queryFn: () => api<SiteSettings>('/admin/settings'),
  });
  const [restoreOpen, setRestoreOpen] = useState(false);
  const [restoreFile, setRestoreFile] = useState<File | null>(null);
  const [confirmation, setConfirmation] = useState('');
  const [restoreError, setRestoreError] = useState('');
  const backup = useMutation({
    mutationFn: async () => {
      const response = await binaryRequest('/admin/backup');
      const blob = await response.blob();
      const signature = new Uint8Array(await blob.slice(0, 4).arrayBuffer());
      if (
        signature.length < 4 ||
        signature[0] !== 0x50 ||
        signature[1] !== 0x4b ||
        !(
          (signature[2] === 0x03 && signature[3] === 0x04) ||
          (signature[2] === 0x05 && signature[3] === 0x06) ||
          (signature[2] === 0x07 && signature[3] === 0x08)
        )
      ) {
        throw new Error('服务端返回的备份文件不是 ZIP 文件。');
      }
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = `classding-backup-${localDate(new Date().toISOString())}.zip`;
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 1000);
    },
    onSuccess: () => toast.success('备份文件已开始下载'),
    onError: (error) => toast.error(`备份导出失败：${errorMessage(error)}`),
  });
  const restore = useMutation({
    mutationFn: async (file: File) => {
      const response = await binaryRequest('/admin/restore', {
        method: 'POST',
        headers: { 'Content-Type': 'application/zip', 'X-ClassDing-Restore': 'replace' },
        body: file,
      });
      const result = (await response.json().catch(() => null)) as { ok?: boolean } | null;
      if (!result?.ok) throw new Error('服务端未确认恢复成功，请检查数据状态后再操作。');
    },
    onSuccess: () => {
      window.dispatchEvent(new Event('classding:site-settings'));
      logout();
      toast.success('全站数据已恢复，请重新登录');
    },
    onError: (error) => setRestoreError(errorMessage(error)),
  });
  function closeRestore() {
    if (restore.isPending) return;
    setRestoreOpen(false);
    setRestoreFile(null);
    setConfirmation('');
    setRestoreError('');
  }
  function submitRestore(event: FormEvent) {
    event.preventDefault();
    setRestoreError('');
    if (!restoreFile || !/\.zip$/i.test(restoreFile.name)) {
      setRestoreError('请选择 ZIP 格式的备份文件。');
      return;
    }
    if (restoreFile.size > 256 * 1024 * 1024) {
      setRestoreError('备份文件不能超过 256 MiB。');
      return;
    }
    if (confirmation !== '覆盖恢复') {
      setRestoreError('请输入“覆盖恢复”以确认操作。');
      return;
    }
    restore.mutate(restoreFile);
  }
  return (
    <>
      <PageHeader title="系统配置" description="管理站点展示、全站备份与数据恢复。" />
      <div className="grid items-start gap-4 lg:grid-cols-2">
        <Card className="settings-content">
          <CardHeader>
            <CardTitle>站点信息</CardTitle>
            <CardDescription>设置教师端与管理后台的站点名称、备案号和主题。</CardDescription>
          </CardHeader>
          {query.isLoading ? (
            <LoadingState />
          ) : query.error ? (
            <CardContent>
              <ErrorState error={query.error} retry={() => void query.refetch()} />
            </CardContent>
          ) : (
            query.data && (
              <SettingsForm
                initial={query.data}
                onSaved={(settings) => {
                  queryClient.setQueryData(['admin', 'settings'], settings);
                  window.dispatchEvent(new Event('classding:site-settings'));
                }}
              />
            )
          )}
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>全站备份与恢复</CardTitle>
            <CardDescription>备份包含全站数据和设备身份，请妥善保管文件。</CardDescription>
          </CardHeader>
          <CardContent>
            <ItemGroup className="rounded-lg border">
              <Item>
                <ItemContent>
                  <ItemTitle>导出全站备份</ItemTitle>
                  <ItemDescription>下载 ZIP 文件，用于迁移或恢复此站点的数据。</ItemDescription>
                </ItemContent>
                <ItemActions>
                  <Button
                    variant="outline"
                    disabled={backup.isPending || restore.isPending}
                    onClick={() => backup.mutate()}
                  >
                    <Download />
                    {backup.isPending ? '正在导出…' : '下载备份'}
                  </Button>
                </ItemActions>
              </Item>
              <ItemSeparator />
              <Item>
                <ItemContent>
                  <ItemTitle>导入备份并恢复</ItemTitle>
                  <ItemDescription>恢复会覆盖当前全站数据，操作完成后需要重新登录。</ItemDescription>
                </ItemContent>
                <ItemActions>
                  <Button
                    variant="destructive"
                    disabled={backup.isPending || restore.isPending}
                    onClick={() => setRestoreOpen(true)}
                  >
                    <Upload />
                    导入并恢复
                  </Button>
                </ItemActions>
              </Item>
            </ItemGroup>
          </CardContent>
        </Card>
      </div>
      <Dialog
        open={restoreOpen}
        onOpenChange={(open) => {
          if (!open) closeRestore();
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>确认覆盖恢复全站数据？</DialogTitle>
            <DialogDescription>
              恢复将用备份覆盖当前全站数据，此操作无法撤销。请先下载当前站点备份。
            </DialogDescription>
          </DialogHeader>
          <form id="restore-form" onSubmit={submitRestore}>
            <FieldGroup>
              <Alert variant="destructive">
                <TriangleAlert />
                <AlertDescription>
                  备份中的账号、消息及设备身份等数据将替换当前数据。恢复成功后，管理员需要重新登录。
                </AlertDescription>
              </Alert>
              <Field>
                <FieldLabel htmlFor="restore-file">选择备份文件</FieldLabel>
                <Input
                  id="restore-file"
                  type="file"
                  accept=".zip,application/zip"
                  onChange={(event) => {
                    setRestoreFile(event.target.files?.[0] ?? null);
                    setRestoreError('');
                  }}
                />
                <FieldDescription>ZIP 文件最大 256 MiB，解压后最大 512 MiB。</FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="restore-confirm">输入“覆盖恢复”确认</FieldLabel>
                <Input
                  id="restore-confirm"
                  value={confirmation}
                  onChange={(event) => setConfirmation(event.target.value)}
                  autoComplete="off"
                />
              </Field>
              {restoreError && <FieldError role="alert">{restoreError}</FieldError>}
            </FieldGroup>
          </form>
          <DialogFooter>
            <Button variant="outline" disabled={restore.isPending} onClick={closeRestore}>
              取消
            </Button>
            <Button
              variant="destructive"
              type="submit"
              form="restore-form"
              disabled={restore.isPending || !restoreFile || confirmation !== '覆盖恢复'}
            >
              {restore.isPending ? '正在恢复，请勿关闭页面…' : '确认覆盖恢复'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
