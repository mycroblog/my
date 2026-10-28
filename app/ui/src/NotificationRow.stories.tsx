// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Avatar } from "./Avatar";
import { Box } from "./Box";
import { Button } from "./Button";
import { Link } from "./Link";
import { NotificationLinks, NotificationOthers, NotificationSentence } from "./NotificationRow";
import { PostCard } from "./PostCard";
import { PostCardAvatarLink, PostCardNameLink, PostCardTimeLink } from "./PostCardParts";
import { PostExcerpt, PostExcerptText, QuoteBlock } from "./Quote";
import { Text } from "./Text";

function Name({ children }: { children: string }) {
  return <PostCardNameLink href="#alice">{children}</PostCardNameLink>;
}

const time = (
  <PostCardTimeLink href="#post">
    <time dateTime="2026-10-16T09:00:00Z" title="Friday, October 16, 2026 at 9:00 AM">
      2h
    </time>
  </PostCardTimeLink>
);

const avatar = (
  <PostCardAvatarLink href="#alice">
    <Avatar name="Alice" decorative />
  </PostCardAvatarLink>
);

const meta = {
  title: "Display/NotificationRow",
  component: NotificationSentence,
  args: { isUnread: false, unreadLabel: "Unread", children: null },
  decorators: [
    (Story) => (
      <Box width="sm" container>
        <Story />
      </Box>
    ),
  ],
} satisfies Meta<typeof NotificationSentence>;

export default meta;
type Story = StoryObj<typeof meta>;

function Row({
  isUnread,
  name = "Alice",
  text = "favourited your post",
  others = 0,
  quote,
}: {
  isUnread: boolean;
  name?: string;
  text?: string;
  others?: number;
  quote?: React.ReactNode;
}) {
  return (
    <PostCard
      avatar={avatar}
      author={
        <>
          {others > 0 && (
            <NotificationOthers>
              {Array.from({ length: others }, (_, i) => (
                <PostCardAvatarLink key={i} href={`#other-${i}`}>
                  <Avatar name={`Other ${i}`} size="xs" decorative />
                </PostCardAvatarLink>
              ))}
            </NotificationOthers>
          )}
          <NotificationSentence isUnread={isUnread} unreadLabel="Unread">
            <Name>{name}</Name> {others > 0 ? `and ${others} others ` : ""}
            {text}
          </NotificationSentence>
        </>
      }
      time={time}
      content={quote ?? null}
    />
  );
}

const excerpt = (
  <PostExcerpt href="#post">
    <PostExcerptText lang="en">
      Hello from the local timeline! A long line follows so that the excerpt reaches its third line and is cut there, as
      a quote of a post is.
    </PostExcerptText>
  </PostExcerpt>
);

/** Unread: the dot, the heavier sentence and the word "Unread" in the article's name. */
export const Unread: Story = {
  render: () => <Row isUnread quote={excerpt} />,
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("article", { name: /Unread\s+Alice favourited your post/ })).toBeVisible();
  },
};

export const Read: Story = {
  render: () => <Row isUnread={false} quote={excerpt} />,
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("article", { name: /^Alice favourited your post/ })).toBeVisible();
  },
};

/** A group: the avatars of the others above the sentence, which counts them all. */
export const Group: Story = {
  render: () => <Row isUnread others={6} quote={excerpt} />,
};

export const LongName: Story = {
  render: () => (
    <Row
      isUnread={false}
      name="Alexandria Montgomery-Featherstonehaugh-Wolfeschlegelsteinhausenbergerdorff"
      quote={excerpt}
    />
  ),
};

export const Rtl: Story = {
  render: () => (
    <Row
      isUnread
      name="مريم"
      text="أعجبها منشورك"
      quote={
        <PostExcerpt href="#post">
          <PostExcerptText lang="ar">
            مرحبا بكم في الخط الزمني المحلي، هذا نص طويل يمتد على عدة أسطر ليظهر القطع بعد السطر الثالث من الاقتباس.
          </PostExcerptText>
        </PostExcerpt>
      }
    />
  ),
};

/** A post behind a content warning quotes the warning only. */
export const ContentWarning: Story = {
  render: () => (
    <Row
      isUnread={false}
      quote={
        <PostExcerpt href="#post">
          <Text>Content warning: spoilers</Text>
        </PostExcerpt>
      }
    />
  ),
};

/** A moderation notice: the action, the moderators' words and when it ends, as a quote block. */
export const Moderation: Story = {
  render: () => (
    <Row
      isUnread
      name="Moderators"
      text="limited your account"
      quote={
        <QuoteBlock>
          <Text as="p" weight="semibold">
            Your account is limited
          </Text>
          <Text as="p" dir="auto" whitespace="preLine" wrap="anywhere" align="matchParent">
            Repeated spam in replies.
          </Text>
          <Text as="p" size="sm" tone="muted">
            Until October 30, 2026
          </Text>
        </QuoteBlock>
      }
    />
  ),
};

/** Links that lead on, and a follow-back button, in the actions row. */
export const WithActions: Story = {
  render: () => (
    <PostCard
      avatar={avatar}
      author={
        <NotificationSentence isUnread={false} unreadLabel="Unread">
          <Name>Alice</Name> followed you
        </NotificationSentence>
      }
      time={time}
      content={null}
      actions={
        <NotificationLinks>
          <Link href="#a" variant="standalone">
            View the account
          </Link>
          <Link href="#b" variant="standalone">
            View the appeal
          </Link>
          <Button size="sm">Follow back</Button>
        </NotificationLinks>
      }
    />
  ),
};
