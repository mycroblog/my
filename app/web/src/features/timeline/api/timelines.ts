// SPDX-License-Identifier: AGPL-3.0-only
// The queries of the timeline feature (web-client/README.md S1). A timeline is an infinite query of pages that
// follow each other by `next_cursor` (TanStack Query, "Infinite Queries"). openapi-react-query has a hook
// for infinite queries but no options factory, so the options are built here with infiniteQueryOptions
// over the openapi-fetch client; the key follows openapi-react-query's `[method, path, init]` and ends with
// "pages", so that it never collides with a plain query of the same operation (an infinite query caches
// another shape of data).
import { type InfiniteData, infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import type { components } from "@mycroblog/mykit";

/** A status (post or boost), as the client API returns it. */
export type Status = components["schemas"]["Status"];

/** Who may read a public timeline: nobody (it is a 404), the signed-in users (a 401 for visitors), or everyone (FR-202). */
export type TimelineAccess = components["schemas"]["TimelineAccess"];

/** A page of a timeline. */
export type StatusPage = components["schemas"]["StatusPage"];

/**
 * A page of any timeline as its query caches it: the first page of a remote account's posts may also tell
 * that it started fetching the account's recent posts from its server (AccountStatusPage, ADR-0044).
 */
export type TimelinePage = components["schemas"]["AccountStatusPage"];

/**
 * Which posts of an account its profile lists (FR-204): its posts and boosts without replies (`posts`),
 * with its replies too (`replies`), or its own posts only, without boosts (`no_boosts`).
 */
export const accountTimelineFilters = ["posts", "replies", "no_boosts"] as const;

export type AccountTimelineFilter = (typeof accountTimelineFilters)[number];

/** Which timeline to show; the route puts it in the URL (web-client/README.md S3). */
export type Timeline =
  | { kind: "home" }
  | { kind: "local" }
  /** The federated timeline (GET /timelines/public): public posts of this server and of the others it knows. */
  | { kind: "public" }
  | { kind: "tag"; tag: string }
  /** The posts the viewer bookmarked (GET /bookmarks, FR-113), newest bookmark first. */
  | { kind: "bookmarks" }
  /** The posts that quote the viewer's post `id` (GET /statuses/{id}/quotes, FR-114), newest first. */
  | { kind: "quotes"; id: string }
  /** The posts of the account `id` on its profile. */
  | { kind: "account"; id: string; filter: AccountTimelineFilter };

/** The query of GET /accounts/{id}/statuses for a filter (without the cursor). */
function accountTimelineQuery(filter: AccountTimelineFilter) {
  switch (filter) {
    case "posts":
      return { exclude_replies: true };
    case "replies":
      return {};
    case "no_boosts":
      return { exclude_replies: true, exclude_boosts: true };
  }
}

/** The query key of a timeline. */
function timelineQueryKey(timeline: Timeline) {
  switch (timeline.kind) {
    case "home":
      return ["get", "/timelines/home", "pages"] as const;
    case "local":
      return ["get", "/timelines/local", "pages"] as const;
    case "public":
      return ["get", "/timelines/public", "pages"] as const;
    case "tag":
      return ["get", "/timelines/tag/{hashtag}", { params: { path: { hashtag: timeline.tag } } }, "pages"] as const;
    case "bookmarks":
      return ["get", "/bookmarks", "pages"] as const;
    case "quotes":
      return ["get", "/statuses/{id}/quotes", { params: { path: { id: timeline.id } } }, "pages"] as const;
    case "account":
      return [
        "get",
        "/accounts/{id}/statuses",
        { params: { path: { id: timeline.id }, query: accountTimelineQuery(timeline.filter) } },
        "pages",
      ] as const;
  }
}

/** Fetches one page of `timeline`; `cursor` is the previous page's `next_cursor`, or undefined for the first. */
async function fetchTimelinePage(
  api: Api,
  timeline: Timeline,
  cursor: string | undefined,
  signal: AbortSignal,
): Promise<TimelinePage> {
  const query = cursor === undefined ? {} : { cursor };
  const { data, error } = await (() => {
    switch (timeline.kind) {
      case "home":
        return api.fetch.GET("/timelines/home", { params: { query }, signal });
      case "local":
        return api.fetch.GET("/timelines/local", { params: { query }, signal });
      case "public":
        return api.fetch.GET("/timelines/public", { params: { query }, signal });
      case "tag":
        return api.fetch.GET("/timelines/tag/{hashtag}", {
          params: { path: { hashtag: timeline.tag }, query },
          signal,
        });
      case "bookmarks":
        return api.fetch.GET("/bookmarks", { params: { query }, signal });
      case "quotes":
        return api.fetch.GET("/statuses/{id}/quotes", { params: { path: { id: timeline.id }, query }, signal });
      case "account":
        return api.fetch.GET("/accounts/{id}/statuses", {
          params: { path: { id: timeline.id }, query: { ...accountTimelineQuery(timeline.filter), ...query } },
          signal,
        });
    }
  })();
  // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
  if (data === undefined) throw new ApiError(error);
  return data;
}

/**
 * The instance's public information, for which public timelines it serves (`timelines`). The key is
 * openapi-react-query's, the same as every other reader of GET /instance, so they share one answer.
 */
export function instanceQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/instance");
}

/** The pages of a timeline, newest first; the last page has no `next_cursor`. */
export function timelineQueryOptions(api: Api, timeline: Timeline) {
  // eslint-disable-next-line @tanstack/query/exhaustive-deps -- the app has one API client (lib/services.ts); the openapi-react-query keys leave it out too.
  return infiniteQueryOptions({
    queryKey: timelineQueryKey(timeline),
    queryFn: ({ pageParam, signal }) => fetchTimelinePage(api, timeline, pageParam, signal),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) => lastPage.next_cursor,
    // A failed page is shown at once with a button to try again (the route's error for the first page,
    // the end of the feed for an older one), instead of after seconds of silent retries.
    retry: false,
    // A refetch while the reader is in the timeline would insert new posts at its top (web-client/README.md
    // Y7). The home timeline's news wait behind its "N new posts" button (newStatuses.ts); the timelines
    // refetch when the reader opens them again.
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
}

/** Whether a timeline's first page started fetching its account's recent posts from the account's server (ADR-0044). */
export function remoteFetchStarted(data: InfiniteData<TimelinePage>): boolean {
  return data.pages[0]?.remote_fetch_started === true;
}

/**
 * Reads the posts of a remote account again once its recent posts had time to arrive from its server
 * (ADR-0044): every cached list of the account's posts is marked stale and the one shown is fetched again
 * (S1). Then the shown timeline's first page forgets that it started the fetch, whether the read succeeded or
 * not, so that the page is read again only once and a failure is not told. Resolves once that is done.
 */
export async function rereadRemoteAccountPosts(
  queryClient: QueryClient,
  api: Api,
  timeline: Extract<Timeline, { kind: "account" }>,
): Promise<void> {
  // A partial key matches the account's posts with any filter (TanStack Query, "Query Filters").
  await queryClient.invalidateQueries({
    queryKey: ["get", "/accounts/{id}/statuses", { params: { path: { id: timeline.id } } }],
  });
  queryClient.setQueryData(timelineQueryOptions(api, timeline).queryKey, (data) => {
    const first = data?.pages[0];
    if (data === undefined || first?.remote_fetch_started !== true) return data;
    const page: TimelinePage = { ...first };
    delete page.remote_fetch_started;
    return { ...data, pages: [page, ...data.pages.slice(1)] };
  });
}
