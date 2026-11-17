// SPDX-License-Identifier: AGPL-3.0-only

// What an article's card and the parts of its page are to a reader and to assistive technology: the card's one link
// and what it says, the page's heading, the cover's description, what a sensitive or folded article keeps from being
// asked for, and the language and direction of what the author wrote.

import { describe, expect, test } from "bun:test";
import { Disclosure, I18nProvider } from "@my/design-system";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import {
  ArticleCard,
  ArticleCover,
  ArticleParts,
  ArticleTable,
  ArticleText,
  ArticleTitle,
  type ArticleCardProps,
  type ArticleCoverImage,
} from "./article";

const cover: ArticleCoverImage = {
  src: "https://my.example/media/loaf.webp",
  width: 640,
  height: 400,
  alt: "A round loaf on a wooden board",
  blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj",
};

const card: ArticleCardProps = {
  href: "/@sam/status/10",
  title: "A sourdough starter in seven days",
  summary: "A diary of a jar of flour.",
  cover,
  lang: "en",
};

const inLocale = (locale: string, node: ReactNode) => render(<I18nProvider locale={locale}>{node}</I18nProvider>);

/** Whether each of `elements` follows the one before it in the document, the order a screen reader reads them in. */
const inOrder = (...elements: Element[]) =>
  elements.every((element, i) => {
    const before = elements[i - 1];
    return before === undefined || (before.compareDocumentPosition(element) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;
  });

/** Every address the document would have the browser fetch from the media's server. */
const asked = () =>
  Array.from(document.querySelectorAll("*"))
    .flatMap((element) => element.getAttributeNames().map((name) => element.getAttribute(name) ?? ""))
    .filter((value) => value.includes("https://my.example/media/"));

describe("the article's card", () => {
  test("one link to the article's page in the app, which says what it is, its title and its summary", () => {
    render(<ArticleCard {...card} />);

    const link = screen.getByRole<HTMLAnchorElement>("link", {
      name: /^Article\s*A sourdough starter in seven days\s*A diary of a jar of flour\.$/,
    });
    expect(screen.getAllByRole("link")).toEqual([link]);
    expect(link.getAttribute("href")).toBe("/@sam/status/10");
    // The page is the app's: it opens in place.
    expect(link.target).toBe("");
  });

  test("what it is reads in the reader's locale, marked with that language; the author's words keep theirs", () => {
    inLocale("ja", <ArticleCard {...card} />);

    const link = screen.getByRole("link", {
      name: /^記事\s*A sourdough starter in seven days\s*A diary of a jar of flour\.$/,
    });
    expect(within(link).getByText("記事").closest("[lang]")?.getAttribute("lang")).toMatch(/^ja/);
    for (const words of [card.title, card.summary])
      expect(within(link).getByText(words).closest("[lang]")?.getAttribute("lang")).toBe("en");
  });

  test("its cover comes after them, with its description", () => {
    render(<ArticleCard {...card} />);

    const image = screen.getByRole<HTMLImageElement>("img", { name: "A round loaf on a wooden board" });
    expect(image.getAttribute("src")).toBe(cover.src);
    expect(inOrder(screen.getByRole("link"), image)).toBe(true);
  });

  test("without a summary or a cover, its title alone", () => {
    render(<ArticleCard {...card} summary="" cover={undefined} />);

    expect(screen.getByRole("link", { name: /^Article\s*A sourdough starter in seven days$/ })).not.toBeNull();
    expect(screen.queryByRole("img")).toBeNull();
  });

  test("sensitive, its cover is covered and its file not asked for; its title and summary stay", () => {
    render(<ArticleCard {...card} sensitive />);

    expect(screen.queryByRole("img")).toBeNull();
    expect(asked()).toEqual([]);
    expect(
      screen.getByRole("button", { name: "Show", description: "Sensitive image", expanded: false }),
    ).not.toBeNull();
    expect(
      screen.getByRole("link", {
        name: /^Article\s*A sourdough starter in seven days\s*A diary of a jar of flour\.$/,
      }),
    ).not.toBeNull();
  });

  test("Show on a sensitive card shows its cover, outside the link, and the focus stays on the button", async () => {
    const user = userEvent.setup();
    render(<ArticleCard {...card} sensitive />);
    const show = screen.getByRole("button", { name: "Show", expanded: false });
    expect(screen.getByRole("link").contains(show)).toBe(false);

    await user.click(show);

    expect(screen.getByRole("img", { name: "A round loaf on a wooden board" }).getAttribute("src")).toBe(cover.src);
    expect(document.activeElement === screen.getByRole("button", { name: "Hide", expanded: true })).toBe(true);
  });
});

describe("the article's page", () => {
  test("its title is the page's heading, over its summary, each in its own direction, in the article's language", () => {
    render(<ArticleTitle title="A sourdough starter" summary="A diary of a jar of flour." lang="ja" />);

    const title = screen.getByRole("heading", { level: 1, name: "A sourdough starter" });
    const summary = screen.getByText("A diary of a jar of flour.");
    expect(inOrder(title, summary)).toBe(true);
    for (const part of [title, summary]) {
      expect(part.dir).toBe("auto");
      expect(part.closest("[lang]")?.getAttribute("lang")).toBe("ja");
    }
  });

  test("where the page has its own heading, the title is a heading of the second level", () => {
    render(<ArticleTitle title="A sourdough starter" summary="" level={2} />);

    expect(screen.getByRole("heading", { level: 2, name: "A sourdough starter" })).not.toBeNull();
    expect(screen.queryByRole("heading", { level: 1 })).toBeNull();
  });

  test("an empty summary is no paragraph", () => {
    render(<ArticleTitle title="A sourdough starter" summary="" />);

    expect(screen.getByRole("heading", { level: 1, name: "A sourdough starter" })).not.toBeNull();
    expect(screen.queryByRole("paragraph")).toBeNull();
  });

  test("its cover has its description", () => {
    render(<ArticleCover cover={cover} />);

    expect(screen.getByRole("img", { name: "A round loaf on a wooden board" }).getAttribute("src")).toBe(cover.src);
  });

  test("a sensitive article's cover is covered, its file not asked for before Show", async () => {
    const user = userEvent.setup();
    render(<ArticleCover cover={cover} sensitive />);

    expect(screen.queryByRole("img")).toBeNull();
    expect(asked()).toEqual([]);
    await user.click(screen.getByRole("button", { name: "Show", description: "Sensitive image", expanded: false }));

    expect(screen.getByRole("img", { name: "A round loaf on a wooden board" })).not.toBeNull();
  });

  test("in a fold, neither the cover's picture nor the text is there before the fold opens", async () => {
    const user = userEvent.setup();
    render(
      <Disclosure summary="Sensitive article">
        <ArticleCover cover={cover} folded />
        <ArticleText folded>
          <p>Nothing, then bubbles.</p>
          <img src="https://my.example/media/bubbles.webp" alt="Bubbles in a jar" />
        </ArticleText>
      </Disclosure>,
    );

    expect(asked()).toEqual([]);
    // Not even hidden: a hidden image is still asked for once the page finds it.
    expect(screen.queryByText("Nothing, then bubbles.")).toBeNull();
    expect(screen.queryByRole("img", { hidden: true })).toBeNull();

    await user.click(screen.getByRole("button", { name: "Show", expanded: false }));

    expect(screen.getByRole("img", { name: "A round loaf on a wooden board" }).getAttribute("src")).toBe(cover.src);
    expect(screen.getByText("Nothing, then bubbles.")).not.toBeNull();
    expect(screen.getByRole("img", { name: "Bubbles in a jar" })).not.toBeNull();
  });

  test("its text holds its language and reads in its own direction", () => {
    render(
      <ArticleText lang="ar">
        <p>مرحبا بالعالم</p>
      </ArticleText>,
    );

    const text = screen.getByText("مرحبا بالعالم").closest("[lang]");
    expect(text?.getAttribute("lang")).toBe("ar");
    expect(text?.getAttribute("dir")).toBe("auto");
  });

  test("a table is in a box a keyboard reaches, to scroll it; its columns run in its own direction, in its language", async () => {
    const user = userEvent.setup();
    render(
      <>
        <ArticleTable lang="ar">
          <tbody>
            <tr>
              <td>50 g</td>
            </tr>
          </tbody>
        </ArticleTable>
        <button type="button">After</button>
      </>,
    );
    const table = screen.getByRole<HTMLTableElement>("table");

    expect(within(table).getByRole("cell", { name: "50 g" })).not.toBeNull();
    expect(table.lang).toBe("ar");
    expect(table.dir).toBe("auto");
    await user.tab();
    expect(document.activeElement?.contains(table)).toBe(true);
    expect(document.activeElement === document.body).toBe(false);
    await user.tab();
    expect(document.activeElement === screen.getByRole("button", { name: "After" })).toBe(true);
  });

  test("its parts are read in the order given: the head, the cover, the text", () => {
    render(
      <ArticleParts>
        <ArticleTitle title="A sourdough starter" summary="A diary of a jar of flour." />
        <ArticleCover cover={cover} />
        <ArticleText>
          <p>Nothing, then bubbles.</p>
        </ArticleText>
      </ArticleParts>,
    );

    expect(
      inOrder(
        screen.getByRole("heading", { level: 1 }),
        screen.getByText("A diary of a jar of flour."),
        screen.getByRole("img", { name: "A round loaf on a wooden board" }),
        screen.getByText("Nothing, then bubbles."),
      ),
    ).toBe(true);
  });
});
