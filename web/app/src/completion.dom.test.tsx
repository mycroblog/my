// SPDX-License-Identifier: AGPL-3.0-only

// A writer completes a hashtag after `#` in the composer (FR-119, ADR-0092), and the hashtags posted lately on the
// device, which a post sent from one composer leaves for the next, come first. How the lists of accounts, hashtags and
// emoji complete by keyboard and by pointer is the composer's (web/app-features/src/compose/completion.dom.test.tsx).
// The list's place under the caret's line is measured, which no simulation does: it stays unseen there, so the test
// finds it from the text that controls it and not as a reader sees it, and writes by pasting (test/composer.ts).

import { afterEach, expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { renderApp, type TestApp } from "./test/app";
import { heldBy, newPost, paste } from "./test/composer";

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
