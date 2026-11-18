// SPDX-License-Identifier: AGPL-3.0-only
// The apps the account authorized (ADR-0090): one row per app that holds a working grant, the most recently used
// first, the server's own web client among them. Revoking one ends its tokens at once; revoking the web client signs
// the account out of every browser, this one included.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { browserSessionsQuery } from "./browser-sessions";
import { useAppData } from "./provider";
import { removeCached } from "./written";

/** An app the account authorized. */
export type AuthorizedApp = components["schemas"]["AuthorizedApp"];

type AuthorizedAppList = components["schemas"]["AuthorizedAppList"];

/** The apps the account authorized, the most recently used first. */
export function authorizedAppsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/authorized_apps");
}

/**
 * Revokes the app of `id`, and takes it from the cache once the server did, the reads of the list on their way
 * cancelled first. An app that holds no working grant any more (404) is revoked already: it resolves all the same, and
 * the list is read again. When the server's own web client was revoked, the browsers signed in are stale and read
 * anew when next shown, never at once: that revocation ended this browser's session too, and nothing more is asked
 * with it. It rejects with the server's problem.
 */
export async function revokeAuthorizedApp(data: AppData, id: string): Promise<void> {
  const key = authorizedAppsQuery(data).queryKey;
  const revoked = data.queryClient.getQueryData<AuthorizedAppList>(key)?.items.find((a) => a.id === id);
  await removeCached<AuthorizedAppList>(
    data,
    key,
    data.api.DELETE("/authorized_apps/{id}", { params: { path: { id } } }),
    (now) => ({ ...now, items: now.items.filter((a) => a.id !== id) }),
  );
  if (revoked?.first_party === true)
    void data.queryClient.invalidateQueries({ queryKey: browserSessionsQuery(data).queryKey, refetchType: "none" });
}

/** The apps the account authorized; undefined until the server answered. */
export function useAuthorizedApps(): readonly AuthorizedApp[] | undefined {
  return useQuery(authorizedAppsQuery(useAppData())).data?.items;
}

/** Revokes an app, from a screen (revokeAuthorizedApp). */
export function useRevokeAuthorizedApp(): (id: string) => Promise<void> {
  const data = useAppData();
  return (id) => revokeAuthorizedApp(data, id);
}
