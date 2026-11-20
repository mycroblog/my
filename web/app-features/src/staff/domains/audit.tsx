// SPDX-License-Identifier: AGPL-3.0-only

// The audit log, at /moderation/audit-log (FR-704, ADR-0058 §7): the operations of the moderators and the
// administrators, newest first, filtered by who did them (the reader, or the account the address names), by the kind
// of what they acted on and the kind of operation, and by one target; a page at a time by the server's cursor. Each
// entry reads as a sentence of who did what to what, the glyph of its target's kind before it and the time after it,
// the actor and the target linked to their screens while they are there, and under Details the ways to the operations
// of the same actor and on the same target. A one-person server has one actor, so the log is not filtered by it
// (ADR-0152).

import { eventPath, moderatedAccountPath, reportPath, sentenceHandleOf } from "@my/app-components";
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
  Pager,
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
import { settingNames, textLabels } from "../settings/words";
import { staffHref, useStaffEntries, type StaffAddress, type StaffArea, type StaffSearch } from "../entries";
import { ReadNotice, useStaffStrings, type T } from "../parts";
import {
  auditLogPath,
  cursorOf,
  domainAllowsPath,
  domainListPath,
  filtered,
  useAddressPages,
  remoteServerPath,
} from "./parts";

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
  rule: "about",
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
  domain_allow: "included",
  domain_list_subscription: "list",
  event: "events",
};

/** A screen of the staff's an entry links to: the entry of its area it is under, and its address. */
type Place = { readonly area: StaffArea; readonly entry: string; readonly href: string };

/**
 * Where the target of an entry is looked at, by its kind (ADR-0058 §1), while it is there: the screen of its own, or the
 * list that holds it, under the entry of the staff's screens it belongs to, which the reader's permissions must open;
 * an event's page; a remote post's own address (its label is its URI). A kind whose screens the app does not have yet
 * names its entry all the same: it links once that entry is there.
 */
function placeOf(e: AuditEntry): Place | { readonly href: string } | undefined {
  if (!e.target_exists) return undefined;
  const id = encodeURIComponent(e.target_id);
  const moderation = (entry: string, href: string): Place => ({ area: "moderation", entry, href });
  const admin = (entry: string, href: string): Place => ({ area: "admin", entry, href });
  switch (e.target_kind) {
    case "account":
      return moderation("accounts", moderatedAccountPath(e.target_id));
    case "status":
      return /^https?:\/\//.test(e.target_label) ? { href: e.target_label } : undefined;
    case "domain":
      return moderation("instances", remoteServerPath(e.target_id));
    case "report":
      return moderation("reports", reportPath(e.target_id));
    case "appeal":
      return moderation("appeals", `/moderation/appeals/${id}`);
    case "user":
      return moderation("registrations", "/moderation/registrations");
    case "role":
      return admin("roles", "/admin/roles");
    case "setting":
      return admin("settings", "/admin/settings");
    case "rule":
      return admin("rules", `/admin/rules/${id}`);
    case "signup_block":
      return moderation("signup-blocks", `/moderation/signup-blocks/${id}`);
    case "custom_emoji":
      return admin("custom-emojis", `/admin/custom-emojis/${id}`);
    case "webhook":
      return admin("webhooks", `/admin/webhooks/${id}`);
    case "announcement":
      return admin("announcements", `/admin/announcements/${id}`);
    case "invite":
      return moderation("invites", "/moderation/invites");
    case "relay":
      return admin("relays", `/admin/relays/${id}`);
    case "billing_source":
      return admin("finance", "/admin/finance/imports");
    case "cost_item":
      return admin("finance", `/admin/finance/costs/${id}`);
    case "supporter_program":
    case "payment_provider":
      return admin("supporters", "/admin/supporters");
    case "supporter_plan":
      return admin("supporters", `/admin/supporters/plans/${id}`);
    case "supporter_subscription":
      return admin("supporters", `/admin/supporters/subscriptions/${id}`);
    case "custom_emoji_request":
      return admin("custom-emojis", "/admin/custom-emojis/requests");
    case "oauth_client":
      return admin("oauth-apps", "/admin/oauth-apps");
    case "oauth_client_host":
      return admin("oauth-apps", "/admin/oauth-apps/hosts");
    case "instance":
      return admin("closure", "/admin/closure");
    case "voice_room":
      return undefined;
    case "login_provider":
      return admin("login-providers", `/admin/login-providers/${id}`);
    case "link_card_domain":
      return admin("link-card-domains", `/admin/link-card-domains/${id}`);
    case "hook_decision":
      return moderation("hook-decisions", `/moderation/hook-decisions/${id}`);
    case "domain_allow":
      return moderation("domain-actions", domainAllowsPath);
    case "domain_list_subscription":
      return moderation("domain-actions", domainListPath(e.target_id));
    case "event":
      return { href: eventPath(e.target_id) };
  }
}

/** The address of a place of `e` the reader may open: one of the staff's under an entry open to them, or another. */
function useTargetHref(e: AuditEntry): string | undefined {
  const moderation = useStaffEntries("moderation");
  const admin = useStaffEntries("admin");
  const place = placeOf(e);
  if (place === undefined || !("area" in place)) return place?.href;
  const open = (place.area === "moderation" ? moderation : admin).some((entry) => entry.path === place.entry);
  return open ? place.href : undefined;
}

/** The words of a setting's key ("registration_mode" → "Registration mode"), until the settings name their own. */
const keyWords = (key: string): string => {
  const words = key.replace(/[_.]+/g, " ").trim();
  return words.charAt(0).toUpperCase() + words.slice(1);
};

/**
 * What an entry names its target by: its name when it happened, as the server kept it, in the reader's words where
 * the server kept a system's value (a sign-up block's kind and value, a setting's or a text's key, which the settings'
 * pages name, a role's key and a handle, a payment provider's key).
 */
function useTargetWords(e: AuditEntry): { readonly target: string; readonly person?: string } {
  const { t } = useStaffStrings();
  switch (e.target_kind) {
    case "signup_block": {
      const [kind = "", ...value] = e.target_label.split(" ");
      return { target: t("staff_audit.signup_block", { kind, value: value.join(" ") }) };
    }
    case "setting": {
      // A setting by its name on the settings' pages, a text by its label there; a key they do not name, in words.
      const table: Readonly<Record<string, MessageId>> = e.action === "text_changed" ? textLabels : settingNames;
      const id = table[e.target_label];
      return { target: id === undefined ? keyWords(e.target_label) : t(id) };
    }
    case "role": {
      // A role granted or revoked is its key and the handle of whom; the others are its key.
      const at = e.target_label.lastIndexOf(" @");
      const key = at < 0 ? e.target_label : e.target_label.slice(0, at);
      return {
        target: t("staff_audit.role", { key }),
        ...(at < 0 ? {} : { person: e.target_label.slice(at + 1) }),
      };
    }
    case "payment_provider":
      return { target: t("staff_audit.provider", { key: e.target_label }) };
    default:
      return { target: e.target_label };
  }
}

/** Who did it, as the sentence names them: the user's account, linked while its screen is open to the reader. */
function useActor(t: T, e: AuditEntry): ReactNode {
  const open = useStaffEntries("moderation").some((entry) => entry.path === "accounts");
  if (e.actor_kind !== "user") return t("staff_audit.actor", { kind: e.actor_kind });
  if (e.actor_account === undefined) return t("staff_audit.actor.gone");
  const handle = sentenceHandleOf(e.actor_account);
  return (
    <SentenceName>
      {open ? <TextLink href={moderatedAccountPath(e.actor_account.id)}>{handle}</TextLink> : handle}
    </SentenceName>
  );
}

/** The kinds of operation the app words; a kind of a newer server reads as a sentence of no kind. */
const worded: ReadonlySet<string> = new Set(auditActions);

/**
 * The sentence of an entry, who did what to what: the actor and the target linked to their screens where the reader
 * may open them, else named; the closure of the service with no target, which is the service itself.
 */
export function AuditSentence({ entry: e }: { entry: AuditEntry }): ReactNode {
  const { t } = useStaffStrings();
  const actor = useActor(t, e);
  const href = useTargetHref(e);
  const words = useTargetWords(e);
  const target = (
    <SentenceName>{href === undefined ? words.target : <TextLink href={href}>{words.target}</TextLink>}</SentenceName>
  );
  const id = worded.has(e.action) ? (`staff_audit.action.${e.action}` as MessageId) : "staff_audit.action.other";
  return formatParts(
    t,
    id,
    {},
    {
      actor,
      target,
      person: <SentenceName>{words.person ?? ""}</SentenceName>,
    },
  );
}

/** An entry of the log: its glyph, its sentence, and under Details the ways to the same actor's and the same target's operations. */
function Entry({ entry: e, search, alone }: { entry: AuditEntry; search: StaffSearch; alone: boolean }) {
  const { t } = useStaffStrings();
  // On a one-person server every operation is the one person's: there is no other actor to tell apart.
  const actorId = e.actor_kind === "user" && !alone ? e.actor_account_id : undefined;
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
    <LogEntry
      icon={(glyphs as Partial<Record<string, IconName>>)[e.target_kind] ?? "history"}
      date={e.occurred_at}
      detail={ways}
    >
      <AuditSentence entry={e} />
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
  const pages = useAddressPages(address);
  const { t } = useStaffStrings();
  const alone = useInstance()?.single_user === true;
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
            <Entry key={e.id} entry={e} search={address.search} alone={alone} />
          ))}
        </LogList>
      )}
      <Pager {...pages.pager(page.next)} />
    </ScreenStack>
  );
}
