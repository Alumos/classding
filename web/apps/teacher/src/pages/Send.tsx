import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@classding/api';
import { NoticeScreen } from '@classding/ui/notice';
import {
  AudioLines,
  BellRing,
  Check,
  CheckCheck,
  ChevronRight,
  Expand,
  Keyboard,
  LayoutGrid,
  Mic,
  Monitor,
  Plus,
  Send,
  ShieldCheck,
  Sparkles,
  UserRound,
  Volume2,
  Zap,
} from 'lucide-react';
import { toast } from 'sonner';
import { Alert, AlertDescription } from '@classding/ui/components/ui/alert';
import { Badge } from '@classding/ui/components/ui/badge';
import { Button } from '@classding/ui/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@classding/ui/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@classding/ui/components/ui/dialog';
import { Input } from '@classding/ui/components/ui/input';
import { Label } from '@classding/ui/components/ui/label';
import { Separator } from '@classding/ui/components/ui/separator';
import { Spinner } from '@classding/ui/components/ui/spinner';
import { Switch } from '@classding/ui/components/ui/switch';
import { Textarea } from '@classding/ui/components/ui/textarea';
import { SimpleSelect } from '@classding/ui/components/classding';
import { cn } from '@classding/ui/lib/utils';
import { useAuth } from '../auth';
import { useClasses, useRecorder } from '../hooks';
import { EmptyState, ErrorState, Loading, PageHeader, SectionCard } from '../components';
import type { Message } from '../models';

const styles = [
  { value: 'notice', label: '日常通知', icon: <BellRing /> },
  { value: 'praise', label: '表扬一下', icon: <Sparkles /> },
  { value: 'urgent', label: '紧急提醒', icon: <Zap /> },
];
const durations = [
  { value: '15', label: '15 秒' },
  { value: '30', label: '30 秒' },
  { value: '60', label: '1 分钟' },
  { value: '120', label: '2 分钟' },
  { value: '180', label: '3 分钟' },
  { value: '300', label: '5 分钟' },
  { value: '600', label: '10 分钟' },
];
const repeats = ['1', '2', '3'].map((value) => ({ value, label: `${value} 遍` }));

export default function SendPage() {
  const { user } = useAuth();
  const cache = useQueryClient();
  const classes = useClasses();
  const [selected, setSelected] = useState<string[]>([]);
  const initialized = useRef(false);
  const [mode, setMode] = useState<'text' | 'voice'>('text');
  const [text, setText] = useState('');
  const [student, setStudent] = useState('');
  const signature = user?.display_name || user?.username || '老师';
  const [style, setStyle] = useState('notice');
  const [duration, setDuration] = useState('60');
  const [repeat, setRepeat] = useState('2');
  const [chime, setChime] = useState(false);
  const [preview, setPreview] = useState(false);
  const [audio, setAudio] = useState<{ audio_id: string; url: string } | null>(null);
  const [uploading, setUploading] = useState(false);
  const [lastSent, setLastSent] = useState<Message | null>(null);
  const charCount = Array.from(text).length;
  useEffect(() => {
    if (classes.data?.length && !initialized.current) {
      initialized.current = true;
      setSelected([classes.data[0].id]);
    }
  }, [classes.data]);
  const selectedClasses = (classes.data ?? []).filter((item) => selected.includes(item.id));
  const onlineCount = (classes.data ?? []).reduce((sum, item) => sum + (item.online_devices ?? 0), 0);
  async function upload(blob: Blob) {
    setUploading(true);
    try {
      const body = new FormData();
      body.append('file', blob, `recording.${blob.type.includes('mp4') ? 'm4a' : 'webm'}`);
      const result = await api<{ audio_id: string; url: string; transcript?: string }>('/audio', {
        method: 'POST',
        body,
      });
      setAudio(result);
      if (result.transcript) setText(result.transcript);
      toast.success('录音已上传，可发送老师原声');
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '录音上传失败');
    } finally {
      setUploading(false);
    }
  }
  const recorder = useRecorder((blob) => void upload(blob));
  const sent = useMutation({
    mutationFn: () =>
      api<Message>('/messages', {
        method: 'POST',
        body: JSON.stringify({
          class_ids: selectedClasses.map((item) => item.id),
          kind: mode === 'voice' ? 'voice' : 'text',
          text,
          student_name: student.trim(),
          signature,
          style,
          display_sec: Number(duration),
          repeat: Number(repeat),
          chime: style === 'urgent' && chime,
          play_mode: mode === 'voice' ? 'voice' : 'tts',
          ...(mode === 'voice' && audio ? { audio_id: audio.audio_id } : {}),
        }),
      }),
    onSuccess: (message) => {
      setLastSent(message);
      toast.success('通知已提交，正在投递到教室');
      void cache.invalidateQueries({ queryKey: ['messages'] });
    },
    onError: (error) => toast.error(error.message),
  });
  function toggle(id: string) {
    setSelected((value) => (value.includes(id) ? value.filter((item) => item !== id) : [...value, id]));
  }
  const allSelected = !!classes.data?.length && selectedClasses.length === classes.data.length;
  const canSend =
    selectedClasses.length > 0 &&
    charCount <= 200 &&
    (mode === 'voice' ? !!audio : !!text.trim()) &&
    !recorder.recording &&
    !uploading;
  const previewProps = {
    text,
    studentName: student,
    signature,
    className: selectedClasses[0]?.name ?? '你的班级',
    style,
  };
  return (
    <>
      <PageHeader
        title="把提醒，送进教室"
        description="选好班级，写下想说的话，剩下的交给叮咚。"
        actions={
          <Badge variant="secondary" className="h-7 gap-1.5 px-2.5">
            <Monitor />
            {onlineCount} 块大屏在线
          </Badge>
        }
      />
      <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)]">
        <div className="grid gap-6">
          <SectionCard
            step="1"
            title="选择接收班级"
            description="只有你所在班级的大屏，会收到这条通知。"
            action={
              <Button
                variant="ghost"
                size="sm"
                disabled={!classes.data?.length}
                onClick={() => setSelected(allSelected ? [] : (classes.data ?? []).map((item) => item.id))}
              >
                {allSelected ? '取消全选' : '全选班级'}
              </Button>
            }
          >
            {classes.isLoading ? (
              <Loading />
            ) : classes.error ? (
              <ErrorState error={classes.error} retry={() => void classes.refetch()} />
            ) : classes.data?.length ? (
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
                {classes.data.map((item) => {
                  const active = selected.includes(item.id);
                  return (
                    <button
                      key={item.id}
                      type="button"
                      aria-pressed={active}
                      onClick={() => toggle(item.id)}
                      className={cn(
                        'class-option relative flex flex-col items-start gap-2 rounded-lg border p-3 text-left transition-colors hover:bg-accent focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
                        active && 'border-primary bg-primary/5 ring-1 ring-primary',
                      )}
                    >
                      <span className="flex size-8 items-center justify-center rounded-md bg-muted">
                        <LayoutGrid className="size-4" />
                      </span>
                      <strong className="text-sm font-medium break-all">{item.name}</strong>
                      <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                        <span
                          className={cn(
                            'size-1.5 rounded-full',
                            item.online_devices ? 'bg-emerald-500' : 'bg-muted-foreground/40',
                          )}
                        />
                        {item.online_devices ? '大屏在线' : '大屏离线'}
                      </span>
                      <span
                        className={cn(
                          'absolute top-3 right-3 flex size-4 items-center justify-center rounded-sm border',
                          active && 'border-primary bg-primary text-primary-foreground',
                        )}
                      >
                        {active && <Check className="size-3" />}
                      </span>
                    </button>
                  );
                })}
              </div>
            ) : (
              <EmptyState
                icon={<LayoutGrid />}
                title="你的第一间教室，等你连接"
                description="使用激活码创建班级，或输入协同码加入现有班级。"
                action={
                  <Button variant="secondary" nativeButton={false} render={<Link to="/classes" />}>
                    <Plus />
                    添加班级
                  </Button>
                }
              />
            )}
          </SectionCard>
          <SectionCard
            step="2"
            title="写一条通知"
            action={
              <div className="flex rounded-lg bg-muted p-0.5">
                {(
                  [
                    ['text', '文字', Keyboard],
                    ['voice', '语音', Mic],
                  ] as const
                ).map(([value, label, Icon]) => (
                  <Button
                    key={value}
                    size="sm"
                    variant={mode === value ? 'outline' : 'ghost'}
                    aria-pressed={mode === value}
                    className={cn(mode === value ? 'bg-background shadow-sm' : 'text-muted-foreground')}
                    onClick={() => setMode(value)}
                  >
                    <Icon />
                    {label}
                  </Button>
                ))}
              </div>
            }
          >
            {mode === 'text' ? (
              <div className="relative">
                <Textarea
                  aria-label="通知内容"
                  value={text}
                  onChange={(event) => {
                    if (Array.from(event.target.value).length <= 200) setText(event.target.value);
                  }}
                  placeholder="写下你的提醒，例如：请带上数学作业，到办公室找我。"
                  className="min-h-36 resize-none pb-8 text-base md:text-base"
                />
                <span
                  className={cn(
                    'absolute right-3 bottom-2 text-xs text-muted-foreground tabular-nums',
                    charCount > 180 && 'text-destructive',
                  )}
                >
                  {charCount} / 200
                </span>
              </div>
            ) : (
              <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed p-6 text-center">
                <span
                  className={cn(
                    'flex size-14 items-center justify-center rounded-full bg-muted',
                    recorder.recording && 'animate-pulse bg-destructive/10 text-destructive',
                  )}
                >
                  <AudioLines className="size-6" />
                </span>
                <p className="font-medium">
                  {uploading
                    ? '正在上传录音…'
                    : recorder.recording
                      ? '正在录音，再点一次结束'
                      : audio
                        ? '原声已准备好'
                        : '用熟悉的声音，传达提醒'}
                </p>
                <p className="text-sm text-muted-foreground">最长 30 秒 · 不进行语音转文字</p>
                <Button
                  disabled={uploading}
                  variant={recorder.recording ? 'destructive' : 'secondary'}
                  onClick={() => (recorder.recording ? recorder.stop() : void recorder.start())}
                >
                  {uploading ? <Spinner /> : <Mic />}
                  {recorder.recording ? '结束录音' : audio ? '重新录音' : '点击开始录音'}
                </Button>
                {audio && <audio controls src={audio.url} className="w-full max-w-sm" />}
                {recorder.error && <p className="text-sm text-destructive">{recorder.error}</p>}
              </div>
            )}
            <div className="student-signature-fields grid grid-cols-2 gap-3 sm:gap-4">
              <label htmlFor="student-name" className="grid gap-2 text-sm font-medium">
                <span>提醒哪位同学</span>
                <Input
                  id="student-name"
                  value={student}
                  onChange={(event) => setStudent(event.target.value)}
                  maxLength={24}
                  placeholder="例如：李沐阳"
                />
                <p className="text-xs font-normal text-muted-foreground">选填 · 在大屏上醒目显示</p>
              </label>
              <label htmlFor="teacher-signature" className="grid gap-2 text-sm font-medium">
                <span>老师落款</span>
                <Input
                  id="teacher-signature"
                  aria-label="老师落款"
                  value={signature}
                  readOnly
                  aria-readonly="true"
                  className="bg-muted"
                />
                <p className="flex items-center gap-1 text-xs font-normal text-muted-foreground">
                  <UserRound className="size-3" />
                  当前登录教师姓名
                </p>
              </label>
            </div>
            <Separator />
            <div className="grid gap-4 sm:grid-cols-3">
              <div className="grid gap-2">
                <Label htmlFor="notice-style">通知样式</Label>
                <SimpleSelect
                  id="notice-style"
                  label="通知样式"
                  value={style}
                  options={styles}
                  onValueChange={(value) => {
                    setStyle(value);
                    setChime(false);
                  }}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="notice-duration">通知显示时长</Label>
                <SimpleSelect
                  id="notice-duration"
                  label="通知显示时长"
                  value={duration}
                  options={durations}
                  onValueChange={setDuration}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="speech-repeat">朗读次数</Label>
                <SimpleSelect
                  id="speech-repeat"
                  label="朗读次数"
                  value={repeat}
                  options={repeats}
                  onValueChange={setRepeat}
                />
              </div>
            </div>
            <p className="text-xs text-muted-foreground">
              学生可点击“已读通知，关闭消息”提前关闭。语音朗读与提示音分别设置。
            </p>
            {style === 'urgent' ? (
              <div className="flex items-center justify-between gap-4 rounded-lg border p-3">
                <Label htmlFor="urgent-chime" className="grid gap-1">
                  <span>紧急事项提示音</span>
                  <span className="text-xs font-normal text-muted-foreground">
                    需要时手动开启，播放叮咚声提醒学生。
                  </span>
                </Label>
                <Switch
                  id="urgent-chime"
                  checked={chime}
                  onCheckedChange={setChime}
                  aria-label="紧急事项提示音"
                />
              </div>
            ) : (
              <p className="flex items-center gap-2 text-xs text-muted-foreground">
                <BellRing className="size-3.5" />
                提示音默认关闭，紧急提醒可手动开启。
              </p>
            )}
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
              <Button
                size="lg"
                className="sm:min-w-48"
                disabled={!canSend || sent.isPending}
                onClick={() => sent.mutate()}
              >
                {sent.isPending ? <Spinner /> : <Send />}
                {sent.isPending ? '正在发送…' : `发送到 ${selectedClasses.length} 个班级`}
              </Button>
              <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <ShieldCheck className="size-3.5" />
                发送后可在记录中查看回执或撤回
              </span>
            </div>
            {lastSent && (
              <Alert role="status">
                <CheckCheck />
                <AlertDescription className="flex flex-wrap items-center gap-x-2">
                  最近一条通知已提交，
                  {lastSent.targets?.filter((target) => target.status === 'played').length ?? 0}{' '}
                  台设备已播放。
                  <Link to="/history" className="inline-flex items-center font-medium text-primary">
                    查看回执
                    <ChevronRight className="size-3.5" />
                  </Link>
                </AlertDescription>
              </Alert>
            )}
          </SectionCard>
        </div>
        <Card className="lg:sticky lg:top-20">
          <CardHeader>
            <CardTitle role="heading" aria-level={2}>
              教室里的样子
            </CardTitle>
            <CardDescription>内容随你输入实时更新</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4">
            <div className="overflow-hidden rounded-lg border bg-neutral-900 p-2 shadow-sm">
              <div className="overflow-hidden rounded-md">
                <NoticeScreen compact {...previewProps} />
              </div>
            </div>
            <Button variant="outline" onClick={() => setPreview(true)}>
              <Expand />
              放大预览
            </Button>
            <Separator />
            <div className="grid gap-3 text-sm">
              <p className="font-medium">每条消息，都有回音</p>
              {[
                { icon: Check, title: '已送达', text: '消息已到达教室大屏' },
                { icon: Volume2, title: '已播放', text: '大屏已完成语音播报' },
                { icon: Monitor, title: '离线等待', text: '网络恢复后自动补发' },
              ].map((item) => (
                <div key={item.title} className="flex items-center gap-3">
                  <span className="flex size-7 items-center justify-center rounded-md bg-muted">
                    <item.icon className="size-3.5" />
                  </span>
                  <span className="font-medium">{item.title}</span>
                  <span className="text-muted-foreground">{item.text}</span>
                </div>
              ))}
            </div>
            <Button
              variant="link"
              className="justify-start px-0"
              nativeButton={false}
              render={<Link to="/classes" />}
            >
              还没连接教室大屏？去连接
              <ChevronRight />
            </Button>
          </CardContent>
        </Card>
      </div>
      <Dialog open={preview} onOpenChange={setPreview}>
        <DialogContent className="sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>教室大屏预览</DialogTitle>
            <DialogDescription>通知会按当前内容和样式显示。</DialogDescription>
          </DialogHeader>
          <div className="overflow-hidden rounded-lg border">
            <NoticeScreen compact {...previewProps} />
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
