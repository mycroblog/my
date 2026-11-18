// SPDX-License-Identifier: AGPL-3.0-only

// The browser's own parts that the notification settings turn on and off, for the tests of a feature: its
// notifications, its push manager behind a Service Worker, and its audio, none of which a simulation has; and what the
// device keeps of them (the notification sound, its kinds, who closed the Web Push prompt). What the browser's own
// parts answer is the platform's test (web/platform/src/push.test.ts, sound.test.ts).

import { onTestFinished } from "bun:test";
import { writeDeviceValue } from "@my/platform";

/** What the stubbed browser does: what the site's notifications are, how subscribing answers, and whether it plays. */
export type PushBrowser = {
  /** `Notification.permission` before any question; `default` by default. */
  permission?: NotificationPermission;
  /** What the question answers; `granted` by default. */
  answer?: NotificationPermission;
  /** How the push manager subscribes: at once (`ok`, by default), or after a first failure (`fails-once`). */
  subscribe?: "ok" | "fails-once";
  /** The browser has no Push API. */
  noPushApi?: boolean;
  /** The page's audio: it plays (`plays`, by default), or the browser keeps it from starting (`blocked`). */
  audio?: "plays" | "blocked";
};

/** What the browser saw of the site: its questions, its subscriptions, and the sounds it started. */
export type SeenByBrowser = { asked: number; subscribed: number; played: number };

// The page keeps one audio context for as long as it lives, and a test process is one page for all its tests: the
// context a test made answers for the test that runs now.
const audio = { state: "plays" as "plays" | "blocked", seen: undefined as SeenByBrowser | undefined };

/** A node or a parameter of the audio graph, which takes whatever a sound asks of it. */
function audioPart(started: () => void): unknown {
  const part: unknown = new Proxy(() => undefined, {
    get: (_target, name) => (name === "start" ? started : name === "then" ? undefined : part),
    set: () => true,
    apply: () => part,
  });
  return part;
}

/** Whether a sound's notes are starting in this turn: a sound is one thing to the reader, however many notes it has. */
let starting = false;

function noteStarted(): void {
  if (starting) return;
  starting = true;
  queueMicrotask(() => {
    starting = false;
  });
  if (audio.seen !== undefined) audio.seen.played += 1;
}

class PageAudio {
  readonly destination = {};
  readonly currentTime = 0;
  get state(): AudioContextState {
    return audio.state === "plays" ? "running" : "suspended";
  }
  resume() {
    return Promise.resolve();
  }
  createOscillator() {
    return audioPart(noteStarted);
  }
  createGain() {
    return audioPart(() => undefined);
  }
}

/**
 * Gives the test a browser with notifications, a Service Worker, audio and (unless `noPushApi`) the Push API, until
 * the test ends; and returns what it saw: how many times the site asked, subscribed, and played a sound.
 */
export function pushBrowser({
  permission = "default",
  answer = "granted",
  subscribe = "ok",
  noPushApi = false,
  audio: plays = "plays",
}: PushBrowser = {}): SeenByBrowser {
  const seen: SeenByBrowser = { asked: 0, subscribed: 0, played: 0 };
  audio.state = plays;
  audio.seen = seen;
  let now = permission;
  let failures = subscribe === "fails-once" ? 1 : 0;
  const endpoint = "https://push.example/s/test";
  const made = {
    endpoint,
    expirationTime: null,
    options: { userVisibleOnly: true, applicationServerKey: null },
    getKey: (name: string) =>
      name === "p256dh" ? new Uint8Array([4, 1, 2]).buffer : name === "auth" ? new Uint8Array([9, 9]).buffer : null,
    toJSON: () => ({ endpoint, expirationTime: null, keys: { p256dh: "BAEC", auth: "CQk" } }),
    unsubscribe: () => Promise.resolve(true),
  };
  let current: typeof made | null = null;
  class Registration {
    readonly active = {};
    readonly pushManager = {
      getSubscription: () => Promise.resolve(current),
      subscribe: () => {
        seen.subscribed += 1;
        if (failures > 0) {
          failures -= 1;
          return Promise.reject(new DOMException("The push service is not available", "AbortError"));
        }
        current = made;
        return Promise.resolve(current);
      },
    };
    showNotification() {
      return Promise.resolve();
    }
  }
  const registration = new Registration();
  const globals = {
    Notification: {
      get permission() {
        return now;
      },
      requestPermission: () => {
        seen.asked += 1;
        now = answer;
        return Promise.resolve(answer);
      },
    },
    ServiceWorkerRegistration: Registration,
    AudioContext: PageAudio,
    ...(noPushApi
      ? {}
      : {
          PushManager: function PushManager() {
            return undefined;
          },
        }),
  };
  Object.assign(globalThis, globals);
  // The browser's navigator, reached as a stub reaches it: only @my/platform reads the platform's globals in the app.
  const browser = Reflect.get(globalThis, "navigator") as object;
  Object.defineProperty(browser, "serviceWorker", {
    configurable: true,
    value: {
      ready: Promise.resolve(registration),
      controller: {},
      getRegistration: () => Promise.resolve(registration),
      register: () => Promise.resolve(registration),
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    },
  });
  onTestFinished(() => {
    for (const name of Object.keys(globals)) Reflect.deleteProperty(globalThis, name);
    Reflect.deleteProperty(browser, "serviceWorker");
    audio.seen = undefined;
  });
  return seen;
}

/**
 * A device that keeps nothing of the notification sound, of its kinds, or of who closed the Web Push prompt, when the
 * test begins and again when it ends: a test process is one device for all its tests. Where the device keeps them is
 * all a test knows of how; it sets them through the app's data (`setNotificationSound`, `setNotificationSoundKinds`).
 */
export function quietDevice(): void {
  const forget = () => {
    for (const key of ["my:notification-sound", "my:notification-sound-kinds", "my:push-prompt-closed"])
      writeDeviceValue(key, null);
  };
  forget();
  onTestFinished(forget);
}
