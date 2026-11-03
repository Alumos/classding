import { useEffect, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api, getAccessToken } from '@classding/api';
import type { Class, Homework, MessagePage, Phrase } from './models';
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
  const [online, setOnline] = useState(false);
  useEffect(() => {
    let stopped = false;
    let socket: WebSocket | undefined;
    let timeout: ReturnType<typeof setTimeout>;
    let ping: ReturnType<typeof setInterval>;
    let attempts = 0;
    let lastFrame = Date.now();
    function connect() {
      if (stopped) return;
      socket = new WebSocket(
        `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/ws/teacher`,
      );
      socket.onopen = () => {
        lastFrame = Date.now();
        socket?.send(JSON.stringify({ t: 'hello', d: { token: getAccessToken() } }));
      };
      socket.onmessage = (event) => {
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
          void cache.invalidateQueries({ queryKey: ['homeworks'] });
          clearInterval(ping);
          ping = setInterval(() => {
            if (Date.now() - lastFrame > 60000) {
              socket?.close();
              return;
            }
            if (socket?.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ t: 'ping', d: {} }));
          }, 25000);
        }
        if (frame.t === 'receipt') void cache.invalidateQueries({ queryKey: ['messages'] });
        if (frame.t === 'homework') void cache.invalidateQueries({ queryKey: ['homeworks'] });
        if (frame.t === 'presence') {
          void cache.invalidateQueries({ queryKey: ['classes'] });
          void cache.invalidateQueries({ queryKey: ['devices'] });
        }
      };
      socket.onclose = () => {
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
      socket.onerror = () => socket?.close();
    }
    connect();
    return () => {
      stopped = true;
      clearTimeout(timeout);
      clearInterval(ping);
      socket?.close();
    };
  }, [cache]);
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
