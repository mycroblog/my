// SPDX-License-Identifier: AGPL-3.0-only

// A writer completes an account after `@`, a hashtag after `#` and an emoji after `:` in the composer, by keyboard and
// by pointer (FR-119, ADR-0092): the text keeps the focus, the list is announced, and what is picked is written with a
// space after it. The list's place under the caret's line is measured, which no simulation does: it stays unseen
// there, so these tests find it from the text that controls it and not as a reader sees it, and write by pasting
// (test/composer.ts). What an input method does to the list stays with the tests in a browser.

import { afterEach, expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { renderApp, type TestApp } from "./test/app";
import { heldBy, newPost, paste, type Composer } from "./test/composer";

/** Where the composer is opened: it opens the same over any screen, and a screen that shows little keeps a test quick. */
const light = "/lists";

afterEach(() => {
  // The hashtags posted lately are kept on the device.
  localStorage.clear();
});

/** The name of a suggestion: its label, else its words. */
const named = (option: Element) => option.getAttribute("aria-label") ?? option.textContent;

/** The list of suggestions the text controls, named `kind` ("Accounts", "Hashtags", "Emoji"); none while it is closed. */
function listOf(text: HTMLElement, kind: string): HTMLElement | null {
  const list = document.getElementById(text.getAttribute("aria-controls") ?? "");
  return list?.role === "listbox" && list.getAttribute("aria-label") === kind ? list : null;
}

/** The suggestions of `kind` the text controls, in their order; none while the list is closed. */
const suggested = (text: HTMLElement, kind: string): string[] =>
  Array.from(listOf(text, kind)?.querySelectorAll("[role=option]") ?? [], named);

/** The text's active suggestion, the option its `aria-activedescendant` names; undefined while it names none. */
function active(text: HTMLElement): string | undefined {
  const option = document.getElementById(text.getAttribute("aria-activedescendant") ?? "");
  return option === null ? undefined : named(option);
}

/**
 * Waits until the suggestions of `kind` start with `options`, in this order, the first of them active, and returns
 * them all.
 */
async function suggests(text: HTMLElement, kind: string, options: readonly (string | RegExp)[]): Promise<string[]> {
  return waitFor(() => {
    const shown = suggested(text, kind);
    const same = options.every((option, i) =>
      typeof option === "string" ? shown[i] === option : option.test(shown[i] ?? ""),
    );
    if (!same || active(text) !== shown[0])
      throw new Error(
        `test: the ${kind} suggested are ${JSON.stringify(shown)}, the active one ${String(active(text))}`,
      );
    return shown;
  });
}

/** Presses the suggestion of `kind` named `name`. */
async function press(app: TestApp, text: HTMLElement, kind: string, name: string): Promise<void> {
  const option = Array.from(listOf(text, kind)?.querySelectorAll<HTMLElement>("[role=option]") ?? []).find(
    (option) => named(option) === name,
  );
  if (option === undefined) throw new Error(`test: ${name} is not among the ${kind} suggested`);
  await app.user.click(option);
}

/** The words the composer announced, each a status of its dialog. */
const announced = ({ dialog }: Composer) =>
  within(dialog)
    .queryAllByRole("status")
    .map((status) => status.textContent);

test("a writer completes an account by keyboard: the followed first, the list announced, the caret after it", async () => {
  const app = await renderApp(light, { viewer: "yuki" });
  const composer = await newPost(app);
  const { text } = composer;
  await paste(app, text, "Hi @m");

  // Marco and Mio by the start of their names, Sam by the m in sam; Sam, whom the writer follows, comes first.
  expect(await suggests(text, "Accounts", ["Sam Rivera, @sam", "Marco, @marco", "Mio 澪, @mio"])).toHaveLength(3);
  expect(announced(composer)).toContain("3 suggestions");
  expect(text.getAttribute("aria-autocomplete")).toBe("list");
  expect(app.focused()).toBe(text);

  await app.user.keyboard("{ArrowDown}");
  expect(active(text)).toBe("Marco, @marco");
  expect(listOf(text, "Accounts")?.querySelector("[role=option][aria-selected=true]")?.getAttribute("id")).toBe(
    text.getAttribute("aria-activedescendant"),
  );
  await app.user.keyboard("{Enter}");
  expect(screen.queryByRole("listbox", { hidden: true })).toBeNull();
  expect(heldBy(text)).toBe("Hi @marco ");
  expect(app.focused()).toBe(text);

  // The caret follows what was written: what comes next goes after its space.
  await app.user.paste("and hello");
  expect(heldBy(text)).toBe("Hi @marco and hello");
});

test("Escape closes the suggestions and leaves the composer open; a press on an account writes it", async () => {
  const app = await renderApp(light, { viewer: "yuki" });
  const { dialog, text } = await newPost(app);
  await paste(app, text, "@m");
  await suggests(text, "Accounts", ["Sam Rivera, @sam", "Marco, @marco", "Mio 澪, @mio"]);

  await app.user.keyboard("{Escape}");
  expect(screen.queryByRole("listbox", { hidden: true })).toBeNull();
  expect(screen.getByRole("dialog", { name: "New post" })).toBe(dialog);
  expect(app.focused()).toBe(text);
  expect(heldBy(text)).toBe("@m");

  // A new word opens them again; a press picks one and leaves the focus in the text.
  await app.user.paste(" and @ma");
  await suggests(text, "Accounts", ["Marco, @marco"]);
  await press(app, text, "Accounts", "Marco, @marco");
  expect(screen.queryByRole("listbox", { hidden: true })).toBeNull();
  expect(app.focused()).toBe(text);
  expect(heldBy(text)).toBe("@m and @marco ");
});

test("a writer completes a hashtag with Tab, which one undo takes back", async () => {
  const app = await renderApp(light, { viewer: "yuki" });
  const { text } = await newPost(app);
  await paste(app, text, "Bread #ba");
  await suggests(text, "Hashtags", [/^#baking, \d+ posts?$/]);

  await app.user.keyboard("{Tab}");
  expect(screen.queryByRole("listbox", { hidden: true })).toBeNull();
  expect(app.focused()).toBe(text);
  expect(heldBy(text)).toBe("Bread #baking ");

  await app.user.keyboard("{Control>}z{/Control}");
  expect(heldBy(text)).toBe("Bread #ba");
});

test("a custom emoji is completed by an alias, by pointer and by keyboard, and shows as its image", async () => {
  const app = await renderApp(light, { viewer: "yuki" });
  const { text } = await newPost(app);
  await paste(app, text, "Tea :och");

  // ocha is an alias of :teacup:.
  await suggests(text, "Emoji", [":teacup:"]);
  await press(app, text, "Emoji", ":teacup:");
  expect(screen.queryByRole("listbox", { hidden: true })).toBeNull();
  expect(app.focused()).toBe(text);
  expect(within(text).getByRole("img", { name: ":teacup:" })).toBeDefined();
  expect(heldBy(text)).toBe("Tea :teacup: ");

  await app.user.paste("and :spark");
  await suggests(text, "Emoji", [":sparkle:"]);
  await app.user.keyboard("{Enter}");
  expect(screen.queryByRole("listbox", { hidden: true })).toBeNull();
  expect(within(text).getByRole("img", { name: ":sparkle:" })).toBeDefined();
  expect(heldBy(text)).toBe("Tea :teacup: and :sparkle: ");
});

test("the hashtags posted lately on the device come first among those the server found", async () => {
  const app = await renderApp(light, { viewer: "yuki" });
  const posting = await newPost(app);
  await paste(app, posting.text, "A third loaf from the #bakery ");
  const sent = app.fake.script("POST /statuses");
  await app.user.click(within(posting.dialog).getByRole("button", { name: "Post" }));
  await sent.arrived();
  await waitFor(() => {
    if (screen.queryByRole("dialog", { name: "New post" }) !== null)
      throw new Error("test: the composer is still open");
  });

  // The server puts the hashtag carried by more posts first: #baking, before #bakery.
  app.fake.script("GET /tags/search", {
    change: (found) => ({ items: [...found.items, { name: "bakery", statuses_count: 1 }] }),
  });
  const { text } = await newPost(app);
  await paste(app, text, "More #bak");
  // The one posted from here comes before it.
  expect(await suggests(text, "Hashtags", ["#bakery, 1 post", /^#baking, \d+ posts$/])).toHaveLength(2);
  await press(app, text, "Hashtags", "#bakery, 1 post");
  expect(heldBy(text)).toBe("More #bakery ");
});
