// SPDX-License-Identifier: AGPL-3.0-only

// The features' strings in a locale other than en, with those of the packages below it, loaded on demand
// (messages-load.gen.ts, ADR-0009 §1): the entry of a page loads its reader's before it shows them, so that it
// downloads no other locale's. Apart from the package's index, so that a page that cannot load a module on demand (the
// payment entry) never imports it.

import { hasStrings as hasComponentsStrings, loadStrings as loadComponentsStrings } from "@my/app-components/strings";
import { stringsIn } from "./locale";
import { loaders } from "./messages-load.gen";

/** Loads the features' strings in the locale `locale` resolves to, and those of the packages below it. */
export const loadStrings = (locale: string): Promise<void> =>
  Promise.all([stringsIn.load(locale, loaders), loadComponentsStrings(locale)]).then(() => undefined);

/** Whether the features' strings in the locale `locale` resolves to are read, and those of the packages below it. */
export const hasStrings = (locale: string): boolean => stringsIn.has(locale) && hasComponentsStrings(locale);
