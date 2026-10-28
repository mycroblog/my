// SPDX-License-Identifier: AGPL-3.0-only
import { useAnnouncer } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { cacheSavedPreferences, type Preferences, preferencesQueryOptions, updatePreferencesMutation } from "../api/me";

/** The preferences that the settings page changes, by their API name. */
type PreferenceKey =
  | "locale"
  | "time_zone"
  | "default_visibility"
  | "default_quote_policy"
  | "default_post_language"
  | "require_media_description"
  | "timeline_languages"
  | "timeline_auto_insert"
  | "email_moderator_notifications";

/** One preference as a control shows it: it is saved as soon as it changes. */
export type PreferenceControl<T> = {
  /** The value being saved while `isPending`, the saved one otherwise (web-client/README.md S8). */
  value: T;
  onChange: (value: T) => void;
  isPending: boolean;
  /** Why the last change was not saved, localized; undefined when it was. */
  error: string | undefined;
};

export type PreferencesControls = {
  locale: PreferenceControl<string>;
  timeZone: PreferenceControl<string>;
  defaultVisibility: PreferenceControl<Preferences["default_visibility"]>;
  defaultQuotePolicy: PreferenceControl<Preferences["default_quote_policy"]>;
  defaultPostLanguage: PreferenceControl<string>;
  requireMediaDescription: PreferenceControl<boolean>;
  timelineLanguages: PreferenceControl<string[]>;
  timelineAutoInsert: PreferenceControl<boolean>;
  emailModeratorNotifications: PreferenceControl<boolean>;
  /** Why emails do not reach the primary address (read-only); undefined when they do. */
  emailSuppression: Preferences["email_suppression"];
};

/**
 * Returns a control for each preference of the settings page. Each saves itself at once (FR-007): there is
 * no form to submit, and a change of the locale displays the app in it as soon as it is saved
 * (useUserDisplaySettings reads the cached preferences).
 */
export function usePreferences(): PreferencesControls {
  const api = useApi();
  const { data } = useSuspenseQuery(preferencesQueryOptions(api));
  return {
    locale: usePreferenceControl("locale", data.locale),
    timeZone: usePreferenceControl("time_zone", data.time_zone),
    defaultVisibility: usePreferenceControl("default_visibility", data.default_visibility),
    defaultQuotePolicy: usePreferenceControl("default_quote_policy", data.default_quote_policy),
    defaultPostLanguage: usePreferenceControl("default_post_language", data.default_post_language),
    requireMediaDescription: usePreferenceControl("require_media_description", data.require_media_description),
    timelineLanguages: usePreferenceControl("timeline_languages", data.timeline_languages),
    timelineAutoInsert: usePreferenceControl("timeline_auto_insert", data.timeline_auto_insert),
    emailModeratorNotifications: usePreferenceControl(
      "email_moderator_notifications",
      data.email_moderator_notifications,
    ),
    emailSuppression: data.email_suppression,
  };
}

function usePreferenceControl<K extends PreferenceKey>(
  key: K,
  saved: Preferences[K],
): PreferenceControl<Preferences[K]> {
  const api = useApi();
  const queryClient = useQueryClient();
  const announce = useAnnouncer();
  const t = useT();
  const mutation = useMutation({
    ...updatePreferencesMutation(api),
    onSuccess: async (preferences, change) => {
      // Saved once what the change shows elsewhere is in the cache (the control stays pending until then).
      await cacheSavedPreferences(api, queryClient, preferences, change);
      announce(t("client:settings.saved"));
    },
  });
  const pending = mutation.isPending ? mutation.variables[key] : undefined;
  return {
    value: (pending ?? saved) as Preferences[K],
    onChange: (value) => {
      mutation.mutate({ [key]: value });
    },
    isPending: mutation.isPending,
    error: mutation.isError ? errorMessage(mutation.error) : undefined,
  };
}
