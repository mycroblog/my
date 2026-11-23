// SPDX-License-Identifier: AGPL-3.0-only

// The mark of a feed's pull from its top, in the room between a screen's title and its feed, in each state of the
// pull: pulled short of the threshold, past it, and let go past it while the feed is read anew, also over a first post
// whose author has a long name; and a feed that a touch pulls, read anew in about two seconds, so that the load can be
// seen. The words of the posts are placeholders.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useId } from "react";
import { useLocale } from "react-aria-components";
import { Avatar } from "./avatar";
import { Byline, Entry } from "./entry";
import { Feed, FeedItem } from "./feed";
import { Layers, Stack } from "./layout";
import { ScreenStack, ScreenTitle } from "./list";
import { FeedPulls, pullMeasure, PullIndicator, PullPlace, type Pull } from "./pull";

const rem = (px: number) => `${px / rootPx}rem`;

const words = {
  en: [
    "The first frost turned every leaf on the path silver this morning.",
    "Two hours of rain, then the whole valley smelled of cedar.",
  ],
  ja: ["今朝は初霜で、道の葉っぱがぜんぶ銀色になっていました。", "二時間の雨のあと、谷じゅうが杉の香りでした。"],
} as const;

/** A long display name, which runs the whole width of its line on a phone. */
const longName = "Alexandria Montgomery-Featherstonehaugh of the Northern Reaches";

/** A post of the feed under the mark, by an author of `name`. */
function Post({ text, name = "Sam Rivera" }: { text: string; name?: string }) {
  const id = useId();
  return (
    <Entry labelledBy={id}>
      <Byline avatar={<Avatar name={name} size="post" decorative />} name={name} nameId={id} handle="@sam" href="#" />
      <p>{text}</p>
    </Entry>
  );
}

/** The placeholder posts in the story's language. */
const usePosts = () => words[new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en"];

/** The top of a screen: its title, then a feed with the mark of `pull` at its top; `name` is the first author's. */
function Pulled({ pull, name }: { pull: Pull; name?: string }) {
  const posts = usePosts();
  return (
    <div style={{ maxWidth: rem(600), padding: rem(16) }}>
      <ScreenStack>
        <ScreenTitle>Home</ScreenTitle>
        <Layers>
          <PullPlace>
            <PullIndicator pull={pull} />
          </PullPlace>
          <Stack gap="feed">
            {posts.map((text, i) => (
              <Post key={text} text={text} {...(i === 0 && name !== undefined ? { name } : {})} />
            ))}
          </Stack>
        </Layers>
      </ScreenStack>
    </div>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Feed pull", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** Pulled halfway to the threshold: the mark half clear, its arc half a ring, muted. */
export const Pulling: Story = {
  render: () => <Pulled pull={{ phase: "pulling", distance: pullMeasure.threshold / 2 }} />,
};

/** Pulled past the threshold: let go now, the feed is read anew; the mark whole, its ring full, in the accent. */
export const PastTheThreshold: Story = {
  render: () => <Pulled pull={{ phase: "armed", distance: (pullMeasure.threshold + pullMeasure.most) / 2 }} />,
};

/** Let go past the threshold: the mark rests above the feed and turns a three-quarter arc while it is read. */
export const Loading: Story = { render: () => <Pulled pull={{ phase: "loading", distance: pullMeasure.threshold }} /> };

/** Read anew over a first post whose author's long name runs the whole line: the mark covers none of it. */
export const LoadingOverALongName: Story = {
  render: () => <Pulled pull={{ phase: "loading", distance: pullMeasure.threshold }} name={longName} />,
};

/** Pulled past the threshold over the same post: the mark goes no lower than its rest. */
export const PastTheThresholdOverALongName: Story = {
  render: () => <Pulled pull={{ phase: "armed", distance: pullMeasure.most }} name={longName} />,
};

/** How long the story's feed takes to be read anew. */
const readTime = 2000;

/** A feed where the app lets feeds be pulled (a phone's), read anew by a touch's pull from the page's top. */
function Pullable() {
  const posts = usePosts();
  return (
    <div style={{ maxWidth: rem(600), padding: rem(16) }}>
      <FeedPulls on>
        <ScreenStack>
          <ScreenTitle>Home</ScreenTitle>
          <Feed
            label="Home"
            count={posts.length}
            more={false}
            busy={false}
            onPull={() =>
              new Promise((resolve) => {
                setTimeout(resolve, readTime);
              })
            }
          >
            {posts.map((text, i) => (
              <FeedItem key={text} id={String(i)} position={i + 1}>
                <Post text={text} />
              </FeedItem>
            ))}
          </Feed>
        </ScreenStack>
      </FeedPulls>
    </div>
  );
}

/** A feed a touch pulls from the page's top: let go past the threshold, it is read anew for about two seconds. */
export const ATouchPulls: Story = { render: () => <Pullable /> };
