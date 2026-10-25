// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the OAuth apps screen (web-client/README.md S1, ADR-0090 §1, manage_oauth_apps): the
// clients that are not deleted, newest first, kept to one registration route on request (GET /admin/oauth_clients),
// disabling one and lifting that (POST .../disable and .../enable), and the hosts whose Client ID Metadata Documents
// are denied (GET and POST /admin/oauth_client_host_denials, DELETE .../{host}). Disabling a client and denying a
// host revoke the tokens at once. A server's own client cannot be disabled (422). Every change is recorded in the
// audit log.
import { infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { pageQuery, refreshAuditLog } from "../../moderation";

type Schemas = components["schemas"];

export type OAuthClient = Schemas["AdminOAuthClient"];
export type OAuthClientRegistration = Schemas["OAuthClientRegistration"];
export type HostDenial = Schemas["AdminOAuthClientHostDenial"];

/** The routes of registration, in the order of the filter. */
export const oauthClientRegistrations = ["first_party", "user", "metadata_document", "dynamic"] as const satisfies readonly OAuthClientRegistration[];

/** The longest host name (DenyOAuthClientHostRequest). */
export const hostLimit = 253;

/** Which clients the list shows: those of one registration route, or every client when `registration` is undefined. */
export type OAuthClientFilter = { registration?: OAuthClientRegistration | undefined };

/** The clients under `filter`, newest first. */
export function oauthClientsQueryOptions(api: Api, filter: OAuthClientFilter) {
  const query = filter.registration === undefined ? {} : { registration: filter.registration };
  return infiniteQueryOptions({
    queryKey: ["get", "/admin/oauth_clients", query, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/admin/oauth_clients", { params: { query: { ...query, ...pageQuery(pageParam) } }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** The denied hosts, newest denial first. */
export function hostDenialsQueryOptions(api: Api) {
  return infiniteQueryOptions({
    queryKey: ["get", "/admin/oauth_client_host_denials", "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/admin/oauth_client_host_denials", { params: { query: pageQuery(pageParam) }, signal });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** Disables a client (`disable`) or lifts that (`enable`), as the options of useMutation. Returns the client as it is now. */
export function setOAuthClientDisabledMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/oauth_clients/{id}/disable"] as const,
    mutationFn: async ({ id, disable }: { id: string; disable: boolean }): Promise<OAuthClient> => {
      const { data, error } = disable
        ? await api.fetch.POST("/admin/oauth_clients/{id}/disable", { params: { path: { id } } })
        : await api.fetch.POST("/admin/oauth_clients/{id}/enable", { params: { path: { id } } });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Denies the Client ID Metadata Documents of a host, as the options of useMutation. */
export function denyHostMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/oauth_client_host_denials"] as const,
    mutationFn: async (host: string): Promise<void> => {
      // The middleware throws for every error status; a 204 has no body to read.
      await api.fetch.POST("/admin/oauth_client_host_denials", { body: { host } });
    },
  };
}

/** Lifts the denial of a host, as the options of useMutation. */
export function allowHostMutation(api: Api) {
  return {
    mutationKey: ["delete", "/admin/oauth_client_host_denials/{host}"] as const,
    mutationFn: async (host: string): Promise<void> => {
      await api.fetch.DELETE("/admin/oauth_client_host_denials/{host}", { params: { path: { host } } });
    },
  };
}

/**
 * Marks what a change changes (ADR-0045): the clients (a denial revokes the tokens of the host's clients, and a
 * disablement shows in the list), the denied hosts and the audit log; the queries shown now are fetched again and
 * awaited.
 */
export async function refreshOAuthApps(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/oauth_clients"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/oauth_client_host_denials"] }),
    refreshAuditLog(queryClient),
  ]);
}
