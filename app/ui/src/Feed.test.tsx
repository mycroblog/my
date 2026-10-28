// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen, within } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ariaSnapshot } from "../test/aria-snapshot";
import { Button } from "./Button";
import { Feed, type FeedProps } from "./Feed";
import { PostCard } from "./PostCard";
import { PostContent, serverSanitizedHtml } from "./PostContent";

type Post = { id: string; author: string; body: string };

const posts: readonly Post[] = [
  { id: "1", author: "Alice", body: "First post" },
  { id: "2", author: "Bob", body: "Second post" },
  { id: "3", author: "Carol", body: "Third post" },
];

function Timeline(props: Partial<FeedProps<Post>>) {
  return (
    <>
      <Button>Compose</Button>
      <h2 id="home">Home</h2>
      <Feed
        aria-labelledby="home"
        items={posts}
        getKey={(post) => post.id}
        isLoading={false}
        hasMore
        onLoadMore={() => undefined}
        loadMoreLabel="Load more posts"
        loadingLabel="Loading posts"
        renderItem={(post) => (
          <PostCard
            author={<strong>{post.author}</strong>}
            time={<time dateTime="2026-10-16T09:00:00Z">2h</time>}
            content={<PostContent html={serverSanitizedHtml(`<p>${post.body}</p>`)} />}
            actions={<Button size="sm">Reply</Button>}
          />
        )}
        {...props}
      />
    </>
  );
}

describe("Feed", () => {
  it("is a feed of articles with their position, labelled by its heading", () => {
    render(<Timeline />);
    const feed = screen.getByRole("feed", { name: "Home" });
    expect(ariaSnapshot(feed)).toBe(
      [
        `- article "Alice 2h" ("First post") [posinset=1] [setsize=-1]:`,
        `  - text: "Alice"`,
        `  - text: "2h"`,
        `  - generic [dir=auto]:`,
        `    - text: "First post"`,
        `  - button "Reply"`,
        `- article "Bob 2h" ("Second post") [posinset=2] [setsize=-1]:`,
        `  - text: "Bob"`,
        `  - text: "2h"`,
        `  - generic [dir=auto]:`,
        `    - text: "Second post"`,
        `  - button "Reply"`,
        `- article "Carol 2h" ("Third post") [posinset=3] [setsize=-1]:`,
        `  - text: "Carol"`,
        `  - text: "2h"`,
        `  - generic [dir=auto]:`,
        `    - text: "Third post"`,
        `  - button "Reply"`,
      ].join("\n"),
    );
    expect(screen.getAllByRole("article").map((article) => article.tabIndex)).toEqual([0, 0, 0]);
  });

  it("knows the set size when there is nothing more to load, or when it is given", () => {
    const { rerender } = render(<Timeline hasMore={false} />);
    expect(screen.getAllByRole("article").map((article) => article.getAttribute("aria-setsize"))).toEqual([
      "3",
      "3",
      "3",
    ]);
    expect(screen.queryByRole("button", { name: "Load more posts" })).not.toBeInTheDocument();
    rerender(<Timeline setSize={40} />);
    expect(screen.getAllByRole("article").map((article) => article.getAttribute("aria-setsize"))).toEqual([
      "40",
      "40",
      "40",
    ]);
  });

  it("is busy exactly while loading", () => {
    const { rerender } = render(<Timeline isLoading />);
    const feed = screen.getByRole("feed", { name: "Home" });
    expect(feed).toHaveAttribute("aria-busy", "true");
    expect(screen.getByRole("button", { name: /Load more posts/ })).toHaveAttribute("aria-disabled", "true");
    rerender(<Timeline isLoading={false} />);
    expect(feed).toHaveAttribute("aria-busy", "false");
  });

  it("moves between articles with Page Down and Page Up", async () => {
    const user = userEvent.setup();
    render(<Timeline />);
    const [first, second, third] = screen.getAllByRole("article");
    await user.tab();
    await user.tab();
    expect(first).toHaveFocus();
    await user.keyboard("{PageDown}");
    expect(second).toHaveFocus();
    await user.keyboard("{PageDown}{PageDown}");
    // The last article stays focused: there is no next one.
    expect(third).toHaveFocus();
    await user.keyboard("{PageUp}");
    expect(second).toHaveFocus();
  });

  it("moves from an element inside an article to the next article", async () => {
    const user = userEvent.setup();
    render(<Timeline />);
    await user.click(within(screen.getByRole("article", { name: "Alice 2h" })).getByRole("button", { name: "Reply" }));
    await user.keyboard("{PageDown}");
    expect(screen.getByRole("article", { name: "Bob 2h" })).toHaveFocus();
  });

  it("leaves the feed with Control + End and Control + Home", async () => {
    const user = userEvent.setup();
    render(<Timeline />);
    await user.click(screen.getByRole("article", { name: "Bob 2h" }));
    await user.keyboard("{Control>}{End}{/Control}");
    expect(screen.getByRole("button", { name: "Load more posts" })).toHaveFocus();

    await user.click(screen.getByRole("article", { name: "Bob 2h" }));
    await user.keyboard("{Control>}{Home}{/Control}");
    expect(screen.getByRole("button", { name: "Compose" })).toHaveFocus();
  });

  it("loads more from the button", async () => {
    const user = userEvent.setup();
    const onLoadMore = vi.fn();
    render(<Timeline onLoadMore={onLoadMore} />);
    await user.click(screen.getByRole("button", { name: "Load more posts" }));
    expect(onLoadMore).toHaveBeenCalledOnce();
  });

  it("leaves keys typed into a text field alone", async () => {
    const user = userEvent.setup();
    render(
      <Timeline
        renderItem={(post) => (
          <PostCard author={post.author} time="2h" content={<input aria-label={`Reply to ${post.author}`} />} />
        )}
      />,
    );
    const field = screen.getByRole("textbox", { name: "Reply to Alice" });
    await user.click(field);
    await user.keyboard("{PageDown}");
    expect(field).toHaveFocus();
  });
});
