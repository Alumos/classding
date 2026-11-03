// How a notification fared on each classroom screen, worded the same in every app.

export type Tone = 'default' | 'secondary' | 'destructive' | 'outline';

const deliveries: Record<string, { label: string; tone: Tone }> = {
  queued: { label: '等待送达', tone: 'outline' },
  delivered: { label: '已送达', tone: 'secondary' },
  playing: { label: '正在播放', tone: 'secondary' },
  played: { label: '已播放', tone: 'default' },
  dismissed: { label: '已关闭', tone: 'default' },
  expired: { label: '离线未送达', tone: 'destructive' },
  failed: { label: '投递失败', tone: 'destructive' },
};

export const deliveryLabel = (status: string) => deliveries[status]?.label ?? status;
export const deliveryTone = (status: string): Tone => deliveries[status]?.tone ?? 'secondary';

/** Summarizes several deliveries: any failure wins, then any unfinished screen. */
export function deliverySummary(statuses: string[]): { text: string; tone: Tone } {
  if (!statuses.length) return { text: '暂无回执', tone: 'outline' };
  const tones = statuses.map(deliveryTone);
  return {
    text: [...new Set(statuses.map(deliveryLabel))].join('、'),
    tone: tones.includes('destructive')
      ? 'destructive'
      : tones.every((t) => t === 'default')
        ? 'default'
        : 'secondary',
  };
}
