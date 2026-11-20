// SPDX-License-Identifier: AGPL-3.0-only

// Follow requests (#3125, FR-302) on a screen of their own, apart from what waits for the reader's approval (#3113),
// in the reader's language (the toolbar's locale; ja, else en): over the notifications, a row for each screen, the
// follow requests first; the screen, under its back link to the notifications, with a request answered in place and
// the rest waiting for Decline or Approve; and its states (none, loading, failed). The words are placeholders of the
// feature's.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useId, useState, type ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { Avatar } from "./avatar";
import { Button, TextButton } from "./button";
import { Entry } from "./entry";
import { FeedNotice } from "./feed";
import { IconButton } from "./icon-button";
import { AccountRow, BackButton, NoticeLine, OpenRow, ScreenStack, ScreenTitle, StateText, TabRow } from "./list";
import { ago, images } from "./samples";

const rem = (px: number) => `${px / rootPx}rem`;
const minute = 60_000;

type Lang = "en" | "ja";
const useLang = (): Lang => (new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en");

const words = {
  en: {
    notifications: "Notifications",
    all: "All",
    mentions: "Mentions",
    filters: "Show",
    readAll: "Mark all as read",
    requests: "Follow requests",
    requestsCount: (n: number) => `${String(n)} follow ${n === 1 ? "request" : "requests"}`,
    waitingCount: (n: number) => `${String(n)} waiting for your approval`,
    held: "3 held-back notifications · from 2 people",
    heldHint: "From people you may not know",
    decline: "Decline",
    approve: "Accept",
    approved: "Accepted",
    declined: "Declined",
    empty: "No follow requests.",
    loading: "Loading the follow requests…",
    failed: "The follow requests could not be loaded.",
    retry: "Try again",
    favourited: (name: string) => `${name} favourited your post`,
    followed: (name: string) => `${name} followed you`,
  },
  ja: {
    notifications: "通知",
    all: "すべて",
    mentions: "メンション",
    filters: "表示",
    readAll: "すべて既読にする",
    requests: "フォローリクエスト",
    requestsCount: (n: number) => `フォローリクエスト ${String(n)}件`,
    waitingCount: (n: number) => `承認待ち ${String(n)}件`,
    held: "保留中の通知 3件 · 2人から",
    heldHint: "知らないかもしれない人から",
    decline: "断る",
    approve: "承認",
    approved: "承認しました",
    declined: "断りました",
    empty: "フォローリクエストはありません。",
    loading: "フォローリクエストを読み込んでいます…",
    failed: "フォローリクエストを読み込めませんでした。",
    retry: "もう一度試す",
    favourited: (name: string) => `${name}さんがあなたの投稿をお気に入りに登録しました`,
    followed: (name: string) => `${name}さんにフォローされました`,
  },
} as const;

type W = (typeof words)[Lang];

type Person = { name: string; handle: string; avatar?: string | undefined };
const people: Person[] = [
  { name: "Mika", handle: "@mika@my.example", avatar: images.dusk },
  { name: "Haruto", handle: "@haruto@photos.example", avatar: images.snow },
  { name: "Aoi Tanaka", handle: "@aoi@my.example" },
  {
    name: "Saoirse with a display name long enough to run out of room",
    handle: "@saoirse_on_a_very_long_handle@photos.example",
  },
];

/** A column of the timeline's width, with its side margin. */
const Column = ({ children }: { children: ReactNode }) => (
  <div style={{ maxWidth: rem(600), paddingInline: rem(16), paddingBlock: rem(16) }}>{children}</div>
);

/** A notification of the feed under the rows. */
function Notice({ text, icon, at }: { text: string; icon: "favourite" | "follow"; at: number }) {
  const id = useId();
  return (
    <Entry labelledBy={id}>
      <NoticeLine icon={icon} avatar={<Avatar name="Ren" size="inline" decorative />} date={ago(at)} id={id}>
        {text}
      </NoticeLine>
    </Entry>
  );
}

/** The top of the notifications: the tabs, a row per screen of what waits (follow requests first), the feed. */
function InNotifications() {
  const w = words[useLang()];
  const [tab, setTab] = useState<"all" | "mentions">("all");
  return (
    <Column>
      <ScreenStack>
        <TabRow
          label={w.filters}
          tabs={[
            { id: "all", name: w.all },
            // A tab with things waiting under it shows their count, as the navigation's badge.
            { id: "mentions", name: w.mentions, count: 3 },
          ]}
          current={tab}
          onChange={setTab}
          end={<IconButton icon="read" label={w.readAll} />}
        >
          <ScreenStack>
            <div>
              <OpenRow href="#requests" icon="follow" title={w.requestsCount(3)} />
              <OpenRow href="#waiting" icon="time" title={w.waitingCount(2)} />
              <OpenRow href="#held" icon="followers" title={w.held} hint={w.heldHint} />
            </div>
            <Notice text={w.favourited("Ren")} icon="favourite" at={5 * minute} />
            <Notice text={w.followed("Lina")} icon="follow" at={40 * minute} />
          </ScreenStack>
        </TabRow>
      </ScreenStack>
    </Column>
  );
}

/** A request: who asks, with Decline beside Approve; once answered, what the reader answered in their place. */
function Request({ p, w, answer }: { p: Person; w: W; answer?: "approved" | "declined" | undefined }) {
  return (
    <AccountRow
      avatar={<Avatar name={p.name} src={p.avatar} size="post" decorative />}
      name={p.name}
      handle={p.handle}
      href="#"
      end={
        answer === undefined ? (
          <>
            <Button size="timeline">{w.decline}</Button>
            <Button size="timeline" kind="primary">
              {w.approve}
            </Button>
          </>
        ) : (
          <StateText>{answer === "approved" ? w.approved : w.declined}</StateText>
        )
      }
    />
  );
}

type State = "normal" | "empty" | "loading" | "failed";

/** The follow requests on a screen of their own: back to the notifications, the title and how many wait, the rows. */
function Requests({ state }: { state: State }) {
  const w = words[useLang()];
  return (
    <Column>
      <ScreenStack>
        <ScreenTitle
          glyph="follow"
          count={state === "normal" ? "3" : undefined}
          back={<BackButton onPress={() => undefined}>{w.notifications}</BackButton>}
        >
          {w.requests}
        </ScreenTitle>
        {state === "empty" ? (
          <FeedNotice start>{w.empty}</FeedNotice>
        ) : state === "loading" ? (
          <FeedNotice start>{w.loading}</FeedNotice>
        ) : state === "failed" ? (
          <FeedNotice start alert action={<TextButton>{w.retry}</TextButton>}>
            {w.failed}
          </FeedNotice>
        ) : (
          people.map((p, i) => <Request key={p.handle} p={p} w={w} answer={i === 0 ? "approved" : undefined} />)
        )}
      </ScreenStack>
    </Column>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Follow requests", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const InNotificationsRows: Story = { render: () => <InNotifications /> };
export const FollowRequests: Story = { render: () => <Requests state="normal" /> };
export const RequestsEmpty: Story = { render: () => <Requests state="empty" /> };
export const RequestsLoading: Story = { render: () => <Requests state="loading" /> };
export const RequestsFailed: Story = { render: () => <Requests state="failed" /> };
