// SPDX-License-Identifier: AGPL-3.0-only
// The mute of a post's thread (ADR-0158 §8): it stops the viewer's notifications of the thread, and the posts stay in
// every list. `Status.muted` tells whether the viewer mutes the thread of a post, so the answer goes into the copies of
// the post and of the posts of its thread that the cache holds (web-client/README.md S1).
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { updateCachedStatuses } from "./cache";
import type { Status } from "./status";

/**
 * Mutes the thread of post `id`, or unmutes it (POST or DELETE /statuses/{id}/mute); returns the post after it. Both
 * are setters on the server, so a retry after a lost answer is safe.
 */
export async function setThreadMute(api: Api, id: string, on: boolean): Promise<Status> {
  const init = { params: { path: { id } } };
  const { data, error } = await (on
    ? api.fetch.POST("/statuses/{id}/mute", init)
    : api.fetch.DELETE("/statuses/{id}/mute", init));
  // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
  if (data === undefined) throw new ApiError(error);
  return data;
}

type Context = { ancestors: readonly Status[]; descendants: readonly Status[] };

function isContext(data: unknown): data is Context {
  return (
    typeof data === "object" &&
    data !== null &&
    Array.isArray((data as Partial<Context>).ancestors) &&
    Array.isArray((data as Partial<Context>).descendants)
  );
}

/**
 * The IDs of the posts in the thread of post `id` that the cache knows: the post, and every post of a cached thread
 * (GET /statuses/{id}/context) that holds it, the post the thread is around included. The API names no thread on a
 * post, so the threads read are what tells which posts share it.
 */
function cachedThreadIds(queryClient: QueryClient, id: string): ReadonlySet<string> {
  const ids = new Set([id]);
  for (const [queryKey, data] of queryClient.getQueriesData({
    predicate: (query) => query.queryKey[0] === "get" && query.queryKey[1] === "/statuses/{id}/context",
  })) {
    if (!isContext(data)) continue;
    const thread = [...data.ancestors, ...data.descendants].map((status) => status.id);
    const init = queryKey[2] as { params?: { path?: { id?: unknown } } } | undefined;
    const around = init?.params?.path?.id;
    if (typeof around === "string") thread.push(around);
    if (thread.includes(id)) for (const member of thread) ids.add(member);
  }
  return ids;
}

/**
 * Puts the server's answer to a mute or an unmute of the thread of post `id` into the cache: every copy of the post
 * and of the posts the cache knows in its thread takes the answer's `muted`. No list changes, since the posts stay.
 */
export function cacheThreadMuteAnswer(queryClient: QueryClient, id: string, answer: Status): void {
  const muted = answer.muted === true;
  const thread = cachedThreadIds(queryClient, id);
  updateCachedStatuses(queryClient, (status) =>
    thread.has(status.id) && status.muted !== muted ? { ...status, muted } : status,
  );
}
