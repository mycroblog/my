// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, expect, test } from "bun:test";
import { clearCaches } from "./caches";

const original = Object.getOwnPropertyDescriptor(globalThis, "caches");

/** Gives the browser the Cache API holding caches named `names`, and returns the names it holds now. */
function cachesNamed(...names: string[]): () => string[] {
  const held = new Set(names);
  Object.defineProperty(globalThis, "caches", {
    configurable: true,
    value: {
      keys: () => Promise.resolve([...held]),
      delete: (name: string) => Promise.resolve(held.delete(name)),
    },
  });
  return () => [...held];
}

afterEach(() => {
  if (original) Object.defineProperty(globalThis, "caches", original);
  else Reflect.deleteProperty(globalThis, "caches");
});

test("every cache the service worker filled for this origin is deleted", async () => {
  const held = cachesNamed("shell-v3", "home-timeline", "media");

  await clearCaches();

  expect(held()).toEqual([]);
});

test("a browser that holds no cache has nothing to delete", async () => {
  const held = cachesNamed();

  await clearCaches();

  expect(held()).toEqual([]);
});

test("a browser without the Cache API is left alone", async () => {
  Object.defineProperty(globalThis, "caches", { configurable: true, value: undefined });
  const cleared = clearCaches().then(
    () => "cleared",
    (error: unknown) => error,
  );

  expect(await cleared).toBe("cleared");
});
