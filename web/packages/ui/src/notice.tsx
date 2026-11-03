import * as React from 'react';
import { BellRing } from 'lucide-react';
import { cn } from './lib/utils';

// The full-screen classroom notification. The classroom client shows it for
// real; the teacher workspace renders it as a preview. Styles live in notice.css.

export interface NoticeScreenProps {
  text: string;
  studentName?: string;
  signature?: string;
  className?: string;
  style?: string;
  progress?: number;
  compact?: boolean;
  sentAt?: string;
}
export function NoticeScreen({
  text,
  studentName,
  signature = '老师',
  className = '教室大屏',
  style = 'notice',
  progress = 68,
  compact = false,
  sentAt,
}: NoticeScreenProps) {
  const body = React.useRef<HTMLDivElement>(null);
  const paragraph = React.useRef<HTMLParagraphElement>(null);
  const name = React.useRef<HTMLElement>(null);
  React.useLayoutEffect(() => {
    const container = body.current;
    const content = paragraph.current;
    if (!container || !content) return;
    const fit = () => {
      const metrics = getComputedStyle(container);
      const height =
        container.clientHeight - parseFloat(metrics.paddingTop) - parseFloat(metrics.paddingBottom);
      const gap = studentName ? parseFloat(metrics.rowGap) || 0 : 0;
      if (height <= 0) return;
      const apply = (size: number) => {
        content.style.fontSize = `${size}px`;
        if (name.current) name.current.style.fontSize = `${size * 1.025}px`;
      };
      let low = compact ? 6 : 12;
      let high = compact ? 21 : Math.max(24, Math.min(150, innerHeight * 0.078));
      for (let i = 0; i < 10; i++) {
        const middle = (low + high) / 2;
        apply(middle);
        const fits = content.scrollHeight + (name.current?.scrollHeight || 0) + gap <= height;
        if (fits) low = middle;
        else high = middle;
      }
      apply(low);
    };
    const observer = new ResizeObserver(fit);
    observer.observe(container);
    fit();
    return () => observer.disconnect();
  }, [text, studentName, compact]);
  const label = style === 'praise' ? '表扬时刻' : style === 'urgent' ? '紧急通知' : '班级通知';
  const time = sentAt
    ? new Date(sentAt).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
    : '现在';
  return (
    <div className={cn('cd-notice-screen', `cd-notice-${style}`, compact && 'cd-notice-compact')}>
      <div className="cd-notice-top">
        <span>
          <BellRing /> {label}
        </span>
        <span>{className}</span>
      </div>
      <div className="cd-notice-body" ref={body}>
        {studentName && (
          <strong ref={name}>
            {studentName}
            <span>同学</span>
          </strong>
        )}
        <p ref={paragraph}>{text || '请带上作业到办公室'}</p>
      </div>
      <div className="cd-notice-footer">
        <span>
          {signature}
          <b>·</b>
          {time}
        </span>
        <span className="cd-notice-watermark">
          <BellRing /> 课堂叮咚
        </span>
      </div>
      <div className="cd-notice-progress">
        <i style={{ width: `${Math.max(0, Math.min(100, progress))}%` }} />
      </div>
    </div>
  );
}
