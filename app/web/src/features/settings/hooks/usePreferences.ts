// SPDX-License-Identifier: AGPL-3.0-only
import { type SaveState, useNotice } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { useAppLocale } from "../../../lib/locale";
import { saveStateOf } from "./saveState";
import { deviceTimeZone } from "./timeZones";
import {
  cacheSavedPreferences,
  type Preferences,
  type PreferencesChange,
  preferencesQueryOptions,
  updatePreferencesMutation,
} from "../api/me";

/** The value of the language and time zone controls for following the browser (FR-007): no language or zone has it. */
export const followBrowser = "browser";

/** The preferences that the settings page changes, by their API name. */
type PreferenceKey =
  | "default_visibility"
  | "default_quote_policy"
  | "default_post_language"
  | "require_media_description"
  | "timeline_languages"
  | "timeline_auto_insert"
  | "link_card_size";

/** One preference as a control shows it: it is saved as soon as it changes. */
export type PreferenceControl<T> = {
  /** The value being saved while `isPending`, the saved one otherwise (web-client/README.md S8). */
  value: T;
  onChange: (value: T) => void;
  isPending: boolean;
  /** Why the last change was not saved, localized; undefined when it was. */
  error: string | undefined;
  /** Where the last change is, for the mark next to the control (web-client/16-look-states-and-wording.md G24). */
  saveState: SaveState;
};

export type PreferencesControls = {
  /** The chosen language, or `followBrowser`. */
  locale: PreferenceControl<string>;
  /** The chosen time zone, or `followBrowser`. */
  timeZone: PreferenceControl<string>;
  defaultVisibility: PreferenceControl<Preferences["default_visibility"]>;
  defaultQuotePolicy: PreferenceControl<Preferences["default_quote_policy"]>;
  defaultPostLanguage: PreferenceControl<string>;
  requireMediaDescription: PreferenceControl<boolean>;
  timelineLanguages: PreferenceControl<string[]>;
  timelineAutoInsert: PreferenceControl<boolean>;
  linkCardSize: PreferenceControl<"large" | "small">;
};

/**
 * Returns a control for each preference of the settings page. Each saves itself at once (FR-007): there is
 * no form to submit, and a change of the locale displays the app in it as soon as it is saved
 * (useUserDisplaySettings reads the cached preferences).
 */
export function usePreferences(): PreferencesControls {
  const api = useApi();
  const appLocale = useAppLocale();
  const { data } = useSuspenseQuery(preferencesQueryOptions(api));
  return {
    locale: useBrowserFollowingControl({
      saved: data.locale_follows_browser ? followBrowser : data.locale,
      following: { locale_follows_browser: true },
      isFollowing: (change) => change.locale_follows_browser === true,
      pendingChoice: (change) => change.locale,
      choose: (locale) => ({ locale }),
      detect: () => ({ locale: appLocale.browserLocale(navigator.languages) }),
    }),
    timeZone: useBrowserFollowingControl({
      saved: data.time_zone_follows_browser ? followBrowser : data.time_zone,
      following: { time_zone_follows_browser: true },
      isFollowing: (change) => change.time_zone_follows_browser === true,
      pendingChoice: (change) => change.time_zone,
      choose: (time_zone) => ({ time_zone }),
      detect: () => ({ time_zone: deviceTimeZone() }),
    }),
    defaultVisibility: usePreferenceControl("default_visibility", data.default_visibility),
    defaultQuotePolicy: usePreferenceControl("default_quote_policy", data.default_quote_policy),
    defaultPostLanguage: usePreferenceControl("default_post_language", data.default_post_language),
    requireMediaDescription: usePreferenceControl("require_media_description", data.require_media_description),
    timelineLanguages: usePreferenceControl("timeline_languages", data.timeline_languages),
    timelineAutoInsert: usePreferenceControl("timeline_auto_insert", data.timeline_auto_insert),
    linkCardSize: usePreferenceControl("link_card_size", data.link_card_size),
  };
}

/** Saves a change of the preferences and says so; what the change shows elsewhere is in the cache by then. */
function useSaveChange() {
  const api = useApi();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const t = useT();
  const mutation = useMutation({
    ...updatePreferencesMutation(api),
    onSuccess: async (preferences, change) => {
      // Saved once what the change shows elsewhere is in the cache (the control stays pending until then).
      await cacheSavedPreferences(api, queryClient, preferences, change);
      // The control's save mark shows it.
      notice.inPlace(t("client:settings.saved"));
    },
  });
  return mutation;
}

function usePreferenceControl<K extends PreferenceKey>(
  key: K,
  saved: Preferences[K],
): PreferenceControl<Preferences[K]> {
  const mutation = useSaveChange();
  const pending = mutation.isPending ? mutation.variables[key] : undefined;
  return {
    value: (pending ?? saved) as Preferences[K],
    onChange: (value) => {
      mutation.mutate({ [key]: value });
    },
    isPending: mutation.isPending,
    error: mutation.isError ? errorMessage(mutation.error) : undefined,
    saveState: saveStateOf(mutation),
  };
}

/**
 * A preference that can follow the browser: choosing `followBrowser` saves what the browser says with the flag, and
 * choosing a value saves it alone, which stops following (PATCH /preferences).
 */
function useBrowserFollowingControl({
  saved,
  following,
  isFollowing,
  pendingChoice,
  choose,
  detect,
}: {
  saved: string;
  following: PreferencesChange;
  isFollowing: (change: PreferencesChange) => boolean;
  pendingChoice: (change: PreferencesChange) => string | undefined;
  choose: (value: string) => PreferencesChange;
  detect: () => PreferencesChange;
}): PreferenceControl<string> {
  const mutation = useSaveChange();
  const pending = mutation.isPending
    ? isFollowing(mutation.variables)
      ? followBrowser
      : pendingChoice(mutation.variables)
    : undefined;
  return {
    value: pending ?? saved,
    onChange: (value) => {
      mutation.mutate(value === followBrowser ? { ...detect(), ...following } : choose(value));
    },
    isPending: mutation.isPending,
    error: mutation.isError ? errorMessage(mutation.error) : undefined,
    saveState: saveStateOf(mutation),
  };
}
