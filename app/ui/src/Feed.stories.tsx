// SPDX-License-Identifier: AGPL-3.0-only
// Feed stories. The play functions test the feed pattern in a real browser: Page Down / Page Up between
// articles, Control + End / Control + Home out of the feed, aria-busy, and, in the long feed, that the
// focused article stays mounted when virtualization unmounts its neighbours.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, waitFor } from "storybook/test";
import { Avatar } from "./Avatar";
import { Button } from "./Button";
import { Feed, type FeedProps } from "./Feed";
import { NewItemsButton } from "./NewItemsButton";
import { PostCard } from "./PostCard";
import { PostContent, serverSanitizedHtml } from "./PostContent";

type Post = { id: string; author: string; body: string };

const names = ["Alice", "Bob", "Carol", "Dave", "Erin", "Frank", "Grace", "Heidi"];

function makePosts(count: number, from = 1): Post[] {
  return Array.from({ length: count }, (_, i) => {
    const n = from + i;
    return { id: String(n), author: names[n % names.length] ?? "Alice", body: `Post number ${String(n)}.` };
  });
}

function renderPost(post: Post) {
  return (
    <PostCard
      avatar={<Avatar name={post.author} decorative size="sm" />}
      author={<strong>{post.author}</strong>}
      time={<time dateTime="2026-10-16T09:00:00Z">2h</time>}
      visibility={{ level: "public", label: "Public" }}
      content={<PostContent html={serverSanitizedHtml(`<p>${post.body}</p>`)} lang="en" />}
      actions={
        <Button variant="ghost" size="sm">
          Reply
        </Button>
      }
    />
  );
}

/** A feed of posts under its heading, with a control before it so Control + Home has somewhere to go. */
function HomeFeed(props: FeedProps<Post>) {
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between gap-2">
        <h2 id="home-heading" className="text-xl font-semibold">
          Home
        </h2>
        <Button size="sm">Compose</Button>
      </div>
      <Feed {...props} />
    </div>
  );
}

const meta = {
  title: "Timeline/Feed",
  component: HomeFeed,
  args: {
    "aria-labelledby": "home-heading",
    items: makePosts(4),
    getKey: (post) => post.id,
    renderItem: renderPost,
    isLoading: false,
    hasMore: true,
    onLoadMore: fn(),
    loadMoreLabel: "Load more posts",
    loadingLabel: "Loading posts",
    estimateSize: 120,
  },
} satisfies Meta<typeof HomeFeed>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Keyboard: Story = {
  play: async ({ canvas, userEvent }) => {
    const feed = canvas.getByRole("feed", { name: "Home" });
    await expect(feed).toHaveAttribute("aria-busy", "false");
    const articles = canvas.getAllByRole("article");
    await expect(articles.map((article) => article.getAttribute("aria-posinset"))).toEqual(["1", "2", "3", "4"]);
    await expect(articles.map((article) => article.getAttribute("aria-setsize"))).toEqual(["-1", "-1", "-1", "-1"]);

    await userEvent.tab();
    await userEvent.tab();
    await expect(articles[0]).toHaveFocus();
    await userEvent.keyboard("{PageDown}{PageDown}");
    await expect(articles[2]).toHaveFocus();
    await userEvent.keyboard("{PageUp}");
    await expect(articles[1]).toHaveFocus();
    await userEvent.keyboard("{Control>}{End}{/Control}");
    await expect(canvas.getByRole("button", { name: "Load more posts" })).toHaveFocus();
    await userEvent.click(articles[1] ?? feed);
    await userEvent.keyboard("{Control>}{Home}{/Control}");
    await expect(canvas.getByRole("button", { name: "Compose" })).toHaveFocus();
  },
};

/** Loading the next page: the feed is busy and the button is pending, keeping its focus. */
export const Loading: Story = {
  args: { isLoading: true },
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("feed", { name: "Home" })).toHaveAttribute("aria-busy", "true");
    await expect(canvas.getByRole("button", { name: /Load more posts/ })).toHaveAttribute("aria-disabled", "true");
  },
};

/** The end of the feed: no button, and the set size is known. */
export const End: Story = {
  args: { hasMore: false },
  play: async ({ canvas }) => {
    await expect(canvas.queryByRole("button", { name: "Load more posts" })).not.toBeInTheDocument();
    await expect(canvas.getAllByRole("article").map((article) => article.getAttribute("aria-setsize"))).toEqual([
      "4",
      "4",
      "4",
      "4",
    ]);
  },
};

/** New posts wait behind the button; pressing it inserts them at the top. */
export const WithNewItems: Story = {
  args: { hasMore: false },
  render: function WithNewItems(args) {
    const [items, setItems] = useState(() => makePosts(3, 3));
    const waiting = items[0]?.id === "3" ? 2 : 0;
    return (
      <div className="flex flex-col gap-2">
        <h2 id="home-heading" className="text-xl font-semibold">
          Home
        </h2>
        <NewItemsButton
          count={waiting}
          label={`${String(waiting)} new posts`}
          onPress={() => {
            setItems([...makePosts(2), ...items]);
          }}
        />
        <Feed {...args} items={items} />
      </div>
    );
  },
  play: async ({ canvas, userEvent }) => {
    await expect(canvas.getAllByRole("article")).toHaveLength(3);
    await userEvent.click(canvas.getByRole("button", { name: "2 new posts" }));
    await waitFor(() => expect(canvas.getAllByRole("article")).toHaveLength(5));
    await expect(canvas.getByRole("status")).toBeEmptyDOMElement();
  },
};

/**
 * A long feed: only the articles near the viewport are mounted, and the focused one stays mounted when
 * the page scrolls away from it. Page Down from there reaches the next article even though it was not mounted.
 */
export const Virtualized: Story = {
  tags: ["no-screenshot"],
  args: { items: makePosts(300), hasMore: false, overscan: 2 },
  play: async ({ canvas, canvasElement, userEvent }) => {
    const feed = canvas.getByRole("feed", { name: "Home" });
    await waitFor(() => expect(canvas.getAllByRole("article").length).toBeLessThan(60));
    await userEvent.tab();
    await userEvent.tab();
    const [first] = canvas.getAllByRole("article");
    await expect(first).toHaveAttribute("aria-posinset", "1");
    await expect(first).toHaveFocus();

    window.scrollTo({ top: feed.getBoundingClientRect().bottom + window.scrollY - window.innerHeight });
    await waitFor(() => expect(canvasElement.querySelector('[aria-posinset="300"]')).not.toBeNull());
    await expect(first).toBeInTheDocument();
    await expect(first).toHaveFocus();
    await expect(canvasElement.querySelector('[aria-posinset="2"]')).toBeNull();

    await userEvent.keyboard("{PageDown}");
    await waitFor(() => expect(canvasElement.querySelector('[aria-posinset="2"]')).toHaveFocus());
    window.scrollTo({ top: 0 });
  },
};
