// SPDX-License-Identifier: AGPL-3.0-only

// The app under the entry's I18nProvider (ADR-0025 §3, §4): the keymap's shortcuts, the session's data when there is
// a session, and the router's screens. It holds no state and fetches nothing of its own.

import { AppDataProvider } from "@my/app-data";
import { ShortcutProvider } from "@my/design-system";
import { RouterProvider } from "@tanstack/react-router";
import { keymap } from "./keymap";
import type { createAppRouter } from "./router";

export function AppRoot({ router }: { router: ReturnType<typeof createAppRouter> }) {
  const { data } = router.options.context;
  const screens = <RouterProvider router={router} />;
  return (
    <ShortcutProvider keymap={keymap}>
      {data === null ? screens : <AppDataProvider data={data}>{screens}</AppDataProvider>}
    </ShortcutProvider>
  );
}
