// SPDX-License-Identifier: AGPL-3.0-only
// The invites screen below its heading (FR-001, manage_invites): the form of a new invite, then the invites, newest
// first, in the admin mode's table form (one page at a time, previous / next). An invite lets people sign up only while the registration mode is `invite` (a server setting, /admin/settings),
// which the screen says when it is not. A new invite's code and link stay under the form in a StatusMessage; each
// usable invite can be copied as its sign-up link or revoked, which takes effect at once and asks nothing first.
import {
  Box,
  Button,
  Cluster,
  DurationSelect,
  EmptyState,
  Heading,
  InlineError,
  Select,
  SelectItem,
  Stack,
  StatusMessage,
  TableCell,
  TableColumnHeader,
  TableRow,
  TableRowHeader,
  Text,
  VisuallyHidden,
} from "@my/ui";
import { useRef, useState } from "react";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useFocusHeadingAfterLift } from "../../../lib/useFocusAfterRemoval";
import type { Invite, InviteState } from "../api/invites";
import {
  inviteExpiries,
  inviteMaxUses,
  type RevokeInvite,
  useCopyInviteLink,
  useInviteForm,
  useInvites,
  useInvitesWork,
  useRevokeInvite,
} from "../hooks/useInvites";
import { DateTime, DetailSection, RecordTable, RequirePermission } from "./parts";

const stateLabels: Readonly<Record<InviteState, ClientMessageKey>> = {
  usable: "client:admin.invites.state.usable",
  revoked: "client:admin.invites.state.revoked",
  expired: "client:admin.invites.state.expired",
  exhausted: "client:admin.invites.state.exhausted",
  creator_ineligible: "client:admin.invites.state.creator_ineligible",
};

/** The Select's key of "no limit", which is not a number of uses. */
const unlimited = "unlimited";

export function Invites() {
  const t = useT();
  return (
    <RequirePermission permission="manage_invites">
      <Stack gap="8">
        <ModeNotice />
        <DetailSection title={t("client:admin.invites.new.title")}>
          <NewInviteForm />
        </DetailSection>
        <InviteList />
      </Stack>
    </RequirePermission>
  );
}

/** Why the invites do not let anyone sign up now, while the registration mode is `closed`. */
function ModeNotice() {
  const t = useT();
  if (useInvitesWork()) return null;
  return (
    <Box as="p" border="md" padding="3">
      {t("client:admin.invites.mode_notice")}
    </Box>
  );
}

function NewInviteForm() {
  const t = useT();
  const form = useInviteForm();
  const formatExpiry = (seconds: number) => {
    if (seconds % 86_400 === 0) return t("client:admin.settings.duration.days", { count: seconds / 86_400 });
    if (seconds % 3600 === 0) return t("client:admin.settings.duration.hours", { count: seconds / 3600 });
    return t("client:admin.invites.duration.minutes", { count: seconds / 60 });
  };
  return (
    <Stack as="form" onSubmit={form.submit} gap="6">
      <Select
        label={t("client:admin.invites.max_uses")}
        description={t("client:admin.invites.max_uses.description")}
        value={form.draft.maxUses === null ? unlimited : String(form.draft.maxUses)}
        onChange={(key) => {
          if (key === unlimited) form.change({ maxUses: null });
          else if (typeof key === "string") form.change({ maxUses: Number(key) });
        }}
        items={[
          ...inviteMaxUses.map((uses) => ({ id: String(uses), name: t("client:admin.invites.uses", { count: uses }) })),
          { id: unlimited, name: t("client:admin.invites.unlimited") },
        ]}
      >
        {(item) => (
          <SelectItem id={item.id} textValue={item.name}>
            {item.name}
          </SelectItem>
        )}
      </Select>
      <DurationSelect
        label={t("client:admin.invites.expires_in")}
        durations={inviteExpiries}
        formatDuration={formatExpiry}
        indefiniteLabel={t("client:admin.invites.never")}
        value={form.draft.expiresIn}
        onChange={(expiresIn) => {
          form.change({ expiresIn });
        }}
      />
      {form.error !== undefined && (
        <InlineError alert size="inherit">
          {t("client:admin.invites.create_failed", { reason: form.error })}
        </InlineError>
      )}
      <Box>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:admin.invites.create")}
        </Button>
      </Box>
      <StatusMessage>{form.status}</StatusMessage>
      {form.created !== undefined && (
        <Stack as="p" gap="1">
          <span>{t("client:admin.invites.link")}</span>
          <Text dir="ltr" mono wrap="anywhere" selectAll>
            {form.created.url}
          </Text>
        </Stack>
      )}
    </Stack>
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
  return (
    <Stack gap="3">
      <Heading level={2}>{t("client:admin.invites.list.title")}</Heading>
      <StatusMessage>{status}</StatusMessage>
      <Box ref={listRef}>
        <RecordTable
          list={list}
          caption={t("client:admin.invites.list.title")}
          getId={(invite) => invite.id}
          empty={
            <EmptyState
              headingLevel={3}
              heading={t("client:admin.invites.empty")}
              description={t("client:admin.invites.empty.description")}
            />
          }
          head={
            <>
              <TableColumnHeader>{t("client:admin.invites.column.invite")}</TableColumnHeader>
              <TableColumnHeader>{t("client:admin.invites.state")}</TableColumnHeader>
              <TableColumnHeader>{t("client:admin.invites.used")}</TableColumnHeader>
              <TableColumnHeader>{t("client:admin.invites.expires_at")}</TableColumnHeader>
              <TableColumnHeader>{t("client:admin.invites.created_at")}</TableColumnHeader>
              <TableColumnHeader>
                <VisuallyHidden>{t("client:admin.list.column.actions")}</VisuallyHidden>
              </TableColumnHeader>
            </>
          }
        >
          {(invite) => <InviteRow invite={invite} revoke={revoke} />}
        </RecordTable>
      </Box>
    </Stack>
  );
}

function InviteRow({ invite, revoke }: { invite: Invite; revoke: RevokeInvite }) {
  const t = useT();
  const intl = useIntl();
  const copy = useCopyInviteLink();
  const created = intl.formatDate(invite.created_at, { dateStyle: "medium", timeStyle: "short" });
  return (
    <TableRow data-row-id={invite.id}>
      {/* Focusable from script: it takes focus when the revoke button goes away (useFocusHeadingAfterLift). */}
      <TableRowHeader tabIndex={-1} wrap="anywhere">
        <Text dir="ltr" mono wrap="anywhere">
          {invite.code}
        </Text>
      </TableRowHeader>
      <TableCell>
        <Text {...(invite.state === "usable" ? { weight: "semibold" as const } : {})}>
          {t(stateLabels[invite.state])}
        </Text>
      </TableCell>
      <TableCell>
        {invite.max_uses === undefined
          ? t("client:admin.invites.used.unlimited", { uses: invite.uses })
          : t("client:admin.invites.used.limited", { uses: invite.uses, max: invite.max_uses })}
      </TableCell>
      <TableCell>
        {invite.expires_at === undefined ? t("client:admin.invites.never") : <DateTime value={invite.expires_at} />}
      </TableCell>
      <TableCell>
        <DateTime value={invite.created_at} />
      </TableCell>
      <TableCell>
        {invite.state === "usable" && (
          <Cluster gap="2">
            <Button
              size="sm"
              aria-label={t("client:admin.invites.copy_invite", { code: invite.code })}
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
        )}
      </TableCell>
    </TableRow>
  );
}
