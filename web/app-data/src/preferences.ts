// SPDX-License-Identifier: AGPL-3.0-only
// The reader's preferences (FR-007, ADR-0009 §2, §3), one resource that every device of the account shares: the
// language and the time zone the app shows, each the account's choice or the browser's; how link cards show and
// whether new timeline posts go in by themselves; the languages of the timelines; and what a new post starts from. A
// change of any of them shows at once to every screen that reads them (usePreferences), laid over what the server last
// said while its request is on its way, and is gone again when the server refuses it or does not answer; changes made
// together each keep their own (the settings save every control by itself, ADR-0110 §5). The cache holds what the
// server said alone. The server filters the timelines by their languages, so a change of those reads them again.
import type { components } from "@mycroblog/mykit";
import { useQuery, type QueryClient } from "@tanstack/react-query";
import { useMemo, useSyncExternalStore } from "react";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";

/** The reader's preferences. */
export type Preferences = components["schemas"]["Preferences"];

/** A change of the preferences: the ones it names, the others staying as they are. */
export type PreferencesChange = components["schemas"]["UpdatePreferencesRequest"];

/** How link cards show: the image above the text, or a square beside it. */
export type LinkCardSize = components["schemas"]["LinkCardSize"];

/** The reader's preferences, as the server last said them. */
export function preferencesQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/preferences");
}

/**
 * The preferences with `change` in place, as the server applies it: a language or a time zone named without whether
 * it follows the browser stops following it, an interaction policy changes the kinds it names, and the timelines'
 * languages come sorted, each once.
 */
function changed(now: Preferences, change: PreferencesChange): Preferences {
  const { default_interaction_policy: policy, timeline_languages: languages, ...plain } = change;
  return {
    ...now,
    ...plain,
    ...(change.locale !== undefined && change.locale_follows_browser === undefined
      ? { locale_follows_browser: false }
      : {}),
    ...(change.time_zone !== undefined ? { time_zone_chosen: true } : {}),
    ...(change.time_zone !== undefined && change.time_zone_follows_browser === undefined
      ? { time_zone_follows_browser: false }
      : {}),
    default_interaction_policy: { ...now.default_interaction_policy, ...policy },
    timeline_languages: languages === undefined ? now.timeline_languages : [...new Set(languages)].toSorted(),
  };
}

/** The changes on their way to the server, oldest first, and who shows them. */
type Flying = { changes: readonly PreferencesChange[]; readonly listeners: Set<() => void> };

const none: readonly PreferencesChange[] = [];
const flights = new WeakMap<QueryClient, Flying>();

function flyingOf({ queryClient }: AppData): Flying {
  let flying = flights.get(queryClient);
  if (flying === undefined) {
    flying = { changes: none, listeners: new Set() };
    flights.set(queryClient, flying);
  }
  return flying;
}

function setFlying(flying: Flying, changes: readonly PreferencesChange[]): void {
  flying.changes = changes;
  for (const listener of flying.listeners) listener();
}

/**
 * Changes the preferences that `change` names and resolves with them all as the server holds them then. The change
 * shows at once (usePreferences) and, when the server refuses it or does not answer, shows no more: it rejects with
 * mykit's ProblemError (a 422's `detail` names the preference that is invalid) or NetworkError. A change of the
 * timelines' languages marks the timelines, which are read again when they next show.
 */
export async function updatePreferences(data: AppData, change: PreferencesChange): Promise<Preferences> {
  const { queryClient } = data;
  const key = preferencesQuery(data).queryKey;
  const flying = flyingOf(data);
  setFlying(flying, [...flying.changes, change]);
  try {
    const { data: answer } = await data.api.PATCH("/preferences", { body: change });
    if (answer === undefined) throw new Error("app-data: the change of the preferences answered without them");
    // A read that left before the change would put back what the server held then.
    await queryClient.cancelQueries({ queryKey: key });
    queryClient.setQueryData(key, answer);
    if (change.timeline_languages !== undefined)
      void queryClient.invalidateQueries({
        predicate: ({ queryKey }) => typeof queryKey[1] === "string" && queryKey[1].startsWith("/timelines/"),
      });
    return answer;
  } finally {
    setFlying(
      flying,
      flying.changes.filter((c) => c !== change),
    );
  }
}

/**
 * The reader's preferences as they show: what the server last said, with the changes on their way to it in place.
 * Undefined until they are read, and without a session (`signedIn` false), which has none: not those the cache holds
 * of an account that read them before either.
 */
export function usePreferences(signedIn = true): Preferences | undefined {
  const data = useAppData();
  const read = useQuery({ ...preferencesQuery(data), enabled: signedIn }).data;
  const said = signedIn ? read : undefined;
  const flying = flyingOf(data);
  const changes = useSyncExternalStore(
    (listener) => {
      flying.listeners.add(listener);
      return () => {
        flying.listeners.delete(listener);
      };
    },
    () => flying.changes,
    () => none,
  );
  return useMemo(() => (said === undefined ? undefined : changes.reduce(changed, said)), [said, changes]);
}

/**
 * Where the read of the reader's preferences is, for a screen that shows them all (the settings): `loading` until the
 * server answered, `failed` when it did not, with `retry`, which reads them again.
 */
export function usePreferencesRead(): { status: "loading" | "failed" | "read"; retry: () => void } {
  const { status, refetch } = useQuery(preferencesQuery(useAppData()));
  return {
    status: status === "success" ? "read" : status === "error" ? "failed" : "loading",
    retry: () => {
      void refetch();
    },
  };
}

/** Changes the reader's preferences from a screen (`updatePreferences`). */
export function useUpdatePreferences(): (change: PreferencesChange) => Promise<Preferences> {
  const data = useAppData();
  return (change) => updatePreferences(data, change);
}
