// SPDX-License-Identifier: AGPL-3.0-only

// The composer's text and content warning are an editor (ADR-0092 §4): what it takes from a paste, gives to a copy,
// and keeps on one line in the content warning. These are the editor's own answers to the events a browser sends it,
// which a simulation sends the same; what a keyboard and an input method write at the caret needs a browser's own
// editing, and stays with the tests there.

import { expect, test } from "bun:test";
import { screen, within } from "@testing-library/react";
import { renderApp } from "./test/app";
import { heldBy, newPost, paste } from "./test/composer";

/** Where the composer is opened: it opens the same over any screen, and a screen that shows little keeps a test quick. */
const light = "/lists";

/** What a clipboard holds: each kind of data under its type. */
function clipboard(kinds: Record<string, string>): DataTransfer {
  const data = new DataTransfer();
  for (const [type, value] of Object.entries(kinds)) data.setData(type, value);
  return data;
}

test("a paste writes its plain text with its line breaks and ignores its HTML, and undo takes it back", async () => {
  const app = await renderApp(light, { viewer: "yuki" });
  const { text } = await newPost(app);
  expect(text.getAttribute("aria-multiline")).toBe("true");
  await paste(app, text, "Start ");

  await app.user.paste(
    clipboard({
      "text/plain": "line one\nline two",
      "text/html":
        '<p><b>Bold</b> <a href="https://example.com/">link</a></p><img src="https://example.com/x.png" alt="pic">',
    }),
  );
  expect(heldBy(text)).toBe("Start line one\nline two");
  expect(within(text).queryAllByRole("img")).toHaveLength(0);
  expect(within(text).queryAllByRole("link")).toHaveLength(0);

  await app.user.keyboard("{Control>}z{/Control}");
  expect(heldBy(text)).toBe("Start ");
  await app.user.keyboard("{Control>}{Shift>}z{/Shift}{/Control}");
  expect(heldBy(text)).toBe("Start line one\nline two");
});

test("a copy gives the plain text: a custom emoji as its shortcode, and a line break as a newline", async () => {
  const app = await renderApp(light, { viewer: "yuki" });
  const { text } = await newPost(app);
  await paste(app, text, "Tea :teacup: time\nnext line");
  // The emoji shows as its image, which has no text of its own to copy.
  expect(within(text).getByRole("img", { name: ":teacup:" })).toBeDefined();

  await app.user.keyboard("{Control>}a{/Control}");
  const copied = await app.user.copy();

  expect(copied?.getData("text/plain")).toBe("Tea :teacup: time\nnext line");
});

test("the content warning is one line: Enter writes nothing, a pasted newline is taken out, an emoji shows as its image", async () => {
  const app = await renderApp(light, { viewer: "yuki" });
  const { dialog } = await newPost(app);
  await app.user.click(within(dialog).getByRole("button", { name: "Add a content warning" }));
  const warning = await within(dialog).findByRole("textbox", { name: "Content warning" });
  expect(warning.getAttribute("aria-multiline")).not.toBe("true");

  await paste(app, warning, "Spoilers");
  await app.user.keyboard("{Enter}");
  expect(heldBy(warning)).toBe("Spoilers");
  expect(screen.getByRole("dialog", { name: "New post" })).toBe(dialog);

  await app.user.paste(" for\nthe finale");
  expect(heldBy(warning)).toBe("Spoilers forthe finale");

  await app.user.paste(" :wave:");
  expect(within(warning).getByRole("img", { name: ":wave:" })).toBeDefined();
  expect(heldBy(warning)).toBe("Spoilers forthe finale :wave:");
});
