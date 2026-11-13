import { useInfiniteQuery, type InfiniteData } from '@tanstack/react-query';
import { ApiError, api } from '@classding/api';
import type { StudentCall, StudentCallPage } from './models';

export const studentCallsKey = (userId: string) => ['student-calls', userId] as const;
export const studentCallsUnavailable = (error: unknown) => error instanceof ApiError && error.status === 404;

export function useStudentCalls(userId: string) {
  return useInfiniteQuery({
    queryKey: studentCallsKey(userId),
    initialPageParam: '',
    queryFn: ({ signal, pageParam }) =>
      api<StudentCallPage>(`/student-calls${pageParam ? `?cursor=${encodeURIComponent(pageParam)}` : ''}`, {
        signal,
      }),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    enabled: !!userId,
    retry: (count, error) => !studentCallsUnavailable(error) && count < 1,
    refetchInterval: (query) => (studentCallsUnavailable(query.state.error) ? false : 30000),
    refetchIntervalInBackground: true,
    refetchOnMount: (query) => !studentCallsUnavailable(query.state.error),
    refetchOnWindowFocus: (query) => !studentCallsUnavailable(query.state.error),
    refetchOnReconnect: (query) => !studentCallsUnavailable(query.state.error),
  });
}

// Keep the badge correct immediately after a confirmed shared read, then reconcile with the server.
export function applyStudentCallRead(data: InfiniteData<StudentCallPage> | undefined, call: StudentCall) {
  if (!data) return data;
  const previous = data.pages.flatMap((page) => page.items).find((item) => item.id === call.id);
  const decrement = previous && !previous.read_at && call.read_at ? 1 : 0;
  return {
    ...data,
    pages: data.pages.map((page) => ({
      ...page,
      items: page.items.map((item) => (item.id === call.id ? call : item)),
      unread_count: Math.max(0, page.unread_count - decrement),
    })),
  };
}

export type StudentCallEvent = { id: string; class_id: string; kind: 'created' | 'read' };
export function readStudentCallEvent(data: unknown): StudentCallEvent | null {
  if (!data || typeof data !== 'object') return null;
  const value = data as Partial<StudentCallEvent>;
  return typeof value.id === 'string' &&
    value.id !== '' &&
    typeof value.class_id === 'string' &&
    value.class_id !== '' &&
    (value.kind === 'created' || value.kind === 'read')
    ? (value as StudentCallEvent)
    : null;
}
