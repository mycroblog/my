// SPDX-License-Identifier: AGPL-3.0-only
// The emoji picker of the post form and the profile (web-client/README.md T4, ADR-0091, ADR-0051 §4): the real router
// and QueryClient, MSW for the server, and the Unicode emoji data of features/emoji/unicode.
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { frequentEmojisKey, skinToneKey } from "../../lib/storage";
import { preferencesFixture } from "../../test/fixtures";
import { apiBase, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { withUse } from "./hooks/frequentEmojis";
import { insertShortcode, insertText } from "./hooks/insertShortcode";
import { customEmojisHandler, pickerEmojiFixture } from "./test/handlers";

beforeEach(() => {
  sessionStorage.clear();
  localStorage.clear();
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

describe("inserting an emoji", () => {
  test("puts a shortcode at the caret with the spaces that keep it a word of its own", () => {
    expect(insertShortcode("", 0, 0, "blobcat")).toEqual({ value: ":blobcat: ", caret: 10 });
    expect(insertShortcode("Hello", 5, 5, "wave")).toEqual({ value: "Hello :wave: ", caret: 13 });
    expect(insertShortcode("Hello world", 6, 6, "wave")).toEqual({ value: "Hello :wave: world", caret: 13 });
    expect(insertShortcode("Hello world", 5, 5, "wave")).toEqual({ value: "Hello :wave: world", caret: 12 });
    // A selection is replaced.
    expect(insertShortcode("I am happy", 5, 10, "smile")).toEqual({ value: "I am :smile: ", caret: 13 });
  });

  test("puts a Unicode emoji at the caret as it is, without spaces", () => {
    expect(insertText("Hello", 5, 5, "👋🏽")).toEqual({ value: "Hello👋🏽", caret: 9 });
    expect(insertText("I am happy", 5, 10, "😀")).toEqual({ value: "I am 😀", caret: 7 });
  });
});

describe("the emoji used often", () => {
  const day = 24 * 60 * 60 * 1000;

  test("rank the emoji used more often first, and the latest first among equals", () => {
    let uses = withUse([], "👍", 0);
    uses = withUse(uses, ":wave:", 1);
    uses = withUse(uses, ":wave:", 2);
    uses = withUse(uses, "🎉", 3);
    expect(uses.map((use) => use.emoji)).toEqual([":wave:", "🎉", "👍"]);
  });

  test("let a burst of use long ago give way to the emoji used lately", () => {
    let uses = withUse([], "👍", 0);
    uses = withUse(uses, "👍", 1);
    uses = withUse(uses, "👍", 2);
    // Three uses four weeks ago count less than one today: each week halves them.
    uses = withUse(uses, "🎉", 28 * day);
    expect(uses.map((use) => use.emoji)).toEqual(["🎉", "👍"]);
  });

  test("keep 32 emoji at most", () => {
    let uses = withUse([], "x0", 0);
    for (let n = 1; n < 40; n++) uses = withUse(uses, `x${String(n)}`, n);
    expect(uses).toHaveLength(32);
    expect(uses[0]?.emoji).toBe("x39");
  });
});

/** Opens the post form and returns it with its text. */
async function openForm(
  user: Awaited<ReturnType<typeof renderApp>>["user"],
  open = "Write a post",
  title = "New post",
  label = "Your post",
) {
  await user.click((await screen.findAllByRole("button", { name: open }))[0] as HTMLElement);
  const sheet = await screen.findByRole("dialog", { name: title });
  return { sheet, text: await within(sheet).findByRole("textbox", { name: label }) };
}

describe("the picker of the post form", () => {
  test("offers the Unicode emoji on a server without custom emoji, found by a Japanese keyword", async () => {
    server.use(
      signedInHandler,
      http.get(`${apiBase}/preferences`, () => HttpResponse.json({ ...preferencesFixture, locale: "ja" })),
    );
    const { user } = await renderApp({ locale: "ja" });
    const { sheet, text } = await openForm(user, "投稿を書く", "新しい投稿", "投稿の本文");
    await user.type(text, "やあ");

    // The one emoji button is the one in the corner of the text; the tools have none.
    const buttons = within(sheet).getAllByRole("button", { name: "絵文字を入れる" });
    expect(buttons).toHaveLength(1);
    await user.click(buttons[0] as HTMLElement);
    const picker = await screen.findByRole("dialog", { name: "絵文字" });
    const search = await within(picker).findByRole("searchbox", { name: "絵文字を検索" }, { timeout: 5000 });
    await waitFor(() => {
      expect(search).toHaveFocus();
    });
    // No custom emoji: the frequent ones (the default reactions), then the nine groups, without a custom section.
    const toolbar = within(picker).getByRole("toolbar", { name: "絵文字の分類" });
    expect(
      within(toolbar)
        .getAllByRole("button")
        .map((button) => button.getAttribute("aria-label")),
    ).toEqual([
      "よく使う絵文字",
      "顔と感情",
      "人と体",
      "動物と自然",
      "食べ物と飲み物",
      "旅行と場所",
      "活動",
      "物",
      "記号",
      "旗",
    ]);

    // A keyword in katakana, compared as hiragana.
    await user.type(search, "ナイス");
    await user.click(within(picker).getByRole("gridcell", { name: "にっこり笑う" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "絵文字" })).not.toBeInTheDocument();
    });
    expect(text).toHaveValue("やあ😀");
    await waitFor(() => {
      expect(text).toHaveFocus();
    });
  }, 30_000);

  test("keeps the skin tone chosen and offers the emoji used first, in that tone", async () => {
    server.use(signedInHandler);
    const { user } = await renderApp();
    const { sheet, text } = await openForm(user);

    await user.click(within(sheet).getByRole("button", { name: "Add emoji" }));
    const picker = await screen.findByRole("dialog", { name: "Emoji" });
    // Nothing used yet: the default reactions come first.
    await within(picker).findByRole("searchbox", { name: "Search emoji" }, { timeout: 5000 });
    expect(within(picker).getAllByRole("gridcell")[0]).toHaveAccessibleName("thumbs up");

    await user.click(within(picker).getByRole("button", { name: "Skin tone" }));
    await user.click(await screen.findByRole("menuitemradio", { name: "Medium skin tone" }));
    expect(skinToneKey.read()).toBe("medium");
    await user.type(within(picker).getByRole("searchbox", { name: "Search emoji" }), "waving hand");
    await user.click(within(picker).getByRole("gridcell", { name: "waving hand: medium skin tone" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Emoji" })).not.toBeInTheDocument();
    });
    expect(text).toHaveValue("👋🏽");
    // It is counted without its skin tone.
    expect(frequentEmojisKey.read().map((use) => use.emoji)).toEqual(["👋"]);

    await user.click(within(sheet).getByRole("button", { name: "Add emoji" }));
    const again = await screen.findByRole("dialog", { name: "Emoji" });
    const headings = within(again).getAllByRole("rowheader");
    const cells = within(again).getAllByRole("gridcell");
    expect(headings[0]).toHaveTextContent("Frequently used");
    expect(cells[0]).toHaveAccessibleName("waving hand: medium skin tone");
  }, 30_000);

  test("stays open while emoji are picked with Shift held, each after the one before", async () => {
    server.use(signedInHandler);
    const { user } = await renderApp();
    const { sheet, text } = await openForm(user);
    await user.type(text, "Hi");

    await user.click(within(sheet).getByRole("button", { name: "Add emoji" }));
    const picker = await screen.findByRole("dialog", { name: "Emoji" });
    await within(picker).findByRole("searchbox", { name: "Search emoji" }, { timeout: 5000 });
    await user.keyboard("{Shift>}");
    await user.click(within(picker).getAllByRole("gridcell", { name: "thumbs up" })[0] as HTMLElement);
    await user.keyboard("{/Shift}");
    // The grid renders only the rows in view, so find the next one by its name.
    await user.type(within(picker).getByRole("searchbox", { name: "Search emoji" }), "party popper");
    await user.keyboard("{Shift>}");
    await user.click(within(picker).getByRole("gridcell", { name: "party popper" }));
    await user.keyboard("{/Shift}");

    expect(screen.getByRole("dialog", { name: "Emoji" })).toBeInTheDocument();
    await waitFor(() => {
      expect(text).toHaveValue("Hi👍🎉");
    });
  }, 30_000);

  test("puts a picked custom emoji into the text at the caret, and offers it first next time", async () => {
    server.use(
      signedInHandler,
      customEmojisHandler([
        pickerEmojiFixture("wave", { aliases: ["hello"] }),
        pickerEmojiFixture("blobcat", { category: "Blobs" }),
      ]),
    );
    const { user } = await renderApp();
    const { sheet, text } = await openForm(user);
    await user.type(text, "Hi");

    await user.click(within(sheet).getByRole("button", { name: "Add emoji" }));
    const picker = await screen.findByRole("dialog", { name: "Emoji" });
    const search = await within(picker).findByRole("searchbox", { name: "Search emoji" }, { timeout: 5000 });
    await waitFor(() => {
      expect(search).toHaveFocus();
    });
    await user.type(search, "hello");
    await user.click(within(picker).getByRole("gridcell", { name: ":wave:" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Emoji" })).not.toBeInTheDocument();
    });
    expect(text).toHaveValue("Hi :wave: ");
    await waitFor(() => {
      expect(text).toHaveFocus();
    });

    await user.click(within(sheet).getByRole("button", { name: "Add emoji" }));
    const again = await screen.findByRole("dialog", { name: "Emoji" });
    const headings = within(again).getAllByRole("rowheader");
    const cells = within(again).getAllByRole("gridcell");
    expect(headings[0]).toHaveTextContent("Frequently used");
    expect(cells[0]).toHaveAccessibleName(":wave:");
    // The custom emoji come before the Unicode groups, under their categories.
    const toolbar = within(again).getByRole("toolbar", { name: "Emoji categories" });
    expect(
      within(toolbar)
        .getAllByRole("button")
        .slice(0, 4)
        .map((button) => button.getAttribute("aria-label")),
    ).toEqual(["Frequently used", "Other", "Blobs", "Smileys & emotion"]);
  }, 30_000);
});

describe("the picker of the profile", () => {
  test("puts a picked emoji into the display name and the bio", async () => {
    server.use(signedInHandler, customEmojisHandler([pickerEmojiFixture("blobcat")]));
    const { user } = await renderApp({ path: "/settings/profile" });
    const name = await screen.findByRole("textbox", { name: "Display name" });
    await user.clear(name);
    await user.type(name, "Alice");

    await user.click(await screen.findByRole("button", { name: "Add emoji to the display name" }));
    const picker = await screen.findByRole("dialog", { name: "Emoji" });
    await within(picker).findByRole("searchbox", { name: "Search emoji" }, { timeout: 5000 });
    await user.click(within(picker).getByRole("gridcell", { name: ":blobcat:" }));
    await waitFor(() => {
      expect(name).toHaveValue("Alice :blobcat: ");
    });

    const bio = screen.getByRole("textbox", { name: "Bio" });
    await user.clear(bio);
    await user.click(screen.getByRole("button", { name: "Add emoji to the bio" }));
    const again = await screen.findByRole("dialog", { name: "Emoji" });
    await user.type(await within(again).findByRole("searchbox", { name: "Search emoji" }), "grinning face");
    await user.click(within(again).getByRole("gridcell", { name: "grinning face" }));
    await waitFor(() => {
      expect(bio).toHaveValue("😀");
    });
  }, 30_000);
});
