// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the review of registrations (FR-721): the sign-ups that await a moderator's
// decision, longest waiting first, and approving or rejecting one with an optional note for the moderators.
// The list follows the API's keyset pagination as an infinite query whose key ends with "pages" (web-client/10-api-client-and-auth.md §10).
import { infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import type { components } from "@mycroblog/mykit";
import { pageQuery, refreshAuditLog } from "./staff";

type Schemas = components["schemas"];

export type Registration = Schemas["AdminRegistration"];

/** What a moderator decides on a registration; each is the last segment of its operation's path. */
export type RegistrationDecision = "approve" | "reject";

/** The most characters of a decision's note (RegistrationDecisionRequest). */
export const registrationNoteLimit = 1000;

/** The registrations that await approval, longest waiting first (GET /admin/registrations). */
export function registrationsQueryOptions(api: Api) {
  return infiniteQueryOptions({
    queryKey: ["get", "/admin/registrations", "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/admin/registrations", {
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

/** One decision: on whose registration, which, and the note for the moderators ("" for none). */
export type DecideRegistration = { userId: string; decision: RegistrationDecision; note: string };

/**
 * Approves or rejects a registration (POST /admin/registrations/{user_id}/approve | reject), as the options of
 * TanStack Query's useMutation (web-client/10-api-client-and-auth.md §10). Returns the registration after it: a registration that was
 * decided already keeps its decision, so the state may differ from the decision sent.
 */
export function decideRegistrationMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/registrations/{user_id}"] as const,
    mutationFn: async ({ userId, decision, note }: DecideRegistration): Promise<Registration> => {
      const init = { params: { path: { user_id: userId } }, body: note === "" ? {} : { note } };
      const { data, error } =
        decision === "approve"
          ? await api.fetch.POST("/admin/registrations/{user_id}/approve", init)
          : await api.fetch.POST("/admin/registrations/{user_id}/reject", init);
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Whether a query is the moderators' counts (GET /admin/counts), which a decision changes. */
function isAdminCountsQuery(queryKey: readonly unknown[]): boolean {
  return queryKey[1] === "/admin/counts";
}

/**
 * Marks stale what a decision on a registration changes beside the list (web-client/README.md S1, ADR-0045): the audit
 * log, which records it, and the account searches and the account's page, which show the account's state.
 * Resolves once those on screen are fetched again.
 */
export async function refreshDecidedRegistration(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    refreshAuditLog(queryClient),
    queryClient.invalidateQueries({
      predicate: (query) => query.queryKey[1] === "/admin/accounts" || query.queryKey[1] === "/admin/accounts/{id}",
    }),
  ]);
}

/**
 * Takes the decided registration of `userId` off the cached list, which keeps only those awaiting approval, and
 * marks the moderators' counts (ADR-0045).
 */
export function cacheDecidedRegistration(api: Api, queryClient: QueryClient, userId: string): void {
  queryClient.setQueryData(
    registrationsQueryOptions(api).queryKey,
    (data) =>
      data && {
        ...data,
        pages: data.pages.map((page) => ({ ...page, items: page.items.filter((item) => item.user_id !== userId) })),
      },
  );
  void queryClient.invalidateQueries({ predicate: (query) => isAdminCountsQuery(query.queryKey) });
}
