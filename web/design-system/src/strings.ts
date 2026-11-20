// SPDX-License-Identifier: AGPL-3.0-only

// The design system's strings in a locale other than en, loaded on demand (messages-load.gen.ts, ADR-0009 §1): the
// entry of a page loads its reader's before it shows them, so that it downloads no other locale's. Apart from the
// package's index, so that a page that cannot load a module on demand (the payment entry) never imports it, and reads
// every locale with strings-every.ts instead.

import { stringsIn } from "./locale";
import { loaders } from "./messages-load.gen";

/** Loads the design system's strings in the locale `locale` resolves to, unless they are read already. */
export const loadStrings = (locale: string): Promise<void> => stringsIn.load(locale, loaders);

/** Whether the design system's strings in the locale `locale` resolves to are read. */
export const hasStrings = (locale: string): boolean => stringsIn.has(locale);
