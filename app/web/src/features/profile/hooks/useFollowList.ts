// SPDX-License-Identifier: AGPL-3.0-only
import { sanitizedHtmlText, serverSanitizedHtml, useNotice } from "@my/ui";
import { useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { handleOf } from "../../../lib/handle";
import { useT } from "../../../lib/i18n";
import { profilePath } from "../../../lib/paths";
import { type Account, type FollowList, followListQueryOptions } from "../api/accounts";
import { nameOf } from "./useProfile";

/** One account of a follow list, as its row shows it. */
type FollowListItem = {
  id: string;
  name: string;
  handle: string;
  href: string;
  /** The profile text as one line of plain text, or "" when there is none. */
  noteLine: string;
  /** The account, for its follow button. */
  account: Account;
};

/** The profile text (sanitized HTML from the server) as plain text on one line: its paragraphs and line breaks become spaces. */
function noteLineOf(html: string): string {
  if (html === "") return "";
  return sanitizedHtmlText(serverSanitizedHtml(html)).replace(/\s+/gu, " ").trim();
}

export type FollowListState = {
  /** The loaded accounts, newest follow first; read from the query cache on every render (S2). */
  items: readonly FollowListItem[];
  hasMore: boolean;
  isLoadingMore: boolean;
  /** Loads the next page. A failure is announced and kept in `loadMoreError` until the next try. */
  loadMore: () => void;
  /** Why the last page failed to load (localized), or null. */
  loadMoreError: string | null;
};

/**
 * Reads the followers or the following of the account `id`, whose first page the route loader has
 * fetched, and loads the next pages on demand. The first page's failure goes to the route's error
 * boundary; a later page's failure stays here with the accounts already shown (web-client/README.md E1, E2).
 */
export function useFollowList(id: string, list: FollowList): FollowListState {
  const api = useApi();
  const t = useT();
  const notice = useNotice();
  const query = useSuspenseInfiniteQuery(followListQueryOptions(api, id, list));

  const loadMore = () => {
    if (!query.hasNextPage || query.isFetchingNextPage) return;
    void query.fetchNextPage().then((result) => {
      if (result.isFetchNextPageError) {
        notice.inPlace(t("client:profile.list.load_more_failed", { reason: errorMessage(result.error) }), "assertive");
      }
    });
  };

  // React Compiler derives the rows again only when the pages change (web-client/README.md R4), so the profile texts
  // are parsed once per page.
  const items = query.data.pages
    .flatMap((page) => page.accounts)
    .map((account) => ({
      id: account.id,
      name: nameOf(account),
      handle: handleOf(account.acct),
      href: profilePath(account.acct),
      noteLine: noteLineOf(account.note),
      account,
    }));

  return {
    items,
    hasMore: query.hasNextPage,
    isLoadingMore: query.isFetchingNextPage,
    loadMore,
    loadMoreError: query.isFetchNextPageError ? errorMessage(query.error) : null,
  };
}
