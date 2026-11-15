// SPDX-License-Identifier: AGPL-3.0-only

// The confirmation before the reader's post is deleted (FR-108), from its more menu's Delete… or Delete and redraft…:
// the question, what follows it (for a redraft, that a poll's votes are lost when the post has one), and Cancel beside
// the confirmation, which names the same verb as the item.

import { ConfirmDialog, useLocale } from "@my/design-system";
import { stringsIn } from "./locale";

export type DeleteConfirmProps = {
  /** Delete and redraft, which opens the post's text, images and poll in the composer, or delete alone. */
  redraft?: boolean | undefined;
  /** The post has a poll, whose votes a redraft loses. */
  poll?: boolean | undefined;
  /** The deletion in flight. */
  busy?: boolean | undefined;
  onCancel: () => void;
  onConfirm: () => void;
  /** Over the page, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
};

export function DeleteConfirm({ redraft = false, poll = false, busy, onCancel, onConfirm, layer }: DeleteConfirmProps) {
  const { t } = stringsIn(useLocale().locale);
  return (
    <ConfirmDialog
      title={t(redraft ? "post.redraft.title" : "post.delete.title")}
      cancel={t("post.delete.cancel")}
      confirm={t(redraft ? "post.redraft.confirm" : "post.delete.confirm")}
      busy={busy}
      onCancel={onCancel}
      onConfirm={onConfirm}
      layer={layer}
    >
      {redraft ? t("post.redraft.body", { poll: poll ? "yes" : "no" }) : t("post.delete.body")}
    </ConfirmDialog>
  );
}
