// SPDX-License-Identifier: AGPL-3.0-only
// Hashtags (FR-203, ADR-0013 §24): a hashtag and whether the reader follows it, and following it or not. A write puts
// the hashtag the server answered into the cache, and marks the home timeline, which the posts of a followed hashtag
// join, and the list of the followed hashtags.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";

/** A hashtag and whether the reader follows it. */
export type Hashtag = components["schemas"]["Hashtag"];

/** The hashtag of `hashtag` (its name without `#`). */
export function tagQuery({ queries }: AppData, hashtag: string) {
  return queries.queryOptions("get", "/tags/{hashtag}", { params: { path: { hashtag } } });
}

/** The hashtag of a name, undefined until the server answered, and its failure. */
export function useTag(hashtag: string): { tag: Hashtag | undefined; error: Error | null } {
  const query = useQuery(tagQuery(useAppData(), hashtag));
  return { tag: query.data, error: query.error as Error | null };
}

/**
 * Follows the hashtag of `hashtag`, or stops following it, and resolves once the cache holds what the server answered
 * and the home timeline is marked. It rejects with the server's problem (too many followed hashtags).
 */
export async function followTag(data: AppData, hashtag: string, follow: boolean): Promise<void> {
  const { api, queryClient } = data;
  const path = { params: { path: { hashtag } } };
  const { data: tag } = follow
    ? await api.POST("/tags/{hashtag}/follow", path)
    : await api.DELETE("/tags/{hashtag}/follow", path);
  if (tag !== undefined) queryClient.setQueryData(tagQuery(data, hashtag).queryKey, tag);
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["get", "/timelines/home"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/followed_tags"] }),
  ]);
}

/** Follows a hashtag or stops following it, from a screen (`followTag`). */
export function useFollowTag(): (hashtag: string, follow: boolean) => Promise<void> {
  const data = useAppData();
  return (hashtag, follow) => followTag(data, hashtag, follow);
}
