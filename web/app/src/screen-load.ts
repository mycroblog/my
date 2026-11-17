// SPDX-License-Identifier: AGPL-3.0-only

// What a screen's loader returns. Once a screen is in place, the loader waits for the screen's data, so that a
// navigation keeps the screen it leaves until the next one can show what is fresh (router.ts). On the first load there
// is no screen to keep: the loader starts the requests without waiting, so that the shell and the screen in its own
// state of waiting show at once, never a blank page, and the screen fills in as its requests answer.

import type { AppData } from "@my/app-data";
import type { RouterContext } from "./router";

/** Starts `load` for a signed-in reader, and waits for it unless this is the first load. */
export function screenLoad(context: RouterContext, load: (data: AppData) => Promise<void>): Promise<void> | undefined {
  if (context.data === null) return undefined;
  const loading = load(context.data);
  return context.shown() ? loading : undefined;
}
