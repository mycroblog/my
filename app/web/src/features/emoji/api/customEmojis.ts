// SPDX-License-Identifier: AGPL-3.0-only
// The custom emoji that the picker offers (GET /custom_emojis, ADR-0051 §4): every enabled local emoji not hidden
// from the picker, by category and shortcode. Like the instance's information (ADR-0028), it changes only when an
// administrator changes the emoji, and the admin screen marks it stale then.
import type { Api } from "../../../lib/api/client";

/** The custom emoji of the picker. */
export function customEmojisQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/custom_emojis");
}
