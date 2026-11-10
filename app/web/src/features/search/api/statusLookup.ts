// SPDX-License-Identifier: AGPL-3.0-only
// The post that a reply button of another server names (the Create intent of WebFinger, FEP-3b86): the stored one,
// or the one the server fetches from its own server (GET /statuses/lookup). Fetching reaches another server and is
// rate limited, so it is a mutation the viewer sends, not a query a page load runs (R7).
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { type Status, statusQueryOptions } from "../../status";

/** Finds the post whose ActivityPub id or page is `uri`, fetching it from its server when this server does not know it. */
export async function lookupStatus(api: Api, uri: string): Promise<Status> {
  const { data, error } = await api.fetch.GET("/statuses/lookup", { params: { query: { uri } } });
  // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
  if (data === undefined) throw new ApiError(error);
  return data;
}

/** Puts a found post into the cache of the post's page, so that the page opens without fetching it again. */
export function storeFoundStatus(queryClient: QueryClient, api: Api, status: Status): void {
  queryClient.setQueryData(statusQueryOptions(api, status.id).queryKey, status);
}
