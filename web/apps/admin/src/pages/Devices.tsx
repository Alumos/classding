import { useState, type FormEvent } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@classding/api';
import { toast } from 'sonner';
import { Monitor, Server, TriangleAlert } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@classding/ui/components/ui/alert';
import { Badge } from '@classding/ui/components/ui/badge';
import { Button } from '@classding/ui/components/ui/button';
import { Checkbox } from '@classding/ui/components/ui/checkbox';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@classding/ui/components/ui/dialog';
import { Field, FieldDescription, FieldError, FieldLabel } from '@classding/ui/components/ui/field';
import { Input } from '@classding/ui/components/ui/input';
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from '@classding/ui/components/ui/table';
import {
  ConfirmDialog,
  PageHeader,
  PaginationBar,
  SimpleSelect,
  usePagination,
} from '@classding/ui/components/classding';
import { saveDeviceConfigurations } from '../deviceConfig';
import {
  DataTable,
  Panel,
  ResultState,
  SearchInput,
  Toolbar,
  dateTime,
  useSession,
  type Classroom,
  type Device,
} from '../lib';
import { ALL_SCHOOLS, SchoolFilter, useSchools } from '../schools';

const statusOptions = [
  { value: 'all', label: '全部状态' },
  { value: 'online', label: '在线' },
  { value: 'offline', label: '离线' },
];

/** Mirrors the screen client: it accepts http(s) site roots only. */
function normalizeServer(value: string) {
  const trimmed = value.trim();
  if (!/^https?:\/\/[^/?#\\@\s]+\/?$/i.test(trimmed)) return null;
  try {
    return new URL(trimmed).origin;
  } catch {
    return null;
  }
}

function switchState(
  device: Device,
): { text: string; variant: 'default' | 'secondary' | 'destructive' } | null {
  const plan = device.remote_config;
  if (!plan) return null;
  if (device.online && device.applied_revision === plan.revision)
    return { text: '切换失败，已回退', variant: 'destructive' };
  if (device.online) return { text: '等待设备切换', variant: 'secondary' };
  return { text: '已切换（本站离线）', variant: 'default' };
}

type Switch = {
  targets: Device[];
  url: string;
  step: 'edit' | 'confirm';
  error: string;
  failures: { device: Device; message: string }[];
};

export default function DevicesPage() {
  const { user } = useSession();
  const superAdmin = user.role === 'super_admin';
  const queryClient = useQueryClient();
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState('all');
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [switching, setSwitching] = useState<Switch | null>(null);
  const [cancelling, setCancelling] = useState<Device | null>(null);
  const query = useQuery({ queryKey: ['admin', 'devices'], queryFn: () => api<Device[]>('/admin/devices') });
  const classes = useQuery({
    queryKey: ['admin', 'classes'],
    queryFn: () => api<Classroom[]>('/admin/classes'),
  });
  const [school, setSchool] = useState(ALL_SCHOOLS);
  const { matches } = useSchools();
  const classOf = (id: string) => classes.data?.find((item) => item.id === id);
  const className = (id: string) => classOf(id)?.name ?? id;
  const devices = (query.data ?? []).filter(
    (item) =>
      matches(school, classOf(item.class_id)?.org_id) &&
      `${item.name} ${className(item.class_id)} ${item.os ?? ''} ${item.client_version ?? ''} ${(item.lan_ips ?? []).join(' ')} ${item.server_url ?? ''}`
        .toLowerCase()
        .includes(search.trim().toLowerCase()) &&
      (status === 'all' || (status === 'online' ? item.online : !item.online)),
  );
  const pager = usePagination(devices, 10, `${search}|${status}|${school}`);
  const selectedDevices = (query.data ?? []).filter((item) => selectedIds.has(item.id));
  const allVisibleSelected = devices.length > 0 && devices.every((item) => selectedIds.has(item.id));
  const save = useMutation({
    mutationFn: ({ items, url }: { items: Device[]; url: string }) => saveDeviceConfigurations(items, url),
    onSuccess: ({ errors, saved, url }) => {
      void queryClient.invalidateQueries({ queryKey: ['admin', 'devices'] });
      setSelectedIds(new Set(errors.map((item) => item.device.id)));
      setCancelling(null);
      if (errors.length) {
        setSwitching(
          (current) =>
            current && {
              ...current,
              targets: errors.map((item) => item.device),
              failures: errors,
              step: 'edit',
            },
        );
        toast.error(`${saved} 台已下发，${errors.length} 台失败，请检查后重试`);
      } else {
        setSwitching(null);
        toast.success(url ? `已向 ${saved} 台设备下发新服务器地址` : `已撤销 ${saved} 台设备的切换`);
      }
    },
  });
  function openSwitch(items: Device[]) {
    setSwitching({
      targets: items,
      url: items.length === 1 ? (items[0].remote_config?.server_url ?? '') : '',
      step: 'edit',
      error: '',
      failures: [],
    });
  }
  function next(event: FormEvent) {
    event.preventDefault();
    if (!switching) return;
    const url = normalizeServer(switching.url);
    if (!url) {
      setSwitching({ ...switching, error: '请填写 http:// 或 https:// 开头的站点根地址，不包含路径。' });
      return;
    }
    setSwitching({ ...switching, url, step: 'confirm', error: '' });
  }
  function toggleVisible() {
    setSelectedIds((previous) => {
      const next = new Set(previous);
      for (const device of devices)
        if (allVisibleSelected) next.delete(device.id);
        else next.add(device.id);
      return next;
    });
  }
  function toggleOne(id: string, checked: boolean) {
    setSelectedIds((previous) => {
      const next = new Set(previous);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
  }
  function closeSwitch() {
    if (!save.isPending) setSwitching(null);
  }
  return (
    <>
      <PageHeader title="设备管理" description="查看教室大屏的连接状态、内网地址和正在使用的服务器。" />
      <Panel
        title="设备列表"
        description="连接状态以服务端数据为准，内网 IP 由大屏在连接时上报"
        action={
          <span className="text-sm text-muted-foreground">
            {query.data ? `${query.data.length} 台设备` : ''}
          </span>
        }
      >
        <Toolbar count={query.data ? `共 ${devices.length} 条` : undefined}>
          <SearchInput value={search} onChange={setSearch} placeholder="搜索设备、班级、IP 或服务器" />
          <SimpleSelect
            label="筛选设备状态"
            value={status}
            onValueChange={setStatus}
            options={statusOptions}
            className="w-full sm:w-32"
          />
          <SchoolFilter value={school} onChange={setSchool} />
          {superAdmin && (
            <Button
              variant="outline"
              disabled={!selectedDevices.length}
              onClick={() => openSwitch(selectedDevices)}
            >
              <Server />
              切换服务器（{selectedDevices.length}）
            </Button>
          )}
        </Toolbar>
        <ResultState
          loading={query.isLoading}
          error={query.error}
          empty={!devices.length}
          emptyText="没有找到设备"
        >
          <DataTable>
            <TableHeader>
              <TableRow>
                {superAdmin && (
                  <TableHead className="w-10">
                    <Checkbox
                      aria-label="选择全部筛选设备"
                      checked={allVisibleSelected}
                      onCheckedChange={toggleVisible}
                    />
                  </TableHead>
                )}
                <TableHead>设备</TableHead>
                <TableHead>连接状态</TableHead>
                <TableHead>内网 IP</TableHead>
                <TableHead>当前服务器</TableHead>
                {superAdmin && (
                  <>
                    <TableHead>服务器切换</TableHead>
                    <TableHead className="text-right">操作</TableHead>
                  </>
                )}
              </TableRow>
            </TableHeader>
            <TableBody>
              {pager.pageItems.map((item) => {
                const state = switchState(item);
                return (
                  <TableRow
                    key={item.id}
                    data-device-id={item.id}
                    data-state={selectedIds.has(item.id) ? 'selected' : undefined}
                  >
                    {superAdmin && (
                      <TableCell>
                        <Checkbox
                          aria-label={`选择设备 ${item.name} ${item.id}`}
                          checked={selectedIds.has(item.id)}
                          onCheckedChange={(checked) => toggleOne(item.id, checked === true)}
                        />
                      </TableCell>
                    )}
                    <TableCell>
                      <div className="person-cell flex items-center gap-3">
                        <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted">
                          <Monitor className="size-4" />
                        </span>
                        <div className="min-w-0">
                          <div className="font-medium">{item.name}</div>
                          <div className="text-xs text-muted-foreground">
                            {className(item.class_id)} · {item.os || '—'}{' '}
                            {item.client_version ? `v${item.client_version}` : ''}
                          </div>
                        </div>
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge variant={item.online ? 'default' : 'secondary'}>
                        {item.online ? '在线' : '离线'}
                      </Badge>
                      <div className="mt-1 text-xs text-muted-foreground">{dateTime(item.last_seen_at)}</div>
                    </TableCell>
                    <TableCell>
                      <div className="font-mono text-sm">
                        {item.lan_ips?.length ? (
                          item.lan_ips.join('、')
                        ) : (
                          <span className="font-sans text-muted-foreground">未上报</span>
                        )}
                      </div>
                      {item.public_ip && (
                        <div className="text-xs text-muted-foreground">出口 {item.public_ip}</div>
                      )}
                    </TableCell>
                    <TableCell className="max-w-56 truncate font-mono text-sm" title={item.server_url ?? ''}>
                      {item.server_url || <span className="font-sans text-muted-foreground">未上报</span>}
                    </TableCell>
                    {superAdmin && (
                      <>
                        <TableCell className="min-w-44">
                          {state && item.remote_config ? (
                            <div className="flex flex-col items-start gap-1">
                              <Badge variant={state.variant}>{state.text}</Badge>
                              <span
                                className="max-w-56 truncate font-mono text-xs text-muted-foreground"
                                title={item.remote_config.server_url}
                              >
                                → {item.remote_config.server_url}
                              </span>
                            </div>
                          ) : (
                            <span className="text-muted-foreground">未设置</span>
                          )}
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end gap-1">
                            <Button variant="ghost" size="sm" onClick={() => openSwitch([item])}>
                              切换服务器
                            </Button>
                            {item.remote_config && (
                              <Button variant="ghost" size="sm" onClick={() => setCancelling(item)}>
                                撤销
                              </Button>
                            )}
                          </div>
                        </TableCell>
                      </>
                    )}
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
      <Dialog
        open={!!switching}
        onOpenChange={(open) => {
          if (!open) closeSwitch();
        }}
      >
        <DialogContent className="sm:max-w-lg">
          {switching?.step === 'confirm' ? (
            <>
              <DialogHeader>
                <DialogTitle>确认新服务器地址</DialogTitle>
                <DialogDescription>
                  请再次核对地址，确认后会立即下发到 {switching.targets.length} 台设备。
                </DialogDescription>
              </DialogHeader>
              <div className="grid gap-4">
                <div className="rounded-lg border bg-muted/50 p-4 text-center">
                  <p className="text-xs text-muted-foreground">新服务器地址</p>
                  <p
                    className="mt-1 font-mono text-lg font-semibold break-all"
                    data-testid="confirm-server-url"
                  >
                    {switching.url}
                  </p>
                </div>
                <div className="max-h-32 overflow-y-auto rounded-lg border p-3 text-sm">
                  {switching.targets.map((device) => (
                    <div key={device.id} className="flex items-center gap-2 py-0.5">
                      <Monitor className="size-3.5 text-muted-foreground" />
                      <span className="truncate">{device.name}</span>
                      <span className="ml-auto shrink-0 truncate text-xs text-muted-foreground">
                        当前 <span className="font-mono">{device.server_url || '未上报'}</span>
                      </span>
                    </div>
                  ))}
                </div>
                <Alert variant="destructive">
                  <TriangleAlert />
                  <AlertTitle>设备会立即离开本服务器</AlertTitle>
                  <AlertDescription>
                    收到地址后，大屏会断开当前服务器并连接新地址，在本后台显示为离线。新服务器需已有这些设备（例如已恢复本站备份）；如果新服务器不认可设备或
                    10 分钟内无法连接，大屏会自动回到本服务器。
                  </AlertDescription>
                </Alert>
              </div>
              <DialogFooter>
                <Button
                  variant="outline"
                  disabled={save.isPending}
                  onClick={() => setSwitching({ ...switching, step: 'edit' })}
                >
                  返回修改
                </Button>
                <Button
                  variant="destructive"
                  disabled={save.isPending}
                  onClick={() => save.mutate({ items: switching.targets, url: switching.url })}
                >
                  {save.isPending ? '正在下发…' : `确认切换 ${switching.targets.length} 台设备`}
                </Button>
              </DialogFooter>
            </>
          ) : (
            switching && (
              <>
                <DialogHeader>
                  <DialogTitle>切换服务器</DialogTitle>
                  <DialogDescription>
                    为 {switching.targets.length} 台设备下发新的服务器地址，下一步需要再次确认。
                  </DialogDescription>
                </DialogHeader>
                <form id="device-switch-form" onSubmit={next} className="grid gap-4">
                  <Field data-invalid={!!switching.error || undefined}>
                    <FieldLabel htmlFor="device-server-url">新服务器地址</FieldLabel>
                    <Input
                      id="device-server-url"
                      autoFocus
                      placeholder="例如：https://classding.example.com"
                      value={switching.url}
                      aria-invalid={!!switching.error}
                      onChange={(event) => setSwitching({ ...switching, url: event.target.value, error: '' })}
                    />
                    <FieldDescription>
                      填写站点根地址，例如 http://192.168.1.10:8080 或 https://classding.example.com。
                    </FieldDescription>
                    {switching.error && <FieldError>{switching.error}</FieldError>}
                  </Field>
                  {switching.failures.length > 0 && (
                    <FieldError role="alert">
                      <strong>以下设备下发失败，可修改后重试：</strong>
                      <ul className="mt-1 list-disc pl-4">
                        {switching.failures.map(({ device, message }) => (
                          <li key={device.id}>
                            {device.name}（{device.id}）：{message}
                          </li>
                        ))}
                      </ul>
                    </FieldError>
                  )}
                </form>
                <DialogFooter>
                  <Button variant="outline" onClick={closeSwitch}>
                    取消
                  </Button>
                  <Button type="submit" form="device-switch-form">
                    下一步
                  </Button>
                </DialogFooter>
              </>
            )
          )}
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={!!cancelling}
        onOpenChange={(open) => {
          if (!open) setCancelling(null);
        }}
        title="撤销服务器切换？"
        description={
          cancelling ? `仍连接本服务器的「${cancelling.name}」将不再切换；已经切换的设备不受影响。` : ''
        }
        confirmLabel={save.isPending ? '正在撤销…' : '确认撤销'}
        destructive={false}
        pending={save.isPending}
        onConfirm={() => cancelling && save.mutate({ items: [cancelling], url: '' })}
      />
    </>
  );
}
