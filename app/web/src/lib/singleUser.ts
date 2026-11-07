// SPDX-License-Identifier: AGPL-3.0-only
// The one-person server (FR-811, ADR-0152), from GET /instance (`single_user`): the home of its operator alone. The
// server holds the registration closed and both public timelines off, which the app reads from `registrations` and
// `timelines` as it does on any server; this tells the rest that the server does not offer (the suggestions of
// accounts, the invites, the review of registrations, the sign-up blocks and the visibility `local`, ADR-0152 §4, §5),
// so that the app hides them before it tries them, and the operator's account, whose profile is the server's front
// page for people who are not signed in (§7).
import type { components } from "@mycroblog/mykit";
import { type QueryClient, useQuery } from "@tanstack/react-query";
import { notFound } from "@tanstack/react-router";
import type { Api } from "./api/client";
import { useApi } from "./auth/SessionProvider";
import { ApiError } from "./errors";
import { type RouterContext, shellQueryReader } from "./query";

/** The problem type of the 404 of an operation that a one-person server does not offer. */
const singleUserModeType = "/api/v1/problems/single-user-mode";

/** The problem type of the 422 of turning the mode on while the server has more than one local account. */
export const singleUserRequiresOneAccountType = "/api/v1/problems/single-user-requires-one-account";

/** Whether `error` is the server's answer that a one-person server does not offer the operation. */
function isSingleUserMode(error: unknown): boolean {
  return error instanceof ApiError && error.type === singleUserModeType;
}

/** The instance, as GET /instance returns it. */
type Instance = components["schemas"]["Instance"];

/** Whether `instance` is a one-person server; false until GET /instance loads. */
export function isSingleUser(instance: Instance | undefined): boolean {
  return instance?.single_user === true;
}

/** The instance as the shell reads it; the shell reads GET /instance on every page and handles its failures. */
function useInstance(): Instance | undefined {
  const api = useApi();
  return useQuery({ ...api.query.queryOptions("get", "/instance"), ...shellQueryReader }).data;
}

/** Whether the server is a one-person server: false until GET /instance loads, and when it cannot. */
export function useSingleUser(): boolean {
  return isSingleUser(useInstance());
}

/**
 * Whether the server offers what only a community has (the suggestions of accounts, the invites): only once GET
 * /instance says it is no one-person server, so that the app asks for none of them before it knows.
 */
export function useCommunityOffered(): boolean {
  const instance = useInstance();
  return instance !== undefined && !isSingleUser(instance);
}

/**
 * For the `beforeLoad` of a screen that a one-person server does not offer: the router's "not found" while the server
 * is one, so that the screen never asks for what the server answers with a 404. A failure of GET /instance lets the
 * screen load, which shows the server's answer.
 */
export async function notFoundOnSingleUser({ context }: { context: RouterContext }): Promise<void> {
  const instance = await context.queryClient
    .query({ ...context.api.query.queryOptions("get", "/instance"), staleTime: Infinity })
    .catch(() => undefined);
  if (isSingleUser(instance)) notFound({ throw: true });
}

/**
 * Fetches GET /instance again whenever the server answers that a one-person server does not offer an operation: the
 * operators turned the mode on after the page loaded, and the app should stop offering what it hides.
 */
export function refetchInstanceOnSingleUserMode(queryClient: QueryClient, api: Api): () => void {
  return queryClient.getQueryCache().subscribe((event) => {
    if (event.type === "updated" && event.action.type === "error" && isSingleUserMode(event.action.error)) {
      void queryClient.invalidateQueries({ queryKey: api.query.queryOptions("get", "/instance").queryKey });
    }
  });
}
