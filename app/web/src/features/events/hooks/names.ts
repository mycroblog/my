// SPDX-License-Identifier: AGPL-3.0-only
import type { EventAccount } from "../api/events";

/**
 * A name inside a sentence of the UI's language (an event's title or an account's name). It is the user's text in any
 * script, so it is wrapped in Unicode directional isolates (FSI … PDI): a right-to-left name does not reorder the
 * sentence around it (web-client/README.md Y10).
 */
export function isolate(name: string): string {
  return `⁨${name}⁩`;
}

/** The name an account goes by in the UI. */
export function accountName(account: EventAccount): string {
  return account.display_name || account.username;
}
