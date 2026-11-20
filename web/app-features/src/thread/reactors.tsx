// SPDX-License-Identifier: AGPL-3.0-only

// Who reacted to a post (FR-116): under the focused post of its page, a row per emoji with the first faces of who
// reacted with it, each a link to that emoji's list (WhoReacted of @my/app-components), from the newest reactions the
// server answers; and that list on a screen of its own, under a button back to the post, titled by the emoji and how
// many reacted with it, the accounts newest first a row each, then a line that they load, that they failed (with a
// retry), that there are none, or a fold that shows more while more follow.

import { AccountLine, ReactorsTitle, WhoReacted, type Reactors } from "@my/app-components";
import {
  statusQuery,
  statusReactorsQuery,
  useStatus,
  useStatusGone,
  useStatusReactors,
  type AppData,
  type Status,
} from "@my/app-data";
import { BackButton, FeedNotice, ScreenStack, ShowMore, TextButton, useAddedFocus, useLocale } from "@my/design-system";
import { useEffect, useRef } from "react";
import { stringsIn } from "../locale";

/**
 * What the route of one emoji's list waits for: the post and the first page of who reacted with `emoji`. A failure is
 * left to the screen, which shows it with a retry.
 */
export async function loadReactors(data: AppData, id: string, emoji: string): Promise<void> {
  const { queryClient } = data;
  await Promise.all([
    queryClient.query({ ...statusQuery(data, id), staleTime: Infinity }),
    queryClient.infiniteQuery({ ...statusReactorsQuery(data, id, emoji), staleTime: Infinity }),
  ]).catch(() => undefined);
}

/**
 * The rows of who reacted to `status`, a row per emoji it has, each with the faces of who reacted with it among the
 * newest reactions; none while it has no reaction.
 */
export function PostReactors({ status }: { status: Status }) {
  const has = status.reactions.length > 0;
  const { items } = useStatusReactors(status.id, undefined, has);
  if (!has) return null;
  const reactors = status.reactions.map((reaction): Reactors => ({
    reaction,
    accounts: items.filter((r) => r.emoji.name === reaction.name).map((r) => r.account),
  }));
  return <WhoReacted status={status} reactors={reactors} />;
}

/**
 * The list of who reacted to the post of `id` with `emoji` (a reaction's name); `onBack` goes back to the post, and
 * once, on its own, when the post is gone (the reader deleted it, or the stream said so).
 */
export function ReactorList({ id, emoji, onBack }: { id: string; emoji: string; onBack: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  const { status, failed } = useStatus(id);
  const list = useStatusReactors(id, emoji);
  const added = useAddedFocus(list.items.length);
  const gone = useStatusGone(id);
  const left = useRef(false);
  useEffect(() => {
    // Once: a render before the list unmounts must not go back a second step.
    if (!gone || left.current) return;
    left.current = true;
    onBack();
  }, [gone, onBack]);
  const back = <BackButton onPress={onBack}>{t("thread.reactors_back")}</BackButton>;
  if (status === undefined)
    return (
      <ScreenStack>
        {back}
        {failed ? <FeedNotice alert>{t("thread.failed")}</FeedNotice> : <FeedNotice>{t("thread.loading")}</FeedNotice>}
      </ScreenStack>
    );
  const reaction = status.reactions.find((r) => r.name === emoji) ?? { name: emoji, count: list.items.length };
  const retry = (onPress: () => void) => <TextButton onPress={onPress}>{t("thread.retry")}</TextButton>;
  const end =
    list.status === "pending" ? (
      <FeedNotice>{t("thread.reactors_loading")}</FeedNotice>
    ) : list.status === "error" && !list.failedNextPage ? (
      <FeedNotice alert action={retry(list.refetch)}>
        {t("thread.reactors_failed")}
      </FeedNotice>
    ) : list.items.length === 0 ? (
      <FeedNotice>{t("thread.reactors_empty")}</FeedNotice>
    ) : (
      <ShowMore
        more={list.hasNextPage}
        busy={list.isFetchingNextPage}
        failed={list.failedNextPage}
        onPress={added.press(list.fetchNextPage)}
      />
    );
  return (
    <ScreenStack>
      {back}
      <ReactorsTitle reaction={reaction} count={reaction.count} />
      {list.items.map((r, i) => (
        <AccountLine key={r.account.id} account={r.account} itemRef={added.item(i)} />
      ))}
      {end}
    </ScreenStack>
  );
}
