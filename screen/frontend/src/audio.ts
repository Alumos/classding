import { native } from './native';

let context: AudioContext | undefined;
let audioTail: Promise<void> = Promise.resolve();
async function exclusiveAudio<T>(work: () => Promise<T>): Promise<T> {
  const previous = audioTail;
  let release!: () => void;
  audioTail = new Promise<void>((resolve) => {
    release = resolve;
  });
  await previous;
  try {
    return await work();
  } finally {
    release();
  }
}
export async function unlockAudio() {
  try {
    context ||= new AudioContext();
    if (context.state === 'suspended') await context.resume();
  } catch {
    /* UI offers a test button if the browser blocks playback. */
  }
}
function aborted(signal: AbortSignal) {
  if (signal.aborted) throw new DOMException('播放已取消', 'AbortError');
}
export async function ding(signal: AbortSignal, volume = 100) {
  aborted(signal);
  await unlockAudio();
  aborted(signal);
  if (!context || context.state !== 'running') throw new Error('浏览器阻止了提示音，请先点击测试播报');
  const ctx = context;
  await new Promise<void>((resolve, reject) => {
    const master = ctx.createGain();
    master.gain.value = Math.max(0, Math.min(1, volume / 100)) * 0.28;
    master.connect(ctx.destination);
    const start = ctx.currentTime;
    const tones = [880, 659.25].map((frequency, index) => {
      const osc = ctx.createOscillator();
      const gain = ctx.createGain();
      osc.type = 'sine';
      osc.frequency.value = frequency;
      gain.gain.setValueAtTime(0.0001, start + index * 0.24);
      gain.gain.exponentialRampToValueAtTime(1, start + index * 0.24 + 0.02);
      gain.gain.exponentialRampToValueAtTime(0.0001, start + index * 0.24 + 0.42);
      osc.connect(gain).connect(master);
      osc.start(start + index * 0.24);
      osc.stop(start + index * 0.24 + 0.44);
      return osc;
    });
    const finish = () => {
      signal.removeEventListener('abort', cancel);
      master.disconnect();
      resolve();
    };
    const cancel = () => {
      tones.forEach((osc) => {
        try {
          osc.stop();
        } catch {}
      });
      signal.removeEventListener('abort', cancel);
      reject(new DOMException('播放已取消', 'AbortError'));
    };
    signal.addEventListener('abort', cancel, { once: true });
    tones[1].onended = finish;
  });
}
function speak(text: string, signal: AbortSignal, volume: number): Promise<void> {
  aborted(signal);
  if (!('speechSynthesis' in window) || typeof SpeechSynthesisUtterance === 'undefined')
    return Promise.reject(new Error('此设备不支持本地语音'));
  return new Promise((resolve, reject) => {
    const utterance = new SpeechSynthesisUtterance(text);
    utterance.lang = 'zh-CN';
    utterance.volume = Math.max(0, Math.min(1, volume / 100));
    const voices = speechSynthesis.getVoices();
    utterance.voice =
      voices.find((v) => /^zh[-_]?CN/i.test(v.lang)) || voices.find((v) => /^zh/i.test(v.lang)) || null;
    const clean = () => signal.removeEventListener('abort', cancel);
    const cancel = () => {
      speechSynthesis.cancel();
      clean();
      reject(new DOMException('播放已取消', 'AbortError'));
    };
    utterance.onend = () => {
      clean();
      resolve();
    };
    utterance.onerror = (event) => {
      clean();
      reject(new Error(`本地语音播放失败：${event.error}`));
    };
    signal.addEventListener('abort', cancel, { once: true });
    speechSynthesis.speak(utterance);
  });
}
function playAudio(url: string, signal: AbortSignal, volume: number): Promise<void> {
  aborted(signal);
  return new Promise((resolve, reject) => {
    const audio = new Audio(url);
    audio.volume = Math.max(0, Math.min(1, volume / 100));
    const clean = () => {
      signal.removeEventListener('abort', cancel);
      audio.onended = null;
      audio.onerror = null;
    };
    const cancel = () => {
      audio.pause();
      audio.removeAttribute('src');
      clean();
      reject(new DOMException('播放已取消', 'AbortError'));
    };
    audio.onended = () => {
      clean();
      resolve();
    };
    audio.onerror = () => {
      clean();
      reject(new Error('音频无法播放'));
    };
    signal.addEventListener('abort', cancel, { once: true });
    void audio.play().catch((error) => {
      clean();
      reject(error);
    });
  });
}
export async function playChime(signal: AbortSignal, volume: number) {
  return exclusiveAudio(async () => {
    aborted(signal);
    let prepared = false;
    try {
      if (native()) {
        await native()!.PrepareAudio(volume);
        prepared = true;
      }
      await ding(signal, native() ? 100 : volume);
    } finally {
      if (prepared) await native()!.RestoreAudio();
    }
  });
}
export async function playNotice(
  notice: {
    text: string;
    student_name?: string;
    audio_url?: string;
    chime?: boolean;
    repeat?: number;
    volume?: number;
  },
  audioURL: (url: string) => string,
  signal: AbortSignal,
) {
  return exclusiveAudio(async () => {
    aborted(signal);
    const volume = Math.max(0, Math.min(100, notice.volume ?? 80));
    let prepared = false;
    try {
      if (native()) {
        await native()!.PrepareAudio(volume);
        prepared = true;
      }
      if (notice.chime) await ding(signal, native() ? 100 : volume);
      const text = [notice.student_name, notice.text].filter(Boolean).join('，');
      // Zero repeats shows the notice silently, apart from the optional chime.
      for (let i = 0; i < Math.max(0, Math.min(10, notice.repeat ?? 1)); i++) {
        aborted(signal);
        if (notice.audio_url) {
          try {
            await playAudio(audioURL(notice.audio_url), signal, native() ? 100 : volume);
            continue;
          } catch (error) {
            if (signal.aborted || !text) throw error;
          }
        }
        if (text) await speak(text, signal, native() ? 100 : volume);
        else throw new Error('没有可播放的文字或音频');
      }
    } finally {
      if (prepared) await native()!.RestoreAudio();
    }
  });
}
