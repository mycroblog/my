// SPDX-License-Identifier: AGPL-3.0-only
// The queries of the user's own profile and preferences (web-client/README.md S1). Their keys come from the
// openapi-react-query client, so the account here shares its cache entry with the session's viewer.
import type { QueryClient, QueryKey } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { updateCachedAccount } from "../../profile";

type Schemas = components["schemas"];

export type Preferences = Schemas["Preferences"];
export type PreferencesChange = Schemas["UpdatePreferencesRequest"];
export type Visibility = Schemas["Visibility"];
export type CredentialAccount = Schemas["CredentialAccount"];
export type ProfileChange = Schemas["UpdateCredentialsRequest"];
export type Permission = Schemas["Permission"];
export type SearchIndexing = Schemas["SearchIndexing"];
export type AIUsage = Schemas["AIUsage"];

/** The signed-in user's preferences (GET /preferences). */
export function preferencesQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/preferences");
}

/** The instance (GET /instance), for `sends_email` and what it states to crawlers; the app shell reads the same query on every page. */
export function instanceQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/instance");
}

/** The signed-in user's own account with the source of its profile (GET /accounts/verify_credentials). */
export function credentialsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/accounts/verify_credentials");
}

/**
 * Saves a change of the preferences (PATCH /preferences), as the options of TanStack Query's useMutation.
 * The client's own useMutation is a hook read from an object, which React Compiler cannot compile around
 * (react.dev, "Don't dynamically use Hooks").
 */
export function updatePreferencesMutation(api: Api) {
  return {
    mutationKey: ["patch", "/preferences"] as const,
    mutationFn: async (change: PreferencesChange): Promise<Preferences> => {
      const { data, error } = await api.fetch.PATCH("/preferences", { body: change });
      // The middleware throws an ApiError for every error status, so this is only for the types.
      if (data === undefined) throw new Error(error.title);
      return data;
    },
  };
}

/** Whether `queryKey` is a timeline's (home, local, federated, hashtag): the lists the timeline languages filter. */
function isTimelineQuery(queryKey: QueryKey): boolean {
  return typeof queryKey[1] === "string" && queryKey[1].startsWith("/timelines/");
}

/**
 * Puts the preferences as the server answered `change` into the cache (web-client/README.md S1, ADR-0045). A change of
 * the timeline languages changes what every timeline shows, so the cached timelines are marked stale, and this
 * resolves once those on screen are fetched again; a profile shows its posts in every language. A change of
 * the locale is handled where the app displays it (lib/services.ts marks every query).
 */
export async function cacheSavedPreferences(
  api: Api,
  queryClient: QueryClient,
  preferences: Preferences,
  change: PreferencesChange,
): Promise<void> {
  queryClient.setQueryData(preferencesQueryOptions(api).queryKey, preferences);
  if (change.timeline_languages !== undefined)
    await queryClient.invalidateQueries({ predicate: (query) => isTimelineQuery(query.queryKey) });
}

/**
 * Puts the viewer's account as the server answered a change of the profile into the cache: the viewer's own
 * query, and every cached copy of the account (the profile page, the author of the viewer's posts), so that
 * each shows the new profile from its first render (web-client/README.md S1, ADR-0045).
 */
export function cacheSavedProfile(api: Api, queryClient: QueryClient, saved: CredentialAccount): void {
  queryClient.setQueryData(credentialsQueryOptions(api).queryKey, saved);
  updateCachedAccount(queryClient, saved);
}

/**
 * Chooses `timeZone`, the time zone of the device in use, for a user who has chosen none yet
 * (PATCH /preferences). `signal` cancels it.
 */
export async function adoptTimeZone(api: Api, timeZone: string, signal: AbortSignal): Promise<Preferences> {
  const { data, error } = await api.fetch.PATCH("/preferences", { body: { time_zone: timeZone }, signal });
  // The middleware throws an ApiError for every error status, so this is only for the types.
  if (data === undefined) throw new Error(error.title);
  return data;
}

/** Saves a change of the profile (PATCH /accounts/update_credentials), as the options of useMutation. */
export function updateProfileMutation(api: Api) {
  return {
    mutationKey: ["patch", "/accounts/update_credentials"] as const,
    mutationFn: async (change: ProfileChange): Promise<CredentialAccount> => {
      const { data, error } = await api.fetch.PATCH("/accounts/update_credentials", { body: change });
      // The middleware throws an ApiError for every error status, so this is only for the types.
      if (data === undefined) throw new Error(error.title);
      return data;
    },
  };
}
