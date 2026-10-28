// SPDX-License-Identifier: AGPL-3.0-only
// Who created an invite (ADR-0108 §5), on the invites screen and on the page of an account that signed up with one:
// the creator's account, a deleted account, or the command line.
import { Link, Text } from "@my/ui";
import type { components } from "@mycroblog/mykit";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { handleOf } from "../hooks/useReports";
import { AccountLabel } from "./reportParts";

type InviteCreatorKind = components["schemas"]["InviteCreatorKind"];

/** The creator fields that the admin invite and the invite of an account share. */
type CreatedInvite = Pick<components["schemas"]["AdminInvite"], "created_by" | "created_by_deleted">;

/** Whether a moderator (or the command line) or a member created an invite. */
export const inviteCreatorKindLabels: Readonly<Record<InviteCreatorKind, ClientMessageKey>> = {
  staff: "client:admin.invites.creator_kind.staff",
  member: "client:admin.invites.creator_kind.member",
};

/**
 * The creator of an invite. With `linked`, the creator's account links to its admin page (for a viewer with
 * manage_accounts); otherwise it is named only.
 */
export function InviteCreator({ invite, linked = false }: { invite: CreatedInvite; linked?: boolean }) {
  const t = useT();
  const creator = invite.created_by;
  if (creator === undefined) {
    return invite.created_by_deleted
      ? t("client:admin.invites.created_by.deleted")
      : t("client:admin.invites.created_by.cli");
  }
  if (!linked) return <AccountLabel account={creator} />;
  return (
    <Link href={`/moderation/accounts/${encodeURIComponent(creator.id)}`} variant="standalone">
      <Text dir="ltr" wrap="anywhere">
        {handleOf(creator)}
      </Text>
    </Link>
  );
}
