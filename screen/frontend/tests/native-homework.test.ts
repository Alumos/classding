import { afterEach, expect, it, vi } from 'vitest';
import { syncHomeworkWindows } from '../src/native';
import type { Homework } from '../src/homework';

const card: Homework = {
  id: 'h1',
  class_id: 'c1',
  subject: '数学',
  text: '课后练习',
  teacher_id: 't1',
  teacher_name: '林老师',
  visible: true,
  always_on_top: true,
  created_at: '2026-11-01T00:00:00Z',
  updated_at: '2026-11-01T00:00:00Z',
};

afterEach(() => vi.unstubAllGlobals());

it('原生窗口更新按提交顺序执行，解绑清理在旧更新完成后执行', async () => {
  let release!: () => void;
  const update = vi
    .fn()
    .mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          release = resolve;
        }),
    )
    .mockResolvedValue(undefined);
  vi.stubGlobal('window', { go: { main: { App: { UpdateHomeworks: update } } } });
  const first = syncHomeworkWindows([card]);
  const clear = syncHomeworkWindows([]);
  await vi.waitFor(() => expect(update).toHaveBeenCalledTimes(1));
  expect(update).toHaveBeenNthCalledWith(1, JSON.stringify([card]));
  release();
  await Promise.all([first, clear]);
  expect(update).toHaveBeenNthCalledWith(2, '[]');
});

it('一次原生更新失败后，后续清理仍会继续执行', async () => {
  const update = vi.fn().mockRejectedValueOnce(new Error('窗口不可用')).mockResolvedValue(undefined);
  vi.stubGlobal('window', { go: { main: { App: { UpdateHomeworks: update } } } });
  const first = syncHomeworkWindows([card]);
  const clear = syncHomeworkWindows([]);
  await expect(first).rejects.toThrow('窗口不可用');
  await expect(clear).resolves.toBeUndefined();
  expect(update).toHaveBeenNthCalledWith(2, '[]');
});
