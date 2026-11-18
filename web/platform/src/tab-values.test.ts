// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, expect, test } from "bun:test";
import { forgetTabValues, readTabValue, writeTabValue } from "./tab-values";

const original = Object.getOwnPropertyDescriptor(globalThis, "sessionStorage");

afterEach(() => {
  if (original) Object.defineProperty(globalThis, "sessionStorage", original);
  else Reflect.deleteProperty(globalThis, "sessionStorage");
});

/**
 * Gives the test a tab whose session storage holds `values`, as a browser's does: its keys by index and as its own
 * members. It returns what the storage holds.
 */
function tab(values: Record<string, string>): () => Record<string, string> {
  const held = new Map(Object.entries(values));
  const storage = {
    get length() {
      return held.size;
    },
    key: (index: number) => [...held.keys()][index] ?? null,
    getItem: (key: string) => held.get(key) ?? null,
    setItem: (key: string, value: string) => {
      held.set(key, value);
    },
    removeItem: (key: string) => {
      held.delete(key);
    },
    clear: () => {
      held.clear();
    },
  };
  const own = new Proxy(storage, {
    ownKeys: () => [...held.keys()],
    getOwnPropertyDescriptor: (target, key) =>
      typeof key === "string" && held.has(key)
        ? { value: held.get(key), writable: true, enumerable: true, configurable: true }
        : Reflect.getOwnPropertyDescriptor(target, key),
  });
  Object.defineProperty(globalThis, "sessionStorage", { configurable: true, value: own });
  return () => Object.fromEntries(held);
}

/** Gives the test a tab whose session storage the browser blocks: reaching it throws, as a blocked one does. */
function blockedTab(): void {
  Object.defineProperty(globalThis, "sessionStorage", {
    configurable: true,
    get: () => {
      throw new DOMException("The operation is insecure.", "SecurityError");
    },
  });
}

test("forgetTabValues removes every key that starts with the prefix, and no other", () => {
  const held = tab({
    "my.confirmation.password_change": "a",
    "my.confirmation.link": "b",
    "my.confirmationless": "c",
    "my.other": "d",
  });

  forgetTabValues("my.confirmation.");

  expect(held()).toEqual({ "my.confirmationless": "c", "my.other": "d" });
});

test("forgetTabValues of a prefix no key has leaves the storage as it was", () => {
  const held = tab({ "my.other": "d" });

  forgetTabValues("my.confirmation.");

  expect(held()).toEqual({ "my.other": "d" });
});

test("a tab whose storage is blocked has nothing to forget, and forgetTabValues throws nothing", () => {
  blockedTab();

  expect(() => {
    forgetTabValues("my.confirmation.");
  }).not.toThrow();
});

test("what a tab wrote is read again until it is forgotten", () => {
  tab({});

  writeTabValue("my.confirmation.link", "b");
  expect(readTabValue("my.confirmation.link")).toBe("b");
  forgetTabValues("my.confirmation.");

  expect(readTabValue("my.confirmation.link")).toBeNull();
});
