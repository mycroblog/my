// SPDX-License-Identifier: AGPL-3.0-only

// A part's action that writes returns once the server has answered the write. The client shows a write at once, before
// the server answers (an optimistic update), so a spec that asserts what the screen shows and then reloads or
// navigates can leave the write unanswered: the browser aborts the request, the server drops a write whose request
// went away, and the screen after the reload lacks it. Waiting for the response here, not in each spec, keeps every
// spec past that.

import type { Page } from "@playwright/test";

/** A write of the client API: one of `methods` on a path that matches `path`. */
type Write = { methods: readonly ("POST" | "PUT" | "PATCH" | "DELETE")[]; path: RegExp };

/** Does `act`, and returns once the server has answered the write it sends. */
export async function answered(page: Page, write: Write, act: () => Promise<void>): Promise<void> {
  const response = page.waitForResponse(
    (r) => write.methods.some((m) => m === r.request().method()) && write.path.test(new URL(r.url()).pathname),
  );
  // An action that fails leaves the wait unawaited: its timeout then rejects nothing that is unhandled.
  response.catch(() => undefined);
  await act();
  await response;
}
