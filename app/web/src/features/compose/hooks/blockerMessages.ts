// SPDX-License-Identifier: AGPL-3.0-only
import type { ClientMessageKey } from "../../../lib/i18n";
import type { ComposeBlocker } from "./useComposeForm";

/** What the send bar, or the schedule popover, says while the draft cannot be sent: the reason and what to do about it. */
export const blockerMessages: Readonly<Record<ComposeBlocker, ClientMessageKey>> = {
  empty: "client:compose.blocked.empty",
  too_long: "client:compose.blocked.too_long",
  uploading: "client:compose.blocked.uploading",
  upload_failed: "client:compose.blocked.upload_failed",
  poll: "client:compose.blocked.poll",
  loading_target: "client:compose.blocked.loading_target",
};
