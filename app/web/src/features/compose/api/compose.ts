// SPDX-License-Identifier: AGPL-3.0-only
// The server data of the compose feature (web-client/README.md S1): the limits and preferences a new post
// follows, the post being replied to, the uploaded media while the server processes them, the accounts
// that mentions name and the viewer's relationships with them, and the writes (uploading a file,
// posting, appealing a post the server's automated review refused, and how the appeal ended). Types come from the
// spec (S9).
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { isPostListQuery } from "../../../lib/query";

export type Status = components["schemas"]["Status"];
export type MediaAttachment = components["schemas"]["MediaAttachment"];
export type Visibility = components["schemas"]["Visibility"];
export type CreateStatusRequest = components["schemas"]["CreateStatusRequest"];
export type PostingLimits = components["schemas"]["PostingLimits"];
type CredentialAccount = components["schemas"]["CredentialAccount"];

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
 * Adds a new post to the viewer's own post count (`statuses_count`) in the cache and returns the new count, or null
 * while the account is not in the cache. A count of 1 after the create is the account's first post (web-client §21.5).
 */
export function countPostedStatus(api: Api, queryClient: QueryClient): number | null {
  const { queryKey } = credentialsQueryOptions(api);
  const viewer = queryClient.getQueryData<CredentialAccount>(queryKey);
  if (viewer === undefined) return null;
  const count = viewer.statuses_count + 1;
  queryClient.setQueryData(queryKey, { ...viewer, statuses_count: count });
  return count;
}

/**
 * The account a mention names, resolved from its server when the instance does not know it yet
 * (`resolve`), as the server resolves the mentions of a post (data-model/04-tables/04-content.md §4.4). A handle that does not
 * resolve is a 404 and a spent fetch budget a 429: answers rather than failures to retry, and the server
 * leaves such a mention as text. Without `resolve` (while the instance takes nothing in from other servers,
 * ADR-0006, when the server refuses to resolve), only an account the instance knows is found.
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

/** The problem type of a post that the server's automated review refused (422): nothing was posted. */
const statusRejectedType = "/api/v1/problems/status-rejected";

/**
 * A post that the server's automated review refused, as its problem says: the fixed `title` and `detail` the server
 * localized (they say nothing of the review's answer), and the decision the author may appeal until `appealDeadline`.
 * The decision and the deadline are null when the problem has none: then there is nothing to appeal.
 */
export type StatusRejection = {
  title: string;
  detail: string | null;
  hookDecisionId: string | null;
  appealDeadline: string | null;
};

/** The refusal that `error` is, or null for any other error. */
export function statusRejection(error: unknown): StatusRejection | null {
  if (!(error instanceof ApiError) || error.status !== 422 || error.type !== statusRejectedType) return null;
  const { hook_decision_id: id, appeal_deadline: deadline } = error.document;
  const hasAppeal = typeof id === "string" && id !== "" && typeof deadline === "string";
  return {
    title: error.title,
    detail: error.detail ?? null,
    hookDecisionId: hasAppeal ? id : null,
    appealDeadline: hasAppeal ? deadline : null,
  };
}

type AppealHookDecisionRequest = components["schemas"]["AppealHookDecisionRequest"];
type OwnHookAppeal = components["schemas"]["OwnHookAppeal"];

/** The longest text and comment an appeal of a refused post takes (the AppealHookDecisionRequest schema). */
export const hookAppealTextLimit = 100_000;
export const hookAppealCommentLimit = 2000;

/**
 * The author's appeal of the decision that refused a post: pending, or how a moderator reviewed it and when. A 404
 * means there is none (or it is no longer kept).
 */
export function ownHookAppealQueryOptions(api: Api, id: string) {
  return api.query.queryOptions("get", "/hook_decisions/{id}/appeal", { params: { path: { id } } });
}

/** Appeals the decision that refused a post, with the text the author tried to post and an optional comment. */
export async function appealHookDecision(
  api: Api,
  id: string,
  body: AppealHookDecisionRequest,
): Promise<OwnHookAppeal> {
  const { data, error } = await api.fetch.POST("/hook_decisions/{id}/appeal", { params: { path: { id } }, body });
  // The middleware throws an ApiError for every error status; this narrows the type.
  if (data === undefined) throw new ApiError(error);
  return data;
}

/**
 * The problem types of an appeal that the decision's own state explains: it was appealed already, its window closed,
 * or it cannot be appealed. Nothing the author changes in the appeal helps, so the form gives way to the reason.
 */
const closedHookAppealProblems = [
  "/api/v1/problems/already-appealed",
  "/api/v1/problems/appeal-window-closed",
  "/api/v1/problems/not-appealable",
] as const;
export type ClosedHookAppealProblem = (typeof closedHookAppealProblems)[number];

/** Why an appeal cannot be sent at all, from its error; null when sending it again may still work. */
export function closedHookAppeal(error: unknown): ClosedHookAppealProblem | null {
  if (!(error instanceof ApiError)) return null;
  // An unknown decision cannot be appealed either.
  if (error.status === 404) return "/api/v1/problems/not-appealable";
  return closedHookAppealProblems.find((type) => type === error.type) ?? null;
}
