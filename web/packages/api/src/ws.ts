import type { components } from './ws-types';
export type WSFrame = components['schemas']['Frame'];
export type FrameData<T extends WSFrame['t']> = Extract<WSFrame, { t: T }>['d'];
export type AckStatus = FrameData<'ack'>['status'];
// Older saved queues omit these fields. The current wire payload requires them.
type OptionalStoredFields =
  'student_name' | 'signature' | 'class_name' | 'audio_url' | 'sent_at' | 'expires_at';
export type Notice = Omit<FrameData<'notify'>, OptionalStoredFields> &
  Partial<Pick<FrameData<'notify'>, OptionalStoredFields>>;
