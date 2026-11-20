// SPDX-License-Identifier: AGPL-3.0-only

// The app's strings in a locale other than en, with those of the packages below it, loaded on demand
// (messages-load.gen.ts, ADR-0009 §1): the entry of a page loads its reader's before it shows them, so that it
// downloads no other locale's. Apart from the package's index, so that a page that cannot load a module on demand (the
// payment entry) never imports it.

import { hasStrings as hasFeaturesStrings, loadStrings as loadFeaturesStrings } from "@my/app-features/strings";
import { stringsIn } from "./locale";
import { loaders } from "./messages-load.gen";

/** Loads the app's strings in the locale `locale` resolves to, and those of the packages below it. */
export const loadStrings = (locale: string): Promise<void> =>
  Promise.all([stringsIn.load(locale, loaders), loadFeaturesStrings(locale)]).then(() => undefined);

/** Whether the app's strings in the locale `locale` resolves to are read, and those of the packages below it. */
export const hasStrings = (locale: string): boolean => stringsIn.has(locale) && hasFeaturesStrings(locale);
