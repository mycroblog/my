// SPDX-License-Identifier: AGPL-3.0-only

// React's development build writes every render of a component, with the props that changed, to the tracks of the
// browser's Performance panel (console.timeStamp, performance.measure): no test reads them, and they took about a tenth
// of a component test's time. React looks for console.timeStamp once, as it loads, so the preloads load it without.

/** Runs `load`, which loads React, with console.timeStamp hidden, and puts it back after. */
export async function withoutReactTracks(load: () => Promise<unknown>): Promise<void> {
  const stamp: unknown = Reflect.get(console, "timeStamp");
  Reflect.deleteProperty(console, "timeStamp");
  try {
    await load();
  } finally {
    Reflect.set(console, "timeStamp", stamp);
  }
}
