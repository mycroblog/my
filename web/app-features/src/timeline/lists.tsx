// SPDX-License-Identifier: AGPL-3.0-only

// The reader's lists (FR-211, ADR-0013 §15): the index of the lists they made under its title, oldest first, each a row that opens
// its timeline with how many accounts are in it; and a list's timeline, under the back button to the index, the list
// as the screen's title with how many, then its posts in the column every timeline has, new ones coming by the stream
// as the home timeline's do; a list that failed to load says so with Try again, and one deleted, that it is gone.
// Making, renaming and filling
// lists is list management's, which neither shows a control of until it exists.

import { Column } from "@my/app-components";
import {
  listQuery,
  listsQuery,
  listTimelineQuery,
  useList,
  useLists,
  useListTimeline,
  type AppData,
  type OneList,
} from "@my/app-data";
import { BackButton, FeedNotice, OpenRow, ScreenStack, ScreenTitle, TextButton, useLocale } from "@my/design-system";
import { stringsIn } from "../locale";
import { usePostActions, usePostOwn, usePostPicker, usePostVoting } from "../post-actions";
import { TranslatablePost } from "../post-translation";
import { useNewPostsColumn } from "./new-posts";

/** What the index's loader waits for: the reader's lists, from the cache unless a write marked them stale. */
export async function loadLists(data: AppData): Promise<void> {
  await data.queryClient.query({ ...listsQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** What a list's loader waits for: the list and its first page. */
export async function loadListTimeline(data: AppData, id: string): Promise<void> {
  await Promise.all([
    data.queryClient.query({ ...listQuery(data, id), staleTime: Infinity }),
    data.queryClient.infiniteQuery({ ...listTimelineQuery(data, id), staleTime: Infinity }),
  ]).catch(() => undefined);
}

/** The reader's lists under the screen's title, each a row that opens its timeline. */
export function ListIndex() {
  const { t } = stringsIn(useLocale().locale);
  return (
    <ScreenStack>
      <ScreenTitle glyph="list">{t("timeline.lists_title")}</ScreenTitle>
      <ListRows />
    </ScreenStack>
  );
}

/** The rows of the reader's lists, or what stands in their place: that they load, failed with Try again, or are none. */
function ListRows() {
  const { t } = stringsIn(useLocale().locale);
  const { lists, status, refetch } = useLists();
  if (status === "pending") return <FeedNotice>{t("timeline.lists_loading")}</FeedNotice>;
  if (status === "error")
    return (
      <FeedNotice alert action={<TextButton onPress={refetch}>{t("timeline.retry")}</TextButton>}>
        {t("timeline.lists_failed")}
      </FeedNotice>
    );
  if (lists.length === 0) return <FeedNotice>{t("timeline.lists_empty")}</FeedNotice>;
  return (
    <nav aria-label={t("timeline.lists")}>
      <ul className="m-0 grid list-none p-0">
        {lists.map((list) => (
          <li key={list.id}>
            <OpenRow
              href={`/lists/${encodeURIComponent(list.id)}`}
              icon="list"
              title={list.name}
              hint={t("timeline.list_members", { count: list.members_count })}
            />
          </li>
        ))}
      </ul>
    </nav>
  );
}

/** What a list's screen says until the list is read: that it loads, that it failed with Try again, or that it is gone. */
function ListNotice({ status, refetch }: { status: OneList["status"]; refetch: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  if (status === "gone") return <FeedNotice>{t("timeline.list_gone")}</FeedNotice>;
  if (status === "error")
    return (
      <FeedNotice alert action={<TextButton onPress={refetch}>{t("timeline.retry")}</TextButton>}>
        {t("timeline.list_failed")}
      </FeedNotice>
    );
  return <FeedNotice>{t("timeline.list_loading")}</FeedNotice>;
}

/**
 * The timeline of the reader's list of `id`, whose new posts come by the stream as the home timeline's do; `onBack`
 * goes back to the index.
 */
export function ListTimeline({ id, onBack }: { id: string; onBack: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  const { list, status, refetch } = useList(id);
  const onAction = usePostActions();
  const votingOf = usePostVoting();
  const ownOf = usePostOwn();
  const picker = usePostPicker();
  const timeline = useListTimeline(id);
  const newPosts = useNewPostsColumn({ list: id });
  const back = <BackButton onPress={onBack}>{t("timeline.list_back")}</BackButton>;
  if (list === undefined)
    return (
      <ScreenStack>
        {back}
        <ListNotice status={status} refetch={refetch} />
      </ScreenStack>
    );
  return (
    <ScreenStack>
      <ScreenTitle back={back} glyph="list" count={t("timeline.list_members", { count: list.members_count })}>
        {list.name}
      </ScreenTitle>
      <Column
        label={t("timeline.list", { name: list.name })}
        {...timeline}
        {...newPosts}
        onAction={onAction}
        postProps={(s) => ({ voting: votingOf(s), own: ownOf(s), picker })}
        postAs={TranslatablePost}
        empty={<FeedNotice>{t("timeline.list_empty")}</FeedNotice>}
      />
    </ScreenStack>
  );
}
