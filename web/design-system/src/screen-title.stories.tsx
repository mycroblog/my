// SPDX-License-Identifier: AGPL-3.0-only

// The head of a screen of one thing a timeline is of, in the reader's language (the toolbar's locale): a hashtag the
// reader does not follow, one they follow, one whose name runs long (it breaks before its capitals), and a list of
// theirs, which has nothing to do yet (making and changing lists comes later). A post under the first shows where the
// column starts. The words are placeholders of the features'.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { Avatar } from "./avatar";
import { Button } from "./button";
import { Byline, Entry, Stamp } from "./entry";
import { BackButton, ScreenStack, ScreenTitle } from "./list";
import { images } from "./samples";

const rem = (px: number) => `${px / rootPx}rem`;

type Lang = "en" | "ja";
const useLang = (): Lang => (new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en");

const words = {
  en: {
    back: "Back",
    lists: "Lists",
    follow: "Follow",
    following: "Following",
    members: (n: number) => `${String(n)} members`,
    post: "Day 3 of the sourdough experiment. The crumb is finally open enough to call it bread. #baking",
    list: "Close friends",
  },
  ja: {
    back: "戻る",
    lists: "リスト",
    follow: "フォロー",
    following: "フォロー中",
    members: (n: number) => `${String(n)} 人`,
    post: "サワードウの実験 3 日目。やっとパンと呼べるくらい気泡が開いた。#baking",
    list: "親しい友だち",
  },
} as const;

/** A column of the timeline's width, with its side margin. */
const Column = ({ children }: { children: ReactNode }) => (
  <div style={{ maxWidth: rem(600), paddingInline: rem(16), display: "grid", gap: rem(48) }}>{children}</div>
);

function Titles() {
  const w = words[useLang()];
  const back = (text: string) => <BackButton onPress={() => undefined}>{text}</BackButton>;
  const follow = (on: boolean) => (
    <Button size="timeline" kind={on ? "secondary" : "primary"}>
      {on ? w.following : w.follow}
    </Button>
  );
  return (
    <Column>
      <ScreenStack>
        <ScreenTitle back={back(w.back)} actions={follow(false)}>
          #baking
        </ScreenTitle>
        <Entry labelledBy="screen-title-post">
          <Byline
            avatar={<Avatar name="Sam" src={images.dusk} size="post" decorative />}
            name="Sam"
            nameId="screen-title-post"
            handle="@sam@my.example"
            end={<Stamp glyph={{ icon: "public", label: "Public" }} date={new Date(Date.now() - 3_600_000)} />}
          />
          <p className="m-0">{w.post}</p>
        </Entry>
      </ScreenStack>
      <ScreenTitle back={back(w.back)} actions={follow(true)}>
        #coffee
      </ScreenTitle>
      <ScreenTitle back={back(w.back)} actions={follow(false)}>
        #TheLongestHashtagSomebodyEverWroteAboutSlowMorningsByTheSea2026
      </ScreenTitle>
      <ScreenTitle back={back(w.lists)} glyph="list" count={w.members(12)}>
        {w.list}
      </ScreenTitle>
    </Column>
  );
}

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "Screen title", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Heads: Story = { render: () => <Titles /> };
