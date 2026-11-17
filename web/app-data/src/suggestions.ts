// SPDX-License-Identifier: AGPL-3.0-only
// The accounts the server suggests the reader follow (FR-308): those the administrators feature, then those that
// posted lately, none the reader follows (`GET /accounts/suggestions`), each with the reader's relationship read in
// the same turn so that its follow shows at once. A one-person server suggests none (ADR-0152): it answers 404, which
// reads as no suggestion, and the screens that offer them leave them out there.
import { ProblemError } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import { viewerQuery, type Account } from "./accounts";
import type { AppData } from "./app-data";
import { readRelationships } from "./follows";
import { instanceQuery } from "./instance";
import { useAppData } from "./provider";

/** The suggested accounts, with the reader's relationships with them in the cache. */
export function suggestionsQuery(data: AppData) {
  return {
    queryKey: [...data.queries.queryOptions("get", "/accounts/suggestions").queryKey, "relationships"] as const,
    queryFn: async ({ signal }: { signal: AbortSignal }): Promise<readonly Account[]> => {
      const answer = await data.api.GET("/accounts/suggestions", { signal }).catch((e: unknown) => {
        // A one-person server suggests no one.
        if (e instanceof ProblemError && e.status === 404) return { data: { items: [] } };
        throw e;
      });
      const accounts = (answer.data?.items ?? []).map((s) => s.account);
      await readRelationships(
        data,
        accounts.map((a) => a.id),
        signal,
      );
      return accounts;
    },
  };
}

/** What a screen shows of the suggestions: the accounts once read, whether they failed, and reading them again. */
export type Suggestions = {
  readonly items: readonly Account[];
  readonly status: "pending" | "error" | "success";
  readonly refetch: () => void;
};

/** The suggested accounts; nothing is asked while `enabled` is false. */
export function useSuggestions(enabled = true): Suggestions {
  const query = useQuery({ ...suggestionsQuery(useAppData()), enabled });
  return {
    items: query.data ?? [],
    status: query.status,
    refetch: () => {
      void query.refetch();
    },
  };
}

/**
 * Whether Home offers people to follow, which it settles before it paints: the suggestions are asked only for a reader
 * who follows no one, on a server with more than one person, and a failure at any step offers none.
 */
export type HomeSuggestions = "pending" | "none" | "some";

/** Reads what Home offers: the viewer, then (for a reader who follows no one) the instance and the suggestions. */
export async function loadHomeSuggestions(data: AppData): Promise<void> {
  const { queryClient } = data;
  const viewer = await queryClient.query({ ...viewerQuery(data), staleTime: Infinity }).catch(() => undefined);
  if (viewer?.following_count !== 0) return;
  const instance = await queryClient.query({ ...instanceQuery(data), staleTime: Infinity }).catch(() => undefined);
  if (instance === undefined || instance.single_user) return;
  await queryClient.query({ ...suggestionsQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** What Home offers; "pending" until everything it depends on is settled, which a loader (loadHomeSuggestions) does. */
export function useHomeSuggestions(): HomeSuggestions {
  const data = useAppData();
  const viewer = useQuery(viewerQuery(data));
  const alone = viewer.data?.following_count === 0;
  const instance = useQuery({ ...instanceQuery(data), enabled: alone });
  const suggested = alone && instance.data !== undefined && !instance.data.single_user;
  // A read that failed in the loader stays failed for this visit: Home does without it, and does not wait for a retry.
  const suggestions = useQuery({ ...suggestionsQuery(data), enabled: suggested, retryOnMount: false });
  if (viewer.status === "pending") return "pending";
  if (!alone) return "none";
  if (instance.status === "pending") return "pending";
  if (!suggested) return "none";
  if (suggestions.status === "pending") return "pending";
  return suggestions.status === "success" && suggestions.data.length > 0 ? "some" : "none";
}
