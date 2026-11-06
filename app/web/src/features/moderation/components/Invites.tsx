// SPDX-License-Identifier: AGPL-3.0-only
// The list page of the invites screen below its heading (FR-001, manage_invites, web-client §14): the link to the
// form of a new invite (/moderation/invites/new), then the invites, newest first, in the admin mode's table (one
// page at a time, previous / next). Each row names its code with who created it (a moderator, the command line, or a
// member through their own invites, ADR-0106 §18), its state with how many signed up, when it expires, and, while it
// is usable, copying its sign-up link and revoking it, which takes effect at once and asks nothing first. An invite
// has no page of its own (the API has no operation for one). An invite lets people sign up in every registration
// mode but `closed` (a server setting, /admin/settings), which the screen says while it is `closed`.
import { Box, Button, ButtonLink, Cluster, EmptyState, Stack, StatusMessage, Text } from "@my/ui";
import { useRef, useState } from "react";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useFocusHeadingAfterLift } from "../../../lib/useFocusAfterRemoval";
import type { Invite, InviteState } from "../api/invites";
import { type RevokeInvite, useCopyInviteLink, useInvites, useInvitesWork, useRevokeInvite } from "../hooks/useInvites";
import { InviteCreator, inviteCreatorKindLabels } from "./inviteParts";
import { DateTime, type RecordColumns, RecordTable, RequirePermission } from "./parts";

const stateLabels: Readonly<Record<InviteState, ClientMessageKey>> = {
  usable: "client:admin.invites.state.usable",
  revoked: "client:admin.invites.state.revoked",
  expired: "client:admin.invites.state.expired",
  exhausted: "client:admin.invites.state.exhausted",
  creator_ineligible: "client:admin.invites.state.creator_ineligible",
};

export function Invites() {
  const t = useT();
  return (
    <RequirePermission permission="manage_invites">
      <Stack gap="4">
        <InviteModeNotice />
        <div>
          <ButtonLink href="/moderation/invites/new" variant="primary">
            {t("client:admin.invites.new.title")}
          </ButtonLink>
        </div>
        <InviteList />
      </Stack>
    </RequirePermission>
  );
}

/** Why the invites do not let anyone sign up now, while the registration mode is `closed`. */
export function InviteModeNotice() {
  const t = useT();
  if (useInvitesWork()) return null;
  return (
    <Box as="p" border="md" padding="3">
      {t("client:admin.invites.mode_notice")}
    </Box>
  );
}

function InviteList() {
  const t = useT();
  const listRef = useRef<HTMLDivElement>(null);
  const list = useInvites();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const keepFocus = useFocusHeadingAfterLift(listRef);
  const revoke = useRevokeInvite((invite) => {
    // The buttons go away from the row, which stays.
    keepFocus(invite.id);
    setStatus(t("client:admin.invites.revoked", { code: invite.code }));
  });
  const columns: RecordColumns<Invite> = [
    {
      id: "invite",
      header: t("client:admin.invites.column.invite"),
      wrap: "anywhere",
      cell: (invite) => (
        <Stack as="span" gap="1" shrink>
          <Text dir="ltr" mono wrap="anywhere">
            {invite.code}
          </Text>
          <Text size="sm" weight="normal" wrap="anywhere">
            <InviteCreator invite={invite} />
          </Text>
          <Text size="sm" weight="normal" tone="muted">
            {t(inviteCreatorKindLabels[invite.creator_kind])}
          </Text>
        </Stack>
      ),
    },
    {
      id: "state",
      header: t("client:admin.invites.state"),
      cell: (invite) => (
        <Stack as="span" gap="1" shrink>
          <Text {...(invite.state === "usable" ? { weight: "semibold" as const } : {})}>
            {t(stateLabels[invite.state])}
          </Text>
          <Text size="sm" tone="muted" numeric>
            {invite.max_uses === undefined
              ? t("client:admin.invites.used.unlimited", { uses: invite.uses })
              : t("client:admin.invites.used.limited", { uses: invite.uses, max: invite.max_uses })}
          </Text>
        </Stack>
      ),
    },
    {
      id: "expires_at",
      header: t("client:admin.invites.expires_at"),
      cell: (invite) =>
        invite.expires_at === undefined ? t("client:admin.invites.never") : <DateTime value={invite.expires_at} />,
    },
    {
      id: "actions",
      kind: "actions",
      header: t("client:admin.list.column.actions"),
      cell: (invite) => <InviteActions invite={invite} revoke={revoke} />,
    },
  ];
  return (
    <Stack gap="3">
      <StatusMessage>{status}</StatusMessage>
      <RecordTable
        listRef={listRef}
        list={list}
        caption={t("client:admin.invites.list.title")}
        columns={columns}
        getId={(invite) => invite.id}
        headingLevel={2}
        empty={
          <EmptyState
            headingLevel={2}
            heading={t("client:admin.invites.empty")}
            description={t("client:admin.invites.empty.description")}
          />
        }
      />
    </Stack>
  );
}

/** Copies and revokes a usable invite; the row's header takes focus when the buttons go away (useFocusHeadingAfterLift). */
function InviteActions({ invite, revoke }: { invite: Invite; revoke: RevokeInvite }) {
  const t = useT();
  const intl = useIntl();
  const copy = useCopyInviteLink();
  if (invite.state !== "usable") return null;
  const created = intl.formatDate(invite.created_at, { dateStyle: "medium", timeStyle: "short" });
  return (
    <Cluster gap="2">
      <Button
        size="sm"
        aria-label={t("client:admin.invites.copy_invite", {
          action: t("client:admin.invites.copy"),
          code: invite.code,
        })}
        onPress={() => {
          copy(invite);
        }}
      >
        {t("client:admin.invites.copy")}
      </Button>
      <Button
        size="sm"
        variant="danger"
        isPending={revoke.pendingIds.includes(invite.id)}
        aria-label={t("client:admin.invites.revoke_invite", { code: invite.code, date: created })}
        onPress={() => {
          revoke.revoke(invite);
        }}
      >
        {t("client:admin.invites.revoke")}
      </Button>
    </Cluster>
  );
}
