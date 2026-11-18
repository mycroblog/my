// SPDX-License-Identifier: AGPL-3.0-only

// The app's one built-in sound (ADR-0086 §12): a short, soft chime of two notes, synthesized with Web Audio as it
// plays, so that the app ships no recording. A browser lets a page sound only once its reader acted on it: a press
// (`trySound`) starts the page's audio, after which `playSound` sounds without one; in a page that may not sound, and
// in a browser without Web Audio, both play nothing and say so. Of the tabs of one browser a single one sounds at a
// time (Web Locks, where the browser has them), so that what every tab hears of the stream is one sound.

/** The chime: A5, then E6 a moment later, each a sine with a little of its octave, a quick attack and a decay. */
const notes = [
  { frequency: 880, start: 0, gain: 0.32 },
  { frequency: 1318.51, start: 0.09, gain: 0.26 },
] as const;
/** The shares of a note's own pitch and of its octave. */
const partials = [
  { multiple: 1, share: 1 },
  { multiple: 2, share: 0.15 },
] as const;
const attackSeconds = 0.005;
const seconds = 0.5;
/** How long a page that may not sound yet is given to start its audio. */
const startMs = 150;
/** The lock one tab holds while it sounds. */
const lockName = "my:sound";

type AudioGlobals = { AudioContext?: typeof AudioContext; webkitAudioContext?: typeof AudioContext };

/** The page's one audio context, made on first use: a browser that let it run once lets it run again. */
let context: AudioContext | undefined;

function pageAudio(): AudioContext | undefined {
  if (context !== undefined) return context;
  const globals = globalThis as AudioGlobals;
  const Context = globals.AudioContext ?? globals.webkitAudioContext;
  if (Context === undefined) return undefined;
  try {
    context = new Context();
  } catch {
    return undefined;
  }
  return context;
}

const wait = (ms: number) => new Promise<void>((done) => setTimeout(done, ms));

/** Whether `audio` runs, starting it where the browser lets the page; a page that may not sound stays silent. */
async function running(audio: AudioContext, ms: number): Promise<boolean> {
  const runs = () => audio.state === "running";
  if (runs()) return true;
  // Where the page may not sound, the browser leaves this pending, or refuses it.
  await Promise.race([audio.resume().catch(() => undefined), wait(ms)]);
  return runs();
}

/** Schedules the chime on `audio`, from now. */
function chime(audio: AudioContext): void {
  const now = audio.currentTime;
  const end = now + seconds;
  for (const note of notes) {
    const at = now + note.start;
    const level = audio.createGain();
    level.gain.setValueAtTime(0, at);
    level.gain.linearRampToValueAtTime(note.gain, at + attackSeconds);
    // Down to silence by the end, so that stopping does not click.
    level.gain.exponentialRampToValueAtTime(0.001, end);
    level.connect(audio.destination);
    for (const partial of partials) {
      const tone = audio.createOscillator();
      tone.type = "sine";
      tone.frequency.value = note.frequency * partial.multiple;
      const part = audio.createGain();
      part.gain.value = partial.share;
      tone.connect(part);
      part.connect(level);
      tone.start(at);
      tone.stop(end);
    }
  }
}

/**
 * Plays the sound for something that arrived, with no press of the reader's: once in the browser, whichever of its
 * tabs is first, and only where the page may sound. It resolves to whether this tab played it.
 */
export async function playSound(): Promise<boolean> {
  const audio = pageAudio();
  if (audio === undefined || !(await running(audio, startMs))) return false;
  const locks = typeof navigator === "undefined" ? undefined : (navigator.locks as LockManager | undefined);
  if (locks === undefined) {
    chime(audio);
    return true;
  }
  try {
    return await locks.request(lockName, { ifAvailable: true }, async (lock): Promise<boolean> => {
      if (lock === null) return false;
      chime(audio);
      await wait(seconds * 1000);
      return true;
    });
  } catch {
    // The browser refused the lock (a document without storage access): this tab sounds by itself.
    chime(audio);
    return true;
  }
}

/**
 * Plays the sound as the reader pressed for it, which lets the page sound from then on. It resolves to whether it
 * played: not in a browser without Web Audio, or one that keeps the page silent even so.
 */
export async function trySound(): Promise<boolean> {
  const audio = pageAudio();
  if (audio === undefined || !(await running(audio, 1000))) return false;
  chime(audio);
  return true;
}
