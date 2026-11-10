// SPDX-License-Identifier: AGPL-3.0-only
import { Dialog, Hint, Stack } from "@my/ui";
import { useT } from "../../../lib/i18n";
import {
  type InteractionKind,
  type InteractionPolicy,
  InteractionPolicyFields,
  type InteractionRule,
} from "../../compose";
import type { Status } from "../api/status";

export type InteractionPolicyDialogProps = {
  value: InteractionPolicy;
  onChange: (kind: InteractionKind, rule: InteractionRule) => void;
  /** The visibility of the post, which decides who may boost and quote a followers-only or direct one. */
  visibility: Status["visibility"];
  isOpen: boolean;
  onOpenChange: (isOpen: boolean) => void;
};

/**
 * Who may reply to, boost, and quote the viewer's post (ADR-0155 §10), each rule changed at once on a choice as the
 * preferences are (the result is a toast); the changes go one at a time, in the order chosen. Interactions approved before stay; a quote's approval is revoked from the
 * quoting post, and a request that the change admits at once is approved.
 */
export function InteractionPolicyDialog({
  value,
  onChange,
  visibility,
  isOpen,
  onOpenChange,
}: InteractionPolicyDialogProps) {
  const t = useT();
  return (
    <Dialog
      title={t("client:status.interaction_policy.title")}
      closeLabel={t("client:status.interaction_policy.close")}
      size="sm"
      isOpen={isOpen}
      onOpenChange={onOpenChange}
    >
      <Stack gap="4">
        <Hint>{t("client:status.interaction_policy.description")}</Hint>
        <InteractionPolicyFields value={value} onChange={onChange} visibility={visibility} />
      </Stack>
    </Dialog>
  );
}
