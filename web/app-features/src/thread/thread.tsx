// SPDX-License-Identifier: AGPL-3.0-only

// A post on its own page, in its thread (FR-106, the rough of #3047), from useThread() of @my/app-data: the thread's
// first post, then a fold of the posts between it and the two the focused post replies to closest, then those two;
// the focused post, a step larger, with when it was posted, to whom and its counts inside it (PostDetails) in place of
// the stamp at the end of its author's line; then its replies, each indented by how deep it replies, the first of them
// and a fold of the rest. All of it is one feed, so that the keys of the next and the previous entry run through it.
// A post's actions are not connected yet, as on the home timeline.

import { FeedKeys, Post, PostDetails } from "@my/app-components";
import { statusContextQuery, statusQuery, useThread, type AppData, type Status } from "@my/app-data";
import {
  BackButton,
  Feed,
  FeedItem,
  FeedNotice,
  Fold,
  GroupHeading,
  ScreenStack,
  TextButton,
  ThreadFocus,
  ThreadReply,
  useLocale,
} from "@my/design-system";
import { useState, type ReactNode } from "react";
import { stringsIn } from "../locale";

/** How many ancestors show around the fold: the first, and as many closest to the focused post. */
const closest = 2;
/** How many replies show at first, and how many more each press of the fold shows. */
const repliesShown = 20;

/**
 * What the route's loader waits for: the post and its thread, from the cache unless a write marked them stale. A
 * failure is left to the screen, which shows it with a retry.
 */
export async function loadThread(data: AppData, id: string): Promise<void> {
  const { queryClient } = data;
  await Promise.all([
    queryClient.query({ ...statusQuery(data, id), staleTime: Infinity }),
    queryClient.query({ ...statusContextQuery(data, id), staleTime: Infinity }),
  ]).catch(() => undefined);
}

/** How deep each reply replies under the focused post: 1 for a reply to it, and one more for each step away. */
function depths(focused: string, descendants: readonly Status[]): ReadonlyMap<string, number> {
  const depth = new Map<string, number>([[focused, 0]]);
  for (const s of descendants) {
    const parent = s.in_reply_to_id === undefined ? undefined : depth.get(s.in_reply_to_id);
    depth.set(s.id, (parent ?? 0) + 1);
  }
  return depth;
}

/** The post of `id` in its thread; `onBack` goes back to where the reader came from. */
export function Thread({ id, onBack }: { id: string; onBack: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  const { status, context, error, refetch } = useThread(id);
  const [earlier, setEarlier] = useState(false);
  const [shown, setShown] = useState(repliesShown);
  const back = <BackButton onPress={onBack}>{t("thread.back")}</BackButton>;
  if (status === undefined)
    return (
      <ScreenStack>
        {back}
        {error === null ? (
          <FeedNotice>{t("thread.loading")}</FeedNotice>
        ) : (
          <FeedNotice alert action={<TextButton onPress={refetch}>{t("thread.retry")}</TextButton>}>
            {t("thread.failed")}
          </FeedNotice>
        )}
      </ScreenStack>
    );
  const ancestors = context?.ancestors ?? [];
  const descendants = context?.descendants ?? [];
  const folded = !earlier && ancestors.length > closest + 1 ? ancestors.length - closest - 1 : 0;
  const first = folded > 0 ? ancestors.slice(0, 1) : ancestors;
  const last = folded > 0 ? ancestors.slice(1 + folded) : [];
  const depth = depths(status.id, descendants);
  const replies = descendants.slice(0, shown);
  // Each entry's position in the feed, in the order they show: the ancestors shown, the focused post, the replies.
  const positions = new Map([...first, ...last, status, ...replies].map((s, i) => [s.id, i + 1]));
  const item = (s: Status, children?: ReactNode) => (
    <FeedItem key={s.id} id={s.id} position={positions.get(s.id) ?? 0}>
      {children ?? <Post status={s} />}
    </FeedItem>
  );
  const count = positions.size;
  return (
    <ScreenStack>
      {back}
      <Feed
        label={t("thread.label")}
        count={count}
        more={folded > 0 || replies.length < descendants.length}
        busy={context === undefined}
        end={
          context !== undefined && descendants.length === 0 ? (
            <FeedNotice>{t("thread.no_replies")}</FeedNotice>
          ) : undefined
        }
      >
        <FeedKeys />
        {first.map((s) => item(s))}
        {folded > 0 && (
          <Fold key="earlier">
            <TextButton
              onPress={() => {
                setEarlier(true);
              }}
            >
              {t("thread.earlier", { count: folded })}
            </TextButton>
          </Fold>
        )}
        {last.map((s) => item(s))}
        <ThreadFocus key="focus">
          {item(status, <Post status={status} stamp={false} details={<PostDetails status={status} />} />)}
        </ThreadFocus>
        {replies.length > 0 && <GroupHeading key="replies">{t("thread.replies")}</GroupHeading>}
        {replies.map((s) => (
          <ThreadReply key={s.id} depth={depth.get(s.id) ?? 1}>
            {item(s)}
          </ThreadReply>
        ))}
        {replies.length < descendants.length && (
          <Fold key="later">
            <TextButton
              onPress={() => {
                setShown(shown + repliesShown);
              }}
            >
              {t("thread.more_replies", { count: descendants.length - replies.length })}
            </TextButton>
          </Fold>
        )}
      </Feed>
    </ScreenStack>
  );
}
