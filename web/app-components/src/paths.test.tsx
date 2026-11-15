// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { hashtagPath, tagPath } from "./paths";
import { PostContent, ServerDomain } from "./post-content";

describe("tagPath", () => {
  test("is the app's address of a hashtag, its name encoded", () => {
    expect(tagPath("baking")).toBe("/tags/baking");
    expect(tagPath("山")).toBe(`/tags/${encodeURIComponent("山")}`);
  });
});

describe("hashtagPath", () => {
  test("a hashtag's page on this server is the app's hashtag screen", () => {
    expect(hashtagPath("https://my.example/tags/baking", "my.example")).toBe("/tags/baking");
    expect(hashtagPath("https://my.example/tags/baking/", "my.example")).toBe("/tags/baking");
    expect(hashtagPath("https://MY.example/tags/baking", "My.Example")).toBe("/tags/baking");
  });

  test("a hashtag in another script keeps its encoded name", () => {
    const encoded = encodeURIComponent("山");
    expect(hashtagPath(`https://my.example/tags/${encoded}`, "my.example")).toBe(`/tags/${encoded}`);
  });

  test("another server's hashtag, or another page of this server, opens as it is", () => {
    expect(hashtagPath("https://other.example/tags/baking", "my.example")).toBeUndefined();
    expect(hashtagPath("https://my.example.evil/tags/baking", "my.example")).toBeUndefined();
    expect(hashtagPath("https://my.example/@yuki", "my.example")).toBeUndefined();
    expect(hashtagPath("https://my.example/tags/baking/extra", "my.example")).toBeUndefined();
    expect(hashtagPath("https://my.example/tags/", "my.example")).toBeUndefined();
  });

  test("without the server's domain, or with no URL, there is no app address", () => {
    expect(hashtagPath("https://my.example/tags/baking", undefined)).toBeUndefined();
    expect(hashtagPath("not a url", "my.example")).toBeUndefined();
  });
});

describe("a hashtag in a post", () => {
  const html =
    '<p><a href="https://my.example/tags/fediverse" class="mention hashtag" rel="tag">#<span>fediverse</span></a> ' +
    '<a href="https://other.example/tags/cats" class="mention hashtag" rel="tag">#<span>cats</span></a></p>';
  const render = (domain: string | undefined) =>
    renderToStaticMarkup(
      <ServerDomain value={domain}>
        <PostContent html={html} lang="en" />
      </ServerDomain>,
    ).replace(/ class="[^"]*"/g, "");

  /** The opening tag of the link whose text is `#name`. */
  const link = (markup: string, name: string) =>
    new RegExp(`<a [^>]*>#<span>${name}</span></a>`).exec(markup)?.[0] ?? "";

  test("of this server opens the app's hashtag screen in place; another server's opens in a new tab", () => {
    const markup = render("my.example");
    expect(link(markup, "fediverse")).toContain('href="/tags/fediverse"');
    expect(link(markup, "fediverse")).not.toContain('target="_blank"');
    expect(link(markup, "cats")).toContain('href="https://other.example/tags/cats"');
    expect(link(markup, "cats")).toContain('target="_blank"');
  });

  test("before the server's domain is known, opens as the server wrote it", () => {
    const markup = render(undefined);
    expect(link(markup, "fediverse")).toContain('href="https://my.example/tags/fediverse"');
    expect(link(markup, "fediverse")).toContain('target="_blank"');
  });
});
