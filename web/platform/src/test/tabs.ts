// SPDX-License-Identifier: AGPL-3.0-only
// A browser of several tabs for the tests: its Web Locks grant each exclusive lock to one holder at a time, in the order
// asked, and its BroadcastChannel delivers a message to the other channels of the same name. They stand in for
// navigator.locks and BroadcastChannel in the globals until `restore`.

type Grant = { readonly name: string; readonly start: () => void };

class FakeLocks {
  readonly #held = new Set<string>();
  readonly #waiting: Grant[] = [];

  request(
    name: string,
    options: { ifAvailable?: boolean; signal?: AbortSignal },
    callback: (lock: { name: string } | null) => unknown,
  ): Promise<unknown> {
    if (options.ifAvailable === true && this.#held.has(name)) return Promise.resolve(callback(null));
    return new Promise((resolve, reject) => {
      const grant: Grant = {
        name,
        start: () => {
          options.signal?.removeEventListener("abort", abort);
          this.#held.add(name);
          Promise.resolve()
            .then(() => callback({ name }))
            .then(resolve, reject)
            .finally(() => {
              this.#held.delete(name);
              const next = this.#waiting.findIndex((g) => g.name === name);
              if (next >= 0) this.#waiting.splice(next, 1)[0]?.start();
            });
        },
      };
      const abort = () => {
        this.#waiting.splice(this.#waiting.indexOf(grant), 1);
        reject(new DOMException("aborted", "AbortError"));
      };
      if (!this.#held.has(name)) {
        grant.start();
        return;
      }
      if (options.signal?.aborted === true) {
        reject(new DOMException("aborted", "AbortError"));
        return;
      }
      options.signal?.addEventListener("abort", abort, { once: true });
      this.#waiting.push(grant);
    });
  }
}

const channels = new Set<FakeBroadcastChannel>();

class FakeBroadcastChannel extends EventTarget {
  readonly name: string;

  constructor(name: string) {
    super();
    this.name = name;
    channels.add(this);
  }

  postMessage(message: unknown): void {
    for (const other of channels) {
      if (other !== this && other.name === this.name) {
        other.dispatchEvent(new MessageEvent("message", { data: structuredClone(message) }));
      }
    }
  }

  close(): void {
    channels.delete(this);
  }
}

/** Gives the globals of the test Web Locks, a BroadcastChannel, both, or neither, until `restore`. */
export function installTabs({ locks = true, broadcast = true }: { locks?: boolean; broadcast?: boolean } = {}): {
  restore: () => void;
} {
  const saved = Object.getOwnPropertyDescriptor(globalThis, "BroadcastChannel");
  Object.defineProperty(navigator, "locks", { configurable: true, value: locks ? new FakeLocks() : undefined });
  Object.defineProperty(globalThis, "BroadcastChannel", {
    configurable: true,
    writable: true,
    value: broadcast ? FakeBroadcastChannel : undefined,
  });
  return {
    restore: () => {
      Reflect.deleteProperty(navigator, "locks");
      if (saved === undefined) Reflect.deleteProperty(globalThis, "BroadcastChannel");
      else Object.defineProperty(globalThis, "BroadcastChannel", saved);
      channels.clear();
    },
  };
}
