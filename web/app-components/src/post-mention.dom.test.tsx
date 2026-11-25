// SPDX-License-Identifier: AGPL-3.0-only

// Where a mention in a post's text leads: to the profile of an account the post's mentions name, in place through the
// app's navigation, and as the server wrote it for one they do not. And where a post's author avatar leads. The rest of
// a post's text is in post-content.dom.test.tsx, the rest of a post in post.dom.test.tsx.

import { describe, expect, test } from "bun:test";
import { NavigationProvider } from "@my/design-system";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { Post } from "./post";
import { PostContent } from "./post-content";
import { QuoteContext, ReplyContext } from "./post-context";
import type { PostMention } from "./status";
import { inLocale, ken, status } from "./test/post";

/** An account of this server. */
const sam: PostMention = { id: "4", username: "sam", acct: "sam", url: "https://my.example/@sam" };
/** An account of another server. */
const alice: PostMention = {
  id: "5",
  username: "alice",
  acct: "alice@mastodon.example",
  url: "https://mastodon.example/@alice",
};

/** A mention as the server renders it, of the account whose profile is at `href`. */
const mention = (href: string, username: string) =>
  `<span class="h-card" translate="no"><a href="${href}" class="u-url mention">@<span>${username}</span></a></span>`;

/** A text that mentions sam, alice, and ravi, whom no post here names. */
const html = `<p>${mention("https://my.example/@sam", "sam")} ${mention("https://mastodon.example/@alice", "alice")} ${mention("https://other.example/@ravi", "ravi")} hello</p>`;

/** `node` alone in the document, under the app's navigation; and where the app was asked to go. */
function navigating(node: ReactNode) {
  cleanup();
  const went: string[] = [];
  inLocale(<NavigationProvider navigate={(href) => went.push(href)}>{node}</NavigationProvider>);
  return went;
}

/** The link of the mention of `username`. */
const mentionOf = (username: string) =>
  screen.getByRole<HTMLAnchorElement>("link", { name: new RegExp(`^@\\s*${username}$`) });

/** Where a link leads, and whether it opens in a new tab. */
const leads = (link: HTMLAnchorElement) => [link.getAttribute("href"), link.target];

describe("a mention in a post's text", () => {
  test("of an account the post names opens its profile in place, a local one and a remote one", async () => {
    const user = userEvent.setup();
    const went = navigating(<PostContent html={html} lang="en" mentions={[sam, alice]} />);

    expect(leads(mentionOf("sam"))).toEqual(["/@sam", ""]);
    expect(leads(mentionOf("alice"))).toEqual(["/@alice@mastodon.example", ""]);
    await user.click(mentionOf("sam"));
    await user.click(mentionOf("alice"));

    expect(went).toEqual(["/@sam", "/@alice@mastodon.example"]);
    // It is still a mention, not to be translated with the text around it.
    expect(mentionOf("sam").closest("[translate]")?.getAttribute("translate")).toBe("no");
  });

  test("of an account the post does not name keeps leaving the app in a new tab, as the server wrote it", () => {
    for (const mentions of [[sam, alice], [], undefined]) {
      navigating(<PostContent html={html} lang="en" mentions={mentions} />);
      const ravi = mentionOf("ravi");

      expect(leads(ravi)).toEqual(["https://other.example/@ravi", "_blank"]);
      expect(ravi.rel.split(" ").sort()).toEqual(["nofollow", "noopener", "noreferrer"]);
    }
    expect(leads(mentionOf("sam"))).toEqual(["https://my.example/@sam", "_blank"]);
  });

  test("in a post opens the profiles its status mentions in place", async () => {
    const user = userEvent.setup();
    const went = navigating(
      <Post status={status({ content: html, mentions: [sam, alice] })} onAction={() => undefined} />,
    );

    expect(leads(mentionOf("sam"))).toEqual(["/@sam", ""]);
    expect(leads(mentionOf("alice"))).toEqual(["/@alice@mastodon.example", ""]);
    expect(leads(mentionOf("ravi"))).toEqual(["https://other.example/@ravi", "_blank"]);
    await user.click(mentionOf("alice"));

    expect(went).toEqual(["/@alice@mastodon.example"]);
  });

  test("in a boosted post opens the profiles the boosted status mentions", () => {
    const boosted = status({ content: html, mentions: [sam] });
    navigating(<Post status={status({ id: "20", account: ken, boost: boosted })} onAction={() => undefined} />);

    expect(leads(mentionOf("sam"))).toEqual(["/@sam", ""]);
    expect(leads(mentionOf("alice"))).toEqual(["https://mastodon.example/@alice", "_blank"]);
  });

  test("in a quoted post opens the profiles the quoted status mentions", () => {
    const quoted = status({ id: "30", account: ken, content: html, mentions: [alice] });
    navigating(
      <Post status={status({ quote: { state: "accepted", quoted_status: quoted } })} onAction={() => undefined} />,
    );

    expect(leads(mentionOf("alice"))).toEqual(["/@alice@mastodon.example", ""]);
    expect(leads(mentionOf("sam"))).toEqual(["https://my.example/@sam", "_blank"]);
  });

  test("in the post a reply answers opens the profiles that post mentions", () => {
    navigating(<ReplyContext status={status({ content: html, mentions: [sam, alice] })} note="Replying to Lena" />);

    expect(leads(mentionOf("sam"))).toEqual(["/@sam", ""]);
    expect(leads(mentionOf("alice"))).toEqual(["/@alice@mastodon.example", ""]);
    expect(leads(mentionOf("ravi"))).toEqual(["https://other.example/@ravi", "_blank"]);
  });

  test("in the post a quote being written embeds opens the profiles that post mentions", () => {
    navigating(<QuoteContext status={status({ content: html, mentions: [alice] })} />);

    expect(leads(mentionOf("alice"))).toEqual(["/@alice@mastodon.example", ""]);
    expect(leads(mentionOf("sam"))).toEqual(["https://my.example/@sam", "_blank"]);
  });
});

describe("a post's author avatar", () => {
  test("leads to the author's profile in place, as the name does, and is not a second link to hear", async () => {
    const user = userEvent.setup();
    const went = navigating(<Post status={status({ account: ken })} onAction={() => undefined} />);
    const [article] = screen.getAllByRole("article");
    if (article === undefined) throw new Error("test: the post drew no article");
    const part = within(article);

    const name = part.getByRole("link", { name: /^Kenji\s*:ramen:$/ });
    const avatar = part
      .getAllByRole("presentation", { hidden: true })
      .find((image) => image.getAttribute("src") === "https://my.example/kenji.webp")
      ?.closest("a");
    expect(avatar?.getAttribute("href")).toBe(name.getAttribute("href"));
    expect([avatar?.getAttribute("tabindex"), avatar?.getAttribute("aria-hidden")]).toEqual(["-1", "true"]);
    expect(
      part.getAllByRole("link").filter((link) => link.getAttribute("href") === name.getAttribute("href")),
    ).toHaveLength(1);

    if (avatar !== null && avatar !== undefined) await user.click(avatar);
    expect(went).toEqual([name.getAttribute("href")]);
  });
});
