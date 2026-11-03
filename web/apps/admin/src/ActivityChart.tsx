import { Area, AreaChart, CartesianGrid, XAxis } from 'recharts';
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from '@classding/ui/components/ui/chart';

const config = { count: { label: '发送数量', color: 'var(--chart-1)' } } satisfies ChartConfig;

export default function ActivityChart({ activity }: { activity: { date: string; count: number }[] }) {
  return <ChartContainer config={config} className="aspect-auto h-60 w-full">
    <AreaChart data={activity} margin={{ top: 12, left: 12, right: 12 }}>
      <defs>
        <linearGradient id="activityFill" x1="0" y1="0" x2="0" y2="1">
          <stop offset="5%" stopColor="var(--color-count)" stopOpacity={0.3} />
          <stop offset="95%" stopColor="var(--color-count)" stopOpacity={0.02} />
        </linearGradient>
      </defs>
      <CartesianGrid vertical={false} />
      <XAxis dataKey="date" tickLine={false} axisLine={false} tickMargin={8} minTickGap={24} tickFormatter={(value: string) => value.slice(5)} />
      <ChartTooltip cursor={false} content={<ChartTooltipContent indicator="dot" />} />
      <Area dataKey="count" type="monotone" fill="url(#activityFill)" stroke="var(--color-count)" strokeWidth={2} />
    </AreaChart>
  </ChartContainer>;
}
