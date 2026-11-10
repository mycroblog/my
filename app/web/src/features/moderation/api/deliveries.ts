// SPDX-License-Identifier: AGPL-3.0-only
// The deliveries to a remote server and their attempts (#2748, ADR-0058 §9), for manage_domains: the list, newest
// first, under a filter by local account (recorded in the audit log by the server) or by activity URI, and the reveal
// of the masked fields of one delivery (recorded too). No response carries the body of an activity.
import { infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { pageQuery, refreshAuditLog } from "./staff";

type Schemas = components["schemas"];

export type Delivery = Schemas["AdminDelivery"];
export type DeliveryAttempt = Schemas["AdminDeliveryAttempt"];
export type DeliveryMask = Schemas["AdminDeliveryMask"];
export type DeliveryReveal = Schemas["AdminDeliveryReveal"];
export type DeliveryOutcome = DeliveryAttempt["outcome"];
export type SignatureScheme = NonNullable<DeliveryAttempt["signature_scheme"]>;

/**
 * The filter of the deliveries: the username of a local account (its query is recorded in the audit log), the URI
 * of an activity, both, or neither (an empty string is no filter).
 */
export type DeliveryFilter = { username: string; activityUri: string };

export const noDeliveryFilter: DeliveryFilter = { username: "", activityUri: "" };

/** The username that the filter by account found no local account for (GET /accounts/lookup answered 404). */
export class UnknownLocalAccountError extends Error {
  override name = "UnknownLocalAccountError";
  readonly username: string;
  constructor(username: string) {
    super(username);
    this.username = username;
  }
}

/** The local account named `username`, as the filter by account sends its ID (GET /accounts/lookup, never resolved). */
function localAccountQueryOptions(api: Api, username: string) {
  return api.query.queryOptions("get", "/accounts/lookup", { params: { query: { acct: username } } }, { retry: false });
}

/** The ID of local account `username`, from the cache when it was looked up before. */
async function localAccountId(api: Api, queryClient: QueryClient, username: string): Promise<string> {
  try {
    const account = await queryClient.query(localAccountQueryOptions(api, username));
    return account.id;
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) throw new UnknownLocalAccountError(username);
    throw error;
  }
}

/**
 * The deliveries to `domain` under `filter`, newest first, each with its attempts (GET
 * /admin/instances/{domain}/deliveries). A filter by account looks the account up first, and every page of it is
 * recorded in the audit log by the server, so the cached audit log is marked stale once a page arrives
 * (web-client/README.md S1). The body of an activity is never part of the answer.
 */
export function deliveriesQueryOptions(api: Api, queryClient: QueryClient, domain: string, filter: DeliveryFilter) {
  // The client and the cache are the same for every key, so they stay out of it.
  const findAccount = (username: string) => localAccountId(api, queryClient, username);
  const refreshAudit = () => refreshAuditLog(queryClient);
  return infiniteQueryOptions({
    queryKey: [
      "get",
      "/admin/instances/{domain}/deliveries",
      { domain, username: filter.username, activity_uri: filter.activityUri },
      "pages",
    ] as const,
    queryFn: async ({ pageParam, signal }) => {
      const accountId = filter.username === "" ? undefined : await findAccount(filter.username);
      const query = {
        ...pageQuery(pageParam),
        ...(accountId === undefined ? {} : { account_id: accountId }),
        ...(filter.activityUri === "" ? {} : { activity_uri: filter.activityUri }),
      };
      const { data, error } = await api.fetch.GET("/admin/instances/{domain}/deliveries", {
        params: { path: { domain }, query },
        signal,
      });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      if (accountId !== undefined) void refreshAudit();
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
    // A filter by account runs when the moderator submits it, not again on its own (each run is an audit log entry).
    staleTime: filter.username === "" ? 30_000 : Infinity,
    refetchOnWindowFocus: filter.username === "",
  });
}

/**
 * Shows the sender, the activity URI and the inbox of one delivery (POST
 * /admin/instances/{domain}/deliveries/{id}/reveal), as the options of useMutation. The server records every call in
 * the audit log before it answers; the answer is kept by the mutation only, never cached.
 */
export function revealDeliveryMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/instances/{domain}/deliveries/{id}/reveal"] as const,
    mutationFn: async ({ domain, id }: { domain: string; id: string }): Promise<DeliveryReveal> => {
      const { data, error } = await api.fetch.POST("/admin/instances/{domain}/deliveries/{id}/reveal", {
        params: { path: { domain, id } },
      });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}
