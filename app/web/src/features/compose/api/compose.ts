// SPDX-License-Identifier: AGPL-3.0-only
// The server data of the compose feature (web-client/README.md S1): the limits and preferences a new post
// follows, the post being replied to, the uploaded media while the server processes them, the accounts
// that mentions name and the viewer's relationships with them, and the writes (uploading a file,
// posting). Types come from the spec (S9).
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { isPostListQuery } from "../../../lib/query";

export type Status = components["schemas"]["Status"];
export type MediaAttachment = components["schemas"]["MediaAttachment"];
export type Visibility = components["schemas"]["Visibility"];
/** Who may quote a post (FR-114, ADR-0069). */
export type QuotePolicy = components["schemas"]["QuotePolicy"];
export type CreateStatusRequest = components["schemas"]["CreateStatusRequest"];
export type PostingLimits = components["schemas"]["PostingLimits"];

/** The instance, for `limits.max_post_characters`. */
export function instanceQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/instance");
}

/** The viewer's preferences: the default visibility and language, and whether media need a description. */
export function preferencesQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/preferences");
}

/** A status, for the post being replied to. */
export function statusQueryOptions(api: Api, id: string) {
  return api.query.queryOptions("get", "/statuses/{id}", { params: { path: { id } } });
}

/** Puts the post being replied to, as the screen that offered the reply shows it, into the cache: the form names it without fetching it. */
export function cacheReplyTarget(api: Api, queryClient: QueryClient, status: Status): void {
  queryClient.setQueryData(statusQueryOptions(api, status.id).queryKey, status);
}

/** How often an uploaded file is polled while the server processes it, in ms. */
const mediaPollInterval = 1000;

/**
 * An uploaded media file. While it is `processing`, the query polls it until it is `ready` or `failed`
 * (the uploadMedia operation's contract).
 */
export function mediaQueryOptions(api: Api, id: string) {
  return api.query.queryOptions(
    "get",
    "/media/{id}",
    { params: { path: { id } } },
    { refetchInterval: (query) => (query.state.data?.state === "processing" ? mediaPollInterval : false) },
  );
}

/** Caches an upload's answer, so that its query starts from it (and polls only while it is processing). */
export function cacheUploadedMedia(api: Api, queryClient: QueryClient, media: MediaAttachment): void {
  queryClient.setQueryData(mediaQueryOptions(api, media.id).queryKey, media);
}

/**
 * Uploads one image, video, or audio file to attach to a post (POST /media, 202) and returns the file in its current
 * state, usually `processing`. A video or audio file may come with its poster, sent as the `thumbnail`.
 */
export async function uploadMedia(api: Api, file: File, poster: Blob | null = null): Promise<MediaAttachment> {
  const { data } = await api.fetch.POST("/media", {
    // The body is serialized by hand: openapi-typescript types a binary part as a string.
    body: { file: file.name, purpose: "attachment" },
    bodySerializer() {
      const form = new FormData();
      form.append("file", file, file.name);
      form.append("purpose", "attachment");
      if (poster !== null) form.append("thumbnail", poster, "poster.jpg");
      return form;
    },
  });
  // The client's middleware throws for every error status, so a response without data cannot happen.
  if (data === undefined) throw new Error("POST /media answered without a body");
  return data;
}

/**
 * Posts a status. `idempotencyKey` stays the same for every retry of one draft, so a retry after a lost
 * response returns the status already posted instead of posting it twice.
 */
export async function createStatus(api: Api, body: CreateStatusRequest, idempotencyKey: string): Promise<Status> {
  const { data } = await api.fetch.POST("/statuses", {
    params: { header: { "Idempotency-Key": idempotencyKey } },
    body,
  });
  if (data === undefined) throw new Error("POST /statuses answered without a body");
  return data;
}

/**
 * Puts a new post into the cache and marks stale every cached list of posts it can appear in (web-client/README.md S1);
 * resolves once those on screen are fetched again. Opening one of the others afterwards fetches it again before
 * it shows (S7), so it shows the post from its first render. A reply also marks the post it answers, which has
 * one more reply.
 */
export async function cachePostedStatus(api: Api, queryClient: QueryClient, status: Status): Promise<void> {
  queryClient.setQueryData(statusQueryOptions(api, status.id).queryKey, status);
  const parentId = status.in_reply_to_id;
  await Promise.all([
    queryClient.invalidateQueries({ predicate: (query) => isPostListQuery(query.queryKey) }),
    parentId === undefined
      ? undefined
      : queryClient.invalidateQueries({ queryKey: statusQueryOptions(api, parentId).queryKey, exact: true }),
  ]);
}

/**
 * The viewer's own account, for its posting limits (data-model/05-states-and-policies.md §5.4). The key is the session's viewer
 * query, so this reads the account already in the cache.
 */
export function credentialsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/accounts/verify_credentials");
}

/**
 * The account a mention names, resolved from its server when the instance does not know it yet
 * (`resolve`), as the server resolves the mentions of a post (data-model/04-tables/04-content.md §4.4). A handle that does not
 * resolve is a 404 and a spent fetch budget a 429: answers rather than failures to retry, and the server
 * leaves such a mention as text. Without `resolve` (while the instance takes nothing in from other servers,
 * ADR-0076, when the server refuses to resolve), only an account the instance knows is found.
 */
export function lookupAccountQueryOptions(api: Api, acct: string, resolve: boolean) {
  return api.query.queryOptions(
    "get",
    "/accounts/lookup",
    { params: { query: resolve ? { acct, resolve: true } : { acct } } },
    { retry: false },
  );
}

/** The viewer's relationships with the accounts `ids` (at most `maxRelationshipIds`). */
export function relationshipsQueryOptions(api: Api, ids: readonly string[]) {
  return api.query.queryOptions("get", "/accounts/relationships", { params: { query: { id: [...ids] } } });
}

/** The most accounts one GET /accounts/relationships asks about (the operation's `maxItems`). */
export const maxRelationshipIds = 40;
