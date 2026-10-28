// SPDX-License-Identifier: AGPL-3.0-only
import type { InfiniteData } from "@tanstack/react-query";
import { useState } from "react";

/** What a list screen needs from an infinite query: the items so far, and loading more. */
export type PagedList<T> = {
  items: T[];
  hasMore: boolean;
  isLoadingMore: boolean;
  loadMore: () => void;
};

/** A PagedList that keeps its pages apart, for a table with previous / next. */
export type PagedPages<T> = PagedList<T> & {
  /** The loaded pages, in order, none of them empty. */
  pages: T[][];
};

type InfiniteList<P> = {
  data: InfiniteData<P>;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  isFetching: boolean;
  fetchNextPage: () => Promise<unknown>;
};

/** The PagedList of an infinite query whose pages have `items`. */
export function toPagedList<T>(query: InfiniteList<{ items: T[] }>): PagedPages<T> {
  return {
    items: query.data.pages.flatMap((page) => page.items),
    pages: query.data.pages.map((page) => page.items).filter((items) => items.length > 0),
    hasMore: query.hasNextPage,
    isLoadingMore: query.isFetchingNextPage,
    loadMore: () => {
      // A fetch in progress would be overwritten by the page fetched now (TanStack Query, "Infinite Queries").
      if (!query.isFetching) void query.fetchNextPage();
    },
  };
}

/** A list that is all there is, in one page, for a table of records the API returns whole. */
export function wholeList<T>(items: T[]): PagedPages<T> {
  return {
    items,
    pages: items.length > 0 ? [items] : [],
    hasMore: false,
    isLoadingMore: false,
    loadMore: () => undefined,
  };
}

export type ListPages<T> = {
  /** The rows of the page on screen. */
  items: T[];
  /** The previous page, or undefined on the first. */
  previous: (() => void) | undefined;
  /** The next page, loading it when it is not loaded yet; undefined on the last. */
  next: (() => void) | undefined;
};

/**
 * Steps through the pages an infinite query has loaded, one page on screen, for a table with previous / next
 * (the API pages by cursor, so there is no page number). Next past the loaded pages loads one more and moves to it
 * once it is there; the screen keeps the page it shows until then. Key the component by whatever the list is of
 * (the filter), so that another list starts at its first page.
 */
export function useListPages<T>(list: PagedPages<T>): ListPages<T> {
  const [wanted, setWanted] = useState(0);
  const last = Math.max(list.pages.length - 1, 0);
  const index = Math.min(wanted, last);
  return {
    items: list.pages[index] ?? [],
    previous:
      index > 0
        ? () => {
            setWanted(index - 1);
          }
        : undefined,
    next:
      index < last || list.hasMore
        ? () => {
            setWanted(index + 1);
            if (index >= last) list.loadMore();
          }
        : undefined,
  };
}
