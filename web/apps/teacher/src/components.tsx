import type { ReactNode } from 'react';
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@classding/ui/components/ui/card';
import { cn } from '@classding/ui/lib/utils';

export {
  EmptyState,
  ErrorState,
  LoadingState as Loading,
  PageHeader,
} from '@classding/ui/components/classding';

/** Card with a titled header; children render inside the content area. */
export function SectionCard({
  title,
  description,
  action,
  children,
  className,
  step,
}: {
  title: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
  step?: string;
}) {
  return (
    <Card className={className}>
      <CardHeader>
        <CardTitle role="heading" aria-level={2} className="flex items-center gap-2">
          {step && (
            <span className="flex size-6 items-center justify-center rounded-md bg-primary/10 text-xs font-semibold text-primary">
              {step}
            </span>
          )}
          {title}
        </CardTitle>
        {description && <CardDescription>{description}</CardDescription>}
        {action && <CardAction>{action}</CardAction>}
      </CardHeader>
      <CardContent className={cn('grid gap-4')}>{children}</CardContent>
    </Card>
  );
}

export function formatTime(value?: string | null) {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return '—';
  return date.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });
}
export const initials = (name?: string | null, fallback = '师') => name?.trim().slice(0, 1) || fallback;
