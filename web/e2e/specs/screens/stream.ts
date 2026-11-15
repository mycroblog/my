// SPDX-License-Identifier: AGPL-3.0-only

// The stream of a signed-in reader's events (GET /api/v1/streaming, Server-Sent Events), which brings the posts made
// after it opened. A spec that makes a post for the stream to bring waits for the stream to open first: a post made
// before is neither on the page read nor on the stream.

import type { Page } from "@playwright/test";

/**
 * Settles once the server has answered the client's stream; ask before the client opens it (before signing in), and
 * await it before making what the stream should bring.
 */
export function streamOpened(page: Page): Promise<void> {
  const opened = page.waitForResponse((r) => new URL(r.url()).pathname === "/api/v1/streaming" && r.ok());
  return opened.then(() => undefined);
}
