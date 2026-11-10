// SPDX-License-Identifier: AGPL-3.0-only
// The hashtags a user follows and features (ADR-0013 §24–§28, ADR-0006 §17): a hashtag with whether the viewer follows
// it, following and unfollowing it, the list of the followed ones, and the hashtags a profile features. Every write says
// what it changes in the cache next to it (web-client/README.md S1, ADR-0028): a follow changes the home timeline, which
// then also shows the hashtag's posts from accounts the viewer does not follow.
import type { QueryClient } from "@tanstack/react-query";
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { textTokens } from "../../../lib/textTokens";

/** A hashtag and whether the viewer follows it. */
export type Hashtag = components["schemas"]["Hashtag"];

/** A hashtag that the viewer follows, with when they followed it. */
export type FollowedHashtag = components["schemas"]["FollowedHashtag"];

/** The most hashtags a user follows (ADR-0013 §27). */
export const maxFollowedTags = 100;

/** The most hashtags a profile features (ADR-0006 §17). */
export const maxFeaturedTags = 10;

/** The longest hashtag the server takes, without its `#` (HashtagName). */
export const hashtagLimit = 256;

/**
 * The hashtag as the path parameter of the API: with its `#`, which the server takes too, so that the hashtag `search`
 * is not the search of hashtags (`/tags/search`).
 */
function hashtagParam(tag: string) {
  return { params: { path: { hashtag: `#${tag}` } } };
}

/** What the user typed for a hashtag, without the spaces around it and one leading `#` (or full-width `＃`). */
export function hashtagInput(text: string): string {
  return text.trim().replace(/^[#＃]/u, "");
}

/**
 * Whether `name` (without `#`) is a hashtag the server takes: letters, marks, digits and underscores with at least one
 * letter or mark (the server's ParseText, through lib/textTokens), no longer than the server takes.
 */
export function isHashtagName(name: string): boolean {
  if (name === "" || name.length > hashtagLimit) return false;
  const tokens = textTokens(`#${name}`);
  return tokens.length === 1 && tokens[0]?.kind === "hashtag" && tokens[0].text.length === name.length + 1;
}

/** The key two names of the same hashtag share: the server matches hashtags under NFKC and lowercase (ADR-0009 §6). */
export function hashtagKey(name: string): string {
  return name.normalize("NFKC").toLowerCase();
}

/** A hashtag and whether the viewer follows it (GET /tags/{hashtag}). */
export function tagQueryOptions(api: Api, tag: string) {
  return api.query.queryOptions("get", "/tags/{hashtag}", hashtagParam(tag));
}

/** The hashtags the viewer follows, the latest followed first (GET /followed_tags). */
export function followedTagsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/followed_tags");
}

/** The hashtags the viewer features on their profile, in their order (GET /featured_tags). */
export function featuredTagsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/featured_tags");
}

/** The hashtags the account `id` features on its profile, in its order (GET /accounts/{id}/featured_tags). */
export function accountFeaturedTagsQueryOptions(api: Api, id: string) {
  return api.query.queryOptions("get", "/accounts/{id}/featured_tags", { params: { path: { id } } });
}

/** A change of a follow: the hashtag as the caller named it, and whether to follow it or stop. */
export type TagFollowChange = { tag: string; follow: boolean };

/**
 * Follows or unfollows a hashtag (POST or DELETE /tags/{hashtag}/follow), as the options of useMutation. Returns the
 * hashtag as the server answers it, with the request it answers.
 */
export function tagFollowMutation(api: Api) {
  return {
    mutationKey: ["post", "/tags/{hashtag}/follow"] as const,
    mutationFn: async (change: TagFollowChange): Promise<{ change: TagFollowChange; hashtag: Hashtag }> => {
      const init = hashtagParam(change.tag);
      const { data, error } = change.follow
        ? await api.fetch.POST("/tags/{hashtag}/follow", init)
        : await api.fetch.DELETE("/tags/{hashtag}/follow", init);
      // The client's middleware throws an ApiError for every error status (lib/api/client.ts); this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return { change, hashtag: data };
    },
  };
}

/**
 * What a follow or an unfollow changes: the hashtag's page as it was asked (and as the server names it, which may be
 * written otherwise), the home timeline, and the list of the followed hashtags, which a new follow joins at the top
 * and an unfollowed hashtag leaves. Resolves once those on screen are fetched again; the home timeline is fetched again
 * when it opens (S7), so the hashtag's posts are on it without a reload. With `keepRow`, an unfollowed hashtag stays on
 * the cached list until the caller takes it off (removeFollowedTag), so that the list can move focus from its row first.
 */
export async function storeTagFollow(
  queryClient: QueryClient,
  api: Api,
  { change, hashtag }: { change: TagFollowChange; hashtag: Hashtag },
  { keepRow = false }: { keepRow?: boolean } = {},
): Promise<void> {
  queryClient.setQueryData(tagQueryOptions(api, change.tag).queryKey, hashtag);
  if (hashtag.name !== change.tag) queryClient.setQueryData(tagQueryOptions(api, hashtag.name).queryKey, hashtag);
  if (!hashtag.following && !keepRow) removeFollowedTag(queryClient, api, hashtag.name);
  await queryClient.invalidateQueries({
    predicate: (query) => {
      const path = query.queryKey[1];
      return path === "/timelines/home" || (hashtag.following && path === "/followed_tags");
    },
  });
}

/** Takes an unfollowed hashtag off the cached list of the followed hashtags at once. */
export function removeFollowedTag(queryClient: QueryClient, api: Api, name: string): void {
  const key = hashtagKey(name);
  queryClient.setQueryData<components["schemas"]["FollowedHashtagCollection"]>(
    followedTagsQueryOptions(api).queryKey,
    (data) => data && { items: data.items.filter((tag) => hashtagKey(tag.name) !== key) },
  );
}

/** Replaces the hashtags the viewer features (PUT /featured_tags), as the options of useMutation. */
export function setFeaturedTagsMutation(api: Api) {
  return {
    mutationKey: ["put", "/featured_tags"] as const,
    mutationFn: async (names: readonly string[]) => {
      const { data, error } = await api.fetch.PUT("/featured_tags", { body: { names: [...names] } });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * What choosing the featured hashtags changes: the viewer's list, which the answer holds, and the list on the viewer's
 * profile (`accountId`), which is the same.
 */
export function storeFeaturedTags(
  queryClient: QueryClient,
  api: Api,
  accountId: string,
  saved: components["schemas"]["FeaturedHashtagCollection"],
): void {
  queryClient.setQueryData(featuredTagsQueryOptions(api).queryKey, saved);
  queryClient.setQueryData(accountFeaturedTagsQueryOptions(api, accountId).queryKey, saved);
}

/** Why a change of the followed or featured hashtags failed, by what the user can do about it. */
export type HashtagFailure = "too-many" | "invalid" | "other";

/** Classifies an error of a follow or of the featured hashtags (api/openapi.yaml, Problem). */
export function hashtagFailure(error: unknown): HashtagFailure {
  if (!(error instanceof ApiError)) return "other";
  if (
    error.type === "/api/v1/problems/too-many-followed-tags" ||
    error.type === "/api/v1/problems/too-many-featured-tags"
  )
    return "too-many";
  if (error.status === 422) return "invalid";
  return "other";
}
