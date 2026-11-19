// SPDX-License-Identifier: AGPL-3.0-only

// The audit log, at /moderation/audit-log (FR-704, ADR-0058 §7): the operations of the moderators and the
// administrators, newest first, filtered by who did them (the reader, or the account the address names), by the kind
// of what they acted on and the kind of operation, and by one target; a page at a time by the server's cursor. Each
// entry reads as a sentence of who did what to what, the glyph of its target's kind before it and the time after it,
// the actor and the target linked to their screens while they are there, and under Details the ways to the operations
// of the same actor and on the same target. A one-person server has one actor, so the log is not filtered by it
// (ADR-0152).

import { moderatedAccountPath, reportPath, sentenceHandleOf, eventPath, voiceRoomPath } from "@my/app-components";
import {
  auditActions,
  auditActionTargets,
  auditTargetKinds,
  useAuditLog,
  useDeliveryAccount,
  useInstance,
  useViewer,
  type AuditEntry,
  type AuditQuery,
  type AuditTargetKind,
} from "@my/app-data";
import {
  FeedNotice,
  FilterBar,
  FilterFacet,
  FilterToggle,
  LogEntry,
  LogList,
  Row,
  ScreenStack,
  ScreenTitle,
  SentenceName,
  TextLink,
  type FilterValue,
  type IconName,
} from "@my/design-system";
import type { ReactNode } from "react";
import type { MessageId } from "../../messages.gen";
import { formatParts } from "../../slots";
import { staffHref, type StaffAddress, type StaffSearch } from "../entries";
import { ReadNotice, useStaffStrings, type T } from "../parts";
import { auditLogPath, cursorOf, domainAllowsPath, domainListPath, filtered, Pages, remoteServerPath } from "./parts";

/** What the log shows at an address's query: every operation, unless it names who, what kind, or what. */
export function auditQueryOf(search: StaffSearch): AuditQuery {
  const action = auditActions.find((a) => a === search["action"]);
  const targetKind =
    auditTargetKinds.find((k) => k === search["target_kind"]) ?? (action && auditActionTargets[action]);
  const target = search["target"] === "" ? undefined : search["target"];
  return {
    actor: search["actor"] === "" ? undefined : search["actor"],
    action,
    targetKind,
    target: targetKind === undefined ? undefined : target,
    cursor: cursorOf(search),
  };
}

/** The address of the log of the operations on the target `id` of `kind`, named `label` in its filter. */
const auditOfTarget = (kind: AuditTargetKind, id: string, label?: string): string =>
  staffHref(auditLogPath, { target_kind: kind, target: id, target_label: label === id ? undefined : label });

/** The glyph of each kind of target, before each of its entries. */
const glyphs: Readonly<Record<AuditTargetKind, IconName>> = {
  account: "profile",
  status: "history",
  domain: "federated",
  report: "report",
  appeal: "appeal",
  user: "signUp",
  role: "security",
  setting: "settings",
  rule: "list",
  signup_block: "block",
  custom_emoji: "emojiCustom",
  webhook: "webhookOff",
  announcement: "announcements",
  invite: "invite",
  relay: "federated",
  billing_source: "finance",
  cost_item: "finance",
  supporter_program: "funding",
  supporter_plan: "funding",
  supporter_subscription: "funding",
  custom_emoji_request: "emojiCustom",
  payment_provider: "finance",
  oauth_client: "apps",
  oauth_client_host: "apps",
  instance: "closure",
  voice_room: "room",
  login_provider: "signIn",
  link_card_domain: "link",
  hook_decision: "reviewFailing",
  domain_allow: "accepted",
  domain_list_subscription: "list",
  event: "events",
};

/**
 * Where the target of an entry is looked at, by its kind (ADR-0058 §1), while it is there: its own screen, or the list
 * that holds it where it has none of its own; a remote post's own address (its label is its URI).
 */
function targetHref(e: AuditEntry): string | undefined {
  if (!e.target_exists) return undefined;
  const id = encodeURIComponent(e.target_id);
  switch (e.target_kind) {
    case "account":
      return moderatedAccountPath(e.target_id);
    case "status":
      return /^https?:\/\//.test(e.target_label) ? e.target_label : undefined;
    case "domain":
      return remoteServerPath(e.target_id);
    case "report":
      return reportPath(e.target_id);
    case "appeal":
      return `/moderation/appeals/${id}`;
    case "user":
      return "/moderation/registrations";
    case "role":
      return "/admin/roles";
    case "setting":
      return "/admin/settings";
    case "rule":
      return `/admin/rules/${id}`;
    case "signup_block":
      return `/moderation/signup-blocks/${id}`;
    case "custom_emoji":
      return `/admin/custom-emojis/${id}`;
    case "webhook":
      return `/admin/webhooks/${id}`;
    case "announcement":
      return `/admin/announcements/${id}`;
    case "invite":
      return "/moderation/invites";
    case "relay":
      return `/admin/relays/${id}`;
    case "billing_source":
      return "/admin/finance/imports";
    case "cost_item":
      return `/admin/finance/costs/${id}`;
    case "supporter_program":
    case "payment_provider":
      return "/admin/supporters";
    case "supporter_plan":
      return `/admin/supporters/plans/${id}`;
    case "supporter_subscription":
      return `/admin/supporters/subscriptions/${id}`;
    case "custom_emoji_request":
      return "/admin/custom-emojis/requests";
    case "oauth_client":
      return "/admin/oauth-apps";
    case "oauth_client_host":
      return "/admin/oauth-apps/hosts";
    case "instance":
      return "/admin/closure";
    case "voice_room":
      return voiceRoomPath(e.target_id);
    case "login_provider":
      return `/admin/login-providers/${id}`;
    case "link_card_domain":
      return `/admin/link-card-domains/${id}`;
    case "hook_decision":
      return `/moderation/hook-decisions/${id}`;
    case "domain_allow":
      return domainAllowsPath;
    case "domain_list_subscription":
      return domainListPath(e.target_id);
    case "event":
      return eventPath(e.target_id);
  }
}

/** Who did it, as the sentence names them: the user's account, linked to its screen; else the command line, the server, or another server. */
function actorOf(t: T, e: AuditEntry): ReactNode {
  if (e.actor_kind !== "user") return t("staff_audit.actor", { kind: e.actor_kind });
  if (e.actor_account === undefined) return t("staff_audit.actor.gone");
  return (
    <SentenceName>
      <TextLink href={moderatedAccountPath(e.actor_account.id)}>{sentenceHandleOf(e.actor_account)}</TextLink>
    </SentenceName>
  );
}

/** What it acted on, as the sentence names it: its name when it happened, linked to where it is looked at. */
function targetOf(e: AuditEntry): ReactNode {
  const href = targetHref(e);
  return (
    <SentenceName>
      {href === undefined ? e.target_label : <TextLink href={href}>{e.target_label}</TextLink>}
    </SentenceName>
  );
}

/** The sentence of an entry, who did what to what, the actor and the target linked. */
export function auditSentence(t: T, e: AuditEntry): ReactNode {
  return formatParts(
    t,
    `staff_audit.action.${e.action}` as MessageId,
    {},
    { actor: actorOf(t, e), target: targetOf(e) },
  );
}

/** An entry of the log: its glyph, its sentence, and under Details the ways to the same actor's and the same target's operations. */
function Entry({ entry: e, search }: { entry: AuditEntry; search: StaffSearch }) {
  const { t } = useStaffStrings();
  const actorId = e.actor_kind === "user" ? e.actor_account_id : undefined;
  const ways = (
    <Row wrap gap="screen-tab-gap">
      {actorId !== undefined && search["actor"] !== actorId && (
        <TextLink href={staffHref(auditLogPath, { actor: actorId })}>{t("staff_audit.same_actor")}</TextLink>
      )}
      {search["target"] !== e.target_id && (
        <TextLink href={auditOfTarget(e.target_kind, e.target_id, e.target_label)}>
          {t("staff_audit.same_target")}
        </TextLink>
      )}
    </Row>
  );
  return (
    <LogEntry icon={glyphs[e.target_kind]} date={e.occurred_at} detail={ways}>
      {auditSentence(t, e)}
    </LogEntry>
  );
}

/** The kind of a target, in words. */
const kindWords = (t: T, kind: AuditTargetKind): string => t("staff_audit.target_kind", { kind });

/** The filters of the log: who, the kind of target and of operation, and the one target the address names. */
function AuditFilters({ address, q }: { address: StaffAddress; q: AuditQuery }) {
  const { t } = useStaffStrings();
  const { search, onSearch } = address;
  const me = useViewer()?.id;
  const alone = useInstance()?.single_user === true;
  const actor = useDeliveryAccount(q.actor !== undefined && q.actor !== me ? q.actor : undefined);
  const actors: FilterValue[] = [
    ...(me === undefined ? [] : [{ id: me, label: t("staff_audit.filter.actor.me"), icon: "profile" as const }]),
    ...(q.actor !== undefined && q.actor !== me
      ? [
          {
            id: q.actor,
            label: actor === undefined ? t("staff_audit.actor.gone") : sentenceHandleOf(actor),
            icon: "profile" as const,
          },
        ]
      : []),
  ];
  const kinds: FilterValue[] = auditTargetKinds.map((k) => ({ id: k, label: kindWords(t, k), icon: glyphs[k] }));
  const actions: FilterValue[] = auditActions
    .filter((a) => q.targetKind === undefined || auditActionTargets[a] === q.targetKind)
    .map((a) => ({ id: a, label: t("staff_audit.action_kind", { action: a }), icon: glyphs[auditActionTargets[a]] }));
  const label = search["target_label"];
  return (
    <FilterBar
      label={t("staff_audit.filters")}
      applied={q.actor !== undefined || q.action !== undefined || q.targetKind !== undefined}
      onClear={() => {
        onSearch({});
      }}
    >
      {!alone && (
        <FilterFacet
          label={t("staff_audit.filter.actor")}
          any={t("staff_audit.filter.actor.any")}
          values={actors}
          chosen={q.actor}
          onChoose={(id) => {
            onSearch(filtered(search, { actor: id }));
          }}
        />
      )}
      <FilterFacet
        label={t("staff_audit.filter.target_kind")}
        any={t("staff_audit.filter.target_kind.any")}
        values={kinds}
        chosen={q.targetKind}
        onChoose={(kind) => {
          onSearch(
            filtered(search, { target_kind: kind, action: undefined, target: undefined, target_label: undefined }),
          );
        }}
      />
      {q.targetKind !== undefined && (
        <FilterFacet
          label={t("staff_audit.filter.action")}
          any={t("staff_audit.filter.action.any")}
          values={actions}
          chosen={q.action}
          onChoose={(action) => {
            onSearch(filtered(search, { action }));
          }}
        />
      )}
      {q.target !== undefined && (
        <FilterToggle
          label={t("staff_audit.filter.target", { target: label ?? q.target })}
          on
          onChange={() => {
            onSearch(filtered(search, { target: undefined, target_label: undefined }));
          }}
        />
      )}
    </FilterBar>
  );
}

/** The audit log: its filters, the page of entries, and the pages. */
export function AuditLogScreen(address: StaffAddress): ReactNode {
  const { t } = useStaffStrings();
  const q = auditQueryOf(address.search);
  const page = useAuditLog(q);
  const head = (
    <>
      <ScreenTitle>{t("staff_audit.entry.audit_log")}</ScreenTitle>
      <AuditFilters address={address} q={q} />
    </>
  );
  if (page.forbidden) return <FeedNotice lead>{t("staff_audit.forbidden")}</FeedNotice>;
  if (page.status !== "success")
    return (
      <ScreenStack>
        {head}
        <ReadNotice
          status={page.status}
          loading={t("staff_audit.loading")}
          failed={t("staff_audit.failed")}
          retry={page.refetch}
        />
      </ScreenStack>
    );
  const anything = q.actor !== undefined || q.targetKind !== undefined;
  return (
    <ScreenStack busy={page.stale}>
      {head}
      {page.items.length === 0 ? (
        <FeedNotice>{t(anything ? "staff_audit.empty.filtered" : "staff_audit.empty")}</FeedNotice>
      ) : (
        <LogList label={t("staff_audit.entry.audit_log")}>
          {page.items.map((e) => (
            <Entry key={e.id} entry={e} search={address.search} />
          ))}
        </LogList>
      )}
      <Pages address={address} next={page.next} />
    </ScreenStack>
  );
}
