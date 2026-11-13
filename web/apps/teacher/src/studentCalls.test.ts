import { describe, expect, it } from 'vitest';
import { applyStudentCallRead, readStudentCallEvent, studentCallsKey } from './studentCalls';
import type { StudentCall, StudentCallPage } from './models';

const call: StudentCall = {
  id: 'call-1',
  class_id: 'class-1',
  class_name: '一年级',
  device_id: 'device-1',
  device_name: '教室',
  created_at: '2026-01-01T01:00:00Z',
  read_at: '',
  read_by: '',
  read_by_name: '',
};
const handled: StudentCall = {
  ...call,
  read_at: '2026-01-01T01:01:00Z',
  read_by: 'teacher-2',
  read_by_name: '李老师',
};
const page = (items: StudentCall[], count: number): StudentCallPage => ({
  items,
  unread_count: count,
  next_cursor: null,
});

describe('student calls shared state', () => {
  it('scopes cache keys by account', () => {
    expect(studentCallsKey('a')).not.toEqual(studentCallsKey('b'));
  });
  it('parses only known websocket frames', () => {
    expect(readStudentCallEvent({ id: '1', class_id: '2', kind: 'created' })?.kind).toBe('created');
    expect(readStudentCallEvent({ id: '1', class_id: '2', kind: 'read' })?.kind).toBe('read');
    for (const value of [
      null,
      {},
      'read',
      { id: 1, class_id: '2', kind: 'read' },
      { id: '', class_id: '2', kind: 'created' },
      { id: '1', class_id: '2', kind: 'delete' },
    ])
      expect(readStudentCallEvent(value)).toBeNull();
  });
  it('updates the first page global badge when reading a later page', () => {
    const original = { pages: [page([], 3), page([call], 3)], pageParams: ['', 'cursor'] };
    const updated = applyStudentCallRead(original, handled)!;
    expect(updated.pages.map((item) => item.unread_count)).toEqual([2, 2]);
    expect(updated.pages[1].items[0]).toEqual(handled);
    expect(original.pages[1].items[0].read_at).toBe('');
    expect(applyStudentCallRead(updated, handled)).toEqual(updated);
  });
  it('never underflows or guesses unknown call counts', () => {
    expect(
      applyStudentCallRead({ pages: [page([call], 0)], pageParams: [''] }, handled)?.pages[0].unread_count,
    ).toBe(0);
    expect(
      applyStudentCallRead({ pages: [page([], 3)], pageParams: [''] }, handled)?.pages[0].unread_count,
    ).toBe(3);
    expect(applyStudentCallRead(undefined, handled)).toBeUndefined();
  });
});
