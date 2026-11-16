// SPDX-License-Identifier: AGPL-3.0-only

// The home timeline (ADR-0025 §2, §4): the column of @my/app-components fed by useHomeTimeline() of @my/app-data, and
// what a route's loader reads before the column shows (ADR-0028 §6). Its new posts follow the policy of new-posts.ts,
// with `timeline_auto_insert` at its default, on (ADR-0013 §22), until the reader's preferences are read. A post's
// actions are answered as on every screen (post-actions.tsx). Empty, as a new account's is right after it signed up,
// it offers people to follow (people.tsx).

import { Column } from "@my/app-components";
import { homeTimelineQuery, useHomeTimeline, type AppData } from "@my/app-data";
import { useLocale } from "@my/design-system";
import { stringsIn } from "../locale";
import { SuggestedPeople } from "../people/people";
import { usePostActions, usePostOwn, usePostPicker, usePostVoting } from "../post-actions";
import { TranslatablePost } from "../post-translation";
import { useNewPostsColumn } from "./new-posts";

/**
 * What the route's loader waits for: the home timeline's first page, from the cache unless a write marked it stale. A
 * failure is left to the column, which shows it with a retry.
 */
export async function loadHomeTimeline(data: AppData): Promise<void> {
  await data.queryClient.infiniteQuery({ ...homeTimelineQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The home timeline's column. */
export function HomeTimeline() {
  const { t } = stringsIn(useLocale().locale);
  const onAction = usePostActions();
  const votingOf = usePostVoting();
  const ownOf = usePostOwn();
  const picker = usePostPicker();
  return (
    <Column
      label={t("timeline.home")}
      {...useHomeTimeline()}
      {...useNewPostsColumn("home")}
      onAction={onAction}
      postProps={(s) => ({ voting: votingOf(s), own: ownOf(s), picker })}
      postAs={TranslatablePost}
      // A new account follows no one yet: its empty home offers people to follow (FR-308).
      empty={<SuggestedPeople intro={t("people.home")} />}
    />
  );
}
