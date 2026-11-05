// SPDX-License-Identifier: AGPL-3.0-only
// The search of posts in the page (web-client/screens/search.md): its form and its results. The hooks return no JSX.
// A search runs only when the viewer sends it: searches have a rate limit of their own, and nothing runs while an
// IME composes, whose Enter commits the conversion rather than sends the form.
import { useNotice } from "@my/ui";
import { useInfiniteQuery } from "@tanstack/react-query";
import { type SubmitEvent, useRef, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { isSearchUnavailable } from "../../../lib/fullTextSearch";
import { useT } from "../../../lib/i18n";
import { useLoadFailure } from "../../../lib/loadFailure";
import { postQueryOf, type SearchedStatus, statusSearchQueryOptions } from "../api/statusSearch";

/** Why a search failed, which decides what the region offers next. */
type StatusSearchFailure =
  /** Over the rate limit: the shell loads it again at the time Retry-After names (lib/rateLimit.ts), and another try only spends the limit. */
  | { kind: "rate-limited" }
  /** The server no longer has a search engine: GET /instance is fetched again, and the page stops offering the search. */
  | { kind: "unavailable"; reason: string }
  | { kind: "failed"; reason: string; retry: () => void; isRetrying: boolean };

export type StatusSearch =
  /** Nothing has been searched yet. */
  | { status: "idle" }
  | { status: "loading" }
  | { status: "error"; failure: StatusSearchFailure }
  | {
      status: "found";
      statuses: readonly SearchedStatus[];
      /** Whether the server has more pages. None need have been found yet: a page can be empty while more follow. */
      hasMore: boolean;
      isLoadingMore: boolean;
      /** Whether the search is being read again, so that what it says of the posts found may change. */
      isRefreshing: boolean;
      loadMore: () => void;
      /** Why the last page failed to load (localized), or null. */
      loadMoreError: string | null;
    };

/** The results of the search `query` (null: none), read from the query cache on every render (S2), and sending it again. */
export function useStatusSearch(query: string | null): { results: StatusSearch; searchAgain: () => void } {
  const api = useApi();
  const t = useT();
  const notice = useNotice();
  const result = useInfiniteQuery({ ...statusSearchQueryOptions(api, query ?? ""), enabled: query !== null });
  const failure = useLoadFailure(result);
  const searchAgain = () => {
    void result.refetch();
  };

  const results = ((): StatusSearch => {
    if (query === null) return { status: "idle" };
    if (result.data === undefined) {
      const { error } = failure;
      if (error === null) return { status: "loading" };
      if (error instanceof ApiError && error.status === 429) {
        return { status: "error", failure: { kind: "rate-limited" } };
      }
      const reason = errorMessage(error);
      if (isSearchUnavailable(error)) return { status: "error", failure: { kind: "unavailable", reason } };
      return {
        status: "error",
        failure: { kind: "failed", reason, retry: searchAgain, isRetrying: failure.isRetrying },
      };
    }
    return {
      status: "found",
      statuses: result.data.pages.flatMap((page) => page.items),
      hasMore: result.hasNextPage,
      isLoadingMore: result.isFetchingNextPage,
      isRefreshing: result.isFetching && !result.isFetchingNextPage,
      loadMore: () => {
        if (!result.hasNextPage || result.isFetchingNextPage) return;
        void result.fetchNextPage().then((next) => {
          if (next.isFetchNextPageError) {
            notice.inPlace(t("client:timeline.load_more.failed", { reason: errorMessage(next.error) }), "assertive");
          }
        });
      },
      loadMoreError: result.isFetchNextPageError ? errorMessage(result.error) : null,
    };
  })();
  return { results, searchAgain };
}

export type StatusSearchFormOptions = {
  /** The search in the page's address (web-client/README.md S3), which the field starts with. */
  query: string;
  /** Called with the trimmed text when the viewer sends a new search. */
  onSearch: (query: string) => void;
  /** Called when the viewer sends the search already shown. */
  onSearchAgain: () => void;
};

/** The form of the search of posts: the typed text, and sending it only once any IME composition has ended. */
export function useStatusSearchForm({ query, onSearch, onSearchAgain }: StatusSearchFormOptions) {
  const [value, setValue] = useState(query);
  // Whether an IME is composing; a ref, since no render depends on it.
  const composingRef = useRef(false);
  return {
    field: {
      value,
      onChange: setValue,
      onCompositionStart: () => {
        composingRef.current = true;
      },
      onCompositionEnd: () => {
        composingRef.current = false;
      },
      onKeyDown: (event: {
        key: string;
        keyCode: number;
        nativeEvent: KeyboardEvent;
        preventDefault: () => void;
        continuePropagation: () => void;
      }) => {
        // The Enter that commits a conversion does not send the form: Chrome and Firefox mark it with isComposing,
        // Safari sends it after compositionend with keyCode 229.
        if (event.key === "Enter" && (event.nativeEvent.isComposing || event.keyCode === 229)) event.preventDefault();
        event.continuePropagation();
      },
    },
    /** Sends the search: the form's submit handler. */
    submit: (event: SubmitEvent<HTMLFormElement>) => {
      event.preventDefault();
      const next = postQueryOf(value);
      if (composingRef.current || next === null) return;
      if (next === query) onSearchAgain();
      else onSearch(next);
    },
  };
}
