// SPDX-License-Identifier: AGPL-3.0-only

// A browser that can have Web Push and plays sounds, for the component tests of the app's routes: its notifications,
// which the reader allows when asked, its push manager behind a Service Worker, and its audio, none of which a
// simulation has. The same contract as the stand-in of the features' tests (web/app-features/src/test/browser.ts),
// which that package does not export; the stories' stand-in (push.samples.ts) replaces the parts of a real browser,
// and gives a simulation only the browsers that cannot have Web Push.

import { onTestFinished } from "bun:test";
import { writeDeviceValue } from "@my/platform";

/** What the device keeps of the notification sound and of who closed the Web Push prompt. */
const kept = ["my:notification-sound", "my:notification-sound-kinds", "my:push-prompt-closed"];

// The page keeps one audio context for as long as it lives, and a test process is one page for all its tests: the
// context a test made counts for the test that runs now.
let heard: { sounds: number } | undefined;
let starting = false;

/** A node or a parameter of the audio graph, which takes whatever a sound asks of it. */
function audioPart(started: () => void): unknown {
  const part: unknown = new Proxy(() => undefined, {
    get: (_target, name) => (name === "start" ? started : name === "then" ? undefined : part),
    set: () => true,
    apply: () => part,
  });
  return part;
}

class PageAudio {
  readonly destination = {};
  readonly currentTime = 0;
  readonly state = "running";
  resume() {
    return Promise.resolve();
  }
  createOscillator() {
    // A sound is one thing to the reader, however many notes it has: they start in one turn.
    return audioPart(() => {
      if (starting) return;
      starting = true;
      queueMicrotask(() => {
        starting = false;
      });
      if (heard !== undefined) heard.sounds += 1;
    });
  }
  createGain() {
    return audioPart(() => undefined);
  }
}

/**
 * A device that keeps nothing of the notification sound or of the Web Push prompt when the test begins, and again
 * when it ends: a test process is one device for all its tests.
 */
export function quietDevice(): void {
  const forget = () => {
    for (const key of kept) writeDeviceValue(key, null);
  };
  forget();
  onTestFinished(forget);
}

/**
 * Gives the test a browser with notifications (allowed once the site asks), a Service Worker, the Push API and audio,
 * on a quiet device, until the test ends; `sounds` is how many sounds it played.
 */
export function browserWithPush(): { sounds: number } {
  quietDevice();
  const seen = { sounds: 0 };
  heard = seen;
  let permission: NotificationPermission = "default";
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
        return permission;
      },
      requestPermission: () => {
        permission = "granted";
        return Promise.resolve(permission);
      },
    },
    ServiceWorkerRegistration: Registration,
    AudioContext: PageAudio,
    PushManager: function PushManager() {
      return undefined;
    },
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
    heard = undefined;
  });
  return seen;
}
