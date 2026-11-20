// SPDX-License-Identifier: AGPL-3.0-only
// The OAuth apps as a holder of `manage_oauth_apps` keeps them (ADR-0090 §1): every app that is not deleted, newest
// first, a page at a time by the server's cursor, kept to one way of registration; disabling one, which revokes every
// token of it at once, and enabling it again; and the hosts whose client metadata documents are denied, denying one and
// lifting a denial. Every write reads the lists again.
import type { components } from "@mycroblog/mykit";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";

/** An OAuth app, as the administrators see it. */
export type AdminOAuthClient = components["schemas"]["AdminOAuthClient"];
/** How an OAuth app was registered. */
export type OAuthClientRegistration = components["schemas"]["OAuthClientRegistration"];
/** A host whose client metadata documents are denied. */
export type OAuthHostDenial = components["schemas"]["AdminOAuthClientHostDenial"];

/** The ways an app is registered, in the server's order. */
export const oAuthClientRegistrations = [
  "first_party",
  "user",
  "metadata_document",
  "dynamic",
] as const satisfies readonly OAuthClientRegistration[];

/** What the list of apps shows: the apps of one way of registration (every way where left out), after `cursor`. */
export type AdminOAuthClientQuery = {
  readonly registration?: OAuthClientRegistration | undefined;
  readonly cursor?: string | undefined;
};

/** A page of the apps. */
export function adminOAuthClientsQuery({ queries }: AppData, q: AdminOAuthClientQuery) {
  return queries.queryOptions("get", "/admin/oauth_clients", {
    params: {
      query: {
        ...(q.registration === undefined ? {} : { registration: q.registration }),
        ...(q.cursor === undefined ? {} : { cursor: q.cursor }),
      },
    },
  });
}

/** A page of the denied hosts. */
export function oAuthHostDenialsQuery({ queries }: AppData, cursor: string | undefined) {
  return queries.queryOptions("get", "/admin/oauth_client_host_denials", {
    params: { query: cursor === undefined ? {} : { cursor } },
  });
}

/** What a screen shows of a page: its items, the next page's cursor, and where its read is. */
export type AdminPage<T> = {
  readonly items: readonly T[];
  readonly next: string | undefined;
  readonly status: "pending" | "error" | "success";
  /** The page shows what the reader asked before while the page they asked for now is on its way. */
  readonly stale: boolean;
  readonly refetch: () => void;
};

/** The page of `query`, the one shown before staying while another is on its way. */
function usePage<T>(query: {
  data: { items: T[]; next_cursor?: string } | undefined;
  status: "pending" | "error" | "success";
  isPlaceholderData: boolean;
  refetch: () => unknown;
}): AdminPage<T> {
  const cursor = query.data?.next_cursor;
  return {
    items: query.data?.items ?? [],
    next: cursor === undefined || cursor === "" ? undefined : cursor,
    status: query.status,
    stale: query.isPlaceholderData,
    refetch: () => {
      void query.refetch();
    },
  };
}

/** The page of the apps `q` asks for. */
export function useAdminOAuthClients(q: AdminOAuthClientQuery): AdminPage<AdminOAuthClient> {
  return usePage(useQuery({ ...adminOAuthClientsQuery(useAppData(), q), placeholderData: keepPreviousData }));
}

/** The page of the denied hosts after `cursor`. */
export function useOAuthHostDenials(cursor: string | undefined): AdminPage<OAuthHostDenial> {
  return usePage(useQuery({ ...oAuthHostDenialsQuery(useAppData(), cursor), placeholderData: keepPreviousData }));
}

/** Reads the page of the apps `q` asks for, for a route's loader. */
export async function loadAdminOAuthClients(data: AppData, q: AdminOAuthClientQuery): Promise<void> {
  await data.queryClient.query(adminOAuthClientsQuery(data, q)).catch(() => undefined);
}

/** Reads the page of the denied hosts after `cursor` (the first without one), for a route's loader. */
export async function loadOAuthHostDenials(data: AppData, cursor?: string): Promise<void> {
  await data.queryClient.query(oAuthHostDenialsQuery(data, cursor)).catch(() => undefined);
}

/** The writes of the apps and the denied hosts; each rejects with the server's problem. */
export type AdminOAuthWrites = {
  /** Disables an app, revoking every token of it at once. */
  readonly disable: (id: string) => Promise<AdminOAuthClient>;
  /** Enables an app again; the tokens disabling it revoked stay revoked. */
  readonly enable: (id: string) => Promise<AdminOAuthClient>;
  /** Denies the client metadata documents of `host`, revoking the tokens of its apps at once (a 422 names `host`). */
  readonly deny: (host: string) => Promise<OAuthHostDenial>;
  /** Lifts the denial of `host`. */
  readonly allow: (host: string) => Promise<void>;
};

/** The writes of the apps and the denied hosts, for a screen. */
export function useAdminOAuthWrites(): AdminOAuthWrites {
  const data = useAppData();
  const lists = () =>
    data.queryClient.invalidateQueries({
      predicate: ({ queryKey }) =>
        queryKey[1] === "/admin/oauth_clients" || queryKey[1] === "/admin/oauth_client_host_denials",
    });
  const client = async (app: AdminOAuthClient | undefined) => {
    if (app === undefined) throw new Error("app-data: a write of an app answered without the app");
    await lists();
    return app;
  };
  return {
    disable: async (id) =>
      client((await data.api.POST("/admin/oauth_clients/{id}/disable", { params: { path: { id } } })).data),
    enable: async (id) =>
      client((await data.api.POST("/admin/oauth_clients/{id}/enable", { params: { path: { id } } })).data),
    deny: async (host) => {
      const { data: denial } = await data.api.POST("/admin/oauth_client_host_denials", { body: { host } });
      if (denial === undefined) throw new Error("app-data: the denial answered without the denial");
      await lists();
      return denial;
    },
    allow: async (host) => {
      await data.api.DELETE("/admin/oauth_client_host_denials/{host}", { params: { path: { host } } });
      await lists();
    },
  };
}
