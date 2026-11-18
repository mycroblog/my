// SPDX-License-Identifier: AGPL-3.0-only

// What the handlers of the fake API share: how a problem, an ID, an image and a page of a list are made as the server
// makes them. The fake's own state and its handlers are api.ts's and settings.ts's.

import { type Refusal, type Schema } from "./spec";

/** The items of a page when the request names no `limit`, and the most it may name: the server's (Limit). */
const pageSize = 20;
const maxPageSize = 40;
export const minute = 60_000;
export const day = 24 * 60 * minute;

/** A problem as the server answers it: `about:blank` unless `more` names its type. */
export const problem = (status: number, title: string, more: Partial<Schema<"Problem">> = {}): Refusal => ({
  problem: { type: "about:blank", title, status, ...more },
});
export const notFound = () => problem(404, "Not Found");

/** An ID as the server's, a UUIDv7: in its canonical form, a later one sorts after an earlier one. */
export function idAt(ms: number, n: number): string {
  const time = Math.max(0, Math.round(ms)).toString(16).padStart(12, "0");
  return `${time.slice(0, 8)}-${time.slice(8)}-7000-8000-${n.toString(16).padStart(12, "0")}`;
}

/** An image in the sizes the server gives it. */
export const variants = (url: string): Schema<"MediaVariants"> => ({ small: { url }, medium: { url }, large: { url } });

/** A page of a list, as the server pages it. */
export type Paged<T> = { from: number; body: { items: T[]; next_cursor?: string } };
export type PageQuery = { get: (name: "cursor" | "limit") => string | null };

/**
 * The page of `items` that the request's `cursor` and `limit` ask for, as the server pages a list; the cursor of a
 * page is the `key` of its last item, its ID unless the items have none.
 */
export function page<T extends { id: string }>(items: readonly T[], query: PageQuery): Paged<T>;
export function page<T>(items: readonly T[], query: PageQuery, key: (item: T) => string): Paged<T>;
export function page<T>(
  items: readonly T[],
  query: PageQuery,
  key: (item: T) => string = (item) => (item as { id: string }).id,
): Paged<T> {
  const cursor = query.get("cursor");
  const limit = Number(query.get("limit") ?? pageSize);
  const size = Number.isInteger(limit) && limit > 0 ? Math.min(limit, maxPageSize) : pageSize;
  const from = cursor === null ? 0 : items.findIndex((s) => key(s) === cursor) + 1;
  const slice = items.slice(from, from + size);
  const last = slice.at(-1);
  return {
    from,
    body: { items: slice, ...(last !== undefined && from + size < items.length ? { next_cursor: key(last) } : {}) },
  };
}
