// SPDX-License-Identifier: AGPL-3.0-only

// The timeline of a hashtag (FR-203), reached from a hashtag in a post: under the back button, the hashtag as the
// screen's title, with following it at the end of its row once the server said whether the reader does; then its
// posts, newest first, in the column every timeline has. The name comes from the address, so the title shows while
// the rest loads or fails. No stream brings its new posts: it is read as it opens (ADR-0013 §3). A post's actions are
// answered as on every screen (post-actions.tsx), and a follow that fails is told in their notice. A visitor reads it
// too, and Follow, like their posts' actions that need an account, leads them to sign in, coming back here.

import { Column, statusPath, tagPath, VoiceRoomLine, type PostStatus } from "@my/app-components";
import {
  tagTimelineQuery,
  useFollowTag,
  useInstance,
  useTag,
  useTagTimeline,
  useVoiceRooms,
  type AppData,
} from "@my/app-data";
import {
  BackButton,
  Button,
  FeedNotice,
  GroupHeading,
  RowItem,
  RowList,
  ScreenStack,
  ScreenTitle,
  Stack,
  useLocale,
} from "@my/design-system";
import { useState } from "react";
import { stringsIn } from "../locale";
import { usePostActions, usePostNotice, usePostOwn, usePostPicker, usePostVoting } from "../post-actions";
import { TranslatablePost } from "../post-translation";

/** What the route's loader waits for: the hashtag's first page, from the cache unless a write marked it stale. */
export async function loadTagTimeline(data: AppData, hashtag: string): Promise<void> {
  await data.queryClient
    .infiniteQuery({ ...tagTimelineQuery(data, hashtag), staleTime: Infinity })
    .catch(() => undefined);
}

/**
 * Following the hashtag of `name`, in its state, once the server said it; with `signIn`, for a visitor, leading to
 * sign in instead.
 */
function Follow({ name, signIn }: { name: string; signIn: ((returnTo: string) => void) | undefined }) {
  const { t } = stringsIn(useLocale().locale);
  const { tag } = useTag(name);
  const follow = useFollowTag();
  const failed = usePostNotice();
  const [busy, setBusy] = useState(false);
  if (signIn !== undefined)
    return (
      <Button
        size="timeline"
        kind="primary"
        onPress={() => {
          signIn(tagPath(name));
        }}
      >
        {t("timeline.tag_follow")}
      </Button>
    );
  if (tag === undefined) return null;
  const press = (on: boolean) => {
    setBusy(true);
    follow(name, on)
      .catch((e: unknown) => {
        failed(e, t("timeline.tag_follow_failed", { follow: on ? "follow" : "unfollow", name: tag.name }), () => {
          press(on);
        });
      })
      .finally(() => {
        setBusy(false);
      });
  };
  return (
    <Button
      size="timeline"
      kind={tag.following ? "secondary" : "primary"}
      busy={busy}
      onPress={() => {
        if (!busy) press(!tag.following);
      }}
    >
      {t(tag.following ? "timeline.tag_following" : "timeline.tag_follow")}
    </Button>
  );
}

/**
 * The open voice chat rooms of the hashtag of `name` the reader sees (ADR-0100 §8), over its posts: the first page,
 * in the server's order; nothing while there is none, for a visitor, or while the voice chat is off.
 */
function TagRooms({ name, signedIn }: { name: string; signedIn: boolean }) {
  const { t } = stringsIn(useLocale().locale);
  const on = useInstance()?.voice_enabled === true;
  const { items } = useVoiceRooms(name, signedIn && on);
  if (items.length === 0) return null;
  return (
    <Stack gap="entry-line">
      <GroupHeading>{t("timeline.tag_rooms")}</GroupHeading>
      <RowList label={t("timeline.tag_rooms")}>
        {items.map((room) => (
          <RowItem key={room.id}>
            <VoiceRoomLine room={room} />
          </RowItem>
        ))}
      </RowList>
    </Stack>
  );
}

/**
 * The screen of the hashtag of `name` (without `#`); `onBack` goes back to where the reader came from. With `signIn`,
 * for a visitor, Follow and the actions of its posts that need an account lead to sign in, given the address to come
 * back to.
 */
export function TagTimeline({
  name,
  onBack,
  signIn,
}: {
  name: string;
  onBack: () => void;
  signIn?: ((returnTo: string) => void) | undefined;
}) {
  const { t } = stringsIn(useLocale().locale);
  const { tag } = useTag(name);
  const onAction = usePostActions();
  const votingOf = usePostVoting();
  const ownOf = usePostOwn();
  const picker = usePostPicker();
  const shown = tag?.name ?? name;
  const visitor =
    signIn === undefined
      ? undefined
      : (status: PostStatus) => {
          signIn(statusPath(status));
        };
  return (
    <ScreenStack>
      <ScreenTitle
        back={<BackButton onPress={onBack}>{t("timeline.tag_back")}</BackButton>}
        actions={<Follow name={name} signIn={signIn} />}
      >{`#${shown}`}</ScreenTitle>
      <TagRooms name={name} signedIn={signIn === undefined} />
      <Column
        label={t("timeline.tag", { name: shown })}
        {...useTagTimeline(name)}
        onAction={onAction}
        postProps={(s) =>
          visitor === undefined ? { voting: votingOf(s), own: ownOf(s), picker } : { signIn: visitor }
        }
        postAs={TranslatablePost}
        empty={<FeedNotice>{t("timeline.tag_empty", { name: shown })}</FeedNotice>}
      />
    </ScreenStack>
  );
}
