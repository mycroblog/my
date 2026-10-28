// SPDX-License-Identifier: AGPL-3.0-only
// The refetch of remote posts from their servers (FR-407, federation.md §6.4): one post, the posts of a remote
// account, or those of the accounts of a remote server. The server queues the fetches and records the request in
// the audit log; nothing on screen shows the posts as they are fetched, which happens in the background.
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { refreshAuditLog } from "./staff";

/** What a refetch is for: a remote post, the posts of a remote account, or those of a remote server's accounts. */
export type RefetchTarget =
  { kind: "status"; id: string } | { kind: "account"; id: string } | { kind: "domain"; domain: string };

/**
 * Queues the refetch of `target` (POST /admin/statuses/{id}/refetch, /admin/accounts/{id}/refetch_statuses or
 * /admin/instances/{domain}/refetch_statuses), as the options of useMutation.
 */
export function refetchMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/refetch"] as const,
    mutationFn: async (target: RefetchTarget): Promise<void> => {
      const { error } =
        target.kind === "status"
          ? await api.fetch.POST("/admin/statuses/{id}/refetch", { params: { path: { id: target.id } } })
          : target.kind === "account"
            ? await api.fetch.POST("/admin/accounts/{id}/refetch_statuses", { params: { path: { id: target.id } } })
            : await api.fetch.POST("/admin/instances/{domain}/refetch_statuses", {
                params: { path: { domain: target.domain } },
              });
      // The middleware throws an ApiError for every error status; this is only for the types.
      if (error !== undefined) throw new ApiError(error);
    },
  };
}

/**
 * What requesting a refetch changes on screen (web-client/README.md S1): the audit log, which records the request. The
 * posts change later, when the queued fetches run, so no list of posts is marked.
 */
export function cacheRefetchRequested(queryClient: QueryClient): Promise<void> {
  return refreshAuditLog(queryClient);
}
