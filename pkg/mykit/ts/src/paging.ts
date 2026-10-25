// SPDX-License-Identifier: MIT

/** A page of a collection with keyset pagination: its items and the cursor of the next page. */
export type Page<T> = {
  readonly items: readonly T[];
  /** The cursor of the next page; absent, null, or empty on the last page. */
  readonly next_cursor?: string | null;
};

/** Fetches the page of a collection that cursor names (undefined for the first page). */
export type PageFetcher<T> = (cursor: string | undefined) => Promise<Page<T>>;

/**
 * Iterates the items of a collection with keyset pagination, fetching the pages one by one as the loop needs them by
 * following next_cursor. It stops after the last page, rejects at the first error, and fetches no more when the loop
 * breaks:
 *
 * ```ts
 * for await (const status of all(async (cursor) => {
 *   const { data } = await api.GET("/timelines/home", { params: { query: { cursor } } });
 *   return data!;
 * })) {
 *   console.log(status.id);
 * }
 * ```
 *
 * A timeline has no end: break out of the loop when you have seen enough.
 */
export async function* all<T>(fetch: PageFetcher<T>): AsyncGenerator<T, void, undefined> {
  let cursor: string | undefined;
  for (;;) {
    const page = await fetch(cursor);
    yield* page.items;
    if (page.next_cursor === undefined || page.next_cursor === null || page.next_cursor === "") return;
    cursor = page.next_cursor;
  }
}
