// SPDX-License-Identifier: AGPL-3.0-only

// The design system's strings in every locale, imported with the page (messages-every.gen.ts): for a page that cannot
// load a module on demand (the payment entry), the tests and the stories.

import { stringsIn } from "./locale";
import { messages } from "./messages-every.gen";

/** Reads the design system's strings in every locale from now on. */
export const readEveryLocale = (): void => {
  stringsIn.add(messages);
};
