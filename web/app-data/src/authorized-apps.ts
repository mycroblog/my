// SPDX-License-Identifier: AGPL-3.0-only
// The apps the account authorized (ADR-0090): one row per app that holds a working grant, the most recently used
// first, the server's own web client among them. Revoking one ends its tokens at once; revoking the web client signs
// the account out of every browser, this one included.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { browserSessionsQuery } from "./browser-sessions";
import { useAppData } from "./provider";

/** An app the account authorized. */
export type AuthorizedApp = components["schemas"]["AuthorizedApp"];

/** The apps the account authorized, the most recently used first. */
export function authorizedAppsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/authorized_apps");
}

/**
 * Revokes the app of `id`, and takes it from the cache once the server did; the browsers are read again when the
 * server's own web client was revoked. It rejects with the server's problem (404: it holds no working grant).
 */
export async function revokeAuthorizedApp(data: AppData, id: string): Promise<void> {
  const key = authorizedAppsQuery(data).queryKey;
  const revoked = data.queryClient
    .getQueryData<components["schemas"]["AuthorizedAppList"]>(key)
    ?.items.find((a) => a.id === id);
  await data.api.DELETE("/authorized_apps/{id}", { params: { path: { id } } });
  data.queryClient.setQueryData<components["schemas"]["AuthorizedAppList"]>(key, (now) =>
    now === undefined ? now : { ...now, items: now.items.filter((a) => a.id !== id) },
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
