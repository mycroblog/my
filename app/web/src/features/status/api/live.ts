// SPDX-License-Identifier: AGPL-3.0-only
// What a notification from the event stream changes in the cached posts (web-client/README.md S10): someone
// favourited, reacted to, boosted, replied to, or quoted one of the viewer's posts, so its counts on screen are old.
// When a cached answer holds the post (itself or inside a boost), it is read again (GET /statuses/{id}) and the
// answer's counts are written into every cached copy (S8). A burst of notifications about one post reads it once, and
// the viewer's own change of the post on its way is not undone. A read that fails changes nothing and says nothing: a
// later notification or read of the post fixes it.
import type { QueryClient } from "@tanstack/react-query";
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";
import { forPost, updateCachedStatuses, updateStatuses, withLikeSlotFrom, withRelationFrom } from "./cache";
import type { Status } from "./status";

type Notification = components["schemas"]["Notification"];

/** How long the notifications about one post are gathered before it is read, in ms. */
export const postRefreshWindow = 1_000;

/** The reads of one QueryClient's posts: the ones waiting for their window to end or on their way. */
type PostRead = { timer: ReturnType<typeof setTimeout> | undefined; inFlight: boolean; again: boolean };

const reads = new WeakMap<QueryClient, Map<string, PostRead>>();

/** The post whose counts a notification changes, or undefined for a notification that changes none. */
function changedPostId(notification: Notification): string | undefined {
  const { status } = notification;
  if (status === undefined) return undefined;
  switch (notification.type) {
    case "favourite":
    case "reaction":
    case "boost":
      return status.id;
    case "mention":
      return status.in_reply_to_id;
    case "quote":
      return status.quote?.quoted_status?.id;
    default:
      return undefined;
  }
}

/** Whether a cached answer of the API holds post `id`, itself or inside a boost. */
function isCached(queryClient: QueryClient, id: string): boolean {
  const seen = { found: false };
  const probe = forPost(id, (post) => {
    seen.found = true;
    return post;
  });
  return queryClient
    .getQueryCache()
    .findAll({ predicate: (query) => query.queryKey[0] === "get" && query.state.data !== undefined })
    .some((query) => {
      updateStatuses(query.state.data, probe);
      return seen.found;
    });
}

/**
 * The viewer's changes of post `id` that the answer of a read started at `since` may not have seen: those on their
 * way, and those started after the read (the mutation keys of useLikeSlot and useStatusActions).
 */
function changedSince(queryClient: QueryClient, id: string, since: number): Set<string> {
  const kinds = new Set<string>();
  for (const mutation of queryClient.getMutationCache().findAll({ mutationKey: ["status", id] })) {
    const kind = mutation.options.mutationKey?.[2];
    if (typeof kind !== "string") continue;
    if (mutation.state.status === "pending" || mutation.state.submittedAt >= since) kinds.add(kind);
  }
  return kinds;
}

/**
 * Copies the counts of `from` into `post`, with the viewer's favourite, reaction, boost, and bookmark, except the ones
 * in `kept`, whose change by the viewer is on its way or newer than `from`: those stay as the cache has them.
 */
function withCountsFrom(post: Status, from: Status, kept: Set<string>): Status {
  let next: Status = { ...post, replies_count: from.replies_count };
  if (!kept.has("like-slot") && !kept.has("favourite")) next = withLikeSlotFrom(next, from);
  if (!kept.has("boost")) next = withRelationFrom(next, "boost", from);
  if (!kept.has("bookmark")) next = withRelationFrom(next, "bookmark", from);
  return next;
}

async function readPost(queryClient: QueryClient, api: Api, id: string): Promise<void> {
  const since = Date.now();
  const { data } = await api.fetch.GET("/statuses/{id}", { params: { path: { id } } });
  if (data === undefined) return;
  const kept = changedSince(queryClient, id, since);
  updateCachedStatuses(
    queryClient,
    forPost(id, (post) => withCountsFrom(post, data, kept)),
  );
}

function start(queryClient: QueryClient, api: Api, pending: Map<string, PostRead>, id: string, read: PostRead): void {
  read.timer = undefined;
  read.inFlight = true;
  readPost(queryClient, api, id)
    .catch(() => {
      // A failed or missing post (404) changes nothing; a later notification or read of the post fixes it.
    })
    .finally(() => {
      read.inFlight = false;
      if (read.again) {
        read.again = false;
        schedule(queryClient, api, pending, id, read);
      } else {
        pending.delete(id);
      }
    });
}

function schedule(queryClient: QueryClient, api: Api, pending: Map<string, PostRead>, id: string, read: PostRead) {
  read.timer = setTimeout(() => {
    start(queryClient, api, pending, id, read);
  }, postRefreshWindow);
}

/**
 * A notification from the stream: reads again the post whose counts it changes, when a cached answer holds it, and
 * writes the new counts into every cached copy. Notifications about one post within `postRefreshWindow` read it
 * once; one that comes while the post is being read reads it once more after.
 */
export function receivePostNotification(queryClient: QueryClient, api: Api, notification: Notification): void {
  const id = changedPostId(notification);
  if (id === undefined || !isCached(queryClient, id)) return;
  let pending = reads.get(queryClient);
  if (pending === undefined) {
    pending = new Map();
    reads.set(queryClient, pending);
  }
  const read = pending.get(id);
  if (read === undefined) {
    const next: PostRead = { timer: undefined, inFlight: false, again: false };
    pending.set(id, next);
    schedule(queryClient, api, pending, id, next);
    return;
  }
  // A read waiting for its window takes this notification too; one on its way is followed by one more.
  if (read.inFlight) read.again = true;
}
