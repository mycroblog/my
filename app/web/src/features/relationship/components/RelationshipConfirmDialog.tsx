// SPDX-License-Identifier: AGPL-3.0-only
// The question before a change of the viewer's relationship with an account that the viewer may not mean: undoing a
// follow or a request, blocking and unblocking (WAI-ARIA APG, alert dialog pattern).
import { Button, Dialog, DialogActions, Stack } from "@my/ui";
import { isolate } from "../../../lib/handle";
import { type Translate, useT } from "../../../lib/i18n";
import type { RelationshipAction } from "../api/relationship";

/** The changes that ask the viewer first. */
export type ConfirmableRelationshipChange = "unfollow" | "withdraw" | "block" | "unblock";

export type RelationshipConfirmDialogProps = {
  isOpen: boolean;
  kind: ConfirmableRelationshipChange;
  name: string;
  onCancel: () => void;
  onConfirm: (action: RelationshipAction) => void;
};

/** The texts of the confirmation of `kind`, and the change it confirms. */
function confirmationOf(t: Translate, kind: ConfirmableRelationshipChange, name: string) {
  const values = { name: isolate(name) };
  switch (kind) {
    case "unfollow":
      return {
        title: t("client:profile.confirm.unfollow.title", values),
        body: t("client:profile.confirm.unfollow.body"),
        action: t("client:profile.confirm.unfollow.action"),
        run: "unfollow",
      } as const;
    case "withdraw":
      return {
        title: t("client:profile.confirm.withdraw.title", values),
        body: t("client:profile.confirm.withdraw.body"),
        action: t("client:profile.confirm.withdraw.action"),
        run: "unfollow",
      } as const;
    case "block":
      return {
        title: t("client:profile.confirm.block.title", values),
        body: t("client:profile.confirm.block.body"),
        action: t("client:profile.confirm.block.action"),
        run: "block",
      } as const;
    case "unblock":
      return {
        title: t("client:profile.confirm.unblock.title", values),
        body: t("client:profile.confirm.unblock.body"),
        action: t("client:profile.confirm.unblock.action"),
        run: "unblock",
      } as const;
  }
}

/** Asks before a change that the viewer may not mean: one alert dialog, whose texts are the change's. */
export function RelationshipConfirmDialog({ isOpen, kind, name, onCancel, onConfirm }: RelationshipConfirmDialogProps) {
  const t = useT();
  const text = confirmationOf(t, kind, name);
  return (
    <Dialog
      role="alertdialog"
      closeLabel={null}
      size="sm"
      title={text.title}
      isOpen={isOpen}
      onOpenChange={(open) => {
        if (!open) onCancel();
      }}
    >
      <Stack gap="4">
        <p>{text.body}</p>
        <DialogActions>
          <Button onPress={onCancel}>{t("client:profile.confirm.cancel")}</Button>
          <Button
            variant={text.run === "block" ? "danger" : "primary"}
            onPress={() => {
              onConfirm(text.run);
            }}
          >
            {text.action}
          </Button>
        </DialogActions>
      </Stack>
    </Dialog>
  );
}
