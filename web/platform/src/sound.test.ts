// SPDX-License-Identifier: AGPL-3.0-only

// The notification sound (ADR-0086 §12): one built-in sound, played through the page's audio. A notification that
// arrives plays it only where the browser lets the page's audio run by itself; the settings' Play the sound, which a
// reader presses, asks the browser to let it run, and plays. Both answer whether the sound played, and neither fails
// in a browser without audio.

import { afterEach, describe, expect, test } from "bun:test";
import { playSound, trySound } from "./sound";

// The page keeps its audio for as long as it lives, so the browser of these tests is one for the whole file: what it
// lets the page's audio do is set by each test.
const browser = {
  /** Whether the audio runs by itself, as after a reader's gesture on the page. */
  running: false,
  /** Whether asking lets it run: a press does, and a page that plays by itself is refused. */
  lets: false,
  /** How many times the page asked to let its audio run. */
  asked: 0,
  /** How many notes started. */
  notes: 0,
  /** The names of the locks the page asked the browser's tabs for. */
  locks: [] as string[],
};

/** A node or a parameter of the audio graph, which takes whatever a sound asks of it. */
function part(started: () => void): unknown {
  const made: unknown = new Proxy(() => undefined, {
    get: (_target, name) => (name === "start" ? started : name === "then" ? undefined : made),
    set: () => true,
    apply: () => made,
  });
  return made;
}

class PageAudio {
  readonly destination = {};
  readonly currentTime = 0;
  get state(): AudioContextState {
    return browser.running ? "running" : "suspended";
  }
  resume() {
    browser.asked += 1;
    if (browser.lets) browser.running = true;
    // A browser that keeps the audio from starting leaves the request waiting.
    return browser.running ? Promise.resolve() : new Promise<void>(() => undefined);
  }
  createOscillator() {
    return part(() => (browser.notes += 1));
  }
  createGain() {
    return part(() => undefined);
  }
}

const wait = globalThis.setTimeout;

/**
 * Gives the page a browser with audio whose state is `running`, and that lets it run when asked if `lets`. What the
 * page waits for (the audio to start, the sound to end) comes at once: the test's clock in place of the wall's.
 */
function audio({ running, lets }: { running: boolean; lets: boolean }) {
  Object.assign(browser, { running, lets, asked: 0, notes: 0, locks: [] });
  Object.assign(globalThis, { AudioContext: PageAudio });
  const sooner = ((handler: TimerHandler, _ms?: number, ...rest: unknown[]) =>
    wait(handler, 0, ...rest)) as typeof setTimeout;
  globalThis.setTimeout = Object.assign(sooner, wait);
}

afterEach(() => {
  globalThis.setTimeout = wait;
  Reflect.deleteProperty(globalThis, "AudioContext");
  Reflect.deleteProperty(navigator, "locks");
});

describe("the notification sound", () => {
  // First: the page has made no audio yet.
  test("in a browser without audio, nothing plays and nothing fails", async () => {
    expect(await playSound()).toBe(false);
    expect(await trySound()).toBe(false);
  });

  test("where the browser keeps the page's audio from starting, a notification's sound does not play", async () => {
    audio({ running: false, lets: false });

    expect(await playSound()).toBe(false);

    expect(browser.notes).toBe(0);
  });

  test("the reader's own press asks the browser to let the audio run, and plays", async () => {
    audio({ running: false, lets: true });

    expect(await trySound()).toBe(true);

    expect(browser.asked).toBeGreaterThan(0);
    expect(browser.notes).toBeGreaterThan(0);
  });

  test("once the audio runs, a notification's sound plays", async () => {
    audio({ running: true, lets: true });

    expect(await playSound()).toBe(true);

    expect(browser.notes).toBeGreaterThan(0);
  });

  test("a press where the browser still refuses says that nothing played", async () => {
    audio({ running: false, lets: false });

    expect(await trySound()).toBe(false);

    expect(browser.notes).toBe(0);
  });
});

// Every open tab of the browser hears of the same notification: they share one lock, and the tab that holds it plays.
describe("among the browser's tabs", () => {
  /** Gives the browser Web Locks, which grant the tabs' lock to this tab (`granted`) or say another tab holds it. */
  function tabs(granted: boolean) {
    Object.defineProperty(navigator, "locks", {
      configurable: true,
      value: {
        request: (name: string, options: LockOptions, held: (lock: { name: string } | null) => unknown) => {
          browser.locks.push(name);
          // Asked with `ifAvailable`, a lock another tab holds is answered with none, at once.
          return Promise.resolve(held(granted || options.ifAvailable !== true ? { name } : null));
        },
      },
    });
  }

  test("a tab that another tab's sound keeps the lock from plays nothing for a notification", async () => {
    audio({ running: true, lets: true });
    tabs(false);

    expect(await playSound()).toBe(false);

    expect(browser.locks).toHaveLength(1);
    expect(browser.notes).toBe(0);
  });

  test("the tab that gets the lock plays", async () => {
    audio({ running: true, lets: true });
    tabs(true);

    expect(await playSound()).toBe(true);

    expect(browser.locks).toHaveLength(1);
    expect(browser.notes).toBeGreaterThan(0);
  });

  test("the reader's own press plays in the tab they pressed in, whatever the other tabs do", async () => {
    audio({ running: true, lets: true });
    tabs(false);

    expect(await trySound()).toBe(true);

    expect(browser.notes).toBeGreaterThan(0);
  });
});
