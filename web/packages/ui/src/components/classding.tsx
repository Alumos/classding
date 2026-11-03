import * as React from "react"
import { AlertCircle, ALargeSmall, BellRing, ChevronLeft, ChevronRight } from "lucide-react"

import { cn } from "@classding/ui/lib/utils"
import { copyText } from "@classding/ui/lib/copy"
import { useSiteSettings } from "../site"
import { Alert, AlertAction, AlertDescription, AlertTitle } from "@classding/ui/components/ui/alert"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@classding/ui/components/ui/alert-dialog"
import { Button } from "@classding/ui/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@classding/ui/components/ui/dialog"
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuTrigger } from "@classding/ui/components/ui/dropdown-menu"
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@classding/ui/components/ui/empty"
import { Pagination, PaginationContent, PaginationEllipsis, PaginationItem } from "@classding/ui/components/ui/pagination"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@classding/ui/components/ui/select"
import { Spinner } from "@classding/ui/components/ui/spinner"
import { Textarea } from "@classding/ui/components/ui/textarea"

export { useSiteSettings, SiteSettingsProvider, type SiteSettings } from "../site"

/** Product mark with the configurable site name. */
export function Brand({ subtitle, className }: { subtitle?: string; className?: string }) {
  const { site_name } = useSiteSettings()
  return (
    <div className={cn("flex min-w-0 items-center gap-2.5", className)}>
      <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground">
        <BellRing className="size-4" />
      </span>
      <span className="grid min-w-0 flex-1 text-left leading-tight">
        <span className="line-clamp-2 font-semibold break-all">{site_name}</span>
        {subtitle && <span className="truncate text-xs text-muted-foreground">{subtitle}</span>}
      </span>
    </div>
  )
}

/** Copyright and ICP filing number, centered under the content. */
export function SiteFooter({ className }: { className?: string }) {
  const { site_name, icp_number } = useSiteSettings()
  return (
    <footer className={cn("flex flex-col items-center gap-1 px-4 py-6 text-center text-xs text-muted-foreground", className)}>
      <span className="w-full break-all">© {site_name}</span>
      {icp_number && (
        <a className="w-full break-all underline-offset-4 hover:text-foreground hover:underline" href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer">
          {icp_number}
        </a>
      )}
    </footer>
  )
}

export function PageHeader({ title, description, actions, className }: { title: React.ReactNode; description?: React.ReactNode; actions?: React.ReactNode; className?: string }) {
  return (
    <div className={cn("flex flex-wrap items-end justify-between gap-4", className)}>
      <div className="min-w-0 space-y-1">
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        {description && <p className="text-sm text-muted-foreground">{description}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  )
}

export function LoadingState({ label = "正在获取最新数据…", className }: { label?: string; className?: string }) {
  return (
    <div className={cn("flex items-center justify-center gap-2 py-12 text-sm text-muted-foreground", className)}>
      <Spinner />
      {label}
    </div>
  )
}

export function ErrorState({ error, retry, title = "暂时无法获取数据" }: { error: unknown; retry?: () => void; title?: string }) {
  return (
    <Alert variant="destructive" role="alert">
      <AlertCircle />
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>{error instanceof Error ? error.message : "请检查网络与服务连接。"}</AlertDescription>
      {retry && (
        <AlertAction>
          <Button variant="outline" size="sm" onClick={retry}>重试</Button>
        </AlertAction>
      )}
    </Alert>
  )
}

export function EmptyState({ icon, title, description, action, className }: { icon: React.ReactNode; title: string; description?: string; action?: React.ReactNode; className?: string }) {
  return (
    <Empty className={cn("flex-none border border-dashed py-12", className)}>
      <EmptyHeader>
        <EmptyMedia variant="icon">{icon}</EmptyMedia>
        <EmptyTitle>{title}</EmptyTitle>
        {description && <EmptyDescription>{description}</EmptyDescription>}
      </EmptyHeader>
      {action && <EmptyContent>{action}</EmptyContent>}
    </Empty>
  )
}

export interface SelectOption {
  value: string
  label: string
  icon?: React.ReactNode
}

/** A labelled single-value select built on the shadcn Select. */
export function SimpleSelect({ value, onValueChange, options, id, label, placeholder = "请选择", disabled, className }: { value: string; onValueChange: (value: string) => void; options: SelectOption[]; id?: string; label: string; placeholder?: string; disabled?: boolean; className?: string }) {
  return (
    <Select items={options} value={value} onValueChange={next => { if (next !== null) onValueChange(String(next)) }} disabled={disabled}>
      <SelectTrigger id={id} aria-label={label} className={cn("w-full", className)}>
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false}>
        {options.map(option => (
          <SelectItem key={option.value} value={option.value}>
            {option.icon}
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

/** Copies text, falling back to a manual-copy dialog over plain HTTP. */
export function CopyButton({ text, onCopied, children, ...props }: Omit<React.ComponentProps<typeof Button>, "onClick"> & { text: string; onCopied?: () => void }) {
  const [manual, setManual] = React.useState(false)
  return (
    <>
      <Button {...props} onClick={async () => { if (await copyText(text)) onCopied?.(); else setManual(true) }}>
        {children ?? "复制"}
      </Button>
      <Dialog open={manual} onOpenChange={setManual}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>手动复制</DialogTitle>
            <DialogDescription>无法自动复制，请选中以下内容后手动复制。</DialogDescription>
          </DialogHeader>
          <Textarea aria-label="待复制内容" className="min-h-28 font-mono" value={text} readOnly onFocus={event => event.currentTarget.select()} onMouseUp={event => { event.preventDefault(); event.currentTarget.select() }} />
          <DialogFooter>
            <Button onClick={() => setManual(false)}>完成</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

/** Confirmation for destructive or irreversible actions. */
export function ConfirmDialog({ open, onOpenChange, title, description, confirmLabel = "确认", cancelLabel = "取消", pending, destructive = true, onConfirm, children }: { open: boolean; onOpenChange: (open: boolean) => void; title: string; description?: React.ReactNode; confirmLabel?: string; cancelLabel?: string; pending?: boolean; destructive?: boolean; onConfirm: () => void; children?: React.ReactNode }) {
  return (
    <AlertDialog open={open} onOpenChange={next => { if (!pending) onOpenChange(next) }}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          {description && <AlertDialogDescription>{description}</AlertDialogDescription>}
        </AlertDialogHeader>
        {children}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>{cancelLabel}</AlertDialogCancel>
          <AlertDialogAction variant={destructive ? "destructive" : "default"} disabled={pending} onClick={event => { event.preventDefault(); onConfirm() }}>
            {pending && <Spinner />}
            {confirmLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

/** Client-side paging; returns to the first page whenever resetKey changes. */
export function usePagination<T>(items: T[], pageSize = 10, resetKey?: unknown) {
  const [page, setPage] = React.useState(1)
  React.useEffect(() => { setPage(1) }, [resetKey])
  const pageCount = Math.max(1, Math.ceil(items.length / pageSize))
  const current = Math.min(page, pageCount)
  return { page: current, pageCount, setPage, total: items.length, pageItems: items.slice((current - 1) * pageSize, current * pageSize) }
}

function pageNumbers(page: number, count: number): (number | "gap")[] {
  if (count <= 7) return Array.from({ length: count }, (_, index) => index + 1)
  const pages: (number | "gap")[] = [1]
  const start = Math.max(2, Math.min(page - 1, count - 4))
  const end = Math.min(count - 1, Math.max(page + 1, 5))
  if (start > 2) pages.push("gap")
  for (let value = start; value <= end; value++) pages.push(value)
  if (end < count - 1) pages.push("gap")
  pages.push(count)
  return pages
}

/** Page controls under a list; hasMore lets cursor-based lists fetch the next server page. */
export function PaginationBar({ page, pageCount, total, onPageChange, hasMore = false, loadingMore = false, onLoadMore, className }: { page: number; pageCount: number; total: number; onPageChange: (page: number) => void; hasMore?: boolean; loadingMore?: boolean; onLoadMore?: () => Promise<unknown>; className?: string }) {
  if (pageCount <= 1 && !hasMore) return null
  const next = async () => {
    if (page < pageCount) { onPageChange(page + 1); return }
    if (hasMore && onLoadMore) { await onLoadMore(); onPageChange(page + 1) }
  }
  return (
    <div className={cn("flex flex-wrap items-center justify-between gap-3 border-t px-4 py-3 text-sm text-muted-foreground md:px-6", className)}>
      <span className="tabular-nums">共 {total}{hasMore ? "+" : ""} 条 · 第 {page} / {pageCount}{hasMore ? "+" : ""} 页</span>
      <Pagination className="mx-0 w-auto">
        <PaginationContent>
          <PaginationItem><Button variant="ghost" size="sm" aria-label="上一页" disabled={page <= 1} onClick={() => onPageChange(page - 1)}><ChevronLeft /><span className="hidden sm:inline">上一页</span></Button></PaginationItem>
          {pageNumbers(page, pageCount).map((value, index) => value === "gap"
            ? <PaginationItem key={`gap-${index}`} className="hidden sm:block"><PaginationEllipsis /></PaginationItem>
            : <PaginationItem key={value} className={value === page ? undefined : "hidden sm:block"}><Button variant={value === page ? "outline" : "ghost"} size="icon-sm" aria-label={`第 ${value} 页`} aria-current={value === page ? "page" : undefined} onClick={() => onPageChange(value)}>{value}</Button></PaginationItem>)}
          <PaginationItem><Button variant="ghost" size="sm" aria-label="下一页" disabled={(page >= pageCount && !hasMore) || loadingMore} onClick={() => void next()}>{loadingMore ? <Spinner /> : null}<span className="hidden sm:inline">下一页</span><ChevronRight /></Button></PaginationItem>
        </PaginationContent>
      </Pagination>
    </div>
  )
}

export const uiScales = [
  { value: 90, label: "小" },
  { value: 100, label: "标准" },
  { value: 115, label: "大" },
  { value: 130, label: "特大" },
] as const

/** Scales every rem-based size; the value is stored per user on the server. */
export function applyUiScale(scale?: number | null) {
  const value = typeof scale === "number" && scale >= 85 && scale <= 150 ? scale : 100
  document.documentElement.style.fontSize = value === 100 ? "" : `${value}%`
}

export function UiScaleMenu({ value, onChange }: { value?: number | null; onChange: (scale: number) => void }) {
  const current = String(value || 100)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button variant="ghost" size="icon" aria-label="调整字号" title="调整字号" />}><ALargeSmall /></DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-40">
        <DropdownMenuGroup>
          <DropdownMenuLabel>界面字号</DropdownMenuLabel>
          <DropdownMenuRadioGroup value={current} onValueChange={next => onChange(Number(next))}>
            {uiScales.map(scale => <DropdownMenuRadioItem key={scale.value} value={String(scale.value)}>{scale.label}<span className="ml-auto text-xs text-muted-foreground tabular-nums">{scale.value}%</span></DropdownMenuRadioItem>)}
          </DropdownMenuRadioGroup>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
