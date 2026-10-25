// SPDX-License-Identifier: AGPL-3.0-only
// The domains the viewer mutes (FR-304): the list, in the order of the domains over the API's keyset pagination, and
// muting and unmuting a domain. A change hides or shows the domain's posts and notifications, so it marks every list of
// posts and the notifications stale (web-client/README.md S1, ADR-0045).
import { infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { isPostListQuery } from "../../../lib/query";

export type DomainMute = components["schemas"]["DomainMute"];

/** The page size of the list. */
const pageSize = 40;

/** The longest domain the server takes (MuteDomainRequest.domain). */
export const domainLimit = 253;

/** The domains the viewer mutes, in the order of the domains (GET /domain_mutes). */
export function domainMutesQueryOptions(api: Api) {
  return infiniteQueryOptions({
    queryKey: ["get", "/domain_mutes", "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const query = pageParam === undefined ? { limit: pageSize } : { limit: pageSize, cursor: pageParam };
      const { data, error } = await api.fetch.GET("/domain_mutes", { params: { query }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** Mutes a domain (POST /domain_mutes), as the options of useMutation. Returns the mute as the server normalized it. */
export function muteDomainMutation(api: Api) {
  return {
    mutationKey: ["post", "/domain_mutes"] as const,
    mutationFn: async (domain: string): Promise<DomainMute> => {
      const { data, error } = await api.fetch.POST("/domain_mutes", { body: { domain } });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Unmutes a domain (DELETE /domain_mutes/{domain}), as the options of useMutation. */
export function unmuteDomainMutation(api: Api) {
  return {
    mutationKey: ["delete", "/domain_mutes/{domain}"] as const,
    mutationFn: async (domain: string): Promise<string> => {
      const { error } = await api.fetch.DELETE("/domain_mutes/{domain}", { params: { path: { domain } } });
      if (error !== undefined) throw new ApiError(error);
      return domain;
    },
  };
}

/**
 * What muting or unmuting a domain changes: every list of posts, the notifications and their unread count, and, with
 * `list`, the list of the muted domains (which a new one joins in the order of the domains; an unmuted one leaves it
 * through removeDomainMute instead). Resolves once those on screen are fetched again; the others are fetched when their
 * page opens (S7).
 */
export function refreshDomainMutes(queryClient: QueryClient, list: boolean): Promise<void> {
  return queryClient.invalidateQueries({
    predicate: (query) => {
      const [method, path] = query.queryKey;
      if (method !== "get") return false;
      if (path === "/domain_mutes") return list;
      return path === "/notifications" || path === "/notifications/marker" || isPostListQuery(query.queryKey);
    },
  });
}

/** Takes an unmuted domain off the cached list at once, so that the screen can move focus to the next row. */
export function removeDomainMute(queryClient: QueryClient, api: Api, domain: string): void {
  queryClient.setQueryData(domainMutesQueryOptions(api).queryKey, (data) =>
    data && { ...data, pages: data.pages.map((page) => ({ ...page, domain_mutes: page.domain_mutes.filter((mute) => mute.domain !== domain) })) },
  );
}
