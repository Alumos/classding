import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '@classding/api';
import { Badge } from '@classding/ui/components/ui/badge';
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from '@classding/ui/components/ui/table';
import { PageHeader, PaginationBar, usePagination } from '@classding/ui/components/classding';
import { DataTable, Panel, ResultState, SearchInput, Toolbar, dateTime, type AuditLog } from '../lib';

export default function AuditPage() {
  const [search, setSearch] = useState('');
  const query = useQuery({
    queryKey: ['admin', 'audit-logs'],
    queryFn: () => api<AuditLog[]>('/admin/audit-logs'),
  });
  const logs = (query.data ?? []).filter((item) =>
    `${item.actor} ${item.action} ${item.target}`.toLowerCase().includes(search.trim().toLowerCase()),
  );
  const pager = usePagination(logs, 10, search);
  return (
    <>
      <PageHeader title="操作日志" description="查看后台操作记录，让每一次管理都有迹可循。" />
      <Panel
        title="操作记录"
        description="记录管理员在后台的操作"
        action={
          <span className="text-sm text-muted-foreground">
            {query.data ? `${query.data.length} 条记录` : ''}
          </span>
        }
      >
        <Toolbar count={query.data ? `共 ${logs.length} 条` : undefined}>
          <SearchInput value={search} onChange={setSearch} placeholder="搜索操作人、操作或对象" />
        </Toolbar>
        <ResultState
          loading={query.isLoading}
          error={query.error}
          empty={!logs.length}
          emptyText="没有找到操作记录"
        >
          <DataTable>
            <TableHeader>
              <TableRow>
                <TableHead>操作时间</TableHead>
                <TableHead>操作人</TableHead>
                <TableHead>操作</TableHead>
                <TableHead>操作对象</TableHead>
                <TableHead>记录 ID</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {pager.pageItems.map((item) => (
                <TableRow key={item.id}>
                  <TableCell className="text-muted-foreground">{dateTime(item.at)}</TableCell>
                  <TableCell className="font-medium">{item.actor || '—'}</TableCell>
                  <TableCell>
                    <Badge variant="secondary" className="font-mono">
                      {item.action}
                    </Badge>
                  </TableCell>
                  <TableCell>{item.target || '—'}</TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">{item.id}</TableCell>
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
    </>
  );
}
