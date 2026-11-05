// SPDX-License-Identifier: AGPL-3.0-only
// The translation screen's form and numbers (web-client/screens/admin-translation.md, ADR-0148): which providers are
// enabled and in what order, what each may receive, whether reports may go to them, the browser's translation, the
// monthly limits, and how long translations are kept. They are instance settings: the form sends the ones that changed together, all or none, and
// each change goes to the audit log. The numbers of the month load apart, and their failure stays in their section.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { useLoadFailure } from "../../../lib/loadFailure";
import { shellQueryRetry } from "../../../lib/query";
import {
  cacheSavedSettings,
  type InstanceSettingKey,
  instanceSettingsQueryOptions,
  updateInstanceSettingsMutation,
} from "../api/settings";
import {
  parseProviderList,
  translationOverviewQueryOptions,
  type TranslationOverview,
  type Visibility,
  visibilitiesOf,
  withVisibilities,
} from "../api/translation";

const translationKeys = [
  "translation_providers",
  "translation_provider_visibilities",
  "translation_browser",
  "translation_reports_to_server",
  "translation_user_monthly_characters",
  "translation_server_monthly_characters",
  "translation_retention_days",
] as const satisfies readonly InstanceSettingKey[];

type TranslationKey = (typeof translationKeys)[number];

/** The settings typed as a whole number, with the bounds to use until the server gives its own. */
const numberFields = {
  translation_user_monthly_characters: { min: 0, max: 1_000_000_000_000 },
  translation_server_monthly_characters: { min: 0, max: 1_000_000_000_000 },
  translation_retention_days: { min: 1, max: 3650 },
} as const satisfies Partial<Record<TranslationKey, { min: number; max: number }>>;

export type TranslationNumberKey = keyof typeof numberFields;

const numberKeys = Object.keys(numberFields) as TranslationNumberKey[];

/** The numbers of the month as the screen shows them. */
export type TranslationUse =
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | { status: "ready"; overview: TranslationOverview };

/** The use of the current month. A failure of the first load stays on screen while it loads again. */
export function useTranslationUse(): TranslationUse {
  const api = useApi();
  const query = useQuery({ ...translationOverviewQueryOptions(api), retry: shellQueryRetry });
  const failure = useLoadFailure(query);
  if (query.data !== undefined) return { status: "ready", overview: query.data };
  if (failure.error !== null) {
    return {
      status: "error",
      reason: errorMessage(failure.error),
      retry: () => void query.refetch(),
      isRetrying: failure.isRetrying,
    };
  }
  return { status: "loading" };
}

/** The form of the settings: loading until they are read. */
export function useTranslationSettings() {
  const api = useApi();
  const queryClient = useQueryClient();
  const t = useT();
  const { data } = useQuery(instanceSettingsQueryOptions(api));
  const saved = new Map((data?.items ?? []).map((setting) => [setting.key, setting]));
  const [drafts, setDrafts] = useState<Readonly<Partial<Record<TranslationKey, string>>>>({});
  const [status, setStatus] = useState<string | undefined>(undefined);

  const savedValue = (key: TranslationKey) => saved.get(key)?.value ?? "";
  const valueOf = (key: TranslationKey) => drafts[key] ?? savedValue(key);
  const change = (key: TranslationKey, value: string) => {
    setStatus(undefined);
    setDrafts((current) => ({ ...current, [key]: value }));
  };
  const changes = Object.fromEntries(
    translationKeys.filter((key) => valueOf(key) !== savedValue(key)).map((key) => [key, valueOf(key).trim()]),
  );

  // A whole number within the bounds the server gives. Only what the operator typed is checked.
  const numberField = (key: TranslationNumberKey) => {
    const setting = saved.get(key);
    const bounds = { min: setting?.minimum ?? numberFields[key].min, max: setting?.maximum ?? numberFields[key].max };
    const text = valueOf(key).trim();
    const value = /^\d+$/u.test(text) ? Number(text) : Number.NaN;
    const valid = drafts[key] === undefined || (value >= bounds.min && value <= bounds.max);
    const range = { min: String(bounds.min), max: String(bounds.max) };
    return { range, error: valid ? undefined : t("client:admin.translation.number.invalid", range) };
  };
  const hasNumberError = numberKeys.some((key) => numberField(key).error !== undefined);

  const enabled = parseProviderList(valueOf("translation_providers"));
  const save = useMutation({
    ...updateInstanceSettingsMutation(api),
    onMutate: () => {
      setStatus(undefined);
    },
    onSuccess: async (settings) => {
      // GET /instance publishes the providers, and tells every client what to offer.
      await cacheSavedSettings(queryClient, api, settings);
      setDrafts({});
      setStatus(t("client:admin.translation.saved"));
    },
  });

  return {
    /** Whether the settings are read: the form shows once they are. */
    ready: data !== undefined,
    /** The enabled IDs, the first tried first; one this server has no provider for stays when the form saves. */
    enabled,
    setEnabled: (provider: string, on: boolean) => {
      const without = enabled.filter((id) => id !== provider);
      change("translation_providers", (on ? [...without, provider] : without).join(","));
    },
    /** Swaps two enabled providers in the order. */
    swap: (provider: string, other: string) => {
      change(
        "translation_providers",
        enabled.map((id) => (id === provider ? other : id === other ? provider : id)).join(","),
      );
    },
    visibilities: (provider: string) => visibilitiesOf(valueOf("translation_provider_visibilities"), provider),
    setVisibilities: (provider: string, visibilities: readonly Visibility[]) => {
      change(
        "translation_provider_visibilities",
        withVisibilities(valueOf("translation_provider_visibilities"), provider, visibilities),
      );
    },
    reportsToServer: valueOf("translation_reports_to_server") === "true",
    setReportsToServer: (on: boolean) => {
      change("translation_reports_to_server", on ? "true" : "false");
    },
    browser: valueOf("translation_browser") === "true",
    setBrowser: (on: boolean) => {
      change("translation_browser", on ? "true" : "false");
    },
    value: valueOf,
    change,
    numberField,
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (save.isPending || hasNumberError) return;
      if (Object.keys(changes).length === 0) {
        setStatus(t("client:admin.translation.unchanged"));
        return;
      }
      save.mutate(changes);
    },
    isSaving: save.isPending,
    /** Why the last save failed, localized; nothing was changed and the form keeps the drafts. */
    saveError: save.isError ? errorMessage(save.error) : undefined,
    /** What the last save did, for the form's action bar. */
    status,
  };
}
