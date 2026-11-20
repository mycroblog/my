// SPDX-License-Identifier: AGPL-3.0-only

// The app's strings in every locale, with those of the packages below it, imported with the page
// (messages-every.gen.ts): for a page that cannot load a module on demand, the tests and the stories.

import { readEveryLocale as readFeaturesLocales } from "@my/app-features/strings-every";
import { stringsIn } from "./locale";
import { messages } from "./messages-every.gen";

/** Reads the app's strings in every locale from now on, and those of the packages below it. */
export const readEveryLocale = (): void => {
  stringsIn.add(messages);
  readFeaturesLocales();
};
