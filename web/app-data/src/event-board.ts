// SPDX-License-Identifier: AGPL-3.0-only
// The board of an event (ADR-0161 §17): its posts, newest first, a page per cursor, for a reader who reads it; and a
// post on it, which goes to the board's top at once and nowhere else (no timeline holds a board's post). Its posts are
// statuses of the API, which every other write (edit, delete, favourite, react, vote) reaches as any status's, the
// shadow showing them over the board's pages too (statuses.ts). A post the server refuses because the board changed
// under the reader (read-only now, or no longer theirs to write on) reads the event again, so that the page shows it.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useInfiniteQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import type { ServerEvent } from "./around";
import { eventQuery } from "./events";
import { pagedQuery } from "./paged";
import { useAppData } from "./provider";
import { cachedStatus, shadowStatus, showList, useShadow } from "./shadow";
import type { Status } from "./statuses";
import { cacheReply } from "./thread";
import type { HomeTimeline } from "./timeline";

/** A new post on an event's board: what a new status has but a visibility and a quote. */
export type EventBoardPost = components["schemas"]["EventBoardPostCreate"];

/** Who writes on an event's board, among those who read it. */
export type EventBoardWriters = components["schemas"]["EventBoardWriters"];

/** The choices of who writes on a board, from the fewest people. */
export const eventBoardWriters = ["organizer", "organizers", "readers"] as const satisfies readonly EventBoardWriters[];

/** How long after an event's end its board takes posts: 7 days, as the server counts them. */
const boardOpenMs = 7 * 24 * 60 * 60_000;

/** When the board of `event` turns read-only by its period (ms since the epoch): 7 days after its end. */
export const eventBoardClosesAt = (event: { readonly ends_at: string }) => Date.parse(event.ends_at) + boardOpenMs;

/**
 * Whether the reader writes on the board of `event` at `now`: as the server last said, and until the 7 days after its
 * end pass, which a page left open sees as they pass.
 */
export const writesOnEventBoard = (event: ServerEvent, now: number) =>
  event.viewer.board.writable && now < eventBoardClosesAt(event);

/** The posts of the board of the event `id`, newest first, a page at a time. */
export function eventBoardQuery({ api, queries }: AppData, id: string) {
  return pagedQuery<Status>(
    queries.queryOptions("get", "/events/{id}/board", { params: { path: { id } } }).queryKey,
    async (cursor, signal) =>
      (
        await api.GET("/events/{id}/board", {
          params: { path: { id }, query: cursor === undefined ? {} : { cursor } },
          signal,
        })
      ).data,
  );
}

/**
 * The board of the event `id`, for its feed, in the shape of a timeline's column, each post as it shows (shadow.ts);
 * nothing is read while `enabled` is false.
 */
export function useEventBoard(id: string, enabled = true): HomeTimeline {
  const query = useInfiniteQuery({ ...eventBoardQuery(useAppData(), id), enabled });
  const show = useShadow();
  return {
    statuses: showList(show, query.data?.pages.flatMap((page) => page.items) ?? []),
    status: query.status,
    error: query.error,
    hasNextPage: query.hasNextPage,
    isFetchingNextPage: query.isFetchingNextPage,
    failedNextPage: query.isFetchNextPageError,
    fetchNextPage: () => {
      if (query.hasNextPage && !query.isFetchingNextPage) void query.fetchNextPage();
    },
    refetch: () => {
      void query.refetch();
    },
  };
}

/**
 * Puts the reader's new post at the top of the board's cached first page (after a fetch on its way, which would write
 * the pages as they were), and one more reply on the post it replies to, which its cached threads show.
 */
function cacheBoardPost(data: AppData, id: string, status: Status): void {
  const { queryKey } = eventBoardQuery(data, id);
  const query = data.queryClient.getQueryCache().find({ queryKey, exact: true });
  if (query?.state.fetchStatus === "fetching" && query.promise) {
    const after = () => {
      cacheBoardPost(data, id, status);
    };
    query.promise.then(after, after);
    return;
  }
  data.queryClient.setQueryData(queryKey, (cached) => {
    const [first, ...rest] = cached?.pages ?? [];
    if (!cached || !first || cached.pages.some((page) => page.items.some((s) => s.id === status.id))) return cached;
    return { ...cached, pages: [{ ...first, items: [status, ...first.items] }, ...rest] };
  });
  const parent = status.in_reply_to_id;
  if (parent === undefined) return;
  const shown = cachedStatus(data, parent);
  if (shown !== undefined) {
    const count = shown.replies_count + 1;
    shadowStatus(data, parent, (s) => ({ ...s, replies_count: Math.max(s.replies_count, count) }), "replies").confirm();
  }
  cacheReply(data.queryClient, status);
}

/** The problem types of a post refused because the board changed under the reader. */
const boardChanged = new Set([
  "/api/v1/problems/event-board-closed",
  "/api/v1/problems/event-board-not-writer",
  "/api/v1/problems/event-board-not-reader",
]);

/**
 * Posts `body` on the board of the event `id` with `idempotencyKey`, the same for every attempt of one post, so that a
 * retry after a lost answer posts it once; the post goes to the board's top at once. It rejects with the server's
 * refusal, and one that says the board changed reads the event again.
 */
export async function postOnEventBoard(
  data: AppData,
  id: string,
  body: EventBoardPost,
  idempotencyKey: string,
): Promise<Status> {
  try {
    const { data: status } = await data.api.POST("/events/{id}/board", {
      params: { path: { id }, header: { "Idempotency-Key": idempotencyKey } },
      body,
    });
    if (status === undefined) throw new Error("app-data: the board answered without a status");
    cacheBoardPost(data, id, status);
    return status;
  } catch (error) {
    if (error instanceof ProblemError && boardChanged.has(error.type))
      void data.queryClient.invalidateQueries({ queryKey: eventQuery(data, id).queryKey });
    throw error;
  }
}
