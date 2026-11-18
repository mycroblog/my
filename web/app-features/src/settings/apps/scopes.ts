// SPDX-License-Identifier: AGPL-3.0-only

// The scopes an app was granted, in the words and the weights of the consent page (ADR-0090 §4, the server's
// authorize.go): each scope by its sentence, with the cautions of the ones that read direct messages, post or change
// the profile, in three groups, the heaviest first (what needs the reader's attention, what changes things, what only
// reads). A permission scope `admin:<permission>` (ADR-0090 §17) is worded with the permission's name, and a scope
// this list does not know stands as its own name, as the server words it.

import type { Strings } from "@my/design-system";
import type { MessageId } from "../../messages.gen";

/** The sentence of each scope the consent page words. */
const sentences: Readonly<Record<string, MessageId>> = {
  admin: "settings.apps.scope.admin",
  "admin:read": "settings.apps.scope.admin_read",
  "admin:write": "settings.apps.scope.admin_write",
  follow: "settings.apps.scope.follow",
  push: "settings.apps.scope.push",
  read: "settings.apps.scope.read",
  "read:accounts": "settings.apps.scope.read_accounts",
  "read:blocks": "settings.apps.scope.read_blocks",
  "read:bookmarks": "settings.apps.scope.read_bookmarks",
  "read:custom_emojis": "settings.apps.scope.read_custom_emojis",
  "read:direct": "settings.apps.scope.read_direct",
  "read:events": "settings.apps.scope.read_events",
  "read:filters": "settings.apps.scope.read_filters",
  "read:finance": "settings.apps.scope.read_finance",
  "read:follows": "settings.apps.scope.read_follows",
  "read:mutes": "settings.apps.scope.read_mutes",
  "read:notifications": "settings.apps.scope.read_notifications",
  "read:reports": "settings.apps.scope.read_reports",
  "read:statuses": "settings.apps.scope.read_statuses",
  "read:supporters": "settings.apps.scope.read_supporters",
  "read:voice": "settings.apps.scope.read_voice",
  write: "settings.apps.scope.write",
  "write:accounts": "settings.apps.scope.write_accounts",
  "write:blocks": "settings.apps.scope.write_blocks",
  "write:bookmarks": "settings.apps.scope.write_bookmarks",
  "write:custom_emojis": "settings.apps.scope.write_custom_emojis",
  "write:events": "settings.apps.scope.write_events",
  "write:favourites": "settings.apps.scope.write_favourites",
  "write:filters": "settings.apps.scope.write_filters",
  "write:follows": "settings.apps.scope.write_follows",
  "write:media": "settings.apps.scope.write_media",
  "write:mutes": "settings.apps.scope.write_mutes",
  "write:notifications": "settings.apps.scope.write_notifications",
  "write:reports": "settings.apps.scope.write_reports",
  "write:statuses": "settings.apps.scope.write_statuses",
  "write:supporters": "settings.apps.scope.write_supporters",
  "write:voice": "settings.apps.scope.write_voice",
};

/** The cautions shown with a scope: reading direct messages, writing posts, changing the profile. */
const cautions: Readonly<Record<string, readonly MessageId[]>> = {
  read: ["settings.apps.caution.dm"],
  "read:direct": ["settings.apps.caution.dm"],
  write: ["settings.apps.caution.post", "settings.apps.caution.profile"],
  "write:statuses": ["settings.apps.caution.post"],
  "write:accounts": ["settings.apps.caution.profile"],
};

/** The weights of the scopes, the heaviest first. */
const scopeRisks = ["caution", "write", "read"] as const;
type ScopeRisk = (typeof scopeRisks)[number];

/** The scopes of `scope`, a space-separated list as the server writes one. */
export const scopesOf = (scope: string): string[] => scope.split(/\s+/).filter((name) => name !== "");

/** The weight of a scope: one with a caution or of the administration, one that changes things, or one that reads. */
function riskOf(scope: string): ScopeRisk {
  if (cautions[scope] !== undefined || scope === "admin" || scope.startsWith("admin:")) return "caution";
  if (scope === "write" || scope.startsWith("write:") || scope === "follow" || scope === "push") return "write";
  return "read";
}

/** A scope as a screen words it. */
export type WordedScope = {
  /** The scope's name (`write:statuses`). */
  readonly scope: string;
  /** What it lets an app do, as a sentence. */
  readonly text: string;
  /** What to mind about it; none for most. */
  readonly cautions: readonly string[];
};

type T = Strings<MessageId>["t"];

/** The scope of `scope` in words. */
function worded(scope: string, t: T): WordedScope {
  const said = (cautions[scope] ?? []).map((caution) => t(caution));
  const sentence = sentences[scope];
  if (sentence !== undefined) return { scope, text: t(sentence), cautions: said };
  const permission = /^admin:(.+)$/.exec(scope)?.[1];
  return {
    scope,
    text: permission === undefined ? scope : t("settings.apps.scope.admin_permission", { permission }),
    cautions: said,
  };
}

/**
 * The scopes of `scope` in their groups, the heaviest first, each scope in words and in the order the server gave
 * it; a group with no scope is left out.
 */
export function groupedScopes(scope: string, t: T): { risk: ScopeRisk; scopes: WordedScope[] }[] {
  const names = scopesOf(scope);
  return scopeRisks
    .map((risk) => ({ risk, scopes: names.filter((name) => riskOf(name) === risk).map((name) => worded(name, t)) }))
    .filter((group) => group.scopes.length > 0);
}
