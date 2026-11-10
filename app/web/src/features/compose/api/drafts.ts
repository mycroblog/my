// SPDX-License-Identifier: AGPL-3.0-only
// The drafts the user saves on the server (ADR-0156): the two lists of the drafts screen (scheduled and not), one
// draft with the ETag of its next save, and the writes (save, schedule, cancel the schedule, delete). The compose
// form and the drafts screen (features/drafts, through the feature's index.ts) share them. Types come from the spec (S9).
import { infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import type { Status } from "./compose";

export type ServerDraft = components["schemas"]["Draft"];
export type DraftRequest = components["schemas"]["DraftRequest"];

/** A draft as the server last answered it, with the `ETag` that the next `PUT /drafts/{id}` sends as `If-Match`. */
export type DraftVersion = { draft: ServerDraft; etag: string };

/** The page size of the lists, the API's default maximum. */
const pageSize = 40;

/** The prefix of the keys of both lists, which every write marks stale. */
const draftListsKey = ["get", "/drafts", "pages"] as const;

/** The scheduled drafts (`scheduled`) or the others, newest first (GET /drafts). */
export function draftsQueryOptions(api: Api, scheduled: boolean) {
  return infiniteQueryOptions({
    queryKey: [...draftListsKey, { scheduled }] as const,
    queryFn: async ({ pageParam, signal }) => {
      const query =
        pageParam === undefined ? { limit: pageSize, scheduled } : { limit: pageSize, scheduled, cursor: pageParam };
      const { data, error } = await api.fetch.GET("/drafts", { params: { query }, signal });
      // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** The `ETag` of a draft's answer; the server sends one with every draft. */
function etagOf(response: Response): string {
  const etag = response.headers.get("ETag");
  if (etag === null) throw new Error("the draft answered without an ETag");
  return etag;
}

/** Reads one draft with its ETag, to edit it (GET /drafts/{id}). */
export async function getDraft(api: Api, id: string): Promise<DraftVersion> {
  const { data, error, response } = await api.fetch.GET("/drafts/{id}", { params: { path: { id } } });
  if (data === undefined) throw new ApiError(error);
  return { draft: data, etag: etagOf(response) };
}

/** Saves a new draft (POST /drafts). */
export async function createDraft(api: Api, body: DraftRequest): Promise<DraftVersion> {
  const { data, error, response } = await api.fetch.POST("/drafts", { body });
  if (data === undefined) throw new ApiError(error);
  return { draft: data, etag: etagOf(response) };
}

/**
 * Replaces the content of the draft `id` as it was when its `etag` was read (PUT /drafts/{id}); a draft that another
 * save changed since is a 412 (`isDraftConflict`).
 */
export async function updateDraft(api: Api, id: string, etag: string, body: DraftRequest): Promise<DraftVersion> {
  const { data, error, response } = await api.fetch.PUT("/drafts/{id}", {
    params: { path: { id }, header: { "If-Match": etag } },
    body,
  });
  if (data === undefined) throw new ApiError(error);
  return { draft: data, etag: etagOf(response) };
}

/** Whether `error` says that another save changed the draft since it was read. */
export function isDraftConflict(error: unknown): boolean {
  return error instanceof ApiError && error.status === 412;
}

/** Whether `error` says that the draft is gone (deleted, or published, on another device). */
export function isDraftGone(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404;
}

/** Schedules the draft `id` for `publishAt`, or moves its schedule there (PUT /drafts/{id}/schedule). */
export async function scheduleDraft(api: Api, id: string, publishAt: Date): Promise<ServerDraft> {
  const { data, error } = await api.fetch.PUT("/drafts/{id}/schedule", {
    params: { path: { id } },
    body: { publish_at: publishAt.toISOString() },
  });
  if (data === undefined) throw new ApiError(error);
  return data;
}

/** Cancels the schedule of the draft `id`, which stays as a draft (DELETE /drafts/{id}/schedule). */
export async function unscheduleDraft(api: Api, id: string): Promise<ServerDraft> {
  const { data, error } = await api.fetch.DELETE("/drafts/{id}/schedule", { params: { path: { id } } });
  if (data === undefined) throw new ApiError(error);
  return data;
}

/**
 * Publishes the draft `id` as it is saved now (POST /drafts/{id}/publish): the server posts it with the checks of a new
 * post and deletes the draft with its schedule in the same transaction, so it is published once. A draft that cannot
 * be published stays as it was (a 422 with the problem of a new post, such as the automated review's refusal).
 */
export async function publishDraft(api: Api, id: string): Promise<Status> {
  const { data, error } = await api.fetch.POST("/drafts/{id}/publish", { params: { path: { id } } });
  if (data === undefined) throw new ApiError(error);
  return data;
}

/** Deletes the draft `id` with its schedule (DELETE /drafts/{id}). A draft already gone is no failure. */
export async function deleteDraft(api: Api, id: string): Promise<void> {
  try {
    await api.fetch.DELETE("/drafts/{id}", { params: { path: { id } } });
  } catch (error) {
    if (!isDraftGone(error)) throw error;
  }
}

/**
 * Marks both lists of drafts stale after a write (web-client/README.md S1); resolves once those on screen are fetched
 * again. Which list a draft is in, and where, changes with every save and schedule.
 */
export async function cacheDraftChange(queryClient: QueryClient): Promise<void> {
  await queryClient.invalidateQueries({ queryKey: draftListsKey });
}
