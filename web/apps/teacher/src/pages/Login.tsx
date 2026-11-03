import { useState, type FormEvent } from 'react';
import { Link, Navigate, useNavigate } from 'react-router';
import { ArrowRight, BellRing, ShieldCheck, Sparkles, Volume2 } from 'lucide-react';
import { api } from '@classding/api';
import { NoticeScreen } from '@classding/ui/notice';
import { Button } from '@classding/ui/components/ui/button';
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@classding/ui/components/ui/field';
import { Input } from '@classding/ui/components/ui/input';
import { Spinner } from '@classding/ui/components/ui/spinner';
import { Brand, SiteFooter, useSiteSettings } from '@classding/ui/components/classding';
import { useAuth } from '../auth';
import type { AuthResponse } from '../models';

export default function LoginPage({ register = false }: { register?: boolean }) {
  const { site_name } = useSiteSettings();
  const { user, authenticate } = useAuth();
  const navigate = useNavigate();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [name, setName] = useState('');
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  if (user) return <Navigate to="/send" replace />;
  async function submit(event: FormEvent) {
    event.preventDefault();
    setError('');
    setPending(true);
    try {
      const result = await api<AuthResponse>(register ? '/auth/register' : '/auth/login', {
        method: 'POST',
        body: JSON.stringify({ username, password, ...(register ? { display_name: name } : {}) }),
      });
      authenticate(result);
      navigate('/send');
    } catch (e) {
      setError(e instanceof Error ? e.message : '暂时无法登录，请稍后重试。');
    } finally {
      setPending(false);
    }
  }
  return (
    <div className="grid min-h-svh lg:grid-cols-2">
      <div className="flex flex-col gap-4 p-6 md:p-10">
        <Brand subtitle="教师工作台" className="w-auto" />
        <div className="flex flex-1 items-center justify-center">
          <form onSubmit={submit} className="w-full max-w-sm">
            <FieldGroup>
              <div className="space-y-1">
                <h1 className="text-2xl font-semibold tracking-tight">
                  {register ? '开启你的叮咚课堂' : `欢迎回到${site_name}`}
                </h1>
                <p className="text-sm text-muted-foreground">
                  {register ? '创建账号，让每一次提醒都有回音。' : '登录后，让每一次沟通都及时抵达。'}
                </p>
              </div>
              {register && (
                <Field>
                  <FieldLabel htmlFor="register-name">老师怎么称呼</FieldLabel>
                  <Input
                    id="register-name"
                    required
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="例如：林老师"
                    maxLength={24}
                    autoComplete="name"
                  />
                </Field>
              )}
              <Field>
                <FieldLabel htmlFor="login-username">用户名</FieldLabel>
                <Input
                  id="login-username"
                  required
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder="输入你的用户名"
                  autoComplete="username"
                  minLength={3}
                  maxLength={32}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="login-password">密码</FieldLabel>
                <Input
                  id="login-password"
                  required
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  type="password"
                  placeholder={register ? '至少 10 位，包含字母和数字' : '输入你的密码'}
                  autoComplete={register ? 'new-password' : 'current-password'}
                  minLength={register ? 10 : 1}
                />
              </Field>
              {error && <FieldError role="alert">{error}</FieldError>}
              <Button type="submit" size="lg" disabled={pending} className="w-full">
                {pending ? <Spinner /> : null}
                {register ? '创建账号' : '进入工作台'}
                {!pending && <ArrowRight />}
              </Button>
              <FieldDescription className="text-center">
                {register ? '已经有账号了？' : '还没有账号？'}
                <Link to={register ? '/login' : '/register'} onClick={() => setError('')}>
                  {register ? '去登录' : '创建账号'}
                </Link>
              </FieldDescription>
              {import.meta.env.DEV && (
                <p className="rounded-md border border-dashed p-3 text-xs text-muted-foreground">
                  本地开发账号：teacher / ClassDing2026!
                  <br />
                  需先启动服务并开启开发种子。
                </p>
              )}
            </FieldGroup>
          </form>
        </div>
        <SiteFooter className="login-copyright py-0" />
      </div>
      <div className="relative hidden flex-col justify-center gap-10 overflow-hidden bg-muted p-12 lg:flex">
        <div className="space-y-3">
          <p className="flex items-center gap-2 text-sm font-medium text-primary">
            <BellRing className="size-4" />
            让教学，多一份从容
          </p>
          <p className="text-4xl leading-tight font-semibold tracking-tight">
            一句提醒，
            <br />
            叮咚，就听见。
          </p>
          <p className="text-muted-foreground">
            手机轻轻一点，消息即刻到达教室。把沟通交给叮咚，把时间留给学生。
          </p>
        </div>
        <div className="overflow-hidden rounded-xl border bg-background shadow-sm">
          <div className="flex items-center gap-1.5 border-b px-3 py-2 text-xs text-muted-foreground">
            <span className="size-2 rounded-full bg-muted-foreground/30" />
            <span className="size-2 rounded-full bg-muted-foreground/30" />
            <span className="size-2 rounded-full bg-muted-foreground/30" />
            <span className="ml-2">三年级（2）班 · 教室大屏</span>
          </div>
          <NoticeScreen
            compact
            studentName="李沐阳"
            text="请带上作业，到办公室找我。"
            signature="林老师"
            className="三年级（2）班"
          />
        </div>
        <div className="flex gap-6 text-sm text-muted-foreground">
          <span className="flex items-center gap-2">
            <Volume2 className="size-4" />
            实时播报
          </span>
          <span className="flex items-center gap-2">
            <ShieldCheck className="size-4" />
            可靠送达
          </span>
          <span className="flex items-center gap-2">
            <Sparkles className="size-4" />
            简单好用
          </span>
        </div>
      </div>
    </div>
  );
}
