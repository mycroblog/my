// SPDX-License-Identifier: AGPL-3.0-only

// The reader's bookmarks (FR-113), a place of the navigation: the posts they bookmarked, the newest bookmark first, in
// the column every timeline has; a post whose bookmark they take back leaves it at once. A post's actions are answered
// as on every screen (post-actions.tsx).

import { Column } from "@my/app-components";
import { bookmarksQuery, useBookmarks, type AppData } from "@my/app-data";
import { FeedNotice, useLocale } from "@my/design-system";
import { stringsIn } from "../locale";
import { usePostActions, usePostOwn, usePostPicker, usePostVoting } from "../post-actions";
import { TranslatablePost } from "../post-translation";

/** What the route's loader waits for: the first page of bookmarks, from the cache unless a write marked it stale. */
export async function loadBookmarks(data: AppData): Promise<void> {
  await data.queryClient.infiniteQuery({ ...bookmarksQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The column of the reader's bookmarks. */
export function Bookmarks() {
  const { t } = stringsIn(useLocale().locale);
  const onAction = usePostActions();
  const votingOf = usePostVoting();
  const ownOf = usePostOwn();
  const picker = usePostPicker();
  return (
    <Column
      label={t("timeline.bookmarks")}
      {...useBookmarks()}
      onAction={onAction}
      postProps={(s) => ({ voting: votingOf(s), own: ownOf(s), picker })}
      postAs={TranslatablePost}
      empty={<FeedNotice>{t("timeline.bookmarks_empty")}</FeedNotice>}
    />
  );
}
