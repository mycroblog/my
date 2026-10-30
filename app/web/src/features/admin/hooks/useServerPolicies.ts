// SPDX-License-Identifier: AGPL-3.0-only
// The server's policies (FR-001, FR-105, FR-202, FR-408, FR-409, FR-413, FR-718, FR-725, FR-903, ADR-0009, ADR-0057): who may sign
// up, who may create invites of their own and the caps of those invites (ADR-0108 §1, §2), the default language, the public timelines, whether images need a description, the federation (its direction,
// ADR-0076, which asks before it narrows), who may read the
// list of the domain actions, whether the landing page shows the server's numbers and who runs it,
// what the server states to search engines and AI crawlers, and voice chat (ADR-0100 §2, §9, §13: who may start a
// room, whether visitors see a room's summary, and how many days the participants' records are kept). They are
// instance settings that used to change only from the CLI (`my instance set`); the form sends the ones that
// changed together, all or none, and each change goes to the audit log (FR-704).
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type FederationMode, federationModes, federationReachOf } from "../../../lib/federation";
import { useLocale, useT } from "../../../lib/i18n";
import {
  cacheSavedSettings,
  type InstanceSettingKey,
  instanceSettingsQueryOptions,
  updateInstanceSettingsMutation,
} from "../api/settings";
import { languageName, selectableLocales } from "./languages";

/** The settings of the form, in the order of its sections. */
const policyKeys = [
  "registration_mode",
  "invite_creators",
  "member_invite_max_active",
  "member_invite_max_uses",
  "member_invite_max_expires_in",
  "user_app_registration",
  "oauth_metadata_document_clients",
  "dynamic_client_registration",
  "default_locale",
  "local_timeline_access",
  "federated_timeline_access",
  "require_media_description",
  "federation_mode",
  "authorized_fetch",
  "remote_media_policy",
  "remote_media_cache_ttl",
  "link_cards_enabled",
  "domain_actions_visibility",
  "domain_actions_rationale_visibility",
  // The publication of the costs, the income and the supporter list is set next to those screens (usePublicationSettings).
  "public_counts",
  "featured_accounts",
  "search_indexing",
  "indexable_default",
  "ai_usage",
  "security_contact",
  "voice_room_starters",
  "voice_public_summary",
  "retention_voice_participants",
] as const satisfies readonly InstanceSettingKey[];

export type PolicyKey = (typeof policyKeys)[number];

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
 * days to keep the participants' records of voice chat rooms (ADR-0100 §13) and the caps of the members' invites
 * (ADR-0108 §2).
 */
const numberFields = {
  retention_voice_participants: { min: 30, max: 3650 },
  member_invite_max_active: { min: 0, max: 100 },
  member_invite_max_uses: { min: 1, max: 1000 },
} as const satisfies Partial<Record<PolicyKey, { min: number; max: number }>>;

/** A setting typed as a whole number. */
export type NumberPolicyKey = keyof typeof numberFields;

const numberKeys: readonly NumberPolicyKey[] = [
  "retention_voice_participants",
  "member_invite_max_active",
  "member_invite_max_uses",
];

function isNumberField(key: PolicyKey): key is NumberPolicyKey {
  return key in numberFields;
}

/** A field of a whole number: its bounds, and why its value cannot be saved (undefined when it can). */
export type NumberField = { min: number; max: number; error: string | undefined };

/** A choice of a setting: the value the server stores and its words. */
export type PolicyChoice = { value: string; label: string };

/**
 * A change of the federation mode that stops something from flowing (ADR-0076, "切り替え"), which the form confirms
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

export function useServerPolicies() {
  const api = useApi();
  const queryClient = useQueryClient();
  const t = useT();
  const displayLocale = useLocale();
  const { data } = useSuspenseQuery(instanceSettingsQueryOptions(api));
  const saved = new Map(data.items.map((setting) => [setting.key, setting]));
  const [drafts, setDrafts] = useState<Readonly<Partial<Record<PolicyKey, string>>>>({});
  const [status, setStatus] = useState<string | undefined>(undefined);
  const [isConfirming, setIsConfirming] = useState(false);

  const savedValue = (key: PolicyKey) => saved.get(key)?.value ?? "";
  const valueOf = (key: PolicyKey) => drafts[key] ?? savedValue(key);
  // A typed number is sent without the spaces around it.
  const sentValue = (key: PolicyKey) => (isNumberField(key) ? valueOf(key).trim() : valueOf(key));
  const changes = Object.fromEntries(
    policyKeys.filter((key) => sentValue(key) !== savedValue(key)).map((key) => [key, sentValue(key)]),
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
  const currentTtl = Number(savedValue("remote_media_cache_ttl"));
  // A whole number within the bounds the server gives. Only what the operator typed is checked: the saved value is
  // the server's.
  const numberField = (key: NumberPolicyKey): NumberField => {
    const setting = saved.get(key);
    const bounds = { min: setting?.minimum ?? numberFields[key].min, max: setting?.maximum ?? numberFields[key].max };
    const text = valueOf(key).trim();
    const value = /^\d+$/u.test(text) ? Number(text) : Number.NaN;
    const valid = drafts[key] === undefined || (value >= bounds.min && value <= bounds.max);
    const range = { min: String(bounds.min), max: String(bounds.max) };
    return {
      ...bounds,
      error: valid
        ? undefined
        : key === "retention_voice_participants"
          ? t("client:admin.settings.voice.retention.invalid", range)
          : t("client:admin.settings.invites.number.invalid", range),
    };
  };
  const numberErrors = numberKeys.map((key) => numberField(key).error);
  const ttlChoices = [
    ...new Set([...cacheTtlChoices, ...(Number.isInteger(currentTtl) && currentTtl > 0 ? [currentTtl] : [])]),
  ].toSorted((a, b) => a - b);
  const currentInviteExpiry = Number(savedValue("member_invite_max_expires_in"));
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
    /** Why the last save failed, localized; nothing was changed and the form keeps the drafts. */
    saveError: save.isError ? errorMessage(save.error) : undefined,
    /** What the last save did, for the region that reports it. */
    status,
  };
}
