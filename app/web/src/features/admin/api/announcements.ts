// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the announcements screen (FR-726, manage_announcements, web-client/README.md S1): the
// announcements with their Markdown and state, a page at a time and kept by where they are in their life
// (GET /admin/announcements), one of them (GET /admin/announcements/{id}), writing one as a draft or published at
// once, revising its text, publishing a draft, and withdrawing one for good. Every change goes to the audit log, and
// the users' list (GET /announcements) follows the published ones.
import { infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { pageQuery, refreshAuditLog } from "../../moderation";

type Schemas = components["schemas"];

export type AdminAnnouncement = Schemas["AdminAnnouncement"];
export type NewAnnouncement = Schemas["AdminAnnouncementCreateRequest"];
export type AnnouncementInput = Schemas["AdminAnnouncementInput"];
type AnnouncementPage = Schemas["AdminAnnouncementPage"];

/** Where an announcement is in its life: its state, and for a published one, whether its window is ahead, now or past. */
export type AnnouncementPhase = Schemas["AdminAnnouncementFilter"];

/** The lists of the announcements screen: every announcement, or those in one phase (the `state` of the URL). */
export type AnnouncementFilter = "all" | AnnouncementPhase;

/** The lists in the order of the filter. */
export const announcementFilters: readonly AnnouncementFilter[] = [
  "all",
  "draft",
  "scheduled",
  "showing",
  "ended",
  "withdrawn",
];

/** The most characters of an announcement's text (AdminAnnouncementInput). */
export const announcementTextLimit = 5000;

/** The most grapheme clusters of an announcement's title (AdminAnnouncementInput, ADR-0125 §1). */
export const announcementTitleLimit = 100;

/** The announcements of a list, newest first, a page at a time (GET /admin/announcements). */
export function adminAnnouncementsQueryOptions(api: Api, filter: AnnouncementFilter) {
  return infiniteQueryOptions({
    queryKey: ["get", "/admin/announcements", { filter }, "pages"] as const,
    queryFn: async ({ pageParam, signal }): Promise<AnnouncementPage> => {
      const query = filter === "all" ? pageQuery(pageParam) : { ...pageQuery(pageParam), state: filter };
      const { data, error } = await api.fetch.GET("/admin/announcements", { params: { query }, signal });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** One announcement in any state (GET /admin/announcements/{id}). */
export function adminAnnouncementQueryOptions(api: Api, id: string) {
  return api.query.queryOptions("get", "/admin/announcements/{id}", { params: { path: { id } } });
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
 * Marks what a change of the announcements changes (ADR-0028): the admin lists and pages, the users' list and the
 * audit log; the queries shown now are fetched again and awaited.
 */
export async function refreshAnnouncements(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/announcements"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/announcements/{id}"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/announcements"] }),
    refreshAuditLog(queryClient),
  ]);
}
