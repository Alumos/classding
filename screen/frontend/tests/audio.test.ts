import { afterEach, describe, expect, it, vi } from 'vitest';
import { playNotice } from '../src/audio';

function stubSpeech() {
  const spoken: string[] = [];
  class Utterance {
    onend?: () => void;
    constructor(public text: string) {}
  }
  const speechSynthesis = {
    getVoices: () => [],
    cancel: vi.fn(),
    speak: (utterance: Utterance) => {
      spoken.push(utterance.text);
      queueMicrotask(() => utterance.onend?.());
    },
  };
  vi.stubGlobal('SpeechSynthesisUtterance', Utterance);
  vi.stubGlobal('speechSynthesis', speechSynthesis);
  vi.stubGlobal('window', { speechSynthesis });
  return spoken;
}

describe('朗读次数', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('0 遍时只显示不朗读', async () => {
    const spoken = stubSpeech();
    await playNotice({ text: '请到办公室', repeat: 0 }, (url) => url, new AbortController().signal);
    expect(spoken).toEqual([]);
  });

  it('按设置的遍数朗读', async () => {
    const spoken = stubSpeech();
    await playNotice(
      { text: '请到办公室', student_name: '小明', repeat: 2 },
      (url) => url,
      new AbortController().signal,
    );
    expect(spoken).toEqual(['小明，请到办公室', '小明，请到办公室']);
  });
});
