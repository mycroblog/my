// SPDX-License-Identifier: AGPL-3.0-only

// What a post says over it, to its reader alone, of the reader's interaction that waits for its author's approval
// (ADR-0155 §7): the reader's own reply waiting for the approval of the author it replies to, or not approved, and a
// post whose boost by the reader waits for its author's. Nothing for any other post.

import type { PostStatus } from "@my/app-components";
import { CaptionLine, useLocale } from "@my/design-system";
import { stringsIn } from "./locale";

/** The line over `status` (a boost's boosted post) of the reader's reply or boost that waits, or none. */
export function ApprovalLine({ status }: { status: PostStatus }) {
  const { t } = stringsIn(useLocale().locale);
  const shown = status.boost ?? status;
  if (shown.approval === "rejected")
    return (
      <CaptionLine icon="error" alert>
        {t("post_actions.approval_rejected")}
      </CaptionLine>
    );
  if (shown.approval === "pending")
    return (
      <CaptionLine icon="time" waiting>
        {t("post_actions.approval_pending")}
      </CaptionLine>
    );
  if (shown.boost_approval === "pending")
    return (
      <CaptionLine icon="time" waiting>
        {t("post_actions.boost_waits")}
      </CaptionLine>
    );
  return null;
}
