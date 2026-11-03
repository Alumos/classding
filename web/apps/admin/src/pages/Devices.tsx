import { useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@classding/api';
import { toast } from 'sonner';
import { Monitor, Settings2 } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@classding/ui/components/ui/alert';
import { Badge } from '@classding/ui/components/ui/badge';
import { Button } from '@classding/ui/components/ui/button';
import { Checkbox } from '@classding/ui/components/ui/checkbox';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@classding/ui/components/ui/dialog';
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@classding/ui/components/ui/field';
import { Input } from '@classding/ui/components/ui/input';
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from '@classding/ui/components/ui/table';
import { PageHeader, SimpleSelect } from '@classding/ui/components/classding';
import { saveDeviceConfigurations } from '../deviceConfig';
import { DataTable, Panel, ResultState, SearchInput, Toolbar, dateTime, useSession, type Device } from '../lib';

const statusOptions = [{ value: 'all', label: '全部状态' }, { value: 'online', label: '在线' }, { value: 'offline', label: '离线' }];

export default function DevicesPage() {
  const { user } = useSession();
  const superAdmin = user.role === 'super_admin';
  const queryClient = useQueryClient();
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState('all');
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [targets, setTargets] = useState<Device[]>([]);
  const [serverUrl, setServerUrl] = useState('');
  const [failures, setFailures] = useState<{ device: Device; message: string }[]>([]);
  const query = useQuery({ queryKey: ['admin', 'devices'], queryFn: () => api<Device[]>('/admin/devices') });
  const devices = (query.data ?? []).filter(item => `${item.name} ${item.class_id} ${item.os ?? ''} ${item.client_version ?? ''}`.toLowerCase().includes(search.trim().toLowerCase()) && (status === 'all' || (status === 'online' ? item.online : !item.online)));
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
      for (const device of devices) if (allVisibleSelected) next.delete(device.id); else next.add(device.id);
      return next;
    });
  }
  function toggleOne(id: string, checked: boolean) {
    setSelectedIds(previous => { const next = new Set(previous); if (checked) next.add(id); else next.delete(id); return next; });
  }
  function submitConfig(event: FormEvent) {
    event.preventDefault();
    if (targets.length && !saveConfig.isPending) saveConfig.mutate({ items: targets, url: serverUrl.trim() });
  }
  function closeConfig() { if (!saveConfig.isPending) { setTargets([]); setFailures([]); } }
  return <>
    <PageHeader title="设备管理" description="了解教室大屏的在线状态和客户端信息。" />
    <Panel title="设备列表" description="连接状态以服务端数据为准" action={<span className="text-sm text-muted-foreground">{query.data ? `${query.data.length} 台设备` : ''}</span>}>
      <Toolbar count={query.data ? `共 ${devices.length} 条` : undefined}>
        <SearchInput value={search} onChange={setSearch} placeholder="搜索设备、班级 ID 或版本" />
        <SimpleSelect label="筛选设备状态" value={status} onValueChange={setStatus} options={statusOptions} className="w-full sm:w-32" />
        {superAdmin && <Button variant="outline" disabled={!selectedDevices.length} onClick={() => openConfig(selectedDevices)}><Settings2 />配置所选设备（{selectedDevices.length}）</Button>}
      </Toolbar>
      <ResultState loading={query.isLoading} error={query.error} empty={!devices.length} emptyText="没有找到设备">
        <DataTable>
          <TableHeader><TableRow>
            {superAdmin && <TableHead className="w-10"><Checkbox aria-label="选择全部筛选设备" checked={allVisibleSelected} onCheckedChange={toggleVisible} /></TableHead>}
            <TableHead>设备名称</TableHead><TableHead>连接状态</TableHead><TableHead>班级 ID</TableHead><TableHead>系统 / 版本</TableHead><TableHead>最后心跳</TableHead><TableHead>IP 地址</TableHead>
            {superAdmin && <><TableHead>服务器切换计划</TableHead><TableHead className="text-right">操作</TableHead></>}
          </TableRow></TableHeader>
          <TableBody>{devices.map(item => <TableRow key={item.id} data-state={selectedIds.has(item.id) ? 'selected' : undefined}>
            {superAdmin && <TableCell><Checkbox aria-label={`选择设备 ${item.name} ${item.id}`} checked={selectedIds.has(item.id)} onCheckedChange={checked => toggleOne(item.id, checked === true)} /></TableCell>}
            <TableCell>
              <div className="person-cell flex items-center gap-3">
                <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted"><Monitor className="size-4" /></span>
                <div className="min-w-0"><div className="font-medium">{item.name}</div><small className="text-xs text-muted-foreground">{item.id}</small></div>
              </div>
            </TableCell>
            <TableCell><Badge variant={item.online ? 'default' : 'secondary'}>{item.online ? '在线' : '离线'}</Badge></TableCell>
            <TableCell className="text-muted-foreground">{item.class_id}</TableCell>
            <TableCell>{item.os || '—'} <span className="text-muted-foreground">/ {item.client_version || '—'}</span></TableCell>
            <TableCell className="text-muted-foreground">{dateTime(item.last_seen_at)}</TableCell>
            <TableCell className="font-mono text-xs">{item.last_ip || '—'}</TableCell>
            {superAdmin && <>
              <TableCell>{item.remote_config?.server_url ? <div><div className="font-medium">{item.remote_config.server_url}</div><div className="text-xs text-muted-foreground">配置已保存，{item.online ? '等待设备接收' : '等待设备上线'} · 版本 {item.remote_config.revision}</div></div> : <span className="text-muted-foreground">未设置</span>}</TableCell>
              <TableCell className="text-right"><Button variant="ghost" size="sm" onClick={() => openConfig([item])}>配置服务器</Button></TableCell>
            </>}
          </TableRow>)}</TableBody>
        </DataTable>
      </ResultState>
    </Panel>
    <Dialog open={targets.length > 0} onOpenChange={open => { if (!open) closeConfig(); }}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader><DialogTitle>配置设备服务器</DialogTitle><DialogDescription>将为 {targets.length} 台设备保存服务器切换计划。</DialogDescription></DialogHeader>
        <form id="device-config-form" onSubmit={submitConfig}>
          <FieldGroup>
            <Alert>
              <AlertTitle>切换前请先完成以下准备</AlertTitle>
              <AlertDescription>
                <p>请先为设备安装支持迁移的新版本客户端，并确保新站在恢复最终备份前无法使用原设备令牌通过验证。</p>
                <ol className="mt-2 list-decimal space-y-1 pl-4">
                  <li>保存新服务器地址，下发切换计划。</li>
                  <li>暂停在旧站发送消息及其他会修改数据的操作。</li>
                  <li>从旧站导出最终备份并恢复到新站，确认新站域名和 HTTPS 配置正确。</li>
                  <li>客户端验证原设备令牌和数据后切换；离线设备会在上线后接收计划。</li>
                </ol>
                <p className="mt-2">备份后请勿继续写入旧站，两站不会实时同步。配置已保存不代表设备已经接收或切换。</p>
              </AlertDescription>
            </Alert>
            <Field>
              <FieldLabel htmlFor="device-server-url">服务器地址</FieldLabel>
              <Input id="device-server-url" type="url" placeholder="例如：https://classding.example.com" value={serverUrl} onChange={event => setServerUrl(event.target.value)} />
              <FieldDescription>请填写 HTTP 或 HTTPS 站点根地址；留空将取消这些设备的切换计划。</FieldDescription>
            </Field>
            {failures.length > 0 && <FieldError role="alert"><strong>以下设备保存失败，可修改后重试：</strong><ul className="mt-1 list-disc pl-4">{failures.map(({ device, message }) => <li key={device.id}>{device.name}（{device.id}）：{message}</li>)}</ul></FieldError>}
          </FieldGroup>
        </form>
        <DialogFooter><Button variant="outline" disabled={saveConfig.isPending} onClick={closeConfig}>取消</Button><Button type="submit" form="device-config-form" disabled={saveConfig.isPending}>{saveConfig.isPending ? '正在保存…' : '保存配置'}</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  </>;
}
