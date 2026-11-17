// SPDX-License-Identifier: AGPL-3.0-only

// Rows of one kind on a screen, in the reader's language (the toolbar's locale; ja, else en): accounts as a list, each
// with what the reader may do; rows that open what waits elsewhere as a list (hashtags); one kind of what a screen
// found under its heading, then several kinds one under another, one of which found none; and what a field found, on
// the raised surface. The words are placeholders.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { Avatar } from "./avatar";
import { Button } from "./button";
import { FeedNotice } from "./feed";
import { AccountRow, FoundRows, OpenRow, ResultGroup, RowItem, RowList, ScreenStack, StateText } from "./list";
import { images } from "./samples";

const rem = (px: number) => `${px / rootPx}rem`;

type Lang = "en" | "ja";
const useLang = (): Lang => (new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en");

const words = {
  en: {
    people: "People",
    hashtags: "Hashtags",
    posts: "Posts",
    follow: "Follow",
    following: "Following",
    found: "Add an account you follow",
    add: "Add",
    none: "No posts found.",
    count: (n: number) => `${String(n)} posts`,
  },
  ja: {
    people: "人",
    hashtags: "ハッシュタグ",
    posts: "投稿",
    follow: "フォローする",
    following: "フォロー中",
    found: "フォロー中のアカウントを追加",
    add: "追加",
    none: "投稿は見つかりませんでした。",
    count: (n: number) => `${String(n)} 件の投稿`,
  },
} as const;

type W = (typeof words)[Lang];

const people = [
  { name: "Mika", handle: "@mika@my.example", avatar: images.dusk, following: false },
  { name: "Haruto", handle: "@haruto@photos.example", avatar: images.snow, following: true },
  {
    name: "Saoirse with a display name long enough to run out of room",
    handle: "@saoirse_on_a_very_long_handle@photos.example",
    avatar: undefined,
    following: false,
  },
];

const tags = [
  { name: "sourdough", count: 128 },
  { name: "filmphotography", count: 42 },
  { name: "SlowMorningsByTheSea", count: 3 },
];

/** A column of the timeline's width, with its side margin. */
const Column = ({ children }: { children: ReactNode }) => (
  <div style={{ maxWidth: rem(600), paddingInline: rem(16), paddingBlock: rem(16) }}>{children}</div>
);

const Accounts = ({ w }: { w: W }) => (
  <RowList label={w.people}>
    {people.map((p) => (
      <RowItem key={p.handle}>
        <AccountRow
          avatar={<Avatar name={p.name} src={p.avatar} size="post" decorative />}
          name={p.name}
          handle={p.handle}
          href="#"
          end={
            p.following ? (
              <StateText>{w.following}</StateText>
            ) : (
              <Button size="timeline" kind="primary">
                {w.follow}
              </Button>
            )
          }
        />
      </RowItem>
    ))}
  </RowList>
);

const Tags = ({ w }: { w: W }) => (
  <RowList label={w.hashtags}>
    {tags.map((t) => (
      <RowItem key={t.name}>
        <OpenRow href="#" icon="hashtag" title={`#${t.name}`} hint={w.count(t.count)} />
      </RowItem>
    ))}
  </RowList>
);

function AccountListStory() {
  const w = words[useLang()];
  return (
    <Column>
      <Accounts w={w} />
    </Column>
  );
}

function LinkListStory() {
  const w = words[useLang()];
  return (
    <Column>
      <Tags w={w} />
    </Column>
  );
}

/** Several kinds of what a search found, one under another; the posts found none. */
function ResultGroupsStory() {
  const w = words[useLang()];
  return (
    <Column>
      <ScreenStack>
        <ResultGroup title={w.people}>
          <Accounts w={w} />
        </ResultGroup>
        <ResultGroup title={w.hashtags}>
          <Tags w={w} />
        </ResultGroup>
        <ResultGroup title={w.posts}>
          <FeedNotice start>{w.none}</FeedNotice>
        </ResultGroup>
      </ScreenStack>
    </Column>
  );
}

function FoundRowsStory() {
  const w = words[useLang()];
  return (
    <Column>
      <FoundRows label={w.found}>
        {people.slice(0, 2).map((p) => (
          <AccountRow
            key={p.handle}
            avatar={<Avatar name={p.name} src={p.avatar} size="post" decorative />}
            name={p.name}
            handle={p.handle}
            end={<Button size="timeline">{w.add}</Button>}
          />
        ))}
      </FoundRows>
    </Column>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Rows", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const AccountList: Story = { render: () => <AccountListStory /> };
export const LinkList: Story = { render: () => <LinkListStory /> };
export const ResultGroups: Story = { render: () => <ResultGroupsStory /> };
export const WhatAFieldFound: Story = { render: () => <FoundRowsStory /> };
