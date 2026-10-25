// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the service closure screen (FR-808, FR-809, ADR-0088, manage_service_closure,
// web-client/README.md S1): the closure's state with its schedule and, once closed, the progress of its `Delete`s
// (GET /admin/closure), the check of what closing ends (GET /admin/closure/check), announcing or changing the
// dates (PUT /admin/closure/announcement), withdrawing the announcement (DELETE), and closing the server for good
// (POST /admin/closure/close). Every change goes to the audit log, and the notice that every user sees follows
// GET /instance.
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { refreshAuditLog } from "../../moderation";

type Schemas = components["schemas"];

export type AdminServiceClosure = Schemas["AdminServiceClosure"];
export type AdminServiceClosureProgress = Schemas["AdminServiceClosureProgress"];
export type AnnounceServiceClosure = Schemas["AnnounceServiceClosureRequest"];

/** The most characters of the operator's message (AnnounceServiceClosureRequest). */
export const closureMessageLimit = 5000;

/** The days of notice below which the users may not have the time to export their data and move (ADR-0088 §1). */
export const shortNoticeDays = 30;

/** How often the progress is read while the deliveries go on, in milliseconds. */
export const progressIntervalMs = 10_000;

/** The state of the closure with its schedule and progress (GET /admin/closure). */
export function adminClosureQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/admin/closure");
}

/** What closing the server would end (GET /admin/closure/check). */
export function adminClosureCheckQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/admin/closure/check");
}

/** Announces the closure, or changes the dates of the announcement in effect (PUT /admin/closure/announcement). */
export function announceClosureMutation(api: Api) {
  return {
    mutationKey: ["put", "/admin/closure/announcement"] as const,
    mutationFn: async (body: AnnounceServiceClosure): Promise<AdminServiceClosure> => {
      const { data, error } = await api.fetch.PUT("/admin/closure/announcement", { body });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Withdraws the announcement in effect (DELETE /admin/closure/announcement). */
export function withdrawClosureMutation(api: Api) {
  return {
    mutationKey: ["delete", "/admin/closure/announcement"] as const,
    mutationFn: async (): Promise<AdminServiceClosure> => {
      const { data, error } = await api.fetch.DELETE("/admin/closure/announcement");
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Closes the server for good (POST /admin/closure/close); `confirmDomain` is the server's domain. */
export function closeServiceMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/closure/close"] as const,
    mutationFn: async (confirmDomain: string): Promise<AdminServiceClosure> => {
      const { data, error } = await api.fetch.POST("/admin/closure/close", { body: { confirm_domain: confirmDomain } });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * Marks what a change of the closure changes (ADR-0045): the screen's state and check, the instance whose
 * `closure` the notice shows, and the audit log; the queries shown now are fetched again and awaited.
 */
export async function refreshClosure(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/closure"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/closure/check"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/instance"] }),
    refreshAuditLog(queryClient),
  ]);
}
