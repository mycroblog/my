// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, expect, test } from "bun:test";
import { clearCaches } from "./caches";

const original = {
  caches: Object.getOwnPropertyDescriptor(globalThis, "caches"),
  navigator: Object.getOwnPropertyDescriptor(globalThis, "navigator"),
};

/**
 * Gives the test a browser: with the Cache API holding the caches named `caches`, unless it has none (`null`), and
 * with the service worker registrations of the scopes `workers`, unless it has no service workers (`null`). It
 * returns what the browser still holds of each.
 */
function browser({ caches, workers }: { caches: readonly string[] | null; workers: readonly string[] | null }) {
  const held = new Set(caches ?? []);
  const registered = new Set(workers ?? []);
  Object.defineProperty(globalThis, "caches", {
    configurable: true,
    value:
      caches === null
        ? undefined
        : { keys: () => Promise.resolve([...held]), delete: (name: string) => Promise.resolve(held.delete(name)) },
  });
  Object.defineProperty(globalThis, "navigator", {
    configurable: true,
    value:
      workers === null
        ? {}
        : {
            serviceWorker: {
              getRegistrations: () =>
                Promise.resolve(
                  [...registered].map((scope) => ({
                    scope,
                    unregister: () => Promise.resolve(registered.delete(scope)),
                  })),
                ),
            },
          },
  });
  return { caches: () => [...held], workers: () => [...registered] };
}

afterEach(() => {
  for (const name of ["caches", "navigator"] as const) {
    const had = original[name];
    if (had) Object.defineProperty(globalThis, name, had);
    else Reflect.deleteProperty(globalThis, name);
  }
});

/** What `clearCaches` settles with: "cleared", or what it rejected with. */
const cleared = () =>
  clearCaches().then(
    () => "cleared",
    (error: unknown) => error,
  );

test("every cache of the origin is deleted and every service worker registration unregistered", async () => {
  const held = browser({ caches: ["shell-v3", "home-timeline", "media"], workers: ["/", "/old/"] });

  expect(await cleared()).toBe("cleared");

  expect(held.caches()).toEqual([]);
  expect(held.workers()).toEqual([]);
});

test("a browser that holds no cache and no registration has nothing to remove", async () => {
  const held = browser({ caches: [], workers: [] });

  expect(await cleared()).toBe("cleared");

  expect(held.caches()).toEqual([]);
  expect(held.workers()).toEqual([]);
});

test("a browser without service workers has no registration to remove, and loses its caches", async () => {
  const held = browser({ caches: ["shell-v3"], workers: null });

  expect(await cleared()).toBe("cleared");

  expect(held.caches()).toEqual([]);
});

test("a browser without the Cache API has no cache to delete, and loses its registrations", async () => {
  const held = browser({ caches: null, workers: ["/"] });

  expect(await cleared()).toBe("cleared");

  expect(held.workers()).toEqual([]);
});

test("a browser with neither is left alone", async () => {
  browser({ caches: null, workers: null });

  expect(await cleared()).toBe("cleared");
});
