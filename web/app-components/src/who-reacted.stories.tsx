// SPDX-License-Identifier: AGPL-3.0-only

// Who reacted to a post (FR-116), in the reader's language (the toolbar's locale): on the post's own page, a row per
// emoji under the focused post, each a link to its list; and that list, on a screen of its own under its back button,
// for a Unicode emoji and for a custom one. The back button's words are a placeholder of the feature's.

import { BackButton, ScreenStack, ThreadFocus, useLocale } from "@my/design-system";
import { images } from "@my/design-system/samples";
import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { AccountLine } from "./account";
import { PostDetails } from "./post-details";
import { accountOf, posts, statusOf } from "./post.samples";
import { SamplePost } from "./sample-post.samples";
import { ReactorsTitle, WhoReacted, type Reactors } from "./who-reacted";

const rem = (px: number) => `${px / rootPx}rem`;

type Lang = "en" | "ja";
const useLang = (): Lang => (new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en");

const people = [
  { name: "Alice Chen", handle: "@alice@example.social", avatar: images.dusk },
  { name: "佐藤 美香", handle: "@mika@mstdn.example.jp", avatar: images.snow },
  { name: "Ravi", handle: "@ravi@fedi.example.net" },
  { name: "Lena Novak", handle: "@lena@toot.example.eu" },
  { name: "たくや", handle: "@takuya@example.jp" },
  { name: "Kenji 🍜 Watanabe", handle: "@kenji@my.example" },
  { name: "Open Source Weekly", handle: "@news@social.example.org", avatar: images.logo },
  { name: "Bartholomew Fitzgerald-Montgomery", handle: "@bartholomew@a-rather-long-instance-name.example.social" },
  { name: "森 ゆい", handle: "@yui@my.example" },
  { name: "Noah", handle: "@noah@misskey.example" },
  { name: "Priya", handle: "@priya@fedi.example.net" },
  { name: "小林 さくら", handle: "@sakura@my.example" },
].map(accountOf);

const post = posts.find((p) => p.id === "hana-four");
if (post === undefined) throw new Error("no sample post hana-four");
const focused = post;
const status = statusOf(focused, focused.id);

/** Who reacted with each emoji: the first accounts the server answered for each. */
const reactors: Reactors[] = status.reactions.map((reaction, i) => ({ reaction, accounts: people.slice(i + 1) }));

const back = { en: "Back to the post", ja: "投稿に戻る" } as const;

/** A column of the timeline's width, with its side margin. */
const Column = ({ children }: { children: ReactNode }) => (
  <div style={{ maxWidth: rem(600), paddingInline: rem(16) }}>{children}</div>
);

/** The focused post of its page, its counts under it, and who reacted under them. */
function Page() {
  const lang = useLang();
  return (
    <Column>
      <ScreenStack>
        <ThreadFocus>
          <SamplePost post={focused} lang={lang} details={<PostDetails status={status} />} />
        </ThreadFocus>
        <WhoReacted status={status} reactors={reactors} />
      </ScreenStack>
    </Column>
  );
}

/** One emoji's list, on a screen of its own: the back button, its title, and its accounts. */
function List({ custom }: { custom: boolean }) {
  const lang = useLang();
  const reaction = status.reactions.find((r) => (r.emoji !== undefined) === custom);
  if (reaction === undefined) throw new Error("no sample reaction of that kind");
  return (
    <Column>
      <ScreenStack>
        <BackButton onPress={() => undefined}>{back[lang]}</BackButton>
        <ReactorsTitle reaction={reaction} count={reaction.count} />
        {(custom ? people.slice(4) : people).map((a) => (
          <AccountLine key={a.id} account={a} />
        ))}
      </ScreenStack>
    </Column>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Who reacted", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const OnThePostsPage: Story = { render: () => <Page /> };
export const UnicodeReaction: Story = { render: () => <List custom={false} /> };
/** A custom emoji's list: its title draws the emoji's image. */
export const CustomEmojiReaction: Story = { render: () => <List custom /> };
