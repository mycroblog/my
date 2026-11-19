// SPDX-License-Identifier: AGPL-3.0-only
// The server's announcements as a holder of `manage_announcements` writes them (FR-726, ADR-0125): every one, drafts
// and withdrawn ones included, newest first, a page at a time, kept to one state where the screen asks (draft,
// scheduled, showing, ended, withdrawn); one of them; and writing one (a draft, or published at once), revising its
// title, text and window, publishing a draft and withdrawing one for good. Every write reads again what it changes:
// the administrators' list and the announcement, and what the readers' screens read (the announcements they see now,
// their history and an announcement's page), so that a published one shows to the reader in this session too.
import { ProblemError, type components } from "@mycroblog/mykit";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { announcementHistoryQuery, announcementsQuery } from "./announcements";
import { useAppData } from "./provider";
import { readOf, type ScreenRead } from "./reads";

/** An announcement as the administrators write it, with its Markdown text and its state. */
export type AdminAnnouncement = components["schemas"]["AdminAnnouncement"];
/** Which announcements the administrators' list keeps. */
export type AdminAnnouncementFilter = components["schemas"]["AdminAnnouncementFilter"];
/** The title, the text and the window of an announcement. */
export type AdminAnnouncementInput = components["schemas"]["AdminAnnouncementInput"];

/** The filters of the list, in the order the screen offers them. */
export const announcementFilters = [
  "draft",
  "scheduled",
  "showing",
  "ended",
  "withdrawn",
] as const satisfies readonly AdminAnnouncementFilter[];

/** The rules of an announcement's fields, as the server checks them. */
export const announcementLimits = { title: 100, text: 5000 } as const;

/** Where an announcement stands now: a draft, waiting for its window, showing, over, or withdrawn. */
export type AnnouncementStanding = AdminAnnouncementFilter;

/** Where `a` stands at `now` (ms since the epoch), as the server's filters tell it. */
export function announcementStanding(a: AdminAnnouncement, now: number): AnnouncementStanding {
  if (a.state === "draft") return "draft";
  if (a.state === "withdrawn") return "withdrawn";
  if (a.ends_at !== undefined && Date.parse(a.ends_at) <= now) return "ended";
  if (a.starts_at !== undefined && Date.parse(a.starts_at) > now) return "scheduled";
  return "showing";
}

/** What the list shows: the announcements of one state, every one where left out, the page after `cursor`. */
export type AdminAnnouncementQuery = {
  readonly state?: AdminAnnouncementFilter | undefined;
  readonly cursor?: string | undefined;
};

/** A page of the administrators' list. */
export function adminAnnouncementsQuery({ queries }: AppData, q: AdminAnnouncementQuery) {
  return queries.queryOptions("get", "/admin/announcements", {
    params: {
      query: {
        ...(q.state === undefined ? {} : { state: q.state }),
        ...(q.cursor === undefined ? {} : { cursor: q.cursor }),
      },
    },
  });
}

/** One announcement as the administrators write it. */
export function adminAnnouncementQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/admin/announcements/{id}", { params: { path: { id } } });
}

/** Reads the page of the list `q` asks for, for a route's loader. */
export async function loadAdminAnnouncements(data: AppData, q: AdminAnnouncementQuery): Promise<void> {
  await data.queryClient.query(adminAnnouncementsQuery(data, q)).catch(() => undefined);
}

/** Reads the announcement of `id`, for a route's loader. */
export async function loadAdminAnnouncement(data: AppData, id: string): Promise<void> {
  await data.queryClient.query(adminAnnouncementQuery(data, id)).catch(() => undefined);
}

/** What a screen shows of a page of the list. */
export type AdminAnnouncementPage = ScreenRead & {
  /** The page's announcements; undefined until it is read. */
  readonly items: readonly AdminAnnouncement[] | undefined;
  /** The cursor of the next page; none on the last one. */
  readonly next: string | undefined;
  /** The page shows what the reader asked before while the one they asked for now is on its way. */
  readonly stale: boolean;
};

/** The page of the list `q` asks for; the page shown before stays while another is on its way. */
export function useAdminAnnouncements(q: AdminAnnouncementQuery): AdminAnnouncementPage {
  const query = useQuery({ ...adminAnnouncementsQuery(useAppData(), q), placeholderData: keepPreviousData });
  const cursor = query.data?.next_cursor;
  return {
    ...readOf([query]),
    items: query.data?.items,
    next: cursor === undefined || cursor === "" ? undefined : cursor,
    stale: query.isPlaceholderData,
  };
}

/** The announcement of `id`, and whether the server has none of it (404). */
export function useAdminAnnouncement(id: string): ScreenRead & {
  readonly announcement: AdminAnnouncement | undefined;
  readonly missing: boolean;
} {
  const query = useQuery(adminAnnouncementQuery(useAppData(), id));
  const missing = query.error instanceof ProblemError && query.error.status === 404;
  return { ...readOf([query]), announcement: query.data, missing };
}

/** Whether `error` says the announcement was withdrawn, so that it can no longer change (409). */
export function announcementWithdrawn(error: unknown): boolean {
  return error instanceof ProblemError && error.status === 409;
}

/** The writes of announcements; each rejects with the server's problem (a 422 names `title`, `text` or `ends_at`). */
export type AdminAnnouncementWrites = {
  /** Writes a new announcement, a draft unless `publish`, and resolves with it. */
  readonly create: (input: AdminAnnouncementInput, publish: boolean) => Promise<AdminAnnouncement>;
  /** Replaces the title, the text and the window of the announcement of `id`, which keeps its state. */
  readonly revise: (id: string, input: AdminAnnouncementInput) => Promise<AdminAnnouncement>;
  /** Publishes the draft of `id` to every local user, within its window. */
  readonly publish: (id: string) => Promise<AdminAnnouncement>;
  /** Withdraws the announcement of `id` for good. */
  readonly withdraw: (id: string) => Promise<AdminAnnouncement>;
};

/** The input the API takes: a window side left empty is none. */
const inputOf = (input: AdminAnnouncementInput): AdminAnnouncementInput => ({
  title: input.title,
  text: input.text,
  ...(input.starts_at === undefined || input.starts_at === "" ? {} : { starts_at: input.starts_at }),
  ...(input.ends_at === undefined || input.ends_at === "" ? {} : { ends_at: input.ends_at }),
});

/**
 * Puts the announcement the server answered into the cache, and reads again what it changes: the administrators'
 * lists, and the readers' announcements, their history and the reader's page of it.
 */
async function written(data: AppData, a: AdminAnnouncement | undefined): Promise<AdminAnnouncement> {
  if (a === undefined) throw new Error("app-data: a write of an announcement answered without it");
  const { queryClient } = data;
  const one = adminAnnouncementQuery(data, a.id).queryKey;
  await queryClient.cancelQueries({ queryKey: one });
  queryClient.setQueryData(one, a);
  void queryClient.invalidateQueries({ predicate: ({ queryKey }) => queryKey[1] === "/admin/announcements" });
  void queryClient.invalidateQueries({ queryKey: announcementsQuery(data).queryKey });
  void queryClient.invalidateQueries({ queryKey: announcementHistoryQuery(data).queryKey });
  void queryClient.invalidateQueries({ predicate: ({ queryKey }) => queryKey[1] === "/announcements/{id}" });
  return a;
}

/** The writes of announcements, for `data`. */
export function adminAnnouncementWrites(data: AppData): AdminAnnouncementWrites {
  const path = (id: string) => ({ params: { path: { id } } });
  return {
    create: async (input, publish) =>
      written(data, (await data.api.POST("/admin/announcements", { body: { ...inputOf(input), publish } })).data),
    revise: async (id, input) =>
      written(data, (await data.api.PUT("/admin/announcements/{id}", { ...path(id), body: inputOf(input) })).data),
    publish: async (id) => written(data, (await data.api.POST("/admin/announcements/{id}/publish", path(id))).data),
    withdraw: async (id) => written(data, (await data.api.POST("/admin/announcements/{id}/withdraw", path(id))).data),
  };
}

/** The writes of announcements, for a screen. */
export function useAdminAnnouncementWrites(): AdminAnnouncementWrites {
  return adminAnnouncementWrites(useAppData());
}
