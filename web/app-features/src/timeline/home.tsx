// SPDX-License-Identifier: AGPL-3.0-only

// The home timeline (ADR-0025 §2, §4): the column of @my/app-components fed by useHomeTimeline() of @my/app-data, and
// what a route's loader reads before the column shows (ADR-0028 §6). Its new posts follow the policy of new-posts.ts,
// with `timeline_auto_insert` at its default, on (ADR-0013 §22), until the reader's preferences are read. A post's
// actions are answered as on every screen (post-actions.tsx). While the reader follows no one, as a new account right
// after it signed up, it offers people to follow over its posts (people.tsx).

import { useState } from "react";
import { Column } from "@my/app-components";
import {
  homeTimelineQuery,
  loadHomeSuggestions,
  useHomeSuggestions,
  useHomeTimeline,
  type AppData,
  type HomeSuggestions,
} from "@my/app-data";
import { ScreenStack, useLocale } from "@my/design-system";
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
  await Promise.all([
    data.queryClient.infiniteQuery({ ...homeTimelineQuery(data), staleTime: Infinity }).catch(() => undefined),
    loadHomeSuggestions(data),
  ]);
}

/** The home timeline's column. */
export function HomeTimeline() {
  const { t } = stringsIn(useLocale().locale);
  const onAction = usePostActions();
  const votingOf = usePostVoting();
  const ownOf = usePostOwn();
  const picker = usePostPicker();
  const newPosts = useNewPostsColumn("home");
  // A reader who follows no one yet (a new account) is offered people to follow over the column (FR-308), where the
  // server suggests anyone. The column waits, as in its loading, until that is known, so that nothing arrives over its
  // posts once they show.
  // It is decided once, when everything it depends on is first known, and holds while the reader stays on Home: a
  // later answer (the viewer read again, the suggestions' list) never adds or takes the block away under a press.
  const live = useHomeSuggestions();
  const [decided, decide] = useState<HomeSuggestions>();
  if (decided === undefined && live !== "pending") decide(live);
  const offered = decided ?? "pending";
  const timeline = useHomeTimeline();
  const column = (
    <Column
      label={t("timeline.home")}
      {...timeline}
      {...(offered === "pending" ? { statuses: [], status: "pending" as const } : {})}
      {...newPosts}
      onAction={onAction}
      postProps={(s) => ({ voting: votingOf(s), own: ownOf(s), picker })}
      postAs={TranslatablePost}
    />
  );
  if (offered === "none") return column;
  return (
    <ScreenStack>
      <SuggestedPeople intro={t("people.home")} shown={offered === "some"} />
      {column}
    </ScreenStack>
  );
}
