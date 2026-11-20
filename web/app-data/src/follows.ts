// SPDX-License-Identifier: AGPL-3.0-only
// The lists of whom an account follows and who follows it (FR-311), newest follow first, paged as the API pages them.
// Each page brings the viewer's relationships with its accounts in one request, so that every row's follow shows at
// once and none reads its own. A list the account's owner keeps from the viewer answers `follows-hidden`, which the
// list says, in place of a failure.
import { ProblemError } from "@mycroblog/mykit";
import type { Account } from "./accounts";
import { relationshipQuery } from "./accounts";
import type { AppData } from "./app-data";
import { pagedQuery, usePagedList, type PagedList } from "./paged";
import { useAppData } from "./provider";

/** Which of an account's lists: whom it follows, or who follows it. */
export type FollowsList = "following" | "followers";

/** The problem of a list that the account's owner keeps from the viewer. */
const followsHidden = "/api/v1/problems/follows-hidden";

/** Puts the viewer's relationships with `ids` into the cache, each where useRelationship reads it; a failure is left. */
export async function readRelationships(data: AppData, ids: readonly string[], signal: AbortSignal): Promise<void> {
  if (ids.length === 0) return;
  const { data: answer } = await data.api
    .GET("/accounts/relationships", { params: { query: { id: [...ids] } }, signal })
    .catch(() => ({ data: undefined }));
  for (const r of answer?.items ?? [])
    data.queryClient.setQueryData(relationshipQuery(data, r.id).queryKey, { items: [r] });
}

/**
 * The accounts of the account of `id`'s list, newest follow first, with the viewer's relationships with them; without
 * a session (`signedIn` false), which has none, the accounts alone.
 */
export function accountFollowsQuery(data: AppData, id: string, list: FollowsList, signedIn = true) {
  const { api, queries } = data;
  const page = async (cursor: string | undefined, signal: AbortSignal) => {
    const params = { path: { id }, query: cursor === undefined ? {} : { cursor } };
    const { data: answer } =
      list === "followers"
        ? await api.GET("/accounts/{id}/followers", { params, signal })
        : await api.GET("/accounts/{id}/following", { params, signal });
    if (answer !== undefined && signedIn)
      await readRelationships(
        data,
        answer.items.map((a) => a.id),
        signal,
      );
    return answer;
  };
  const key =
    list === "followers"
      ? queries.queryOptions("get", "/accounts/{id}/followers", { params: { path: { id } } }).queryKey
      : queries.queryOptions("get", "/accounts/{id}/following", { params: { path: { id } } }).queryKey;
  return pagedQuery<Account>(key, page);
}

/** A list of an account's follows as a screen shows it, and whether its owner keeps it from the viewer. */
export type AccountFollows = PagedList<Account> & {
  /** The account's owner keeps the list from the viewer (its `follows_visibility`): it shows no account. */
  readonly hidden: boolean;
};

/** Whether `error` is the server's answer that the account's owner keeps the list from the viewer. */
const isFollowsHidden = (error: unknown) => error instanceof ProblemError && error.type === followsHidden;

/**
 * The list of the account of `id`; nothing is fetched while `enabled` is false, and without a session (`signedIn`
 * false) no relationship.
 */
export function useAccountFollows(id: string, list: FollowsList, enabled = true, signedIn = true): AccountFollows {
  const paged = usePagedList(accountFollowsQuery(useAppData(), id, list, signedIn), enabled);
  return { ...paged, hidden: paged.status === "error" && isFollowsHidden(paged.error) };
}
