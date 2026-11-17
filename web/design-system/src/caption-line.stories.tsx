// SPDX-License-Identifier: AGPL-3.0-only

// A caption line in an entry's place, in the reader's language (the toolbar's locale; ja, else en): what the reader
// did to a post's author from the post's menu (muted, blocked, its server muted), with its undoing at the line's end,
// between the posts around it. The words are placeholders of the feature's.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useId } from "react";
import { useLocale } from "react-aria-components";
import { Avatar } from "./avatar";
import { TextButton } from "./button";
import { Byline, CaptionLine, Entry } from "./entry";

const rem = (px: number) => `${px / rootPx}rem`;

const words = {
  en: {
    muted: "You muted @sora@photos.example.",
    blocked: "You blocked @sora@photos.example.",
    server: "You muted the server photos.example.",
    undo: "Undo",
    unblock: "Unblock",
    post: "The first frost turned every leaf on the path silver this morning.",
  },
  ja: {
    muted: "@sora@photos.example をミュートしました。",
    blocked: "@sora@photos.example をブロックしました。",
    server: "サーバー photos.example をミュートしました。",
    undo: "元に戻す",
    unblock: "ブロックを解除",
    post: "今朝は初霜で、道の葉っぱがぜんぶ銀色になっていました。",
  },
} as const;

/** A post of the feed around the line. */
function Post({ text }: { text: string }) {
  const id = useId();
  return (
    <Entry labelledBy={id}>
      <Byline
        avatar={<Avatar name="Sam Rivera" size="post" decorative />}
        name="Sam Rivera"
        nameId={id}
        handle="@sam"
        href="#"
      />
      <p>{text}</p>
    </Entry>
  );
}

/** What the reader did, each in the place of the post it was done from, between two posts. */
function Done() {
  const w = words[new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en"];
  return (
    <div style={{ maxWidth: rem(600), padding: rem(16), display: "grid", gap: rem(28) }}>
      <Post text={w.post} />
      <Entry>
        <CaptionLine icon="mute" action={<TextButton>{w.undo}</TextButton>}>
          {w.muted}
        </CaptionLine>
      </Entry>
      <Entry>
        <CaptionLine icon="block" action={<TextButton>{w.unblock}</TextButton>}>
          {w.blocked}
        </CaptionLine>
      </Entry>
      <Entry>
        <CaptionLine icon="mute" action={<TextButton>{w.undo}</TextButton>}>
          {w.server}
        </CaptionLine>
      </Entry>
      <Post text={w.post} />
    </div>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Caption line", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** In a post's place: what was done to its author, and the action that undoes it. */
export const InAPostsPlace: Story = { render: () => <Done /> };
