// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the invites (FR-001, manage_invites): the codes that let people sign up in every
// registration mode but `closed` (ADR-0108 §4), the moderators' and the members' own, listed newest first with how they
// were used and who created them, created and revoked. The list
// follows the API's keyset pagination as a TanStack Query infinite query whose key ends with "pages"
// (web-client/10-api-client-and-auth.md §10).
import { type InfiniteData, infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { pageQuery } from "./staff";

type Schemas = components["schemas"];

export type Invite = Schemas["AdminInvite"];
export type InviteState = Schemas["AdminInviteState"];
export type NewInvite = Schemas["CreateInviteRequest"];

/** The invites, newest first (GET /admin/invites). */
export function invitesQueryOptions(api: Api) {
  return infiniteQueryOptions({
    queryKey: ["get", "/admin/invites", "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/admin/invites", {
        params: { query: pageQuery(pageParam) },
        signal,
      });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** Creates an invite (POST /admin/invites), as the options of useMutation. */
export function createInviteMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/invites"] as const,
    mutationFn: async (body: NewInvite): Promise<Invite> => {
      const { data, error } = await api.fetch.POST("/admin/invites", { body });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Revokes an invite (POST /admin/invites/{id}/revoke), as the options of useMutation. */
export function revokeInviteMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/invites/{id}/revoke"] as const,
    mutationFn: async (id: string): Promise<Invite> => {
      const { data, error } = await api.fetch.POST("/admin/invites/{id}/revoke", { params: { path: { id } } });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Marks the list of invites stale (web-client/README.md S1): a new invite is at its top. */
export function refreshInvites(queryClient: QueryClient): Promise<void> {
  return queryClient.invalidateQueries({ queryKey: ["get", "/admin/invites"] });
}

/** Writes the revoked invite from the answer into the cached list (ADR-0045), which shows it right. */
export function cacheRevokedInvite(queryClient: QueryClient, revoked: Invite): void {
  queryClient.setQueriesData<InfiniteData<{ items: Invite[] }>>(
    { queryKey: ["get", "/admin/invites"] },
    (data) =>
      data && {
        ...data,
        pages: data.pages.map((page) => ({
          ...page,
          items: page.items.map((item) => (item.id === revoked.id ? revoked : item)),
        })),
      },
  );
}
