// SPDX-License-Identifier: AGPL-3.0-only

// The switch, in the reader's language (the toolbar's locale; ja, else en): off, on, with a line that describes it,
// disabled with the reason, and a label long enough to wrap at 320 px, each with nothing to say of its saving (its
// save states are among the settings' parts). The words are placeholders of the notification settings.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { Switch } from "./switch";

const rem = (px: number) => `${px / rootPx}rem`;

const words = {
  en: {
    favourites: "Favourites",
    boosts: "Boosts",
    mentions: "Mentions and replies",
    mentionsAbout: "Also when someone you do not follow writes to you",
    email: "Email me about new followers while I am away",
    push: "Push notifications",
    pushOff: "Your browser blocks notifications from this site. Allow them in the browser's settings.",
  },
  ja: {
    favourites: "お気に入り",
    boosts: "ブースト",
    mentions: "メンションと返信",
    mentionsAbout: "フォローしていない人からのものも含みます",
    email: "しばらく開いていないとき、新しいフォロワーをメールで知らせる",
    push: "プッシュ通知",
    pushOff: "ブラウザがこのサイトの通知をブロックしています。ブラウザの設定で許可してください。",
  },
} as const;

const useWords = () => words[new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en"];

function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(600), padding: rem(16), display: "grid", gap: rem(8) }}>{children}</div>;
}

function All() {
  const w = useWords();
  return (
    <Column>
      <Switch label={w.favourites} save={null} />
      <Switch label={w.boosts} defaultSelected save={null} />
      <Switch label={w.mentions} description={w.mentionsAbout} defaultSelected save={null} />
      <Switch label={w.email} save={null} />
      <Switch label={w.push} isDisabled reason={w.pushOff} save={null} />
      <Switch label={w.boosts} isDisabled defaultSelected save={null} />
    </Column>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Switch", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const States: Story = { render: () => <All /> };
