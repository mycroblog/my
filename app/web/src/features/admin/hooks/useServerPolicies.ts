// SPDX-License-Identifier: AGPL-3.0-only
// The server's policies (FR-001, FR-105, FR-202, FR-408, FR-409, FR-413, FR-718, FR-725, FR-903, ADR-0009, ADR-0057): who may sign
// up, whether the server is a one-person server (FR-811, ADR-0152), who may create invites of their own and the caps of those invites (ADR-0106 §14, §15), the default language, the public timelines, whether images need a description, the federation (its direction,
// ADR-0006, which asks before it narrows), who may read the
// list of the domain actions, whether the landing page shows the server's numbers and who runs it,
// what the server states to search engines and AI crawlers, which posts its search covers (ADR-0149 §5), voice chat (ADR-0100 §2, §9, §13: who may start a
// room, whether visitors see a room's summary, and how many days the participants' records are kept), and the
// automatic deletion of old posts (ADR-0151: the default of new accounts, the maximum and its grace period, which
// asks before it binds more accounts). They are
// instance settings that used to change only from the CLI (`my instance set`). Each page of the server settings has one
// form of the settings of its topic (policyPages); the form sends the ones of its page that changed together, all or
// none, and each change goes to the audit log (FR-704).
import { useMutation, useQuery, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { type FederationMode, federationModes, federationReachOf } from "../../../lib/federation";
import { useLocale, useT } from "../../../lib/i18n";
import { singleUserRequiresOneAccountType } from "../../../lib/singleUser";
import {
  cacheSavedSettings,
  type InstanceSettingKey,
  instanceSettingsQueryOptions,
  loginProvidersOnlyImpactQueryOptions,
  updateInstanceSettingsMutation,
} from "../api/settings";
import { languageName, selectableLocales } from "./languages";

/** The settings of the forms of the policies. */
const policyKeys = [
  "single_user",
  "registration_mode",
  "login_providers_only",
  "invite_creators",
  "member_invite_max_active",
  "member_invite_max_uses",
  "member_invite_max_lifetime_seconds",
  "user_app_registration",
  "oauth_metadata_document_clients",
  "dynamic_client_registration",
  "default_locale",
  "local_timeline_visibility",
  "federated_timeline_visibility",
  "require_media_description",
  "federation_mode",
  "authorized_fetch",
  "remote_media_policy",
  "remote_media_cache_ttl_seconds",
  "link_cards",
  "link_cards_ignore_robots_txt",
  "domain_actions_visibility",
  "domain_actions_rationale_visibility",
  // The publication of the costs, the income and the supporter list is set next to those screens (usePublicationSettings).
  "public_counts",
  "featured_accounts",
  "search_indexing",
  "indexable_default",
  "ai_usage",
  "security_contact",
  "search_visibilities",
  "voice_room_starters",
  "voice_public_summary",
  "voice_participants_retention_days",
  "hook_inbound_status_mode",
  "hook_inbound_follow_mode",
  "hook_signup_check_mode",
  "hook_media_check_mode",
  "hook_local_status_mode",
  "hook_inbound_actor_mode",
  "hook_local_profile_mode",
  "hook_inbound_status_on_failure",
  "hook_inbound_status_unfollowed_direct",
  "hook_inbound_follow_on_failure",
  "hook_signup_check_on_failure",
  "hook_signup_check_reject",
  "hook_local_status_account_age_days",
  "hook_local_profile_account_age_days",
  "hook_media_check_on_failure",
  "hook_inbound_actor_on_failure",
  "hook_decisions_retention_days",
  "auto_delete_default_enabled",
  "auto_delete_default_min_age_days",
  "auto_delete_default_keep_pinned",
  "auto_delete_default_keep_bookmarked",
  "auto_delete_default_keep_favourited",
  "auto_delete_default_keep_media",
  "auto_delete_default_keep_polls",
  "auto_delete_default_keep_public",
  "auto_delete_default_keep_local",
  "auto_delete_default_keep_unlisted",
  "auto_delete_default_keep_private",
  "auto_delete_default_keep_direct",
  "auto_delete_default_keep_min_boosts",
  "auto_delete_default_keep_min_favourites",
  "auto_delete_max_age_days",
  "auto_delete_max_allow_pinned",
  "auto_delete_max_kept_pins",
  "auto_delete_max_allow_bookmarked",
  "auto_delete_max_allow_favourited",
  "auto_delete_max_allow_media",
  "auto_delete_max_allow_polls",
  "auto_delete_max_allow_visibilities",
  "auto_delete_max_allow_thresholds",
  "auto_delete_grace_days",
] as const satisfies readonly InstanceSettingKey[];

export type PolicyKey = (typeof policyKeys)[number];

/**
 * The settings of each page of the server settings that holds policies, in the order of its sections: the pages save
 * only their own settings (PATCH /admin/settings sets only the keys sent).
 */
const policyPages = {
  registration: [
    "single_user",
    "registration_mode",
    "login_providers_only",
    "invite_creators",
    "member_invite_max_active",
    "member_invite_max_uses",
    "member_invite_max_lifetime_seconds",
    "user_app_registration",
    "oauth_metadata_document_clients",
    "dynamic_client_registration",
  ],
  content: [
    "default_locale",
    "local_timeline_visibility",
    "federated_timeline_visibility",
    "require_media_description",
    "public_counts",
    "featured_accounts",
  ],
  federation: [
    "federation_mode",
    "authorized_fetch",
    "remote_media_policy",
    "remote_media_cache_ttl_seconds",
    "link_cards",
    "link_cards_ignore_robots_txt",
    "domain_actions_visibility",
    "domain_actions_rationale_visibility",
    "hook_inbound_status_mode",
    "hook_inbound_status_on_failure",
    "hook_inbound_status_unfollowed_direct",
    "hook_inbound_follow_mode",
    "hook_inbound_follow_on_failure",
    "hook_signup_check_mode",
    "hook_signup_check_on_failure",
    "hook_signup_check_reject",
    "hook_media_check_mode",
    "hook_media_check_on_failure",
    "hook_local_status_mode",
    "hook_local_status_account_age_days",
    "hook_inbound_actor_mode",
    "hook_inbound_actor_on_failure",
    "hook_local_profile_mode",
    "hook_local_profile_account_age_days",
    "hook_decisions_retention_days",
  ],
  discovery: ["search_indexing", "indexable_default", "ai_usage", "security_contact", "search_visibilities"],
  voice: ["voice_room_starters", "voice_public_summary", "voice_participants_retention_days"],
  auto_delete: [
    "auto_delete_default_enabled",
    "auto_delete_default_min_age_days",
    "auto_delete_default_keep_pinned",
    "auto_delete_default_keep_bookmarked",
    "auto_delete_default_keep_favourited",
    "auto_delete_default_keep_media",
    "auto_delete_default_keep_polls",
    "auto_delete_default_keep_public",
    "auto_delete_default_keep_local",
    "auto_delete_default_keep_unlisted",
    "auto_delete_default_keep_private",
    "auto_delete_default_keep_direct",
    "auto_delete_default_keep_min_boosts",
    "auto_delete_default_keep_min_favourites",
    "auto_delete_max_age_days",
    "auto_delete_max_allow_pinned",
    "auto_delete_max_kept_pins",
    "auto_delete_max_allow_bookmarked",
    "auto_delete_max_allow_favourited",
    "auto_delete_max_allow_media",
    "auto_delete_max_allow_polls",
    "auto_delete_max_allow_visibilities",
    "auto_delete_max_allow_thresholds",
    "auto_delete_grace_days",
  ],
} as const satisfies Record<string, readonly PolicyKey[]>;

/** A page of the server settings that holds policies. */
export type PolicyPage = keyof typeof policyPages;

/** The lengths of time offered for how long remote media may be kept, in seconds: an hour to a year. */
const cacheTtlChoices: readonly number[] = [3600, 86_400, 7 * 86_400, 30 * 86_400, 365 * 86_400];

/** The lengths of time offered for how long a member's invite may work at most, in seconds: an hour to a year. */
const inviteExpiryChoices: readonly number[] = [
  3600,
  86_400,
  7 * 86_400,
  30 * 86_400,
  90 * 86_400,
  180 * 86_400,
  365 * 86_400,
];

/**
 * The settings typed as a whole number in a text field, with the bounds to use until the server gives its own: the
 * days to keep the participants' records of voice chat rooms (ADR-0100 §13), the days of the decision hooks
 * (ADR-0147 §2, §6), the caps of the members' invites (ADR-0106 §15), and the numbers of the automatic deletion
 * (ADR-0151 §2, §3).
 */
const numberFields = {
  voice_participants_retention_days: { min: 30, max: 3650 },
  hook_decisions_retention_days: { min: 30, max: 3650 },
  hook_local_status_account_age_days: { min: 0, max: 3650 },
  hook_local_profile_account_age_days: { min: 0, max: 3650 },
  member_invite_max_active: { min: 0, max: 100 },
  member_invite_max_uses: { min: 1, max: 1000 },
  auto_delete_default_min_age_days: { min: 1, max: 3650 },
  auto_delete_default_keep_min_boosts: { min: 0, max: 1_000_000 },
  auto_delete_default_keep_min_favourites: { min: 0, max: 1_000_000 },
  auto_delete_max_age_days: { min: 0, max: 3650 },
  auto_delete_max_kept_pins: { min: 1, max: 5 },
  auto_delete_grace_days: { min: 0, max: 365 },
} as const satisfies Partial<Record<PolicyKey, { min: number; max: number }>>;

/** A setting typed as a whole number. */
export type NumberPolicyKey = keyof typeof numberFields;

const numberKeys: readonly NumberPolicyKey[] = [
  "voice_participants_retention_days",
  "hook_decisions_retention_days",
  "hook_local_status_account_age_days",
  "hook_local_profile_account_age_days",
  "member_invite_max_active",
  "member_invite_max_uses",
  "auto_delete_default_min_age_days",
  "auto_delete_default_keep_min_boosts",
  "auto_delete_default_keep_min_favourites",
  "auto_delete_max_age_days",
  "auto_delete_max_kept_pins",
  "auto_delete_grace_days",
];

function isNumberField(key: PolicyKey): key is NumberPolicyKey {
  return key in numberFields;
}

/** A field of a whole number: its bounds, and why its value cannot be saved (undefined when it can). */
export type NumberField = { min: number; max: number; error: string | undefined };

/** A choice of a setting: the value the server stores and its words. */
export type PolicyChoice = { value: string; label: string };

/**
 * A change of the federation mode that stops something from flowing (ADR-0006, "切り替え"), which the form confirms
 * before it saves: what stops reaching other servers, what stops coming in, and whether deletions stop too.
 */
export type FederationNarrowing = {
  from: FederationMode;
  to: FederationMode;
  /** The local posts, profiles and reactions stop reaching other servers; remote followers stay but receive nothing. */
  stopsSending: boolean;
  /** Remote posts and reactions stop coming in; the local users' follows of remote accounts stay but are not updated. */
  stopsReceiving: boolean;
  /** `off`: nothing flows, and a deletion made meanwhile never reaches other servers. */
  stopsAll: boolean;
};

/** The allowances of the maximum of the automatic deletion: taking one away binds the accounts that use it. */
const autoDeleteAllowances = [
  "auto_delete_max_allow_pinned",
  "auto_delete_max_allow_bookmarked",
  "auto_delete_max_allow_favourited",
  "auto_delete_max_allow_media",
  "auto_delete_max_allow_polls",
  "auto_delete_max_allow_visibilities",
  "auto_delete_max_allow_thresholds",
] as const satisfies readonly PolicyKey[];

/**
 * A change of the maximum of the automatic deletion that may bind more local accounts (ADR-0151 §5, §7), which the form
 * confirms before it saves: the maximum is turned on or lowered, or it allows fewer exceptions or fewer pins. The next
 * hourly run notifies every newly bound account, and their posts go by the maximum after the grace period.
 */
export type RetentionTightening = {
  /** The most days of the maximum as it will be. */
  maxAgeDays: number;
  /** The days of the grace period as it will be. */
  graceDays: number;
  /** Whether the maximum is turned on, rather than made stricter. */
  turnsOn: boolean;
};

/** Whether a typed whole number is one; NaN otherwise. */
function wholeNumber(text: string): number {
  return /^\d+$/u.test(text.trim()) ? Number(text.trim()) : Number.NaN;
}

/** What saving the drafts does to the maximum of the automatic deletion, or null when it binds no more accounts. */
function retentionTightening(
  saved: (key: PolicyKey) => string,
  draft: (key: PolicyKey) => string,
): RetentionTightening | null {
  const before = wholeNumber(saved("auto_delete_max_age_days"));
  const after = wholeNumber(draft("auto_delete_max_age_days"));
  if (!(after > 0)) return null;
  const graceDays = wholeNumber(draft("auto_delete_grace_days"));
  const turnsOn = !(before > 0);
  const tightens =
    turnsOn ||
    after < before ||
    autoDeleteAllowances.some((key) => saved(key) === "true" && draft(key) !== "true") ||
    wholeNumber(draft("auto_delete_max_kept_pins")) < wholeNumber(saved("auto_delete_max_kept_pins"));
  return tightens ? { maxAgeDays: after, graceDays, turnsOn } : null;
}

/** The saved or chosen federation mode; the default `both` when the value is not a mode. */
function federationModeOf(value: string): FederationMode {
  return federationModes.find((mode) => mode === value) ?? "both";
}

/** What changing the federation mode from `from` to `to` stops, or null when it stops nothing. */
function federationNarrowing(from: FederationMode, to: FederationMode): FederationNarrowing | null {
  const before = federationReachOf(from);
  const after = federationReachOf(to);
  const stopsSending = before.reachesRemote && !after.reachesRemote;
  const stopsReceiving = before.followsRemote && !after.followsRemote;
  if (!stopsSending && !stopsReceiving) return null;
  return { from, to, stopsSending, stopsReceiving, stopsAll: to === "off" };
}

/** The form of the policies of one page of the server settings: only its settings are checked and sent. */
export function useServerPolicies(page: PolicyPage) {
  const keys: readonly PolicyKey[] = policyPages[page];
  const api = useApi();
  const queryClient = useQueryClient();
  const t = useT();
  const displayLocale = useLocale();
  const { data } = useSuspenseQuery(instanceSettingsQueryOptions(api));
  const saved = new Map(data.items.map((setting) => [setting.key, setting]));
  const [drafts, setDrafts] = useState<Readonly<Partial<Record<PolicyKey, string>>>>({});
  const [status, setStatus] = useState<string | undefined>(undefined);
  const [isConfirming, setIsConfirming] = useState(false);
  const [isConfirmingLimit, setIsConfirmingLimit] = useState(false);
  const [isConfirmingRetention, setIsConfirmingRetention] = useState(false);

  const savedValue = (key: PolicyKey) => saved.get(key)?.value ?? "";
  const valueOf = (key: PolicyKey) => drafts[key] ?? savedValue(key);
  // A typed number is sent without the spaces around it.
  const sentValue = (key: PolicyKey) => (isNumberField(key) ? valueOf(key).trim() : valueOf(key));
  const changes = Object.fromEntries(
    keys.filter((key) => sentValue(key) !== savedValue(key)).map((key) => [key, sentValue(key)]),
  );

  const save = useMutation({
    ...updateInstanceSettingsMutation(api),
    onMutate: () => {
      setStatus(undefined);
    },
    onSuccess: async (settings) => {
      // The navigation offers the timelines and the sign-up page says who may sign up from GET /instance, which is
      // fetched again before the form says it is done (the button stays pending until then).
      await cacheSavedSettings(queryClient, api, settings);
      setDrafts({});
      setStatus(t("client:admin.settings.policies.saved"));
    },
  });

  const durationLabel = (seconds: number): string => {
    if (seconds % 86_400 === 0) return t("client:admin.settings.duration.days", { count: seconds / 86_400 });
    if (seconds % 3600 === 0) return t("client:admin.settings.duration.hours", { count: seconds / 3600 });
    return t("client:admin.settings.duration.seconds", { count: seconds });
  };
  const currentTtl = Number(savedValue("remote_media_cache_ttl_seconds"));
  // A whole number within the bounds the server gives. Only what the operator typed is checked: the saved value is
  // the server's.
  const numberField = (key: NumberPolicyKey): NumberField => {
    const setting = saved.get(key);
    const bounds = {
      min: setting?.minimum ?? numberFields[key].min,
      max: setting?.maximum ?? numberFields[key].max,
    };
    const text = valueOf(key).trim();
    const value = /^\d+$/u.test(text) ? Number(text) : Number.NaN;
    const valid = drafts[key] === undefined || (value >= bounds.min && value <= bounds.max);
    const range = { min: String(bounds.min), max: String(bounds.max) };
    return {
      ...bounds,
      error: valid
        ? undefined
        : key === "voice_participants_retention_days"
          ? t("client:admin.settings.voice.retention.invalid", range)
          : key === "hook_decisions_retention_days" ||
              key === "hook_local_status_account_age_days" ||
              key === "hook_local_profile_account_age_days"
            ? t("client:admin.settings.hooks.retention.invalid", range)
            : key.startsWith("auto_delete_")
              ? t("client:admin.settings.auto_delete.number.invalid", range)
              : t("client:admin.settings.invites.number.invalid", range),
    };
  };
  const numberErrors = numberKeys.filter((key) => keys.includes(key)).map((key) => numberField(key).error);
  const ttlChoices = [
    ...new Set([...cacheTtlChoices, ...(Number.isInteger(currentTtl) && currentTtl > 0 ? [currentTtl] : [])]),
  ].toSorted((a, b) => a - b);
  const currentInviteExpiry = Number(savedValue("member_invite_max_lifetime_seconds"));
  const inviteExpiries = [
    ...new Set([
      ...inviteExpiryChoices,
      ...(Number.isInteger(currentInviteExpiry) && currentInviteExpiry > 0 ? [currentInviteExpiry] : []),
    ]),
  ].toSorted((a, b) => a - b);
  const localeChoices = [...new Set([...selectableLocales(), savedValue("default_locale")])].filter(
    (locale) => locale !== "",
  );
  const narrowing = federationNarrowing(
    federationModeOf(savedValue("federation_mode")),
    federationModeOf(valueOf("federation_mode")),
  );
  // Turning on the limit to login providers can keep users from signing in, the administrator too; the count comes
  // from the server, which never refuses: the administrator decides.
  const limitTurnsOn = savedValue("login_providers_only") !== "true" && valueOf("login_providers_only") === "true";
  const impact = useQuery({ ...loginProvidersOnlyImpactQueryOptions(api), enabled: limitTurnsOn });
  // Turning on or tightening the maximum of the automatic deletion notifies every account it newly binds.
  const tightening = page === "auto_delete" ? retentionTightening(savedValue, sentValue) : null;

  return {
    /** The value of a setting in the form: the draft, else what is saved. */
    value: valueOf,
    /** Changes a setting in the form; nothing is sent until the form is saved. */
    change: (key: PolicyKey, value: string) => {
      setDrafts({ ...drafts, [key]: value });
    },
    /** The languages people may pick as the default one, named in the app's language. */
    localeChoices: localeChoices.map((locale): PolicyChoice => ({
      value: locale,
      label: languageName(locale, displayLocale),
    })),
    /** The lengths of time remote media may be kept, with the saved one when it is not among them. */
    cacheTtlChoices: ttlChoices.map((seconds): PolicyChoice => ({
      value: String(seconds),
      label: durationLabel(seconds),
    })),
    /** The longest times a member's invite may work, with the saved one when it is not among them. */
    inviteExpiryChoices: inviteExpiries.map((seconds): PolicyChoice => ({
      value: String(seconds),
      label: durationLabel(seconds),
    })),
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      // The field under the error says what to fix; nothing is sent until every number is in bounds.
      if (save.isPending || numberErrors.some((error) => error !== undefined)) return;
      if (Object.keys(changes).length === 0) {
        setStatus(t("client:admin.settings.policies.unchanged"));
        return;
      }
      if (tightening !== null) {
        setIsConfirmingRetention(true);
        return;
      }
      if (limitTurnsOn) {
        void impact.refetch();
        setIsConfirmingLimit(true);
        return;
      }
      // Narrowing the federation mode stops what other servers receive and send; the operator confirms it first.
      if (narrowing !== null) {
        setIsConfirming(true);
        return;
      }
      save.mutate(changes);
    },
    /** A setting typed as a whole number: its bounds, and why the value cannot be saved. */
    numberField,
    /** What the chosen federation mode stops compared with the saved one, or null when it stops nothing. */
    narrowing,
    /** The confirmation of turning on the limit to login providers: whether it is open and what it says. */
    limitConfirmation: {
      isOpen: isConfirmingLimit && limitTurnsOn,
      impact: impact.data,
      isFailed: impact.isError,
      /** Goes on to the confirmation of a narrowing, or saves. */
      confirm: () => {
        setIsConfirmingLimit(false);
        if (narrowing !== null) {
          setIsConfirming(true);
        } else if (!save.isPending) {
          save.mutate(changes);
        }
      },
      /** Closes the confirmation without saving; the form keeps the drafts. */
      cancel: () => {
        setIsConfirmingLimit(false);
      },
    },
    /** The confirmation of a stricter maximum of the automatic deletion: what it will be, and whether it is open. */
    retentionConfirmation: {
      tightening,
      isOpen: isConfirmingRetention && tightening !== null,
      confirm: () => {
        setIsConfirmingRetention(false);
        if (!save.isPending) save.mutate(changes);
      },
      /** Closes the confirmation without saving; the form keeps the drafts. */
      cancel: () => {
        setIsConfirmingRetention(false);
      },
    },
    /** Whether the confirmation of `narrowing` is open. */
    isConfirming: isConfirming && narrowing !== null,
    /** Saves the form after the operator confirmed the narrowing. */
    confirm: () => {
      setIsConfirming(false);
      if (!save.isPending) save.mutate(changes);
    },
    /** Closes the confirmation without saving; the form keeps the drafts. */
    cancel: () => {
      setIsConfirming(false);
    },
    isSaving: save.isPending,
    /**
     * Why the last save failed, localized; nothing was changed and the form keeps the drafts. A refused one-person
     * server says both why and how many local accounts the server has (ADR-0152 §1).
     */
    saveError: save.isError
      ? save.error instanceof ApiError &&
        save.error.type === singleUserRequiresOneAccountType &&
        save.error.detail !== undefined
        ? t("client:admin.settings.single_user.refused", { reason: save.error.title, detail: save.error.detail })
        : errorMessage(save.error)
      : undefined,
    /**
     * Whether the server is a one-person server as saved (ADR-0152 §8): the pages leave out what makes no sense for
     * one person until it is turned off and saved.
     */
    singleUser: savedValue("single_user") === "true",
    /** What the last save did, for the region that reports it. */
    status,
  };
}
