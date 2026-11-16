// SPDX-License-Identifier: AGPL-3.0-only
// The servers the viewer mutes (FR-304, ADR-0013 §30): a remote domain and its subdomains, whose posts leave the
// viewer's timelines other than home, whose boosts leave home, and whose accounts notify them no more; the accounts of
// it they follow stay on home. The server filters; the client keeps the list of the muted domains, read once whole, to
// say on a profile that its server is muted, and marks what a mute changes.
import { useQuery } from "@tanstack/react-query";
import type { Marking } from "./accounts";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";

/** The domains the viewer mutes, in IDNA A-labels, every page of them read. */
export function domainMutesQuery(data: AppData) {
  const { api, queries } = data;
  return {
    queryKey: queries.queryOptions("get", "/domain_mutes").queryKey,
    queryFn: async ({ signal }: { signal: AbortSignal }): Promise<readonly string[]> => {
      const domains: string[] = [];
      let cursor: string | undefined;
      do {
        const { data: page } = await api.GET("/domain_mutes", {
          params: { query: cursor === undefined ? { limit: 40 } : { limit: 40, cursor } },
          signal,
        });
        domains.push(...(page?.items.map((m) => m.domain) ?? []));
        cursor = page?.next_cursor === "" ? undefined : page?.next_cursor;
      } while (cursor !== undefined);
      return domains;
    },
  };
}

/** Whether `domain` is `muted` or one of its subdomains. */
const under = (domain: string, muted: string) => domain === muted || domain.endsWith(`.${muted}`);

/** The domain the viewer mutes that `domain` falls under (itself, or a parent); undefined until read, or when none. */
export function useMutedDomain(domain: string | undefined): string | undefined {
  const { data } = useQuery({ ...domainMutesQuery(useAppData()), enabled: domain !== undefined });
  const lower = domain?.toLowerCase();
  return lower === undefined ? undefined : data?.find((muted) => under(lower, muted));
}

/**
 * Marks what a mute of a server changes: the timelines, the notifications, and the profiles' posts; the shown ones read
 * now, or with `later` when they open again.
 */
async function markMuted(data: AppData, marking: Marking): Promise<void> {
  await data.queryClient.invalidateQueries({
    predicate: ({ queryKey }) =>
      typeof queryKey[1] === "string" &&
      (queryKey[1].startsWith("/timelines/") ||
        queryKey[1] === "/notifications" ||
        queryKey[1] === "/accounts/{id}/statuses"),
    refetchType: marking === "shown" ? "active" : "none",
  });
}

/**
 * Mutes `domain` (`on`) or stops muting it, and resolves once the cache holds the list as the server left it and what it
 * marked is read (`marking`). It rejects with the server's problem.
 */
export async function muteDomain(
  data: AppData,
  domain: string,
  on: boolean,
  marking: Marking = "shown",
): Promise<void> {
  const key = domainMutesQuery(data).queryKey;
  if (on) {
    const { data: mute } = await data.api.POST("/domain_mutes", { body: { domain } });
    const muted = mute?.domain ?? domain.toLowerCase();
    data.queryClient.setQueryData<readonly string[]>(key, (now) =>
      now === undefined || now.includes(muted) ? now : [...now, muted],
    );
  } else {
    await data.api.DELETE("/domain_mutes/{domain}", { params: { path: { domain } } });
    const lower = domain.toLowerCase();
    data.queryClient.setQueryData<readonly string[]>(key, (now) => now?.filter((muted) => muted !== lower));
  }
  await markMuted(data, marking);
}

/** Mutes a server or stops muting it, from a screen (`muteDomain`). */
export function useMuteDomain(): (domain: string, on: boolean, marking?: Marking) => Promise<void> {
  const data = useAppData();
  return (domain, on, marking) => muteDomain(data, domain, on, marking);
}
