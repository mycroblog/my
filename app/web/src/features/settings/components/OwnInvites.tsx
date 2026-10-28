// SPDX-License-Identifier: AGPL-3.0-only
// The page of the user's own invites (`/settings/invites`, FR-011, ADR-0108 §2, §5), below its heading: two regions.
// "Create an invite" says how many usable invites the user holds against the cap and holds the form of a new invite,
// whose number of sign-ups and expiry are chosen within the caps of the instance settings; the new invite's link stays
// under the form. A user who may not create invites now is told so instead of the form. "Your invites" lists the
// user's invites in every state, newest first; a usable one is copied as its sign-up link or revoked, which takes
// effect at once and asks nothing first, as on the moderation screen. The user lists and revokes their invites even
// when they may no longer create them. What a change did stays in a StatusMessage (role="status").
import {
  Button,
  Cluster,
  DescriptionItem,
  DescriptionList,
  DurationSelect,
  EmptyState,
  Heading,
  Hint,
  InlineError,
  SectionHeader,
  Select,
  SelectItem,
  Stack,
  StatusMessage,
  Text,
} from "@my/ui";
import { useRef, useState } from "react";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { useFocusHeadingAfterLift } from "../../../lib/useFocusAfterRemoval";
import { DateTime, Section, RecordList } from "../../moderation";
import type { OwnInvite, OwnInviteLimits, OwnInviteState } from "../api/invites";
import {
  type RevokeOwnInvite,
  useCopyOwnInviteLink,
  useOwnInviteForm,
  useOwnInvites,
  useRevokeOwnInvite,
} from "../hooks/useOwnInvites";

const stateLabels: Readonly<Record<OwnInviteState, ClientMessageKey>> = {
  usable: "client:settings.invites.state.usable",
  revoked: "client:settings.invites.state.revoked",
  expired: "client:settings.invites.state.expired",
  exhausted: "client:settings.invites.state.exhausted",
  creator_ineligible: "client:settings.invites.state.creator_ineligible",
};

export function OwnInvites() {
  const t = useT();
  return (
    <Stack gap="8">
      <Hint size="inherit">{t("client:settings.invites.description")}</Hint>
      <Section surface="card" title={t("client:settings.invites.new.title")}>
        <NewInvite />
      </Section>
      <Section surface="card" title={t("client:settings.invites.list.title")}>
        <InviteList />
      </Section>
    </Stack>
  );
}

/** The form of a new invite, or why the user may not create one now. */
function NewInvite() {
  const t = useT();
  const { canCreate, usable, limits } = useOwnInvites();
  if (!canCreate || limits.max_active === 0) return <p>{t("client:settings.invites.not_allowed")}</p>;
  return (
    <Stack gap="6">
      <Hint>{t("client:settings.invites.usage", { usable, max: limits.max_active })}</Hint>
      {usable >= limits.max_active && <p>{t("client:settings.invites.full")}</p>}
      <NewInviteForm limits={limits} />
    </Stack>
  );
}

function NewInviteForm({ limits }: { limits: OwnInviteLimits }) {
  const t = useT();
  const form = useOwnInviteForm(limits);
  const formatExpiry = (seconds: number) => {
    if (seconds % 86_400 === 0) return t("client:settings.invites.duration.days", { count: seconds / 86_400 });
    if (seconds % 3600 === 0) return t("client:settings.invites.duration.hours", { count: seconds / 3600 });
    return t("client:settings.invites.duration.minutes", { count: Math.ceil(seconds / 60) });
  };
  return (
    <Stack as="form" onSubmit={form.submit} gap="6">
      <Select
        label={t("client:settings.invites.max_uses")}
        description={t("client:settings.invites.max_uses.description", { max: limits.max_uses })}
        value={String(form.maxUses)}
        onChange={(key) => {
          if (typeof key === "string") form.change({ maxUses: Number(key) });
        }}
        items={form.maxUsesChoices.map((uses) => ({
          id: String(uses),
          name: t("client:settings.invites.uses", { count: uses }),
        }))}
      >
        {(item) => (
          <SelectItem id={item.id} textValue={item.name}>
            {item.name}
          </SelectItem>
        )}
      </Select>
      <DurationSelect
        label={t("client:settings.invites.expires_in")}
        description={t("client:settings.invites.expires_in.description", {
          max: formatExpiry(limits.max_expires_in),
        })}
        durations={form.expiryChoices}
        formatDuration={formatExpiry}
        value={form.expiresIn}
        onChange={(expiresIn) => {
          if (expiresIn !== null) form.change({ expiresIn });
        }}
      />
      {form.error !== undefined && (
        <InlineError alert size="inherit">
          {t("client:settings.invites.create_failed", { reason: form.error })}
        </InlineError>
      )}
      <div>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:settings.invites.create")}
        </Button>
      </div>
      <StatusMessage>{form.status}</StatusMessage>
      {form.created !== undefined && (
        <Stack as="p" gap="1">
          <span>{t("client:settings.invites.link")}</span>
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
  const listRef = useRef<HTMLUListElement>(null);
  const list = useOwnInvites();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const keepFocus = useFocusHeadingAfterLift(listRef);
  const revoke = useRevokeOwnInvite((invite) => {
    // The buttons go away from the row, which stays.
    keepFocus(invite.id);
    setStatus(t("client:settings.invites.revoked", { code: invite.code }));
  });
  return (
    <Stack gap="3">
      <StatusMessage>{status}</StatusMessage>
      <RecordList
        list={list}
        getId={(invite) => invite.id}
        listRef={listRef}
        empty={
          <EmptyState
            headingLevel={3}
            heading={t("client:settings.invites.empty")}
            description={t("client:settings.invites.empty.description")}
          />
        }
      >
        {(invite) => <InviteRow invite={invite} revoke={revoke} />}
      </RecordList>
    </Stack>
  );
}

function InviteRow({ invite, revoke }: { invite: OwnInvite; revoke: RevokeOwnInvite }) {
  const t = useT();
  const intl = useIntl();
  const copy = useCopyOwnInviteLink();
  const created = intl.formatDate(invite.created_at, { dateStyle: "medium", timeStyle: "short" });
  return (
    <Stack gap="3">
      <SectionHeader
        actions={
          invite.state === "usable" && (
            <Cluster gap="2">
              <Button
                size="sm"
                aria-label={t("client:settings.invites.copy_invite", {
                  action: t("client:settings.invites.copy"),
                  code: invite.code,
                })}
                onPress={() => {
                  copy(invite);
                }}
              >
                {t("client:settings.invites.copy")}
              </Button>
              <Button
                size="sm"
                variant="danger"
                isPending={revoke.pendingIds.includes(invite.id)}
                aria-label={t("client:settings.invites.revoke_invite", { code: invite.code, date: created })}
                onPress={() => {
                  revoke.revoke(invite);
                }}
              >
                {t("client:settings.invites.revoke")}
              </Button>
            </Cluster>
          )
        }
      >
        {/* Focusable from script: it takes focus when the revoke button goes away (useFocusHeadingAfterLift). */}
        <Heading level={3} focusable mono wrap="anywhere">
          <span dir="ltr">{invite.code}</span>
        </Heading>
      </SectionHeader>
      <DescriptionList columns={2}>
        <DescriptionItem term={t("client:settings.invites.state")}>
          <Text {...(invite.state === "usable" ? { weight: "semibold" as const } : {})}>
            {t(stateLabels[invite.state])}
          </Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:settings.invites.used")}>
          {t("client:settings.invites.used.value", { uses: invite.uses, max: invite.max_uses })}
        </DescriptionItem>
        <DescriptionItem term={t("client:settings.invites.expires_at")}>
          <DateTime value={invite.expires_at} />
        </DescriptionItem>
        <DescriptionItem term={t("client:settings.invites.created_at")}>
          <DateTime value={invite.created_at} />
        </DescriptionItem>
      </DescriptionList>
    </Stack>
  );
}
