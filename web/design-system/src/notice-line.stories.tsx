// SPDX-License-Identifier: AGPL-3.0-only

// The line of a notification that leads to what it tells of (#3141), in the reader's language (the toolbar's locale;
// ja, else en): unread and read, with the account's avatar and without one (a notice of the server), with the post it
// is about under the sentence, and a sentence long enough to wrap at 320 px; the whole line leads there. The words are
// placeholders of the notifications'.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useId, type ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { expect, within } from "storybook/test";
import { Avatar } from "./avatar";
import { Entry } from "./entry";
import { type IconName } from "./icons";
import { Excerpt, NoticeLine, SentenceName } from "./list";
import { ago } from "./samples";

const rem = (px: number) => `${px / rootPx}rem`;
const minute = 60_000;

const words = {
  en: {
    unread: "Unread",
    favourited: " favourited your post",
    followed: " followed you",
    accepted: " approved your quote of their post",
    autoDelete: "mycroblog.example now deletes posts older than 365 days. Yours older than that go in 30 days.",
    post: "Morning bread is out of the oven: rye, walnut and a few croissants.",
  },
  ja: {
    unread: "未読",
    favourited: " があなたの投稿をお気に入りにしました",
    followed: " があなたをフォローしました",
    accepted: " があなたの引用を承認しました",
    autoDelete:
      "mycroblog.example は 365 日より前の投稿を削除するようになりました。それより前のあなたの投稿は 30 日後に削除されます。",
    post: "朝のパンが焼き上がりました。ライ麦、くるみ、それにクロワッサンを少し。",
  },
} as const;

const useWords = () => words[new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en"];

/** One notification of the feed, its line leading to `href`. */
function Notice({
  icon,
  name,
  sentence,
  at,
  unread,
  href,
  excerpt,
}: {
  icon: IconName;
  name?: string;
  sentence: ReactNode;
  at: number;
  unread?: string | undefined;
  href: string;
  excerpt?: string;
}) {
  const id = useId();
  return (
    <Entry labelledBy={id}>
      <NoticeLine
        icon={icon}
        avatar={name === undefined ? undefined : <Avatar name={name} size="inline" decorative />}
        date={ago(at)}
        id={id}
        unread={unread}
        href={href}
        excerpt={excerpt === undefined ? undefined : <Excerpt>{excerpt}</Excerpt>}
      >
        {name !== undefined && <SentenceName>{name}</SentenceName>}
        {sentence}
      </NoticeLine>
    </Entry>
  );
}

function Feed() {
  const w = useWords();
  return (
    <div style={{ maxWidth: rem(600), padding: rem(16), display: "grid", gap: rem(24) }}>
      <Notice
        icon="favourite"
        name="Ren"
        sentence={w.favourited}
        at={4 * minute}
        unread={w.unread}
        href="/@ren/status/1"
        excerpt={w.post}
      />
      <Notice icon="follow" name="Kiri" sentence={w.followed} at={40 * minute} href="/@kiri" />
      <Notice icon="accepted" name="Mio 澪" sentence={w.accepted} at={3 * 60 * minute} href="/@mio/status/2" />
      <Notice icon="autoDelete" sentence={w.autoDelete} at={26 * 60 * minute} href="/notifications" />
    </div>
  );
}

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "Notice line", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** Each line leads to what it tells of, and its sentence alone names its entry. */
export const Linked: Story = {
  render: () => <Feed />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const links = canvas.getAllByRole("link");
    await expect(links).toHaveLength(4);
    await expect(links[0]).toHaveAttribute("href", "/@ren/status/1");
    await expect(canvas.getAllByRole("article")[1]).toHaveAccessibleName(/^Kiri|^Kiri があなた/);
  },
};
