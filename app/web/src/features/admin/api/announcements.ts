// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the announcements screen (FR-726, manage_announcements, web-client/README.md S1): every
// announcement with its Markdown and state (GET /admin/announcements), writing one as a draft or published at
// once, revising its text, publishing a draft, and withdrawing one for good. Every change goes to the audit log,
// and the users' list (GET /announcements) follows the published ones.
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { refreshAuditLog } from "../../moderation";

type Schemas = components["schemas"];

export type AdminAnnouncement = Schemas["AdminAnnouncement"];
export type AnnouncementState = Schemas["AdminAnnouncementState"];
export type NewAnnouncement = Schemas["AdminAnnouncementCreateRequest"];
export type AnnouncementInput = Schemas["AdminAnnouncementInput"];

/** The most characters of an announcement's text (AdminAnnouncementInput). */
export const announcementTextLimit = 5000;

/** Every announcement, newest first (GET /admin/announcements). */
export function adminAnnouncementsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/admin/announcements");
}

/** Writes an announcement (POST /admin/announcements), as the options of useMutation. */
export function createAnnouncementMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/announcements"] as const,
    mutationFn: async (body: NewAnnouncement): Promise<AdminAnnouncement> => {
      const { data, error } = await api.fetch.POST("/admin/announcements", { body });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Replaces the text and window of an announcement (PUT /admin/announcements/{id}), as the options of useMutation. */
export function updateAnnouncementMutation(api: Api) {
  return {
    mutationKey: ["put", "/admin/announcements/{id}"] as const,
    mutationFn: async ({ id, body }: { id: string; body: AnnouncementInput }): Promise<AdminAnnouncement> => {
      const { data, error } = await api.fetch.PUT("/admin/announcements/{id}", { params: { path: { id } }, body });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Publishes a draft (POST /admin/announcements/{id}/publish). */
export function publishAnnouncementMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/announcements/{id}/publish"] as const,
    mutationFn: async (id: string): Promise<AdminAnnouncement> => {
      const { data, error } = await api.fetch.POST("/admin/announcements/{id}/publish", { params: { path: { id } } });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Withdraws an announcement for good (POST /admin/announcements/{id}/withdraw). */
export function withdrawAnnouncementMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/announcements/{id}/withdraw"] as const,
    mutationFn: async (id: string): Promise<AdminAnnouncement> => {
      const { data, error } = await api.fetch.POST("/admin/announcements/{id}/withdraw", { params: { path: { id } } });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * Marks what a change of the announcements changes (ADR-0045): the admin list, the users' list and the audit log;
 * the queries shown now are fetched again and awaited.
 */
export async function refreshAnnouncements(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/announcements"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/announcements"] }),
    refreshAuditLog(queryClient),
  ]);
}
