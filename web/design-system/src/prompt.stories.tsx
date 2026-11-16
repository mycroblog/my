// SPDX-License-Identifier: AGPL-3.0-only

// A prompt inside a screen (#3141), as the Web Push prompt over the notifications will be (ADR-0086 §18–§19), in the
// reader's language (the toolbar's locale; ja, else en), above the first notification: offered; with the reason the
// browser's question was closed, the action still there; and blocked, with only its dismiss left. The words are
// placeholders of the feature's.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useId, useState } from "react";
import { useLocale } from "react-aria-components";
import { expect, userEvent, within } from "storybook/test";
import { Avatar } from "./avatar";
import { Entry } from "./entry";
import { NoticeLine, SentenceName } from "./list";
import { InlinePrompt } from "./prompt";
import { ago } from "./samples";

const rem = (px: number) => `${px / rootPx}rem`;

const words = {
  en: {
    offer: "Get notified on this device, even when mycroblog is closed.",
    turnOn: "Turn on",
    notNow: "Not now",
    closed: "You closed the browser's question without choosing. Turn on to ask again.",
    blocked: "Your browser blocks notifications from this site. Allow them in the browser's settings.",
    followed: " followed you",
    unread: "Unread",
  },
  ja: {
    offer: "mycroblog を閉じていても、この端末に通知を届けられます。",
    turnOn: "有効にする",
    notNow: "今はしない",
    closed: "ブラウザの確認が選ばれずに閉じられました。もう一度確認するには「有効にする」を押してください。",
    blocked: "ブラウザがこのサイトの通知をブロックしています。ブラウザの設定で許可してください。",
    followed: " があなたをフォローしました",
    unread: "未読",
  },
} as const;

const useWords = () => words[new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en"];

type State = "offered" | "closed" | "blocked";

/** The top of the notifications: the prompt in `state`, then a notification under it. */
function Top({ state }: { state: State }) {
  const w = useWords();
  const id = useId();
  const [shown, setShown] = useState(true);
  return (
    <div style={{ maxWidth: rem(600), padding: rem(16), display: "grid", gap: rem(24) }}>
      {shown && (
        <InlinePrompt
          icon="notifications"
          action={state === "blocked" ? undefined : { label: w.turnOn, onPress: () => undefined }}
          dismiss={{
            label: w.notNow,
            onPress: () => {
              setShown(false);
            },
          }}
          reason={state === "closed" ? w.closed : state === "blocked" ? w.blocked : undefined}
        >
          {w.offer}
        </InlinePrompt>
      )}
      <Entry labelledBy={id}>
        <NoticeLine
          icon="follow"
          avatar={<Avatar name="Kiri" size="inline" decorative />}
          date={ago(5 * 60_000)}
          id={id}
          unread={w.unread}
        >
          <SentenceName>Kiri</SentenceName>
          {w.followed}
        </NoticeLine>
      </Entry>
    </div>
  );
}

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "Inline prompt", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** Offered: its action and its dismiss; dismissing it takes it away. */
export const Offered: Story = {
  render: () => <Top state="offered" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const prompt = canvas.getByRole("region", { name: /notified|通知を届け/ });
    await expect(within(prompt).getByRole("button", { name: /Turn on|有効にする/ })).toBeVisible();
    await userEvent.click(within(prompt).getByRole("button", { name: /Not now|今はしない/ }));
    await expect(canvas.queryByRole("region")).toBeNull();
  },
};

/** The browser's question closed without a choice: the reason under it, its action still there. */
export const QuestionClosed: Story = {
  render: () => <Top state="closed" />,
  play: async ({ canvasElement }) => {
    const prompt = within(canvasElement).getByRole("region");
    await expect(within(prompt).getByRole("alert")).toBeVisible();
    await expect(within(prompt).getByRole("button", { name: /Turn on|有効にする/ })).toBeVisible();
  },
};

/** The site's notifications blocked: the reason, and only its dismiss. */
export const Blocked: Story = {
  render: () => <Top state="blocked" />,
  play: async ({ canvasElement }) => {
    const prompt = within(canvasElement).getByRole("region");
    await expect(within(prompt).queryByRole("button", { name: /Turn on|有効にする/ })).toBeNull();
    await expect(within(prompt).getByRole("button", { name: /Not now|今はしない/ })).toBeVisible();
  },
};
