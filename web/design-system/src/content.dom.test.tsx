// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { RouterProvider } from "react-aria-components";
import { ContentBody, ContentLink, CustomEmoji, UrlPart } from "./content";

test("a body keeps its blocks in order, in its language", () => {
  render(
    <ContentBody lang="ja">
      <p>一つ目の段落</p>
      <blockquote>引用</blockquote>
      <ul>
        <li>項目</li>
      </ul>
    </ContentBody>,
  );

  const body = screen.getByText("一つ目の段落").closest("[lang]");
  expect(body?.getAttribute("lang")).toBe("ja");
  expect(Array.from(body?.children ?? [], (block) => block.textContent)).toEqual(["一つ目の段落", "引用", "項目"]);
  expect(within(screen.getByRole("list")).getByRole("listitem").textContent).toBe("項目");
});

test("a link in a text opens in a new tab and tells the page it leads to nothing of where it came from", () => {
  render(
    <ContentBody>
      <p>
        See{" "}
        <ContentLink href="https://example.com/a/long/path" kind="link">
          example.com
        </ContentLink>
      </p>
    </ContentBody>,
  );

  const link = screen.getByRole<HTMLAnchorElement>("link", { name: "example.com" });
  expect(link.getAttribute("href")).toBe("https://example.com/a/long/path");
  expect(link.target).toBe("_blank");
  expect(new Set(link.rel.split(" "))).toEqual(new Set(["nofollow", "noopener", "noreferrer"]));
});

test("a mention and a hashtag are links by their words that keep their language and that no translation may change", () => {
  render(
    <ContentBody lang="ja">
      <p>
        <ContentLink href="https://far.example/@alice" kind="mention" translate="no" lang="en">
          @alice
        </ContentLink>{" "}
        <ContentLink href="https://my.example/tags/cats" kind="hashtag" translate="no">
          #cats
        </ContentLink>
      </p>
    </ContentBody>,
  );

  const mention = screen.getByRole("link", { name: "@alice" });
  expect(mention.getAttribute("href")).toBe("https://far.example/@alice");
  expect(mention.lang).toBe("en");
  expect(mention.getAttribute("translate")).toBe("no");
  const hashtag = screen.getByRole("link", { name: "#cats" });
  expect(hashtag.getAttribute("href")).toBe("https://my.example/tags/cats");
  expect(hashtag.getAttribute("translate")).toBe("no");
  expect(hashtag.hasAttribute("lang")).toBe(false);
});

test("a link of the app opens in place through the app's navigation, by a click and by Enter", async () => {
  const user = userEvent.setup();
  const went: string[] = [];
  render(
    <RouterProvider navigate={(href) => went.push(href)}>
      <ContentBody>
        <p>
          <ContentLink href="/tags/cats" kind="hashtag" inApp>
            #cats
          </ContentLink>
        </p>
      </ContentBody>
    </RouterProvider>,
  );
  const link = screen.getByRole<HTMLAnchorElement>("link", { name: "#cats" });
  expect(link.getAttribute("href")).toBe("/tags/cats");
  expect(link.target).toBe("");

  await user.click(link);
  expect(went).toEqual(["/tags/cats"]);
  expect(document.activeElement).toBe(link);
  await user.keyboard("{Enter}");
  expect(went).toEqual(["/tags/cats", "/tags/cats"]);
});

test("a shortened URL keeps every part in the link's text, so that it is copied whole", () => {
  render(
    <ContentLink href="https://example.com/a/long/path" kind="link">
      <UrlPart part="hidden">https://</UrlPart>
      <UrlPart part="cut">example.com/a/lo</UrlPart>
      <UrlPart part="hidden">ng/path</UrlPart>
    </ContentLink>,
  );

  expect(screen.getByRole("link").textContent).toBe("https://example.com/a/long/path");
});

test("a custom emoji is an image read as its shortcode, which moves only where the reader allows motion", () => {
  render(
    <p>
      <CustomEmoji shortcode="blobcat" url="https://m.example/blobcat.gif" staticUrl="https://m.example/blobcat.png" />
    </p>,
  );

  const emoji = screen.getByRole<HTMLImageElement>("img", { name: ":blobcat:" });
  expect(emoji.title).toBe(":blobcat:");
  // The image by itself is the still one; the moving one is offered under the reader's preference alone.
  expect(emoji.getAttribute("src")).toBe("https://m.example/blobcat.png");
  const offered = Array.from(emoji.parentElement?.getElementsByTagName("source") ?? [], (source) => [
    source.getAttribute("media"),
    source.getAttribute("srcset"),
  ]);
  expect(offered).toEqual([["(prefers-reduced-motion: no-preference)", "https://m.example/blobcat.gif"]]);
});

test("a custom emoji that has no still image of its own offers its one image alone", () => {
  render(
    <p>
      <CustomEmoji shortcode="wave" url="https://m.example/wave.png" staticUrl="https://m.example/wave.png" />
    </p>,
  );

  const emoji = screen.getByRole("img", { name: ":wave:" });
  expect(emoji.getAttribute("src")).toBe("https://m.example/wave.png");
  expect(emoji.parentElement?.getElementsByTagName("source")).toHaveLength(0);
});
