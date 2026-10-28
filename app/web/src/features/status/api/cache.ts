// SPDX-License-Identifier: AGPL-3.0-only
// One post appears in many cached answers at once: its own query, the threads it is part of, and the pages
// of every timeline, either itself or inside a boost. An action on it rewrites every copy, so that each
// screen shows the same state (web-client/README.md S8: the change reaches several screens, so the cache is
// rewritten and restored on failure). The answers are matched by shape, not by key, so a list that a
// later feature caches (a profile's posts, the bookmarks) is rewritten too as long as it holds statuses.
import type { QueryClient } from "@tanstack/react-query";
import type { ReactionEmoji, Relation, Status } from "./status";

/** Returns the status to keep in place of `status`, or null to drop it from the lists it is in. */
export type StatusUpdate = (status: Status) => Status | null;

/** The fields of a post that a relation sets. */
const relationFields = {
  boost: { flag: "boosted", count: "boosts_count" },
  favourite: { flag: "favourited", count: "favourites_count" },
  bookmark: { flag: "bookmarked", count: null },
} as const satisfies Record<Relation, { flag: keyof Status; count: keyof Status | null }>;

/** Whether the viewer has `relation` to `status`. */
export function hasRelation(status: Status, relation: Relation): boolean {
  return status[relationFields[relation].flag] === true;
}

/** How many accounts have `relation` to `status`, or null for a relation without a public count (bookmarks). */
export function relationCount(status: Status, relation: Relation): number | null {
  const { count } = relationFields[relation];
  return count === null ? null : status[count];
}

/** Sets the viewer's `relation` to `on`, moving its count with it; a status already so is returned as it is. */
export function withRelation(status: Status, relation: Relation, on: boolean): Status {
  if (hasRelation(status, relation) === on) return status;
  const { flag, count } = relationFields[relation];
  const next: Status = { ...status, [flag]: on };
  if (count !== null) next[count] = Math.max(0, status[count] + (on ? 1 : -1));
  return next;
}

/** Copies the viewer's `relation` and its count from the server's answer `from`. */
export function withRelationFrom(status: Status, relation: Relation, from: Status): Status {
  const { flag, count } = relationFields[relation];
  const next: Status = { ...status, [flag]: from[flag] === true };
  if (count !== null) next[count] = from[count];
  return next;
}

/**
 * What fills the viewer's Like slot of a post (ADR-0052 §1): a favourite, a reaction with one emoji, or nothing. A
 * favourite and a reaction never both: setting one takes back the other.
 */
export type LikeSlot = { kind: "none" } | { kind: "favourite" } | { kind: "reaction"; emoji: ReactionEmoji };

/** The viewer's reaction to `status`, or undefined without one. */
function myReaction(status: Status): ReactionEmoji | undefined {
  return status.reactions.find((reaction) => reaction.me === true);
}

/** What fills the viewer's Like slot of `status`. */
export function likeSlotOf(status: Status): LikeSlot {
  if (status.favourited === true) return { kind: "favourite" };
  const reaction = myReaction(status);
  return reaction === undefined ? { kind: "none" } : { kind: "reaction", emoji: reaction };
}

/**
 * Sets what fills the viewer's Like slot of `status`, moving the counts with it: the favourite and its count, the
 * viewer's reaction and the count of its emoji (a count that reaches zero goes, and a new emoji comes last).
 */
export function withLikeSlot(status: Status, slot: LikeSlot): Status {
  const current = likeSlotOf(status);
  const sameReaction =
    current.kind === "reaction" && slot.kind === "reaction" && current.emoji.name === slot.emoji.name;
  if (current.kind === slot.kind && (slot.kind !== "reaction" || sameReaction)) return status;
  let next = withRelation(status, "favourite", slot.kind === "favourite");
  const reactions = next.reactions.flatMap((reaction) => {
    if (reaction.me !== true) return [reaction];
    const count = reaction.count - 1;
    return count > 0 ? [{ ...reaction, count, me: false }] : [];
  });
  if (slot.kind === "reaction") {
    const i = reactions.findIndex((reaction) => reaction.name === slot.emoji.name);
    const found = reactions[i];
    if (found === undefined) reactions.push({ ...slot.emoji, count: 1, me: true });
    else reactions[i] = { ...found, count: found.count + 1, me: true };
  }
  next = { ...next, reactions };
  return next;
}

/** Copies the viewer's Like slot and the counts it moves (favourites and reactions) from the server's answer `from`. */
export function withLikeSlotFrom(status: Status, from: Status): Status {
  return { ...withRelationFrom(status, "favourite", from), reactions: from.reactions };
}

function isStatus(value: unknown): value is Status {
  if (typeof value !== "object" || value === null) return false;
  const { id, uri, content, account, media_attachments } = value as Record<string, unknown>;
  return (
    typeof id === "string" &&
    typeof uri === "string" &&
    typeof content === "string" &&
    typeof account === "object" &&
    Array.isArray(media_attachments)
  );
}

/** Applies `update` to a status and to the post it boosts; a boost of a dropped post is dropped too. */
function updateStatus(status: Status, update: StatusUpdate): Status | null {
  let next = status;
  if (status.boost !== undefined) {
    const boost = updateStatus(status.boost, update);
    if (boost === null) return null;
    if (boost !== status.boost) next = { ...status, boost };
  }
  return update(next);
}

/** The fields of cached answers that hold statuses or pages of them (InfiniteData, StatusPage, StatusContext). */
const listFields = ["pages", "items", "ancestors", "descendants"] as const;

/**
 * Returns `data` with `update` applied to every status in it, or `data` itself when nothing changed (so
 * that the queries whose answers do not hold the post do not render again). A status at the top (a post's
 * own query) is never dropped; remove that query instead.
 */
export function updateStatuses(data: unknown, update: StatusUpdate): unknown {
  if (isStatus(data)) return updateStatus(data, update) ?? data;
  return updateNested(data, update);
}

function updateNested(data: unknown, update: StatusUpdate): unknown {
  if (typeof data !== "object" || data === null || Array.isArray(data)) return data;
  const record = data as Record<string, unknown>;
  let next: Record<string, unknown> | null = null;
  for (const field of listFields) {
    const list = record[field];
    if (!Array.isArray(list)) continue;
    const updated = updateList(list, update);
    if (updated !== list) next = { ...(next ?? record), [field]: updated };
  }
  return next ?? data;
}

function updateList(list: readonly unknown[], update: StatusUpdate): readonly unknown[] {
  const next: unknown[] = [];
  for (const item of list) {
    const updated = isStatus(item) ? updateStatus(item, update) : updateNested(item, update);
    if (updated !== null) next.push(updated);
  }
  return next.length === list.length && next.every((item, i) => item === list[i]) ? list : next;
}

/** Applies `update` to every status in every cached answer of the API. */
export function updateCachedStatuses(queryClient: QueryClient, update: StatusUpdate): void {
  // The API's keys start with the method (openapi-react-query's `[method, path, init]`); reads are "get".
  queryClient.setQueriesData(
    { predicate: (query) => query.queryKey[0] === "get" && query.state.data !== undefined },
    (data: unknown) => {
      const next = updateStatuses(data, update);
      // undefined leaves the query as it is.
      return next === data ? undefined : next;
    },
  );
}

/** An update that applies `change` to post `id` wherever it is. */
export function forPost(id: string, change: (post: Status) => Status | null): StatusUpdate {
  return (status) => (status.id === id ? change(status) : status);
}
