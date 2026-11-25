// SPDX-License-Identifier: AGPL-3.0-only

// An article as a post shows it (ADR-0160 §8, #3089): its card in a timeline, and in full on its page, with the
// post's author line and actions either way; and what a sensitive article keeps from being asked for before Show.
// What the card and the page's parts are to assistive technology is the design system's (article.dom.test.tsx).

import { describe, expect, test } from "bun:test";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Post, type PostProps } from "./post";
import { PostContent } from "./post-content";
import type { PostAccount, PostArticle, PostStatus } from "./status";
import { asked, inLocale, inOrder, status as post, type Part } from "./test/post";

const sam: PostAccount = {
  id: "1",
  username: "sam",
  acct: "sam",
  domain: "my.example",
  display_name: "Sam Rivera",
  emojis: [],
};

const alice: PostAccount = {
  id: "2",
  username: "alice",
  acct: "alice@example.social",
  domain: "example.social",
  display_name: "Alice Chen",
  emojis: [],
};

const starter: PostArticle = {
  title: "A sourdough starter in seven days",
  summary: "What I fed it, when it rose, and the three mistakes that cost me a week.",
  cover: {
    id: "c",
    type: "image",
    url: "https://my.example/media/loaf.webp",
    preview_url: "https://my.example/media/loaf-small.webp",
    description: "A round loaf on a wooden board",
    meta: { width: 640, height: 400 },
    blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
  },
};

/** An article as a list gives it: its title, summary and cover, and an empty text. */
const status = (over: Partial<PostStatus> = {}): PostStatus =>
  post({ account: sam, content: "", boosts_count: 3, favourites_count: 4, article: starter, ...over });

const body =
  '<h2>The first three days</h2><p>Nothing, then bubbles.</p><p><img src="https://my.example/media/large/bubbles.webp" alt="Bubbles in a jar" width="800" height="600" loading="eager"></p><table><tbody><tr><td align="right">50 g</td></tr></tbody></table>';

const reading = <PostContent html={body} article />;

/** The post of `props`, alone in the document, for a reader of `locale`. */
function shown(props: PostProps, locale = "en") {
  cleanup();
  inLocale(<Post {...props} />, locale);
  const [article] = screen.getAllByRole("article");
  if (article === undefined) throw new Error("test: the post drew no article");
  return within(article);
}

/** The images of the post by their names; who may see it is one of them. */
const images = (part: Part) =>
  part
    .queryAllByRole("img", { hidden: true })
    .map((image) => image.getAttribute("aria-label") ?? image.getAttribute("alt"));

const name = (what: string, article: PostArticle = starter) =>
  new RegExp(
    `^${what}\\s*${article.title}${article.summary === "" ? "" : `\\s*${article.summary.replaceAll(".", "\\.")}`}$`,
  );

describe("an article in a timeline", () => {
  test("its card is one link to its page, saying what it is, with its title and summary", () => {
    const part = shown({ status: status() });

    expect(part.getByRole("link", { name: name("Article") }).getAttribute("href")).toBe("/@sam/status/10");
  });

  test("its cover shows with its description, after its title", () => {
    const part = shown({ status: status() });

    const cover = part.getByRole("img", { name: "A round loaf on a wooden board" });
    expect(inOrder(part.getByRole("link", { name: name("Article") }), cover)).toBe(true);
  });

  test("it keeps the post's author line and actions", () => {
    const part = shown({ status: status() });

    expect(part.getByRole("link", { name: "Sam Rivera" }).getAttribute("href")).toBe("/@sam");
    expect(part.getByRole("link", { name: "3m ago" }).getAttribute("href")).toBe("/@sam/status/10");
    for (const action of [/^Reply/, /^Favourite/, "More"])
      expect(part.getByRole("button", { name: action })).not.toBeNull();
  });

  test("in Japanese, it says it is an article in Japanese", () => {
    const part = shown({ status: status() }, "ja");

    expect(part.getByRole("link", { name: name("記事") })).not.toBeNull();
  });

  test("without a summary or a cover, the card is its title alone", () => {
    const bare = { title: starter.title, summary: "" };
    const part = shown({ status: status({ article: bare }) });

    expect(part.getByRole("link", { name: name("Article", bare) })).not.toBeNull();
    expect(images(part)).toEqual(["Public"]);
  });

  test("a sensitive article covers its cover, and its title and summary stay shown", () => {
    const part = shown({ status: status({ sensitive: true }) });

    expect(asked("https://my.example/media/")).toEqual([]);
    expect(part.getByRole("link", { name: name("Article") })).not.toBeNull();
  });

  test("a boosted article from another server links to its page there, under who boosted it", () => {
    const boosted = status({ id: "20", account: alice });
    const part = shown({ status: status({ id: "30", article: undefined, boost: boosted }) });

    expect(part.getByText(/boosted$/).textContent).toMatch(/^Sam Rivera\s*boosted$/);
    expect(part.getByRole("link", { name: name("Article") }).getAttribute("href")).toBe(
      "/@alice@example.social/status/20",
    );
  });

  test("the card is not its text: the reader is shown no heading of the article and none of its text", () => {
    const part = shown({ status: status() });

    expect(part.queryAllByRole("heading")).toHaveLength(0);
  });
});

describe("an article on its page", () => {
  test("its title is the page's heading over its summary, then its cover, then its text", () => {
    const part = shown({ status: status(), reading });

    expect(
      inOrder(
        part.getByRole("link", { name: "Sam Rivera" }),
        part.getByRole("heading", { level: 1, name: starter.title }),
        part.getByText(starter.summary),
        part.getByRole("img", { name: "A round loaf on a wooden board" }),
        part.getByRole("heading", { level: 2, name: "The first three days" }),
        part.getByRole("button", { name: /^Reply/ }),
      ),
    ).toBe(true);
  });

  test("its text is the full text, its headings, its image and its table, and no card leads away from it", () => {
    const part = shown({ status: status(), reading });

    expect(part.getByText("Nothing, then bubbles.")).not.toBeNull();
    expect(part.getByRole("img", { name: "Bubbles in a jar" }).getAttribute("src")).toBe(
      "https://my.example/media/large/bubbles.webp",
    );
    expect(within(part.getByRole("table")).getByRole("cell", { name: "50 g" }).getAttribute("align")).toBe("right");
    expect(part.queryAllByRole("link", { name: name("Article") })).toHaveLength(0);
  });

  test("it keeps the post's author line and actions", () => {
    const part = shown({ status: status(), reading });

    expect(part.getByRole("link", { name: "Sam Rivera" })).not.toBeNull();
    for (const action of [/^Reply/, /^Favourite/, "More"])
      expect(part.getByRole("button", { name: action })).not.toBeNull();
  });

  test("its text reads in its language", () => {
    const part = shown({ status: status({ language: "ja" }), reading });

    expect(part.getByText("Nothing, then bubbles.").closest("[lang]")?.getAttribute("lang")).toBe("ja");
  });

  test("without a summary, the heading stands alone", () => {
    const part = shown({ status: status({ article: { title: starter.title, summary: "" } }), reading });

    expect(part.getByRole("heading", { level: 1, name: starter.title }).parentElement?.childElementCount).toBe(1);
  });
});

describe("a sensitive article's page", () => {
  test("before Show: its head and its line are there, and none of its cover or text is, not even hidden", () => {
    const part = shown({ status: status({ sensitive: true }), reading });

    expect(part.getByRole("heading", { level: 1, name: starter.title })).not.toBeNull();
    expect(part.getByText(starter.summary)).not.toBeNull();
    expect(
      part.getByRole("button", { name: "Show", description: "Sensitive article", expanded: false }),
    ).not.toBeNull();
    // Nothing of the media's server is asked for: no image, no preload, nothing a hidden panel would still load.
    expect(asked("https://my.example/media/")).toEqual([]);
    expect(images(part)).toEqual(["Public"]);
    for (const words of ["The first three days", "Nothing, then bubbles.", "50 g"])
      expect(part.queryAllByText(words)).toHaveLength(0);
    expect(part.queryAllByRole("heading", { hidden: true })).toHaveLength(1);
    expect(part.queryAllByRole("table", { hidden: true })).toHaveLength(0);
  });

  test("Show shows its cover and its whole text", async () => {
    const user = userEvent.setup();
    const part = shown({ status: status({ sensitive: true }), reading });

    await user.click(part.getByRole("button", { name: "Show", expanded: false }));

    expect(part.getByRole("img", { name: "A round loaf on a wooden board" }).getAttribute("src")).toBe(
      "https://my.example/media/loaf.webp",
    );
    expect(part.getByRole("heading", { level: 2, name: "The first three days" })).not.toBeNull();
    expect(part.getByRole("img", { name: "Bubbles in a jar" })).not.toBeNull();
    expect(part.getByRole("table")).not.toBeNull();
  });

  test("its line reads in the reader's locale", () => {
    const part = shown({ status: status({ sensitive: true }), reading }, "ja");

    expect(part.getByRole("button", { name: "表示", description: "センシティブな記事" })).not.toBeNull();
  });

  test("an article that is not sensitive shows its cover and its whole text at once, with no line", () => {
    const part = shown({ status: status(), reading });

    expect(part.getByRole("img", { name: "A round loaf on a wooden board" }).getAttribute("src")).toBe(
      "https://my.example/media/loaf.webp",
    );
    expect(part.getByRole("img", { name: "Bubbles in a jar" })).not.toBeNull();
    expect(part.queryAllByText("Sensitive article")).toHaveLength(0);
  });
});

describe("a reply under the article", () => {
  const reply = () =>
    status({
      id: "11",
      account: alice,
      content: "<p>@sam Same smell here on day three.</p>",
      article: undefined,
      in_reply_to_account_id: "1",
      mentions: [{ id: "1", username: "sam", acct: "sam", url: "https://my.example/@sam" }],
    });

  test("says whom it replies to, as any reply does", () => {
    const part = shown({ status: reply() });

    expect(part.getByText("@sam").parentElement?.textContent).toBe("Replying to @sam");
  });

  test("drops that line where the screen shows it already", () => {
    const part = shown({ status: reply(), replyingTo: false });

    expect(part.queryAllByText(/Replying to/)).toHaveLength(0);
    expect(part.getByText("@sam Same smell here on day three.")).not.toBeNull();
  });
});
