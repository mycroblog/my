// SPDX-License-Identifier: AGPL-3.0-only
// The reader's drafts and scheduled posts kept on the server (ADR-0156): the list of both, each read with the ETag
// its next save sends (§9), its schedule set, moved or taken back, and the limits of the server (§7). Every write reads
// the list again. Saving a draft's content and publishing it are the composer's writes (compose.ts).
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";

/** A draft as the server keeps it: its content, when it was saved, its schedule and how the last one ended. */
export type ServerDraft = components["schemas"]["Draft"];

/** Why a scheduled post was not published at its time. */
export type DraftFailureReason = components["schemas"]["DraftFailureReason"];

/** The server's limits of drafts and scheduled posts. */
export type DraftLimits = components["schemas"]["InstanceDraftLimits"];

/** The limits until the instance's are read: the server's defaults. */
export const defaultDraftLimits: DraftLimits = {
  max_drafts: 100,
  max_scheduled_posts: 300,
  max_scheduled_posts_per_day: 25,
  scheduled_post_min_lead_minutes: 5,
  scheduled_post_max_lead_days: 366,
};

/** A draft as saved: its ID, the ETag the next save sends to replace it, and when it is scheduled, if it is. */
export type SavedDraft = { readonly id: string; readonly etag: string; readonly scheduledAt?: string | undefined };

/** A save refused because another device saved the draft since it was read (412). */
export class DraftConflictError extends Error {
  override readonly name = "DraftConflictError";
}

/**
 * A write refused because the draft is no longer on the server (404): deleted elsewhere, or published already, by its
 * schedule or by a publication whose answer was lost.
 */
export class DraftGoneError extends Error {
  override readonly name = "DraftGoneError";
}

/** What a draft's answer makes of it: its ID, its ETag and its schedule. */
export function savedOf(draft: ServerDraft | undefined, response: Response): SavedDraft {
  if (draft === undefined) throw new Error("app-data: the draft answered without a draft");
  return {
    id: draft.id,
    etag: response.headers.get("ETag") ?? "",
    ...(draft.scheduled_at === undefined ? {} : { scheduledAt: draft.scheduled_at }),
  };
}

/** The key of the lists of drafts, which every write marks stale. */
const draftsKey = ["get", "/drafts"] as const;

/** Reads the lists of drafts again. */
export function invalidateDrafts(data: AppData): Promise<void> {
  return data.queryClient.invalidateQueries({ queryKey: draftsKey });
}

/** The reader's drafts and scheduled posts, apart. */
export type DraftLists = {
  /** The drafts without a schedule, newest first. */
  readonly drafts: readonly ServerDraft[];
  /** The scheduled posts, soonest first. */
  readonly scheduled: readonly ServerDraft[];
};

/** Every page of the drafts that are scheduled, or of those that are not. */
async function listAll(data: AppData, scheduled: boolean): Promise<ServerDraft[]> {
  const items: ServerDraft[] = [];
  let cursor: string | undefined;
  do {
    const { data: page } = await data.api.GET("/drafts", {
      params: { query: { scheduled, limit: 40, ...(cursor === undefined ? {} : { cursor }) } },
    });
    if (page === undefined) throw new Error("app-data: the drafts answered without a page");
    items.push(...page.items);
    cursor = page.next_cursor;
  } while (cursor !== undefined);
  return items;
}

/** The query of the reader's drafts and scheduled posts. */
export function draftListsQuery(data: AppData) {
  return {
    queryKey: [...draftsKey, "lists"] as const,
    queryFn: async (): Promise<DraftLists> => {
      const [drafts, scheduled] = await Promise.all([listAll(data, false), listAll(data, true)]);
      return {
        drafts,
        scheduled: scheduled.toSorted((a, b) => Date.parse(a.scheduled_at ?? "") - Date.parse(b.scheduled_at ?? "")),
      };
    },
  };
}

/** Reads a draft of the reader's, with the ETag its next save sends. */
export async function readDraft(data: AppData, id: string): Promise<{ draft: ServerDraft; saved: SavedDraft }> {
  const { data: draft, response } = await data.api.GET("/drafts/{id}", { params: { path: { id } } });
  if (draft === undefined) throw new Error("app-data: the draft answered without a draft");
  return { draft, saved: savedOf(draft, response) };
}

/**
 * Schedules a draft to be published at `publishAt` (ISO 8601), or moves its schedule there; it rejects with the
 * problem of a time or a count past the server's limits, or of a post the server would refuse.
 */
export async function scheduleDraft(data: AppData, id: string, publishAt: string): Promise<SavedDraft> {
  try {
    const { data: draft, response } = await data.api.PUT("/drafts/{id}/schedule", {
      params: { path: { id } },
      body: { publish_at: publishAt },
    });
    return savedOf(draft, response);
  } finally {
    void invalidateDrafts(data);
  }
}

/** Takes a draft's schedule back: it stays, a draft again; one not scheduled (409) stays as it is. */
export async function unscheduleDraft(data: AppData, id: string): Promise<SavedDraft> {
  try {
    const { data: draft, response } = await data.api.DELETE("/drafts/{id}/schedule", { params: { path: { id } } });
    return savedOf(draft, response);
  } catch (e) {
    if (e instanceof ProblemError && e.status === 409) return (await readDraft(data, id)).saved;
    throw e;
  } finally {
    void invalidateDrafts(data);
  }
}

/** Deletes a draft with its schedule; one already gone is no failure. */
export async function deleteDraft(data: AppData, id: string): Promise<void> {
  try {
    await data.api.DELETE("/drafts/{id}", { params: { path: { id } } });
  } catch (e) {
    if (!(e instanceof ProblemError && e.status === 404)) throw e;
  } finally {
    void invalidateDrafts(data);
  }
}

export type Drafts = {
  /** The lists, once read. */
  readonly lists: DraftLists | undefined;
  /** The lists could not be read. */
  readonly failed: boolean;
  readonly retry: () => void;
  readonly unschedule: (id: string) => Promise<void>;
  readonly remove: (id: string) => Promise<void>;
};

/** The reader's drafts and scheduled posts, with what their list does to them. */
export function useDrafts(): Drafts {
  const data = useAppData();
  const query = useQuery(draftListsQuery(data));
  return {
    lists: query.data,
    failed: query.isError,
    retry: () => {
      void query.refetch();
    },
    unschedule: async (id) => {
      await unscheduleDraft(data, id);
    },
    remove: (id) => deleteDraft(data, id),
  };
}
