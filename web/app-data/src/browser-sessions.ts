// SPDX-License-Identifier: AGPL-3.0-only
// The browsers signed in to the account (FR-012, ADR-0090 §11): the one that asks first, then the most recently used.
// The reader signs one out, or every other; the browser that asks is signed out by signing out, never from the list.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";

/** A browser signed in to the account. */
export type BrowserSession = components["schemas"]["BrowserSession"];

/** The browsers signed in, the current one first. */
export function browserSessionsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/sessions");
}

/**
 * Signs out the browser of `id`, and takes it from the cache once the server did. It rejects with the server's
 * problem (404: not in the list any more; 409: it is the browser that asks).
 */
export async function revokeBrowserSession(data: AppData, id: string): Promise<void> {
  await data.api.DELETE("/sessions/{id}", { params: { path: { id } } });
  data.queryClient.setQueryData<components["schemas"]["BrowserSessionList"]>(
    browserSessionsQuery(data).queryKey,
    (now) => (now === undefined ? now : { ...now, items: now.items.filter((s) => s.id !== id) }),
  );
}

/** Signs out every browser but the one that asks, and leaves the cache with that one. It rejects with the problem. */
export async function revokeOtherBrowserSessions(data: AppData): Promise<void> {
  await data.api.POST("/sessions/revoke_others");
  data.queryClient.setQueryData<components["schemas"]["BrowserSessionList"]>(
    browserSessionsQuery(data).queryKey,
    (now) => (now === undefined ? now : { ...now, items: now.items.filter((s) => s.current) }),
  );
}

/** The browsers signed in, current first; undefined until the server answered. */
export function useBrowserSessions(): readonly BrowserSession[] | undefined {
  return useQuery(browserSessionsQuery(useAppData())).data?.items;
}

export type BrowserSessionWrites = {
  readonly revoke: (id: string) => Promise<void>;
  readonly revokeOthers: () => Promise<void>;
};

/** Signing out one browser, or the others, from a screen. */
export function useBrowserSessionWrites(): BrowserSessionWrites {
  const data = useAppData();
  return { revoke: (id) => revokeBrowserSession(data, id), revokeOthers: () => revokeOtherBrowserSessions(data) };
}
