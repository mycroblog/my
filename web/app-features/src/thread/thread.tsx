// SPDX-License-Identifier: AGPL-3.0-only

// A post on its own page, in its thread (FR-106, the rough of #3047), from useThread() of @my/app-data: the thread's
// first post, then a fold of the posts between it and the two the focused post replies to closest, then those two;
// the focused post, a step larger, with when it was posted and to whom inside it (PostDetails) in place of
// the stamp at the end of its author's line; then its replies, each indented by how deep it replies, the first of them
// and a fold of the rest. All of it is one feed, so that the keys of the next and the previous entry run through it;
// a feed owns its articles alone, so the replies' heading stands in the first reply, above it, and each fold in the
// entry before it. Who reacted to the focused post stands inside it, after its actions (reactors.tsx).
// A post's actions are answered as on every screen (post-actions.tsx). An article (ADR-0160) focused here is its page:
// its text in full, its replies under it as a thread's. A visitor reads it too, and the actions of its posts that need
// an account lead them to sign in, coming back to the post they pressed one on. A post the reader may not see is not
// found, as the server answers it.

import { FeedKeys, PostContent, PostDetails, statusPath, type PostStatus } from "@my/app-components";
import { ProblemError } from "@mycroblog/mykit";
import { statusContextQuery, statusQuery, useStatusGone, useThread, type AppData, type Status } from "@my/app-data";
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
import { useEffect, useRef, useState, type ReactNode } from "react";
import { stringsIn } from "../locale";
import { usePostActions, usePostOwn, usePostPicker, usePostVoting } from "../post-actions";
import { TranslatablePost as Post } from "../post-translation";
import { PostReactors } from "./reactors";

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

/**
 * The post of `id` in its thread; `onBack` goes back to where the reader came from. With `signIn`, for a visitor, the
 * actions of its posts that need an account lead to sign in, given the address of the post to come back to.
 */
export function Thread({
  id,
  onBack,
  signIn,
}: {
  id: string;
  onBack: () => void;
  signIn?: ((returnTo: string) => void) | undefined;
}) {
  const { t } = stringsIn(useLocale().locale);
  const { status, context, error, refetch } = useThread(id);
  const [earlier, setEarlier] = useState(false);
  const [shown, setShown] = useState(repliesShown);
  const onAction = usePostActions();
  const votingOf = usePostVoting();
  const ownOf = usePostOwn();
  // The post of the page gone (the reader deleted it, or the stream said so): the page has nothing left to show, so it
  // goes back, where the notice of the deletion stays.
  const gone = useStatusGone(id);
  const left = useRef(false);
  useEffect(() => {
    // Once: a render before the page unmounts must not go back a second step.
    if (!gone || left.current) return;
    left.current = true;
    onBack();
  }, [gone, onBack]);
  const picker = usePostPicker();
  // What a post's actions are given: for a visitor, the sign-in in place of what is the reader's own.
  const reader = (s: Status) =>
    signIn === undefined
      ? { voting: votingOf(s), own: ownOf(s), picker }
      : {
          signIn: (shown: PostStatus) => {
            signIn(statusPath(shown));
          },
        };
  const back = <BackButton onPress={onBack}>{t("thread.back")}</BackButton>;
  const missing = error instanceof ProblemError && (error.status === 404 || error.status === 410);
  if (status === undefined)
    return (
      <ScreenStack>
        {back}
        {error === null ? (
          <FeedNotice>{t("thread.loading")}</FeedNotice>
        ) : missing ? (
          <FeedNotice>{t("thread.not_found")}</FeedNotice>
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
      {children ?? <Post status={s} onAction={onAction} {...reader(s)} />}
    </FeedItem>
  );
  const count = positions.size;
  // The feed owns its articles alone: the folds and the replies' heading stand inside the entries beside them.
  const earlierFold = folded > 0 && (
    <Fold>
      <TextButton
        onPress={() => {
          setEarlier(true);
        }}
      >
        {t("thread.earlier", { count: folded })}
      </TextButton>
    </Fold>
  );
  const laterFold = replies.length < descendants.length && (
    <Fold>
      <TextButton
        onPress={() => {
          setShown(shown + repliesShown);
        }}
      >
        {t("thread.more_replies", { count: descendants.length - replies.length })}
      </TextButton>
    </Fold>
  );
  const lastReply = replies.at(-1);
  // An article reads in full on its page (GET /statuses/{id} gives its text; a list gives none), and the replies right
  // under it do not say whom they reply to, which the page shows already.
  const article = status.article !== undefined;
  const reading = article ? (
    <PostContent html={status.content} emojis={status.emojis} lang={undefined} article />
  ) : undefined;
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
        {first.map((s) =>
          item(s, <Post status={s} onAction={onAction} {...reader(s)} after={earlierFold || undefined} />),
        )}
        {last.map((s) => item(s))}
        <ThreadFocus key="focus">
          {item(
            status,
            <Post
              status={status}
              reading={reading}
              stamp={false}
              details={
                <PostDetails
                  status={status}
                  onHistory={() => {
                    onAction({ type: "history", id: status.id });
                  }}
                />
              }
              onAction={onAction}
              {...reader(status)}
              after={<PostReactors status={status} />}
            />,
          )}
        </ThreadFocus>
        {replies.map((s, i) => (
          <ThreadReply key={s.id} depth={depth.get(s.id) ?? 1}>
            {item(
              s,
              <Post
                status={s}
                onAction={onAction}
                {...reader(s)}
                context={i === 0 ? <GroupHeading>{t("thread.replies")}</GroupHeading> : undefined}
                replyingTo={!(article && s.in_reply_to_id === status.id)}
                after={(s === lastReply && laterFold) || undefined}
              />,
            )}
          </ThreadReply>
        ))}
      </Feed>
    </ScreenStack>
  );
}
