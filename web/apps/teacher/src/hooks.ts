import { useEffect, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api, getAccessToken, refreshSession, tokenExpiresSoon } from '@classding/api';
import type { Class, Homework, MessagePage, Phrase, StudentCallPage } from './models';
import type { InfiniteData } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useAuth } from './auth';
import { readStudentCallEvent, studentCallsKey, studentCallsUnavailable } from './studentCalls';
export const useClasses = () => useQuery({ queryKey: ['classes'], queryFn: () => api<Class[]>('/classes') });
export const useHomeworks = (classId: string) =>
  useQuery({
    queryKey: ['homeworks', classId],
    queryFn: () => api<{ items: Homework[] }>(`/homeworks?class_id=${encodeURIComponent(classId)}`),
    enabled: !!classId,
    refetchInterval: 30000,
    refetchIntervalInBackground: true,
  });
export const usePhrases = () => useQuery({ queryKey: ['phrases'], queryFn: () => api<Phrase[]>('/phrases') });
export const useMessages = (classId = '') =>
  useQuery({
    queryKey: ['messages', classId],
    queryFn: () => api<MessagePage>(`/messages${classId ? `?class_id=${encodeURIComponent(classId)}` : ''}`),
  });
export function useTeacherSocket() {
  const cache = useQueryClient();
  const { user } = useAuth();
  const userId = user?.id ?? '';
  const [online, setOnline] = useState(false);
  useEffect(() => {
    if (!userId) return;
    setOnline(false);
    let stopped = false;
    const announced = new Set<string>();
    let socket: WebSocket | undefined;
    let timeout: ReturnType<typeof setTimeout>;
    let ping: ReturnType<typeof setInterval>;
    let attempts = 0;
    let lastFrame = Date.now();
    function connect() {
      if (stopped) return;
      const current = new WebSocket(
        `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/ws/teacher`,
      );
      socket = current;
      current.onopen = () => {
        if (stopped || socket !== current) return;
        lastFrame = Date.now();
        current.send(JSON.stringify({ t: 'hello', d: { token: getAccessToken() } }));
      };
      current.onmessage = (event) => {
        if (stopped || socket !== current) return;
        lastFrame = Date.now();
        let frame;
        try {
          frame = JSON.parse(event.data);
        } catch {
          return;
        }
        if (frame.t === 'welcome') {
          attempts = 0;
          setOnline(true);
          // Anything may have changed while the socket was down.
          void cache.invalidateQueries({ queryKey: ['homeworks'] });
          void cache.invalidateQueries({ queryKey: ['messages'] });
          void cache.invalidateQueries({ queryKey: ['classes'] });
          void cache.invalidateQueries({ queryKey: ['devices'] });
          if (!studentCallsUnavailable(cache.getQueryState(studentCallsKey(userId))?.error))
            void cache.invalidateQueries({ queryKey: studentCallsKey(userId) });
          clearInterval(ping);
          ping = setInterval(() => {
            if (Date.now() - lastFrame > 60000) {
              socket?.close();
              return;
            }
            // Refresh shortly before the access token expires and hand the new one to the
            // server, so the socket outlives the 15-minute token.
            const renew = tokenExpiresSoon(getAccessToken())
              ? refreshSession().catch(() => {})
              : Promise.resolve();
            void renew.then(() => {
              if (!stopped && socket === current && current.readyState === WebSocket.OPEN)
                current.send(JSON.stringify({ t: 'ping', d: { token: getAccessToken() } }));
            });
          }, 25000);
        }
        if (frame.t === 'student_call') {
          const call = readStudentCallEvent(frame.d);
          if (!call) return;
          void cache.invalidateQueries({ queryKey: studentCallsKey(userId) }).then(() => {
            if (stopped || socket !== current || call.kind !== 'created' || announced.has(call.id)) return;
            announced.add(call.id);
            if (announced.size > 512) announced.delete(announced.values().next().value!);
            const data = cache.getQueryData<InfiniteData<StudentCallPage>>(studentCallsKey(userId));
            const item = data?.pages.flatMap((page) => page.items).find((item) => item.id === call.id);
            toast.info(item ? `${item.class_name}的学生正在呼叫老师` : '收到新的学生呼叫', {
              id: `student-call-${userId}-${call.id}`,
              description: '请在“学生呼叫”中查看，处理后标记已读。',
            });
          });
        }
        if (frame.t === 'receipt') void cache.invalidateQueries({ queryKey: ['messages'] });
        if (frame.t === 'homework') void cache.invalidateQueries({ queryKey: ['homeworks'] });
        if (frame.t === 'presence') {
          void cache.invalidateQueries({ queryKey: ['classes'] });
          void cache.invalidateQueries({ queryKey: ['devices'] });
        }
      };
      current.onclose = () => {
        if (stopped || socket !== current) return;
        setOnline(false);
        clearInterval(ping);
        if (!stopped) {
          void api('/me')
            .catch(() => {})
            .finally(() => {
              if (!stopped)
                timeout = setTimeout(
                  connect,
                  Math.min(30000, 1000 * 2 ** attempts++) * (0.7 + Math.random() * 0.6),
                );
            });
        }
      };
      current.onerror = () => current.close();
    }
    connect();
    return () => {
      stopped = true;
      clearTimeout(timeout);
      clearInterval(ping);
      socket?.close();
    };
  }, [cache, userId]);
  return online;
}
export function useNow(interval = 1000) {
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), interval);
    return () => clearInterval(timer);
  }, [interval]);
  return now;
}
export function useRecorder(onComplete: (blob: Blob) => void) {
  const recorder = useRef<MediaRecorder | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const stream = useRef<MediaStream | null>(null);
  const mounted = useRef(false);
  const starting = useRef(false);
  const [recording, setRecording] = useState(false);
  const [error, setError] = useState('');
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      clearTimeout(timer.current);
      if (recorder.current?.state === 'recording') recorder.current.stop();
      stream.current?.getTracks().forEach((track) => track.stop());
    };
  }, []);
  async function start() {
    if (starting.current || recorder.current?.state === 'recording') return;
    starting.current = true;
    setError('');
    try {
      if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === 'undefined')
        throw new Error('当前浏览器不支持录音，请使用输入法语音输入，或在浏览器中打开。');
      const media = await navigator.mediaDevices.getUserMedia({ audio: true });
      if (!mounted.current) {
        media.getTracks().forEach((track) => track.stop());
        return;
      }
      stream.current = media;
      const type = ['audio/webm;codecs=opus', 'audio/mp4', 'audio/webm'].find((t) =>
        MediaRecorder.isTypeSupported(t),
      );
      const current = new MediaRecorder(media, type ? { mimeType: type } : undefined);
      recorder.current = current;
      const chunks: BlobPart[] = [];
      current.ondataavailable = (event) => {
        if (event.data.size) chunks.push(event.data);
      };
      current.onstop = () => {
        clearTimeout(timer.current);
        media.getTracks().forEach((track) => track.stop());
        if (mounted.current) {
          setRecording(false);
          if (chunks.length) onComplete(new Blob(chunks, { type: current.mimeType }));
        }
      };
      current.start();
      setRecording(true);
      timer.current = setTimeout(() => current.state === 'recording' && current.stop(), 30000);
    } catch (e) {
      stream.current?.getTracks().forEach((track) => track.stop());
      if (mounted.current) setError(e instanceof Error ? e.message : '无法使用麦克风，请检查权限。');
    } finally {
      starting.current = false;
    }
  }
  function stop() {
    if (recorder.current?.state === 'recording') recorder.current.stop();
  }
  return { recording, error, start, stop };
}
