// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { Avatar } from "./Avatar";
import { AutoplayContext } from "./Picture";
import { PostCard, PostCardAuthor } from "./PostCard";
import { PostContent, serverSanitizedHtml } from "./PostContent";

describe("PostCard", () => {
  it("is an article labelled by its author and time and described by its content", () => {
    render(
      <PostCard
        avatar={<Avatar name="Alice" decorative />}
        author="Alice @alice"
        time={<time dateTime="2026-10-16T09:00:00Z">2h</time>}
        content={<PostContent html={serverSanitizedHtml("<p>Hello</p>")} lang="en" />}
      />,
    );
    const article = screen.getByRole("article", { name: "Alice @alice 2h" });
    expect(article).toHaveAccessibleDescription("Hello");
    // Outside a feed it is a plain article: not focusable, no position.
    expect(article).not.toHaveAttribute("tabindex");
    expect(article).not.toHaveAttribute("aria-posinset");
  });

  it("starts its name with the context line when there is one", () => {
    render(
      <PostCard
        context="Bob boosted"
        author="Alice @alice"
        time={<time dateTime="2026-10-16T09:00:00Z">2h</time>}
        content={<PostContent html={serverSanitizedHtml("<p>Hello</p>")} lang="en" />}
      />,
    );
    expect(screen.getByRole("article", { name: "Bob boosted Alice @alice 2h" })).toHaveAccessibleDescription("Hello");
  });

  it("starts its name with its label, and shows its details outside the description", () => {
    render(
      <PostCard
        label="Selected post"
        highlighted
        lineAbove
        author="Alice @alice"
        time={<time dateTime="2026-10-16T09:00:00Z">2h</time>}
        content={<PostContent html={serverSanitizedHtml("<p>Hello</p>")} lang="en" />}
        details="3 boosts"
      />,
    );
    const article = screen.getByRole("article", { name: "Selected post Alice @alice 2h" });
    expect(article).toHaveAccessibleDescription("Hello");
    expect(screen.getByText("3 boosts")).toBeInTheDocument();
  });
});

const dance = { still: [{ url: "/still.webp", width: 96 }], animated: [{ url: "/moving.webp", width: 96 }] };

/** A post with an animated avatar and a link in its body, with animations that do not play by themselves unless `autoplay`. */
function renderAnimatedPost(autoplay = false) {
  render(
    <AutoplayContext value={autoplay}>
      <PostCard
        avatar={<Avatar sources={dance} name="Alice" decorative />}
        author="Alice @alice"
        time={<time dateTime="2026-10-16T09:00:00Z">2h</time>}
        content={
          <PostContent html={serverSanitizedHtml('<p>Hello <a href="https://example.com/">there</a></p>')} lang="en" />
        }
        actions={<button type="button">Reply</button>}
      />
    </AutoplayContext>,
  );
  const article = screen.getByRole("article", { name: "Alice @alice 2h" });
  // eslint-disable-next-line testing-library/no-node-access -- <source> has no role.
  const moves = () => article.querySelector("source") !== null;
  return { article, moves };
}

describe("PostCard animations", () => {
  it("plays them while the pointer is over the card", async () => {
    const user = userEvent.setup();
    const { article, moves } = renderAnimatedPost();
    expect(moves()).toBe(false);
    await user.hover(screen.getByText("Hello", { exact: false }));
    expect(moves()).toBe(true);
    await user.unhover(article);
    expect(moves()).toBe(false);
  });

  it("plays them while keyboard focus is inside the card", async () => {
    const user = userEvent.setup();
    const { moves } = renderAnimatedPost();
    await user.tab();
    expect(screen.getByRole("link", { name: "there" })).toHaveFocus();
    expect(moves()).toBe(true);
    await user.tab();
    expect(moves()).toBe(true);
    await user.tab();
    expect(moves()).toBe(false);
  });

  it("starts and stops them with a tap on the card, but not with a tap on a control, a drag or a long press", async () => {
    const user = userEvent.setup();
    const { article, moves } = renderAnimatedPost();
    const tap = (target: Element) =>
      user.pointer([
        { keys: "[TouchA>]", target },
        { keys: "[/TouchA]", target },
      ]);
    await tap(screen.getByRole("button", { name: "Reply" }));
    expect(moves()).toBe(false);
    await tap(screen.getByRole("link", { name: "there" }));
    expect(moves()).toBe(false);
    await user.pointer([
      { keys: "[TouchA>]", target: article, coords: { clientX: 10, clientY: 10 } },
      { pointerName: "TouchA", target: article, coords: { clientX: 10, clientY: 80 } },
      { keys: "[/TouchA]", target: article, coords: { clientX: 10, clientY: 80 } },
    ]);
    expect(moves()).toBe(false);
    await user.pointer({ keys: "[TouchA>]", target: article });
    await new Promise((resolve) => setTimeout(resolve, 550));
    await user.pointer({ keys: "[/TouchA]", target: article });
    expect(moves()).toBe(false);
    await tap(article);
    expect(moves()).toBe(true);
    await tap(screen.getByText("Hello", { exact: false }));
    expect(moves()).toBe(false);
  });

  it("changes nothing while animations play by themselves", async () => {
    const user = userEvent.setup();
    const { article, moves } = renderAnimatedPost(true);
    expect(moves()).toBe(true);
    await user.pointer([
      { keys: "[TouchA>]", target: article },
      { keys: "[/TouchA]", target: article },
    ]);
    expect(moves()).toBe(true);
  });
});

describe("PostCardAuthor", () => {
  it("names a link around it by the full name and handle, the handle left to right", () => {
    const handle = "@carol@a-rather-long-community-server-name.social.example.com";
    render(
      <a href="#profile">
        <PostCardAuthor name="Carol" handle={handle} />
      </a>,
    );
    expect(screen.getByRole("link", { name: `Carol ${handle}` })).toBeInTheDocument();
    // The ellipsis of a cut handle is at its end, the domain's side, in a right-to-left page too.
    expect(screen.getByText(handle)).toHaveAttribute("dir", "ltr");
    expect(screen.getByText("Carol")).toHaveAttribute("dir", "auto");
  });

  it("is the name alone without a handle", () => {
    render(
      <a href="#profile">
        <PostCardAuthor name="Bob" />
      </a>,
    );
    expect(screen.getByRole("link", { name: "Bob" })).toBeInTheDocument();
  });
});
