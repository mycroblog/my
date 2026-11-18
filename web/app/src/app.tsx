// SPDX-License-Identifier: AGPL-3.0-only

// The app under the I18nProvider of the browser's locale, which the entry places (ADR-0025 §3, §4): the server's data
// (the session's when there is a session, else a visitor's, which reads what the server shows everyone), the reader's
// language and time zone over everything it shows (reader.tsx), the keymap's shortcuts, named for the reader's
// keyboard, and the router's screens. It holds no state and fetches nothing of its own.

import { AppDataProvider, type AppData } from "@my/app-data";
import { ShortcutProvider } from "@my/design-system";
import { namesCommandKey } from "@my/platform";
import { RouterProvider } from "@tanstack/react-router";
import { keymap } from "./keymap";
import { Reader } from "./reader";
import type { createAppRouter } from "./router";

const mac = namesCommandKey();

export function AppRoot({ router, visitor }: { router: ReturnType<typeof createAppRouter>; visitor: AppData }) {
  const { data } = router.options.context;
  return (
    <AppDataProvider data={data ?? visitor}>
      <Reader signedIn={data !== null}>
        <ShortcutProvider keymap={keymap} mac={mac}>
          <RouterProvider router={router} />
        </ShortcutProvider>
      </Reader>
    </AppDataProvider>
  );
}
