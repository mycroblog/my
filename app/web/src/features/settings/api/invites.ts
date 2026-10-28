// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the user's own invites (FR-011, ADR-0108 §5): the invites the user created as a member,
// newest first, each page with whether they may create one now, how many usable invites they hold, and the caps of the
// instance settings (GET /invites); creating one within the caps (POST /invites); and revoking one (POST
// /invites/{id}/revoke). A user who may not create invites gets a 403 `invites-not-allowed`, one who holds as many usable
// invites as allowed a 409 `too-many-invites`, and a value over a cap a 422 naming it; the problem's detail says why.
import { type InfiniteData, infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { pageQuery } from "../../moderation";

type Schemas = components["schemas"];

export type OwnInvite = Schemas["OwnInvite"];
export type OwnInviteState = Schemas["AdminInviteState"];
export type OwnInviteLimits = Schemas["MemberInviteLimits"];
export type NewOwnInvite = Schemas["CreateOwnInviteRequest"];

/** The user's invites, newest first, each page with what the user may do. */
export function ownInvitesQueryOptions(api: Api) {
  return infiniteQueryOptions({
    queryKey: ["get", "/invites", "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/invites", { params: { query: pageQuery(pageParam) }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** Creates an invite of the user's own (POST /invites), as the options of useMutation. */
export function createOwnInviteMutation(api: Api) {
  return {
    mutationKey: ["post", "/invites"] as const,
    mutationFn: async (body: NewOwnInvite): Promise<OwnInvite> => {
      const { data, error } = await api.fetch.POST("/invites", { body });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Revokes an invite of the user's own (POST /invites/{id}/revoke), as the options of useMutation. */
export function revokeOwnInviteMutation(api: Api) {
  return {
    mutationKey: ["post", "/invites/{id}/revoke"] as const,
    mutationFn: async (id: string): Promise<OwnInvite> => {
      const { data, error } = await api.fetch.POST("/invites/{id}/revoke", { params: { path: { id } } });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Marks the user's invites stale (web-client/README.md S1): a new invite is at the top, and the usable count changes. */
export function refreshOwnInvites(queryClient: QueryClient): Promise<void> {
  return queryClient.invalidateQueries({ queryKey: ["get", "/invites"] });
}

/**
 * Writes the revoked invite from the answer into the cached list (ADR-0045), which shows it right at once, and marks
 * the list stale for the count of usable invites, which the next fetch answers.
 */
export function cacheRevokedOwnInvite(queryClient: QueryClient, revoked: OwnInvite): Promise<void> {
  queryClient.setQueriesData<InfiniteData<{ items: OwnInvite[] }>>(
    { queryKey: ["get", "/invites"] },
    (data) =>
      data && {
        ...data,
        pages: data.pages.map((page) => ({
          ...page,
          items: page.items.map((item) => (item.id === revoked.id ? revoked : item)),
        })),
      },
  );
  return refreshOwnInvites(queryClient);
}
