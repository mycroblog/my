// SPDX-License-Identifier: AGPL-3.0-only
// The server's policies (FR-001, FR-105, FR-202, FR-408, FR-409, FR-413, FR-718, FR-725, FR-903, ADR-0009, ADR-0057): who may sign
// up, the default language, the public timelines, whether images need a description, the federation (its direction,
// ADR-0076, which asks before it narrows), who may read the
// list of the domain actions, how much of the running costs is published (FR-903) and shown to the holders of view_finance (ADR-0070 §5), how much of the income is published (FR-917), who may read the supporter list (ADR-0071 §4),
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
import { cacheSavedSettings, type InstanceSettingKey, instanceSettingsQueryOptions, updateInstanceSettingsMutation } from "../api/settings";
import { languageName, selectableLocales } from "./languages";

/** The settings of the form, in the order of its sections. */
const policyKeys = [
  "registration_mode",
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
  "finance_visibility",
  "finance_viewer_item_names",
  "finance_viewer_item_memos",
  "finance_viewer_income",
  "income_visibility",
  "supporter_list_visibility",
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
  // A number of days is sent without the spaces around it.
  const sentValue = (key: PolicyKey) => (key === "retention_voice_participants" ? valueOf(key).trim() : valueOf(key));
  const changes = Object.fromEntries(policyKeys.filter((key) => sentValue(key) !== savedValue(key)).map((key) => [key, sentValue(key)]));

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
  // The days the participants' records of voice chat rooms are kept (ADR-0100 §13), within the bounds the server gives.
  const retentionSetting = saved.get("retention_voice_participants");
  const retentionBounds = { min: retentionSetting?.minimum ?? 30, max: retentionSetting?.maximum ?? 3650 };
  const retentionText = valueOf("retention_voice_participants").trim();
  const retentionDays = /^\d+$/u.test(retentionText) ? Number(retentionText) : Number.NaN;
  // Only what the operator typed is checked: the saved value is the server's.
  const retentionError =
    drafts.retention_voice_participants === undefined || (retentionDays >= retentionBounds.min && retentionDays <= retentionBounds.max)
      ? undefined
      : t("client:admin.settings.voice.retention.invalid", { min: String(retentionBounds.min), max: String(retentionBounds.max) });
  const ttlChoices = [...new Set([...cacheTtlChoices, ...(Number.isInteger(currentTtl) && currentTtl > 0 ? [currentTtl] : [])])].toSorted((a, b) => a - b);
  const localeChoices = [...new Set([...selectableLocales(), savedValue("default_locale")])].filter((locale) => locale !== "");
  const narrowing = federationNarrowing(federationModeOf(savedValue("federation_mode")), federationModeOf(valueOf("federation_mode")));

  return {
    /** The value of a setting in the form: the draft, else what is saved. */
    value: valueOf,
    /** Changes a setting in the form; nothing is sent until the form is saved. */
    change: (key: PolicyKey, value: string) => {
      setDrafts({ ...drafts, [key]: value });
    },
    /** The languages people may pick as the default one, named in the app's language. */
    localeChoices: localeChoices.map((locale): PolicyChoice => ({ value: locale, label: languageName(locale, displayLocale) })),
    /** The lengths of time remote media may be kept, with the saved one when it is not among them. */
    cacheTtlChoices: ttlChoices.map((seconds): PolicyChoice => ({ value: String(seconds), label: durationLabel(seconds) })),
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      // The field under the error says what to fix; nothing is sent until it holds a number of days in bounds.
      if (save.isPending || retentionError !== undefined) return;
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
    /** The days to keep the participants' records of voice chat rooms: the bounds, and why the value cannot be saved. */
    voiceRetention: { ...retentionBounds, error: retentionError },
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
