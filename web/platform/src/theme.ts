// SPDX-License-Identifier: AGPL-3.0-only

// The colour scheme the reader chose for the app on this device (ADR-0028 §18, the device's state): the device's own
// (`prefers-color-scheme`), light, or dark. It is kept on the device, not with the account, so that a phone can be
// dark and a computer light. The stylesheet follows `<html data-theme>` (the design system's theme.ts); the shell's
// two `theme-color` metas, each for one scheme of the system, follow it too: a chosen scheme's meta applies at every
// scheme of the system and the other's at none, and with the device's own each applies to its scheme again.

import { readDeviceValue, writeDeviceValue } from "./device-values";

/** The reader's choice of colour scheme. */
export type ThemeChoice = "system" | "light" | "dark";

/** The device's key of the choice, by which another tab's change is told too (the `storage` event). */
const key = "my:theme";

const isChoice = (value: string | null): value is ThemeChoice =>
  value === "system" || value === "light" || value === "dark";

/** The choice kept on this device; the device's own scheme when none is kept. */
export function readThemeChoice(): ThemeChoice {
  const kept = readDeviceValue(key);
  return isChoice(kept) ? kept : "system";
}

/** The scheme of each `theme-color` meta of the shell, by its media query: what it is for while the device decides. */
function schemeOf(meta: HTMLMetaElement): "light" | "dark" | undefined {
  const media = meta.dataset["scheme"] ?? meta.media;
  if (media.includes("prefers-color-scheme: dark")) return "dark";
  if (media.includes("prefers-color-scheme: light")) return "light";
  return undefined;
}

/** Puts `choice` on the document: `data-theme` on its root, and the `theme-color` of the chosen scheme. */
function applyThemeChoice(choice: ThemeChoice, doc: Document = document): void {
  const root = doc.documentElement;
  if (choice === "system") delete root.dataset["theme"];
  else root.dataset["theme"] = choice;
  for (const meta of doc.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]')) {
    const scheme = schemeOf(meta);
    if (scheme === undefined) continue;
    // The query the shell wrote, kept once, to go back to.
    meta.dataset["scheme"] ??= meta.media;
    meta.media = choice === "system" ? meta.dataset["scheme"] : choice === scheme ? "all" : "not all";
  }
}

const listeners = new Set<() => void>();

/** Keeps `choice` on this device and shows it at once, here and in the app's other tabs. */
export function keepThemeChoice(choice: ThemeChoice): void {
  writeDeviceValue(key, choice === "system" ? null : choice);
  applyThemeChoice(choice);
  for (const listener of listeners) listener();
}

/**
 * Calls `listener` when the choice changes, here or in another tab of the app (which this tab shows too, once
 * `followThemeChoice` runs); returns the call that stops it.
 */
export function watchThemeChoice(listener: () => void): () => void {
  listeners.add(listener);
  const fromElsewhere = (e: StorageEvent) => {
    if (e.key === key || e.key === null) listener();
  };
  window.addEventListener("storage", fromElsewhere);
  return () => {
    listeners.delete(listener);
    window.removeEventListener("storage", fromElsewhere);
  };
}

/** Shows the kept choice now, and the one another tab keeps from then on: the app's entry calls it before it renders. */
export function followThemeChoice(doc: Document = document): void {
  applyThemeChoice(readThemeChoice(), doc);
  watchThemeChoice(() => {
    applyThemeChoice(readThemeChoice(), doc);
  });
}
