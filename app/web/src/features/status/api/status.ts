// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the status feature (web-client/README.md S1, FR-106 to FR-109, FR-113): one post,
// the thread around it, its edit history, and the viewer's actions on it (editing their own post among them). Keys follow openapi-react-query's
// `[method, path, init]`, so the post's key is the one the compose and notifications features use too.
import { infiniteQueryOptions, type QueryClient, type QueryKey } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import type { components } from "@mycroblog/mykit";
import {
  forPost,
  type LikeSlot,
  updateCachedStatuses,
  withLikeSlot,
  withLikeSlotFrom,
  withRelation,
  withRelationFrom,
} from "./cache";

/** A status (post or boost), as the client API returns it. */
export type Status = components["schemas"]["Status"];

/** Who can see a post (ADR-0017). */
export type Visibility = components["schemas"]["Visibility"];

/** The post that a status quotes (FR-114, ADR-0069). */
export type StatusQuote = components["schemas"]["StatusQuote"];

/** Who may quote a post (FR-114, ADR-0069 §1). */
export type QuotePolicy = components["schemas"]["QuotePolicy"];

/** The card of the first link in a status (ADR-0062). */
export type LinkCard = components["schemas"]["LinkCard"];

/** One attachment of a status. */
export type MediaAttachment = components["schemas"]["MediaAttachment"];

/** The new content of an edited post (PUT /statuses/{id}). */
export type EditStatusRequest = components["schemas"]["EditStatusRequest"];

/** What the author typed, for editing a post (GET /statuses/{id}/source). */
export type StatusSource = components["schemas"]["StatusSource"];

/** One revision of a post (GET /statuses/{id}/history). */
export type StatusEdit = components["schemas"]["StatusEdit"];

/** The emoji of a reaction: a Unicode emoji, or a custom emoji with its image (ADR-0052). */
export type ReactionEmoji = components["schemas"]["ReactionEmoji"];

/** The poll of a post (FR-112). */
export type Poll = components["schemas"]["Poll"];

/** The actions that the viewer turns on and off on a post. */
export type Relation = "boost" | "favourite" | "bookmark";

/** A post (GET /statuses/{id}). */
export function statusQueryOptions(api: Api, id: string) {
  return api.query.queryOptions("get", "/statuses/{id}", { params: { path: { id } } });
}

/** The thread around a post (GET /statuses/{id}/context). */
export function statusContextQueryOptions(api: Api, id: string) {
  return api.query.queryOptions("get", "/statuses/{id}/context", { params: { path: { id } } });
}

/** Every revision of a post, oldest first; the last one is the current content (GET /statuses/{id}/history). */
export function statusHistoryQueryOptions(api: Api, id: string) {
  return api.query.queryOptions("get", "/statuses/{id}/history", { params: { path: { id } } });
}

/**
 * What the viewer typed for their post `id`, for editing it (GET /statuses/{id}/source). It is read again
 * every time the edit dialog opens (`staleTime: 0`), so that an edit starts from the latest text. A failure
 * shows at once with a button to try again, instead of after seconds of silent retries.
 */
export function statusSourceQueryOptions(api: Api, id: string) {
  return api.query.queryOptions(
    "get",
    "/statuses/{id}/source",
    { params: { path: { id } } },
    { staleTime: 0, gcTime: 0, retry: false },
  );
}

/** The instance, for the most characters a post may have (`limits.max_post_characters`). */
export function instanceQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/instance");
}

/** Replaces the content of the viewer's post `id` (PUT /statuses/{id}); returns the edited post. */
export async function editStatus(api: Api, id: string, body: EditStatusRequest): Promise<Status> {
  const { data, error } = await api.fetch.PUT("/statuses/{id}", { params: { path: { id } }, body });
  // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
  if (data === undefined) throw new ApiError(error);
  return data;
}

/**
 * Puts the server's answer to an edit into the cache (web-client/README.md S1, ADR-0045): every cached copy of the
 * post, in a list or inside a boost, becomes the edited post, where it is, so that no list on screen moves
 * (Y7). The history has one more revision: it is marked stale, and fetched again if it is on screen.
 */
export async function cacheEditedStatus(api: Api, queryClient: QueryClient, edited: Status): Promise<void> {
  updateCachedStatuses(
    queryClient,
    forPost(edited.id, () => edited),
  );
  await queryClient.invalidateQueries({ queryKey: statusHistoryQueryOptions(api, edited.id).queryKey, exact: true });
}

/**
 * Turns the viewer's `relation` to post `id` on or off; returns the post after it. The server treats each
 * as a setter (turning on what is on changes nothing), so a retry after a lost answer is safe.
 */
export async function setRelation(api: Api, relation: Relation, id: string, on: boolean): Promise<Status> {
  const init = { params: { path: { id } } };
  const { data, error } = await (() => {
    switch (relation) {
      case "boost":
        return on ? api.fetch.POST("/statuses/{id}/boost", init) : api.fetch.DELETE("/statuses/{id}/boost", init);
      case "favourite":
        return on
          ? api.fetch.POST("/statuses/{id}/favourite", init)
          : api.fetch.DELETE("/statuses/{id}/favourite", init);
      case "bookmark":
        return on ? api.fetch.POST("/statuses/{id}/bookmark", init) : api.fetch.DELETE("/statuses/{id}/bookmark", init);
    }
  })();
  // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
  if (data === undefined) throw new ApiError(error);
  return data;
}

/**
 * Turns the viewer's `relation` to post `id` to `on` in every cached copy of the post before the server answers
 * (web-client/README.md S8), after cancelling a refetch of the post that would overwrite it with the old state.
 */
export async function cacheRelationPending(
  api: Api,
  queryClient: QueryClient,
  id: string,
  relation: Relation,
  on: boolean,
): Promise<void> {
  await queryClient.cancelQueries({ queryKey: statusQueryOptions(api, id).queryKey });
  updateCachedStatuses(
    queryClient,
    forPost(id, (status) => withRelation(status, relation, on)),
  );
}

/** Puts back the viewer's `relation` to post `id` as it was before a press to `on` that the server refused. */
export function restoreRelation(queryClient: QueryClient, id: string, relation: Relation, on: boolean): void {
  updateCachedStatuses(
    queryClient,
    forPost(id, (status) => withRelation(status, relation, !on)),
  );
}

/**
 * Sets the viewer's emoji reaction to post `id` to `emoji`, the `name` of a ReactionEmoji (PUT
 * /statuses/{id}/reaction), or takes it back with null (DELETE); returns the post after it. The viewer has one
 * favourite or one reaction on a post (ADR-0052 §1): the server takes back the favourite or the other reaction. Both
 * are setters, so a retry after a lost answer is safe.
 */
async function setReaction(api: Api, id: string, emoji: string | null): Promise<Status> {
  const init = { params: { path: { id } } };
  const { data, error } = await (emoji === null
    ? api.fetch.DELETE("/statuses/{id}/reaction", init)
    : api.fetch.PUT("/statuses/{id}/reaction", { ...init, body: { emoji } }));
  // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
  if (data === undefined) throw new ApiError(error);
  return data;
}

/**
 * Turns the viewer's Like slot of post `id` from `previous` into `slot` (ADR-0052 §1): favourites, reacts with an
 * emoji, or takes back what is there; returns the post after it.
 */
export function setLikeSlot(api: Api, id: string, slot: LikeSlot, previous: LikeSlot): Promise<Status> {
  switch (slot.kind) {
    case "favourite":
      return setRelation(api, "favourite", id, true);
    case "reaction":
      return setReaction(api, id, slot.emoji.name);
    case "none":
      return previous.kind === "favourite" ? setRelation(api, "favourite", id, false) : setReaction(api, id, null);
  }
}

/**
 * Fills the viewer's Like slot of post `id` (a favourite, a reaction, or nothing) in every cached copy of the post
 * before the server answers (web-client/README.md S8), after cancelling a refetch of the post that would overwrite it.
 */
export async function cacheLikeSlotPending(
  api: Api,
  queryClient: QueryClient,
  id: string,
  slot: LikeSlot,
): Promise<void> {
  await queryClient.cancelQueries({ queryKey: statusQueryOptions(api, id).queryKey });
  updateCachedStatuses(
    queryClient,
    forPost(id, (status) => withLikeSlot(status, slot)),
  );
}

/** Puts back the viewer's Like slot of post `id` as it was before a change that the server refused. */
export function restoreLikeSlot(queryClient: QueryClient, id: string, slot: LikeSlot): void {
  updateCachedStatuses(
    queryClient,
    forPost(id, (status) => withLikeSlot(status, slot)),
  );
}

/**
 * Puts the server's answer to a change of the viewer's Like slot of post `id` into the cache: the favourite, the
 * reactions and their counts, which include others' actions, taken only from the `last` change on its way.
 */
export function cacheLikeSlotAnswer(queryClient: QueryClient, id: string, answer: Status, last: boolean): void {
  if (!last) return;
  const from = answer.boost ?? answer;
  updateCachedStatuses(
    queryClient,
    forPost(id, (status) => withLikeSlotFrom(status, from)),
  );
}

/** How many reactions a page of the accounts that reacted holds. */
const reactorsPageSize = 40;

/**
 * The reactions to post `id`, newest first, each with the account that reacted (GET /statuses/{id}/reactions),
 * page by page. The key ends in "pages" so that it never meets a plain query of the operation (web-client/10-api-client-and-auth.md §10).
 */
export function reactorsQueryOptions(api: Api, id: string) {
  return infiniteQueryOptions({
    queryKey: ["get", "/statuses/{id}/reactions", { params: { path: { id } } }, "pages"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const { data, error } = await api.fetch.GET("/statuses/{id}/reactions", {
        params: {
          path: { id },
          query: { limit: reactorsPageSize, ...(pageParam !== undefined && { cursor: pageParam }) },
        },
        signal,
      });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor ?? undefined,
  });
}

/** Whether `queryKey` is a page of the posts of the account `accountId` (GET /accounts/{id}/statuses), under any filter. */
function isAccountStatusesQuery(queryKey: QueryKey, accountId: string): boolean {
  if (queryKey[1] !== "/accounts/{id}/statuses") return false;
  const init = queryKey[2] as { params?: { path?: { id?: string } } } | undefined;
  return init?.params?.path?.id === accountId;
}

/**
 * Puts the server's answer to a press of the viewer `viewerId`'s `relation` to post `id` into the cache
 * (web-client/README.md S1, ADR-0045). The server's count includes others' actions, so it is taken only from the
 * `last` press: an answer to an earlier press does not undo a later one still on its way. A boost is also an
 * entry of the viewer's home timeline and of their own posts, which it joins or leaves: those lists are marked,
 * and only marked stale: a list on screen keeps its top (Y7), and a list opened next is fetched again before it shows (S7).
 * A bookmark likewise joins or leaves the viewer's list of bookmarks, which is only marked: on the bookmarks page a
 * removed post stays, shown unbookmarked, so that the press can be undone and focus stays on its button; the post is
 * gone when the list is opened again.
 */
export function cacheRelationAnswer(
  queryClient: QueryClient,
  id: string,
  relation: Relation,
  answer: Status,
  last: boolean,
  viewerId: string,
): void {
  if (last) {
    const from = answer.boost ?? answer;
    updateCachedStatuses(
      queryClient,
      forPost(id, (status) => withRelationFrom(status, relation, from)),
    );
  }
  if (relation === "boost") {
    void queryClient.invalidateQueries({
      predicate: (query) => query.queryKey[1] === "/timelines/home" || isAccountStatusesQuery(query.queryKey, viewerId),
      refetchType: "none",
    });
  }
  if (relation === "bookmark") {
    void queryClient.invalidateQueries({
      predicate: (query) => query.queryKey[1] === "/bookmarks",
      refetchType: "none",
    });
  }
}

/**
 * The posts that account `accountId` pinned to its profile, newest pin first, in a single page
 * (GET /accounts/{id}/statuses?pinned=true, FR-403). For a remote account, those this server knows of.
 */
export function pinnedStatusesQueryOptions(api: Api, accountId: string) {
  return api.query.queryOptions("get", "/accounts/{id}/statuses", {
    params: { path: { id: accountId }, query: { pinned: true } },
  });
}

/**
 * Pins the viewer's post `id` to their profile, or unpins it; returns the post after it. The server treats
 * both as setters, so a retry after a lost answer is safe; a pin beyond the limit is a 422 whose detail says so.
 */
export async function setPin(api: Api, id: string, on: boolean): Promise<Status> {
  const init = { params: { path: { id } } };
  const { data, error } = await (on
    ? api.fetch.POST("/statuses/{id}/pin", init)
    : api.fetch.DELETE("/statuses/{id}/pin", init));
  // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
  if (data === undefined) throw new ApiError(error);
  return data;
}

/**
 * Puts the server's answer to a pin or unpin of the viewer `viewerId`'s post `id` into the cache (web-client/README.md
 * S1): every copy of the post takes its `pinned`, and the viewer's pinned posts, a list that changes, are read
 * again. Resolves once the list on screen has been read again, so that the success shows with it.
 */
export async function cachePinAnswer(
  api: Api,
  queryClient: QueryClient,
  id: string,
  answer: Status,
  viewerId: string,
): Promise<void> {
  const pinned = answer.pinned === true;
  updateCachedStatuses(
    queryClient,
    forPost(id, (status) => (status.pinned === pinned ? status : { ...status, pinned })),
  );
  await queryClient.invalidateQueries({ queryKey: pinnedStatusesQueryOptions(api, viewerId).queryKey });
}

/**
 * Votes for the options at `choices` in the poll `pollId` of post `id` (POST /statuses/{id}/poll/votes, FR-112);
 * returns the post after it. A second vote is refused (422), so a retry after a lost answer changes nothing.
 */
export async function votePoll(api: Api, id: string, pollId: string, choices: readonly number[]): Promise<Status> {
  const { data, error } = await api.fetch.POST("/statuses/{id}/poll/votes", {
    params: { path: { id } },
    body: { poll_id: pollId, choices: [...choices] },
  });
  // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
  if (data === undefined) throw new ApiError(error);
  return data;
}

/**
 * Puts the server's answer to a vote in the poll of post `id` into the cache (web-client/README.md S1): every copy of the
 * post takes the poll of the answer, with the viewer's vote, where it is.
 */
export function cacheVoteAnswer(queryClient: QueryClient, id: string, answer: Status): void {
  const { poll } = answer.boost ?? answer;
  if (poll === undefined) return;
  updateCachedStatuses(
    queryClient,
    forPost(id, (status) => ({ ...status, poll })),
  );
}

/**
 * Marks post `id` stale after the server refused a vote, so that the post on screen is read again: its poll may
 * have ended, been replaced by an edit, or have the viewer's vote from another device.
 */
export async function refreshAfterRefusedVote(api: Api, queryClient: QueryClient, id: string): Promise<void> {
  await queryClient.invalidateQueries({ queryKey: statusQueryOptions(api, id).queryKey, exact: true });
}

/** Deletes the viewer's post `id` (DELETE /statuses/{id}). */
export async function deleteStatus(api: Api, id: string): Promise<void> {
  // The client's middleware throws for every error status; a 204 has no body to read.
  await api.fetch.DELETE("/statuses/{id}", { params: { path: { id } } });
}

/**
 * Takes the deleted post `post` out of the cache (web-client/README.md S1, ADR-0045): its own query and thread go,
 * every list drops it and the boosts of it, and its parent has one reply less.
 */
export function cacheDeletedStatus(api: Api, queryClient: QueryClient, post: Status): void {
  queryClient.removeQueries({ queryKey: statusQueryOptions(api, post.id).queryKey, exact: true });
  queryClient.removeQueries({ queryKey: statusContextQueryOptions(api, post.id).queryKey, exact: true });
  updateCachedStatuses(
    queryClient,
    forPost(post.id, () => null),
  );
  const parentId = post.in_reply_to_id;
  if (parentId !== undefined) {
    updateCachedStatuses(
      queryClient,
      forPost(parentId, (parent) => ({ ...parent, replies_count: Math.max(0, parent.replies_count - 1) })),
    );
  }
}

/**
 * The web client's address of a post: `/@{acct}/status/{id}` (ADR-0053), where acct is the author's username
 * for a local account and `username@domain` for a remote one. The name only makes the address readable; the ID
 * finds the post (the server answers the same path with the post's link preview, ADR-0028).
 */
export function statusPath(status: Pick<Status, "id" | "account">): string {
  return `/@${status.account.acct}/status/${status.id}`;
}

/** The web client's address of a post's edit history: `/@{acct}/status/{id}/history`. */
export function statusHistoryPath(status: Pick<Status, "id" | "account">): string {
  return `${statusPath(status)}/history`;
}

/** The web client's address of the posts that quote a post: `/@{acct}/status/{id}/quotes`. */
export function statusQuotesPath(status: Pick<Status, "id" | "account">): string {
  return `${statusPath(status)}/quotes`;
}

/** The address to share for a post: its HTML page, or its ActivityPub ID when it has none. */
export function statusShareUrl(status: Pick<Status, "url" | "uri">): string {
  return status.url ?? status.uri;
}

/** Changes who may quote the viewer's post `id` (PUT /statuses/{id}/quote_policy, FR-114); returns the post after it. */
export async function setQuotePolicy(api: Api, id: string, policy: QuotePolicy): Promise<Status> {
  const { data, error } = await api.fetch.PUT("/statuses/{id}/quote_policy", {
    params: { path: { id } },
    body: { quote_policy: policy },
  });
  // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
  if (data === undefined) throw new ApiError(error);
  return data;
}

/** Puts the server's answer to a change of the quote policy into every cached copy of the post (S1). */
export function cacheQuotePolicyAnswer(queryClient: QueryClient, answer: Status): void {
  updateCachedStatuses(
    queryClient,
    forPost(answer.id, (status) => ({
      ...status,
      ...(answer.quote_policy === undefined ? {} : { quote_policy: answer.quote_policy }),
    })),
  );
}

/**
 * Revokes the approval of the quote `quotingId` of the viewer's post `quotedId`
 * (POST /statuses/{id}/quotes/{quote_id}/revoke, ADR-0069 §3): the quote shows the post no more.
 */
export async function revokeQuote(api: Api, quotedId: string, quotingId: string): Promise<void> {
  // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
  await api.fetch.POST("/statuses/{id}/quotes/{quote_id}/revoke", {
    params: { path: { id: quotedId, quote_id: quotingId } },
  });
}

/**
 * Puts a revocation into the cache (S1): every copy of the quoting post `quotingId` shows its quote as revoked,
 * without the quoted post, and the list of the quotes of `quotedId`, which changes, is marked stale.
 */
export async function cacheRevokedQuote(queryClient: QueryClient, quotedId: string, quotingId: string): Promise<void> {
  updateCachedStatuses(
    queryClient,
    forPost(quotingId, (status) => (status.quote === undefined ? status : { ...status, quote: { state: "revoked" } })),
  );
  await queryClient.invalidateQueries({
    queryKey: ["get", "/statuses/{id}/quotes", { params: { path: { id: quotedId } } }],
  });
}
