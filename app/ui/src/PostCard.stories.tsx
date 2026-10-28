// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactElement } from "react";
import { expect } from "storybook/test";
import { Avatar } from "./Avatar";
import { Button } from "./Button";
import { ContentWarning } from "./ContentWarning";
import { MediaGrid } from "./MediaGrid";
import { PostCard } from "./PostCard";
import {
  PostCardAuthorLink,
  PostCardAvatarLink,
  PostCardBoostContext,
  PostCardNameLink,
  PostCardOrigin,
  PostCardTimeLink,
} from "./PostCardParts";
import { AutoplayContext } from "./Picture";
import { Text } from "./Text";
import { PostContent, serverSanitizedHtml } from "./PostContent";

function picture(fill: string): string {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 160 90"><rect width="160" height="90" fill="${fill}"/><circle cx="80" cy="45" r="20" fill="#f2c14e"/></svg>`;
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
}

function Author({ name, handle }: { name: string; handle: string }) {
  return <PostCardAuthorLink href="#profile" name={name} handle={handle} />;
}

function Time() {
  return (
    <PostCardTimeLink href="#post">
      <time dateTime="2026-10-16T09:00:00Z" title="Friday, October 16, 2026 at 9:00 AM">
        2h
      </time>
    </PostCardTimeLink>
  );
}

/**
 * Checks that the header of the card takes at most two lines on a 320 px card: the name and the time,
 * and the handle under them. The frame is the story's container, set back after the check.
 */
async function expectHeaderInTwoLines(canvasElement: HTMLElement) {
  const frame = canvasElement.closest<HTMLElement>("[data-story-frame]") ?? canvasElement;
  const previous = frame.style.inlineSize;
  frame.style.inlineSize = "320px";
  try {
    const header = canvasElement.querySelector("header");
    const name = header?.querySelector<HTMLElement>("[dir=auto]");
    if (header == null || name == null) throw new Error("the card has no header with a name");
    const lineHeight = Number.parseFloat(getComputedStyle(name).lineHeight);
    await expect(header.getBoundingClientRect().height).toBeLessThanOrEqual(2 * lineHeight);
  } finally {
    frame.style.inlineSize = previous;
  }
}

function Actions() {
  return (
    <>
      <Button variant="ghost" size="sm">
        Reply
      </Button>
      <Button variant="ghost" size="sm">
        Boost
      </Button>
      <Button variant="ghost" size="sm">
        Favourite
      </Button>
    </>
  );
}

const meta = {
  title: "Timeline/PostCard",
  component: PostCard,
  args: {
    avatar: (
      <PostCardAvatarLink href="#profile">
        <Avatar name="Alice" decorative />
      </PostCardAvatarLink>
    ),
    author: <Author name="Alice" handle="@alice" />,
    time: <Time />,
    visibility: { level: "public", label: "Public" },
    content: (
      <PostContent
        html={serverSanitizedHtml(
          "<p>Hello from the local timeline! Containers decide this layout, not the viewport.</p>",
        )}
        lang="en"
      />
    ),
    actions: <Actions />,
  },
} satisfies Meta<typeof PostCard>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  play: async ({ canvas }) => {
    const article = canvas.getByRole("article", { name: /Alice\s+@alice\s+2h/ });
    await expect(article).toHaveAccessibleDescription(/Hello from the local timeline/);
  },
};

export const WithMedia: Story = {
  args: {
    media: (
      <MediaGrid
        items={[
          { id: "1", src: picture("#4f6d8f"), alt: "A sun over a blue sky" },
          { id: "2", src: picture("#3d7a52"), alt: null },
        ]}
        noDescriptionLabel="No description"
        revealLabel="Show sensitive media"
        audioLabels={{
          play: "Play",
          pause: "Pause",
          seek: "Position",
          loading: "Loading media",
          failed: "Couldn't load this media.",
          retry: "Try again",
        }}
      />
    ),
  },
};

/**
 * With animations off, the card plays its animations while it is hovered, has keyboard focus inside, or after a tap on
 * it. The moving renditions stay behind the reduced-motion query, so this story only checks that they come and go; the
 * other stories carry the card's screenshots.
 */
export const AutoplayOff: Story = {
  tags: ["no-screenshot"],
  args: {
    avatar: (
      <PostCardAvatarLink href="#profile">
        <Avatar
          sources={{ still: [{ url: picture("#4f6d8f") }], animated: [{ url: picture("#b3261e") }] }}
          name="Alice"
          decorative
        />
      </PostCardAvatarLink>
    ),
  },
  render: (args) => (
    <AutoplayContext value={false}>
      <PostCard {...args} />
    </AutoplayContext>
  ),
  play: async ({ canvas, canvasElement, userEvent }) => {
    const article = canvas.getByRole("article", { name: /Alice\s+@alice\s+2h/ });
    const moving = () => canvasElement.querySelector("source");
    await expect(moving()).toBeNull();
    await userEvent.hover(canvas.getByText(/Hello from the local timeline/));
    await expect(moving()).toHaveAttribute("media", "(prefers-reduced-motion: no-preference)");
    await userEvent.unhover(article);
    await expect(moving()).toBeNull();
    const tap = () =>
      userEvent.pointer([
        { keys: "[TouchA>]", target: article },
        { keys: "[/TouchA]", target: article },
      ]);
    await tap();
    await expect(moving()).not.toBeNull();
    await tap();
    await expect(moving()).toBeNull();
  },
};

/** The reactions in a row of their own, between the body and the actions. */
export const WithReactions: Story = {
  args: {
    reactions: (
      <>
        <Button variant="ghost" size="sm">
          👍 3
        </Button>
        <Button variant="ghost" size="sm">
          🎉 1
        </Button>
      </>
    ),
  },
};

export const WithContentWarning: Story = {
  args: {
    content: (
      <ContentWarning summary="Food (pictures of a very large cake)" toggleLabel="Show content">
        <PostContent html={serverSanitizedHtml("<p>It took three days to bake.</p>")} lang="en" />
        <MediaGrid
          items={[{ id: "1", src: picture("#7a3d6b"), alt: "A cake" }]}
          sensitive
          noDescriptionLabel="No description"
          revealLabel="Show sensitive media"
          audioLabels={{
            play: "Play",
            pause: "Pause",
            seek: "Position",
            loading: "Loading media",
            failed: "Couldn't load this media.",
            retry: "Try again",
          }}
        />
      </ContentWarning>
    ),
  },
  play: async ({ canvas, userEvent }) => {
    await expect(canvas.queryByRole("img", { name: "A cake" })).not.toBeInTheDocument();
    await userEvent.click(canvas.getByRole("button", { name: "Show content" }));
    await expect(canvas.getByText("It took three days to bake.")).toBeVisible();
    // The media inside stays concealed until they are shown on their own.
    await expect(canvas.getByRole("button", { name: "Show sensitive media" })).toHaveAttribute("aria-pressed", "false");
    await userEvent.click(canvas.getByRole("button", { name: "Show content" }));
  },
};

/** A boost: the line above the post says who boosted it, and starts the article's name. */
export const Boosted: Story = {
  args: {
    context: (
      <PostCardBoostContext>
        <PostCardNameLink href="#bob">Bob</PostCardNameLink> boosted
      </PostCardBoostContext>
    ),
  },
  play: async ({ canvas }) => {
    await expect(
      canvas.getByRole("article", { name: /^Bob boosted\s+Alice\s+@alice\s+2h\s+Public$/ }),
    ).toBeInTheDocument();
  },
};

/** A long display name and handle are cut with an ellipsis; the link is still named by both in full. */
export const LongNames: Story = {
  args: {
    avatar: <Avatar name="Bartholomew" decorative />,
    author: (
      <Author
        name="Bartholomew Montgomery-Fitzwilliam the Third"
        handle="@bartholomew_montgomery_fitzwilliam@social.example.com"
      />
    ),
  },
  play: async ({ canvas, canvasElement }) => {
    await expect(
      canvas.getByRole("link", {
        name: "Bartholomew Montgomery-Fitzwilliam the Third @bartholomew_montgomery_fitzwilliam@social.example.com",
      }),
    ).toBeInTheDocument();
    await expectHeaderInTwoLines(canvasElement);
  },
};

/** A handle of a server with a long domain: the domain is cut, the user part stays. */
export const LongDomain: Story = {
  args: {
    avatar: <Avatar name="Carol" decorative />,
    author: <Author name="Carol" handle="@carol@a-rather-long-community-server-name.social.example.com" />,
  },
  play: async ({ canvas, canvasElement }) => {
    await expect(
      canvas.getByRole("link", { name: "Carol @carol@a-rather-long-community-server-name.social.example.com" }),
    ).toBeInTheDocument();
    await expectHeaderInTwoLines(canvasElement);
  },
};

/** A display name in a right-to-left script lines up with the card's direction, not its own. */
export const RightToLeftName: Story = {
  args: {
    avatar: <Avatar name="سارة" decorative />,
    author: <Author name="سارة الحسيني من مجتمع الرسامين المستقلين" handle="@sara@fann.example" />,
  },
  play: async ({ canvasElement }) => {
    await expectHeaderInTwoLines(canvasElement);
  },
};

/** A post written in a right-to-left script. */
export const RightToLeftPost: Story = {
  args: {
    avatar: <Avatar name="سارة" decorative />,
    author: <Author name="سارة" handle="@sara" />,
    content: <PostContent html={serverSanitizedHtml("<p>صباح الخير من الخط الزمني المحلي!</p>")} lang="ar" />,
  },
};

/**
 * The post a page is about: a 2px accent border, a name that tells it apart, and
 * its full date and counts in words between the body and the actions.
 */
export const Highlighted: Story = {
  args: {
    label: "Selected post",
    highlighted: true,
    time: <time dateTime="2026-10-16T09:00:00Z">2h</time>,
    content: (
      <PostContent
        html={serverSanitizedHtml("<p>The post this page is about, in larger text.</p>")}
        lang="en"
        size="lg"
      />
    ),
    details: (
      <>
        <time dateTime="2026-10-16T09:00:00Z">Friday, October 16, 2026 at 9:00 AM</time>
        <Text numeric>3 boosts</Text>
        <Text numeric>12 favourites</Text>
      </>
    ),
  },
  play: async ({ canvas }) => {
    await expect(
      canvas.getByRole("article", { name: /^Selected post\s+Alice\s+@alice\s+2h\s+Public$/ }),
    ).toHaveAccessibleDescription(/The post this page is about/);
    await expect(canvas.getByText("3 boosts")).toBeVisible();
  },
};

/**
 * Three posts of a thread, each a reply to the one above: a line joins their avatars. The middle one is
 * the highlighted post of the page; the cards with a line below keep their avatar's column free.
 */
export const Thread: Story = {
  render: (args) => (
    <div>
      <PostCard {...args} lineBelow />
      <PostCard
        {...args}
        avatar={<Avatar name="Bob" decorative />}
        author={<Author name="Bob" handle="@bob" />}
        content={
          <PostContent
            html={serverSanitizedHtml("<p>A reply, and the post this page is about.</p>")}
            lang="en"
            size="lg"
          />
        }
        label="Selected post"
        highlighted
        lineAbove
        lineBelow
      />
      <PostCard
        {...args}
        content={<PostContent html={serverSanitizedHtml("<p>A reply to the reply.</p>")} lang="en" />}
        lineAbove
      />
    </div>
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getAllByRole("article")).toHaveLength(3);
    await expect(canvas.getByRole("article", { name: /^Selected post\s+Bob/ })).toBeInTheDocument();
  },
};

/** A pinned post of a profile: the line above says so, and starts the article's name. */
export const Pinned: Story = {
  args: { context: <span>Pinned</span> },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("article", { name: /^Pinned\s+Alice\s+@alice\s+2h\s+Public$/ })).toBeInTheDocument();
  },
};

/** A post of another server: its page on that server is a link at the end of the actions row. */
export const WithOrigin: Story = {
  args: {
    actions: (
      <>
        <Actions />
        <PostCardOrigin
          href="https://social.example/@alice/1"
          host="social.example"
          label="Open the post on social.example"
        />
      </>
    ),
  },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("link", { name: "Open the post on social.example" })).toHaveAttribute(
      "href",
      "https://social.example/@alice/1",
    );
  },
};

/** The mark of an edited post leads to its revisions, in the details line. */
export const Edited: Story = {
  args: { details: <PostCardTimeLink href="#history">Edited</PostCardTimeLink> },
};

export const WithoutAvatarOrActions: Story = {
  args: { avatar: undefined, actions: undefined },
};

const levels = [
  { level: "public", label: "Public" },
  { level: "local", label: "This server" },
  { level: "unlisted", label: "Unlisted" },
  { level: "private", label: "Followers" },
  { level: "direct", label: "Mentioned people" },
] as const;

/** Every visibility under the time, each with its own icon: the shape tells them apart, not a color. */
export const Visibilities: Story = {
  render: (args) => (
    <div>
      {levels.map((visibility) => (
        <PostCard key={visibility.level} {...args} visibility={visibility} />
      ))}
    </div>
  ),
  play: async ({ canvas }) => {
    for (const { label } of levels) {
      await expect(canvas.getByRole("article", { name: new RegExp(`2h\\s+${label}$`) })).toBeInTheDocument();
    }
  },
};

/**
 * Checks that the time and the visibility end the header row (its inline end, so a right-to-left card mirrors) and
 * that the author stays in the room before them.
 */
async function expectTimeAtRowEnd(canvasElement: HTMLElement) {
  const header = canvasElement.querySelector("header");
  const author = header?.firstElementChild;
  const time = header?.lastElementChild;
  if (header == null || author == null || time == null) throw new Error("the card has no header");
  const row = header.getBoundingClientRect();
  const timeBox = time.getBoundingClientRect();
  const authorBox = author.getBoundingClientRect();
  if (getComputedStyle(header).direction === "rtl") {
    await expect(timeBox.left).toBeCloseTo(row.left, 0);
    await expect(authorBox.left).toBeGreaterThanOrEqual(timeBox.right - 0.5);
  } else {
    await expect(timeBox.right).toBeCloseTo(row.right, 0);
    await expect(authorBox.right).toBeLessThanOrEqual(timeBox.left + 0.5);
  }
}

/** The card in a column of the given width: the card's own width decides its layout, not the viewport. */
function atWidth(inlineSize: number) {
  return [
    (Story: () => ReactElement) => (
      <div style={{ inlineSize }}>
        <Story />
      </div>
    ),
  ];
}

const longAuthor = (
  <Author
    name="Bartholomew Montgomery-Fitzwilliam the Third"
    handle="@bartholomew@a-rather-long-community-server-name.social.example.com"
  />
);

/** A narrow card (below `@sm`): the time ends the header row. */
export const NarrowCard: Story = {
  decorators: atWidth(320),
  play: async ({ canvasElement }) => expectTimeAtRowEnd(canvasElement),
};

/** A medium card (`@sm` up to `@md`): the name and handle share a row and the time still ends it. */
export const MediumCard: Story = {
  decorators: atWidth(480),
  play: async ({ canvasElement }) => expectTimeAtRowEnd(canvasElement),
};

/** A wide card (from `@md`, the avatar in its own column): the time ends the header row. */
export const WideCard: Story = {
  decorators: atWidth(720),
  play: async ({ canvasElement }) => expectTimeAtRowEnd(canvasElement),
};

/** A long name and domain on a medium card are cut before the time, which stays at the end. */
export const MediumCardLongNames: Story = {
  decorators: atWidth(480),
  args: { author: longAuthor },
  play: async ({ canvasElement }) => expectTimeAtRowEnd(canvasElement),
};

/** A long name and domain on a wide card are cut before the time, which stays at the end. */
export const WideCardLongNames: Story = {
  decorators: atWidth(720),
  args: { author: longAuthor },
  play: async ({ canvasElement }) => expectTimeAtRowEnd(canvasElement),
};
