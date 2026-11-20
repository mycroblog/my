// SPDX-License-Identifier: AGPL-3.0-only
// The remote servers as a holder of `manage_domains` reads them (ADR-0006 §3, ADR-0058 §1): those deliveries failed to
// reach or that are marked unavailable, in domain order, a page at a time; one server's health, the times it was marked
// unavailable or available again, and the deliveries to it with their attempts, newest first, of one activity or of
// one local account (which the audit log records); the masked fields of one delivery, shown on demand and recorded;
// marking a server available again; and queueing the refetch of a server's posts, or of one remote post. Every write
// reads again what it changes.
import { type components } from "@mycroblog/mykit";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { accountByIdQuery, accountQuery, type Account } from "./accounts";
import type { AppData } from "./app-data";
import { pagedQuery, usePagedList, type PagedList } from "./paged";
import { useAppData } from "./provider";
import { oneOf, pageOf, staffChanged, type StaffOne, type StaffPage } from "./staff-reads";

/** Whether deliveries go to a remote server. */
export type RemoteServer = components["schemas"]["AdminInstanceHealth"];
/** `available`, or `unavailable`: sent only `Follow`, `Accept` and `Reject`. */
export type RemoteServerState = components["schemas"]["AdminInstanceState"];
/** A time a server was marked unavailable or available again, and why. */
export type AvailabilityEvent = components["schemas"]["AdminAvailabilityEvent"];
/** The delivery of an activity to one inbox of a remote server, with its attempts. */
export type Delivery = components["schemas"]["AdminDelivery"];
/** One attempt of a delivery. */
export type DeliveryAttempt = components["schemas"]["AdminDeliveryAttempt"];
/** Why a delivery's sender, activity and personal inbox are masked. */
export type DeliveryMask = components["schemas"]["AdminDeliveryMask"];
/** The masked fields of a delivery, shown on demand. */
export type DeliveryReveal = components["schemas"]["AdminDeliveryReveal"];

export const remoteServerStates = ["available", "unavailable"] as const satisfies readonly RemoteServerState[];

/** What the list of servers shows: those of one state, every one listed where left out, and the page after `cursor`. */
export type RemoteServerQuery = {
  readonly state?: RemoteServerState | undefined;
  readonly cursor?: string | undefined;
};

/** A page of the servers `q` asks for. */
export function remoteServersQuery({ queries }: AppData, q: RemoteServerQuery) {
  return queries.queryOptions("get", "/admin/instances", {
    params: {
      query: {
        ...(q.state === undefined ? {} : { state: q.state }),
        ...(q.cursor === undefined ? {} : { cursor: q.cursor }),
      },
    },
  });
}

/** The page of servers `q` asks for; the page shown before stays while another is on its way (`stale`). */
export function useRemoteServers(q: RemoteServerQuery): StaffPage<RemoteServer> {
  return pageOf(useQuery({ ...remoteServersQuery(useAppData(), q), placeholderData: keepPreviousData }));
}

/** Reads the page of servers `q` asks for, for a route's loader. */
export async function loadRemoteServers(data: AppData, q: RemoteServerQuery): Promise<void> {
  await data.queryClient.query(remoteServersQuery(data, q)).catch(() => undefined);
}

/** The health of the server of `domain`. */
export function remoteServerQuery({ queries }: AppData, domain: string) {
  return queries.queryOptions("get", "/admin/instances/{domain}", { params: { path: { domain } } });
}

/** The health of the server of `domain`, as a screen shows it. */
export function useRemoteServer(domain: string): StaffOne<RemoteServer> {
  return oneOf(useQuery(remoteServerQuery(useAppData(), domain)));
}

/** The times the server of `domain` was marked unavailable or available, newest first, a page at a time. */
export function availabilityEventsQuery({ api, queries }: AppData, domain: string) {
  return pagedQuery<AvailabilityEvent>(
    queries.queryOptions("get", "/admin/instances/{domain}/availability_events", {
      params: { path: { domain }, query: {} },
    }).queryKey,
    async (cursor, signal) =>
      (
        await api.GET("/admin/instances/{domain}/availability_events", {
          params: { path: { domain }, query: cursor === undefined ? {} : { cursor } },
          signal,
        })
      ).data,
  );
}

/** The availability events of the server of `domain`, newest first, grown under Show more. */
export function useAvailabilityEvents(domain: string): PagedList<AvailabilityEvent> {
  return usePagedList(availabilityEventsQuery(useAppData(), domain));
}

/**
 * What the deliveries show: those of one activity, and those of one local account (which the server records in the
 * audit log before it answers each page).
 */
export type DeliveryQuery = {
  readonly activity?: string | undefined;
  readonly account?: string | undefined;
};

/** The deliveries to the server of `domain` that `q` asks for, newest first, a page at a time. */
export function deliveriesQuery({ api, queries }: AppData, domain: string, q: DeliveryQuery) {
  const query = {
    ...(q.activity === undefined ? {} : { activity_uri: q.activity }),
    ...(q.account === undefined ? {} : { account_id: q.account }),
  };
  return pagedQuery<Delivery>(
    queries.queryOptions("get", "/admin/instances/{domain}/deliveries", { params: { path: { domain }, query } })
      .queryKey,
    async (cursor, signal) =>
      (
        await api.GET("/admin/instances/{domain}/deliveries", {
          params: { path: { domain }, query: cursor === undefined ? query : { ...query, cursor } },
          signal,
        })
      ).data,
  );
}

/**
 * The deliveries to the server of `domain` that `q` asks for, grown under Show more; a 400 (an account that is not
 * local) and a 404 (one not known) are `refused`.
 */
export function useDeliveries(domain: string, q: DeliveryQuery): PagedList<Delivery> & { readonly refused: boolean } {
  const list = usePagedList(deliveriesQuery(useAppData(), domain, q));
  const error = list.error as { status?: unknown } | null;
  return { ...list, refused: list.items.length === 0 && (error?.status === 400 || error?.status === 404) };
}

/** Reads the server of `domain`, its latest availability events and deliveries, for a route's loader. */
export async function loadRemoteServer(data: AppData, domain: string): Promise<void> {
  await Promise.all([
    data.queryClient.query(remoteServerQuery(data, domain)).catch(() => undefined),
    data.queryClient.infiniteQuery(availabilityEventsQuery(data, domain)).catch(() => undefined),
    data.queryClient.infiniteQuery(deliveriesQuery(data, domain, {})).catch(() => undefined),
  ]);
}

/** What a holder of `manage_domains` does to a remote server; each rejects with the server's problem. */
export type RemoteServerWrites = {
  /** Clears the server's failure days and marks it available again; it answers the server's health. */
  readonly markAvailable: () => Promise<RemoteServer>;
  /** Queues the refetch of the stored posts of the server's accounts; a 422 for this server's domain. */
  readonly refetchPosts: () => Promise<void>;
  /** The masked fields of the delivery of `id`, which the server records in the audit log first. */
  readonly reveal: (id: string) => Promise<DeliveryReveal>;
};

/** What a holder of `manage_domains` does to the server of `domain`, as `RemoteServerWrites`. */
export function remoteServerWrites(data: AppData, domain: string): RemoteServerWrites {
  const path = { params: { path: { domain } } };
  return {
    markAvailable: async () => {
      const { data: health } = await data.api.POST("/admin/instances/{domain}/mark_available", path);
      if (health === undefined) throw new Error("app-data: marking a server available answered without its health");
      const key = remoteServerQuery(data, domain).queryKey;
      await data.queryClient.cancelQueries({ queryKey: key });
      data.queryClient.setQueryData(key, health);
      // The list of servers and the audit log, never the deliveries: reading one account's again would be audited.
      void data.queryClient.invalidateQueries({
        predicate: ({ queryKey }) =>
          queryKey[1] === "/admin/instances" || queryKey[1] === "/admin/audit_log" || queryKey[1] === "/admin/counts",
      });
      return health;
    },
    refetchPosts: async () => {
      await data.api.POST("/admin/instances/{domain}/refetch_statuses", path);
      staffChanged(data, ["/admin/audit_log"]);
    },
    reveal: async (id) => {
      const { data: shown } = await data.api.POST("/admin/instances/{domain}/deliveries/{id}/reveal", {
        params: { path: { domain, id } },
      });
      if (shown === undefined) throw new Error("app-data: a reveal answered without the delivery's fields");
      staffChanged(data, ["/admin/audit_log"]);
      return shown;
    },
  };
}

/** What a holder of `manage_domains` does to the server of `domain`, for a screen (`remoteServerWrites`). */
export function useRemoteServerWrites(domain: string): RemoteServerWrites {
  return remoteServerWrites(useAppData(), domain);
}

/** Queues a fetch of the remote post of `id` from its server; a 404 for a post unknown or deleted, a 422 for a local one. */
export async function refetchStatus(data: AppData, id: string): Promise<void> {
  await data.api.POST("/admin/statuses/{id}/refetch", { params: { path: { id } } });
  staffChanged(data, ["/admin/audit_log"]);
}

/** Queues a fetch of a remote post from its server, for a screen (`refetchStatus`). */
export function useRefetchStatus(): (id: string) => Promise<void> {
  const data = useAppData();
  return (id) => refetchStatus(data, id);
}

/**
 * The account of the handle `name` (a local username, with or without its @), whose deliveries the reader asks for;
 * undefined for none the server knows.
 */
export async function findAccount(data: AppData, name: string): Promise<Account | undefined> {
  const acct = name.trim().replace(/^@/, "");
  if (acct === "") return undefined;
  return data.queryClient.query(accountQuery(data, acct)).catch(() => undefined);
}

/** Finds the account of a handle, for a screen (`findAccount`). */
export function useFindAccount(): (name: string) => Promise<Account | undefined> {
  const data = useAppData();
  return (name) => findAccount(data, name);
}

/** The account of `id` that a filter of the deliveries names; undefined until it is read, or for none. */
export function useDeliveryAccount(id: string | undefined): Account | undefined {
  return useQuery({ ...accountByIdQuery(useAppData(), id ?? ""), enabled: id !== undefined }).data;
}
