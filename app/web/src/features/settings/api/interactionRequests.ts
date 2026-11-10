// SPDX-License-Identifier: AGPL-3.0-only
// The replies, boosts, and quotes of the viewer's posts that wait for their approval (ADR-0155 §10), as an infinite
// query over the API's keyset pagination like the lists of relationships.ts, and the answers to them.
import { infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import type { InteractionRequest } from "../../status";

/** The page size of the list, the API's maximum. */
const pageSize = 40;

/** The requests that wait for the viewer's approval, newest first (GET /interaction_requests). */
export function interactionRequestsQueryOptions(api: Api) {
  return infiniteQueryOptions({
    queryKey: ["get", "/interaction_requests", "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/interaction_requests", {
        params: { query: pageParam === undefined ? { limit: pageSize } : { limit: pageSize, cursor: pageParam } },
        signal,
      });
      // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** How the viewer answers a request. */
export type RequestAnswer = "accept" | "reject";

/** Approves or refuses the request `id` (POST /interaction_requests/{id}/accept or /reject); returns it answered. */
export async function answerInteractionRequest(
  api: Api,
  id: string,
  answer: RequestAnswer,
): Promise<InteractionRequest> {
  const path = answer === "accept" ? "/interaction_requests/{id}/accept" : "/interaction_requests/{id}/reject";
  const { data, error } = await api.fetch.POST(path, { params: { path: { id } } });
  if (data === undefined) throw new ApiError(error);
  return data;
}

/** Takes the answered request `id` off the cached pages: the list holds only the requests that wait. */
export function removeAnsweredRequest(api: Api, queryClient: QueryClient, id: string): void {
  queryClient.setQueryData(
    interactionRequestsQueryOptions(api).queryKey,
    (data) =>
      data && {
        ...data,
        pages: data.pages.map((page) => ({ ...page, items: page.items.filter((request) => request.id !== id) })),
      },
  );
}
