// SPDX-License-Identifier: AGPL-3.0-only

// The app under the entry's I18nProvider (ADR-0025 §3, §4): the keymap's shortcuts, named for the reader's keyboard,
// the server's data (the session's when there is a session, else a visitor's, which reads what the server shows
// everyone), and the router's screens. It holds no state and fetches nothing of its own.

import { AppDataProvider, type AppData } from "@my/app-data";
import { ShortcutProvider } from "@my/design-system";
import { namesCommandKey } from "@my/platform";
import { RouterProvider } from "@tanstack/react-router";
import { keymap } from "./keymap";
import type { createAppRouter } from "./router";

const mac = namesCommandKey();

export function AppRoot({ router, visitor }: { router: ReturnType<typeof createAppRouter>; visitor: AppData }) {
  const { data } = router.options.context;
  return (
    <ShortcutProvider keymap={keymap} mac={mac}>
      <AppDataProvider data={data ?? visitor}>
        <RouterProvider router={router} />
      </AppDataProvider>
    </ShortcutProvider>
  );
}
