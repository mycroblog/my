// SPDX-License-Identifier: AGPL-3.0-only

// What the reader's device keeps between two runs of the app, for the component tests of its routes: the reader's
// choice of language and time zone, which the app paints first with, and the caches a service worker fills. A test
// process is one device for every test in it, so a test that leaves something there starts and ends without it.

import { onTestFinished } from "bun:test";
import { writeDeviceValue } from "@my/platform";

/**
 * A device that keeps nothing of a reader's choice when the test begins, and again when it ends: what one test's app
 * left there is not the next test's first paint. Where the device keeps it is all a test knows of how.
 */
export function ownDevice(): void {
  const forget = () => {
    writeDeviceValue("my:reader-choice", null);
  };
  forget();
  onTestFinished(forget);
}

/**
 * Gives the browser the Cache API, which no simulation has, holding caches named `names` as a service worker fills
 * them, until the test ends; it returns the names the browser still holds.
 */
export function browserCaches(...names: string[]): () => readonly string[] {
  const held = new Set(names);
  const had = Reflect.getOwnPropertyDescriptor(globalThis, "caches");
  Object.defineProperty(globalThis, "caches", {
    configurable: true,
    value: {
      keys: () => Promise.resolve([...held]),
      delete: (name: string) => Promise.resolve(held.delete(name)),
    },
  });
  onTestFinished(() => {
    if (had === undefined) Reflect.deleteProperty(globalThis, "caches");
    else Object.defineProperty(globalThis, "caches", had);
  });
  return () => [...held];
}
