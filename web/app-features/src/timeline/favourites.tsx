// SPDX-License-Identifier: AGPL-3.0-only

// The reader's favourites (FR-107), a place of the navigation: the posts they favourited, the newest favourite first,
// in the column every timeline has; a post whose favourite they take back leaves it at once. A post's actions are
// answered as on every screen (post-actions.tsx).

import { Column } from "@my/app-components";
import { favouritesQuery, useFavourites, type AppData } from "@my/app-data";
import { FeedNotice, useLocale } from "@my/design-system";
import { stringsIn } from "../locale";
import { usePostActions, usePostOwn, usePostPicker, usePostVoting } from "../post-actions";
import { TranslatablePost } from "../post-translation";

/** What the route's loader waits for: the first page of favourites, from the cache unless a write marked it stale. */
export async function loadFavourites(data: AppData): Promise<void> {
  await data.queryClient.infiniteQuery({ ...favouritesQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The column of the reader's favourites. */
export function Favourites() {
  const { t } = stringsIn(useLocale().locale);
  const onAction = usePostActions();
  const votingOf = usePostVoting();
  const ownOf = usePostOwn();
  const picker = usePostPicker();
  return (
    <Column
      label={t("timeline.favourites")}
      {...useFavourites()}
      onAction={onAction}
      postProps={(s) => ({ voting: votingOf(s), own: ownOf(s), picker })}
      postAs={TranslatablePost}
      empty={<FeedNotice>{t("timeline.favourites_empty")}</FeedNotice>}
    />
  );
}
