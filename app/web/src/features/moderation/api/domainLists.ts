// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the lists of domains beside the domain actions (ADR-0159, manage_domains,
// web-client/README.md S1): the allowlist of federation (GET /admin/domain_allows, whole, by domain), adding and
// removing an allow; the subscriptions to the domain blocklists that others publish (newest first, by cursor), one
// subscription, subscribing, changing and deleting one, fetching it now, its fetches (newest first, by cursor), and its
// drafts with accepting and rejecting them. Every change is in the audit log; an accepted draft is a domain action too.
import { infiniteQueryOptions, type QueryClient, queryOptions } from "@tanstack/react-query";
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { pageQuery, refreshAuditLog, refreshChangedDomainActions } from "./staff";

type Schemas = components["schemas"];

export type DomainAllow = Schemas["DomainAllow"];
export type DomainListSubscription = Schemas["AdminDomainListSubscription"];
export type DomainListFormat = Schemas["AdminDomainListFormat"];
export type DomainListMode = Schemas["AdminDomainListMode"];
export type DomainListSuspendAs = Schemas["AdminDomainListSuspendAs"];
export type DomainListSilenceAs = Schemas["AdminDomainListSilenceAs"];
export type DomainListFetch = Schemas["AdminDomainListFetch"];
export type DomainListFetchOutcome = Schemas["AdminDomainListFetchOutcome"];
export type DomainListFetchError = Schemas["AdminDomainListFetchError"];
export type DomainListDraft = Schemas["AdminDomainListDraft"];
export type DomainListHold = Schemas["AdminDomainListHold"];
export type DomainListDraftResult = Schemas["AdminDomainListDraftResult"];
export type NewDomainListSubscription = Schemas["CreateDomainListSubscriptionRequest"];
export type DomainListSubscriptionChanges = Schemas["UpdateDomainListSubscriptionRequest"];

/** The most characters of an allow's comment and of a domain (CreateDomainAllowRequest). */
export const domainAllowCommentLimit = 5000;
export const domainLimit = 253;
/** The most characters of a subscription's title and URL (CreateDomainListSubscriptionRequest). */
export const subscriptionTitleLimit = 200;
export const subscriptionUrlLimit = 2048;

/** The formats of a list, in the order of the form. */
export const domainListFormats = [
  "mastodon_csv",
  "mastodon_json",
  "plain_text",
] as const satisfies readonly DomainListFormat[];
export const domainListModes = ["draft", "automatic"] as const satisfies readonly DomainListMode[];
export const domainListSuspendAs = ["suspend", "limit", "ignore"] as const satisfies readonly DomainListSuspendAs[];
export const domainListSilenceAs = ["limit", "ignore"] as const satisfies readonly DomainListSilenceAs[];

/** The allowlist of federation, by domain (GET /admin/domain_allows). */
export function domainAllowsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/admin/domain_allows");
}

/** What adding an allow did: the allow, and whether the domain was allowed already (a 200 rather than a 201). */
export type AddedDomainAllow = { allow: DomainAllow; existed: boolean };

/** Adds a domain to the allowlist (POST /admin/domain_allows), as the options of useMutation. */
export function addDomainAllowMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/domain_allows"] as const,
    mutationFn: async (body: Schemas["CreateDomainAllowRequest"]): Promise<AddedDomainAllow> => {
      const { data, error, response } = await api.fetch.POST("/admin/domain_allows", { body });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return { allow: data, existed: response.status === 200 };
    },
  };
}

/** Removes a domain from the allowlist (DELETE /admin/domain_allows/{domain}), as the options of useMutation. */
export function removeDomainAllowMutation(api: Api) {
  return {
    mutationKey: ["delete", "/admin/domain_allows/{domain}"] as const,
    mutationFn: async (domain: string): Promise<void> => {
      // The middleware throws for every error status; a 204 has no body to read.
      await api.fetch.DELETE("/admin/domain_allows/{domain}", { params: { path: { domain } } });
    },
  };
}

/**
 * Marks what a change of the allowlist changes (ADR-0028): the allowlist, the audit log, and the count of what switching
 * to the allowlist would end; resolves once those on screen are fetched again.
 */
export async function refreshDomainAllows(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/domain_allows"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/settings/federation_scope/impact"] }),
    refreshAuditLog(queryClient),
  ]);
}

/** The subscriptions to domain blocklists, newest first (GET /admin/domain_list_subscriptions). */
export function domainListSubscriptionsQueryOptions(api: Api) {
  return infiniteQueryOptions({
    queryKey: ["get", "/admin/domain_list_subscriptions", "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/admin/domain_list_subscriptions", {
        params: { query: pageQuery(pageParam) },
        signal,
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** One subscription (GET /admin/domain_list_subscriptions/{id}). */
export function domainListSubscriptionQueryOptions(api: Api, id: string) {
  return queryOptions({
    queryKey: ["get", "/admin/domain_list_subscriptions/{id}", { id }] as const,
    queryFn: async ({ signal }) => {
      const { data, error } = await api.fetch.GET("/admin/domain_list_subscriptions/{id}", {
        params: { path: { id } },
        signal,
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  });
}

/** The fetches of a subscription, newest first (GET /admin/domain_list_subscriptions/{id}/fetches). */
export function domainListFetchesQueryOptions(api: Api, id: string) {
  return infiniteQueryOptions({
    queryKey: ["get", "/admin/domain_list_subscriptions/{id}/fetches", { id }, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/admin/domain_list_subscriptions/{id}/fetches", {
        params: { path: { id }, query: pageQuery(pageParam) },
        signal,
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** The drafts of a subscription, the lifts first (GET /admin/domain_list_subscriptions/{id}/drafts). */
export function domainListDraftsQueryOptions(api: Api, id: string) {
  return queryOptions({
    queryKey: ["get", "/admin/domain_list_subscriptions/{id}/drafts", { id }] as const,
    queryFn: async ({ signal }) => {
      const { data, error } = await api.fetch.GET("/admin/domain_list_subscriptions/{id}/drafts", {
        params: { path: { id } },
        signal,
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  });
}

/** Subscribes to a list (POST /admin/domain_list_subscriptions), as the options of useMutation. */
export function createDomainListSubscriptionMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/domain_list_subscriptions"] as const,
    mutationFn: async (body: NewDomainListSubscription): Promise<DomainListSubscription> => {
      const { data, error } = await api.fetch.POST("/admin/domain_list_subscriptions", { body });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Changes the settings of a subscription (PATCH /admin/domain_list_subscriptions/{id}), as the options of useMutation. */
export function updateDomainListSubscriptionMutation(api: Api, id: string) {
  return {
    mutationKey: ["patch", "/admin/domain_list_subscriptions/{id}", { id }] as const,
    mutationFn: async (body: DomainListSubscriptionChanges): Promise<DomainListSubscription> => {
      const { data, error } = await api.fetch.PATCH("/admin/domain_list_subscriptions/{id}", {
        params: { path: { id } },
        body,
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Deletes a subscription, lifting its actions when asked (DELETE /admin/domain_list_subscriptions/{id}). */
export function deleteDomainListSubscriptionMutation(api: Api, id: string) {
  return {
    mutationKey: ["delete", "/admin/domain_list_subscriptions/{id}", { id }] as const,
    mutationFn: async (liftActions: boolean): Promise<void> => {
      await api.fetch.DELETE("/admin/domain_list_subscriptions/{id}", {
        params: { path: { id }, query: liftActions ? { lift_actions: true } : {} },
      });
    },
  };
}

/** Queues a fetch of a subscribed list (POST /admin/domain_list_subscriptions/{id}/fetch, a 202). */
export function fetchDomainListMutation(api: Api, id: string) {
  return {
    mutationKey: ["post", "/admin/domain_list_subscriptions/{id}/fetch", { id }] as const,
    mutationFn: async (): Promise<void> => {
      await api.fetch.POST("/admin/domain_list_subscriptions/{id}/fetch", { params: { path: { id } } });
    },
  };
}

/** Whether drafts are accepted or rejected. */
export type DraftDecision = "accept" | "reject";

/** Accepts or rejects drafts of a subscription by key (POST .../drafts/accept, .../drafts/reject). */
export function decideDomainListDraftsMutation(api: Api, id: string) {
  return {
    mutationKey: ["post", "/admin/domain_list_subscriptions/{id}/drafts", { id }] as const,
    mutationFn: async ({
      decision,
      keys,
    }: {
      decision: DraftDecision;
      keys: readonly string[];
    }): Promise<DomainListDraftResult> => {
      const options = { params: { path: { id } }, body: { keys: [...keys] } };
      const { data, error } =
        decision === "accept"
          ? await api.fetch.POST("/admin/domain_list_subscriptions/{id}/drafts/accept", options)
          : await api.fetch.POST("/admin/domain_list_subscriptions/{id}/drafts/reject", options);
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Whether a query is of the subscriptions: their list, one of them, its fetches or its drafts. */
function isSubscriptionQuery(queryKey: readonly unknown[]): boolean {
  const path = queryKey[1];
  return typeof path === "string" && path.startsWith("/admin/domain_list_subscriptions");
}

/**
 * Marks what a change of a subscription changes (ADR-0028): the list of subscriptions and every page of one (its
 * settings, fetches and drafts), and the audit log; resolves once those on screen are fetched again.
 */
export async function refreshDomainListSubscriptions(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ predicate: (query) => isSubscriptionQuery(query.queryKey) }),
    refreshAuditLog(queryClient),
  ]);
}

/**
 * Marks what accepting or rejecting drafts changes: the drafts and the subscription, the audit log, and, for accepted
 * drafts, the domain actions they issued or lifted and the lists of posts that a limit or a suspension changes.
 */
export async function refreshDecidedDrafts(
  queryClient: QueryClient,
  decision: DraftDecision,
  done: readonly DomainListDraft[],
): Promise<void> {
  await Promise.all([
    refreshDomainListSubscriptions(queryClient),
    decision === "accept" && done.length > 0
      ? refreshChangedDomainActions(
          queryClient,
          done.map((draft) => draft.kind),
        )
      : undefined,
  ]);
}

/** Writes a subscription from the answer of a change into its page, and marks the rest it changes. */
export async function cacheSavedSubscription(
  queryClient: QueryClient,
  api: Api,
  subscription: DomainListSubscription,
): Promise<void> {
  queryClient.setQueryData(domainListSubscriptionQueryOptions(api, subscription.id).queryKey, subscription);
  await refreshDomainListSubscriptions(queryClient);
}

/**
 * Drops the queries of a deleted subscription, which would answer 404, and marks what deleting it changes: the list of
 * subscriptions, the audit log, and the domain actions it issued when they were lifted with it.
 */
export async function forgetDeletedSubscription(
  queryClient: QueryClient,
  id: string,
  liftedActions: boolean,
): Promise<void> {
  queryClient.removeQueries({
    predicate: ({ queryKey: [, path, params] }) =>
      typeof path === "string" &&
      path.startsWith("/admin/domain_list_subscriptions/{id}") &&
      typeof params === "object" &&
      params !== null &&
      "id" in params &&
      params.id === id,
  });
  await Promise.all([
    refreshDomainListSubscriptions(queryClient),
    liftedActions ? refreshChangedDomainActions(queryClient, ["suspend"]) : undefined,
  ]);
}
