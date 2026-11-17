// SPDX-License-Identifier: AGPL-3.0-only

// The composer's text and content warning are a Lexical editor (#3097, ADR-0092 §4), and this is what of it needs the
// server: what is sent is the same plain text the server keeps, counted as the server counts it. What the editor does
// under an input method and the caret's keys, how its lines break, what it marks over the limit, and what it says when
// its code could not be fetched are tests in a browser on its story (catalog/specs/browser); that a post over the limit
// is held back is the features' (web/app-features/src/compose/limit.dom.test.tsx).

import { Frame } from "./screens";
import { expect, test } from "./test";

test("what is sent from the editor at the server's limit is the text and content warning the server keeps, exactly", async ({
  page,
  signIn,
  fresh,
}) => {
  const writer = await fresh.person();
  await signIn(writer);
  // The server counts the code points of the text and the content warning together, 500 at most: a custom emoji's
  // shortcode counts its characters, an astral emoji one.
  const spoiler = "Mountain :wave: 写真 😀";
  const typed = "朝の散歩 #山 :teacup: 😀\nhttps://example.com/path?q=1 は\n\n終わり ";
  const rest = "a".repeat(500 - Array.from(spoiler).length - Array.from(typed).length);
  const text = typed + rest;

  const composer = await new Frame(page).newPost();
  // Typed a key at a time, each line break an Enter; the rest of the limit comes at the caret at once.
  await composer.write(typed);
  await page.keyboard.insertText(rest);
  await composer.holds(text);
  await composer.addWarning();
  await page.keyboard.type(spoiler);
  await composer.warningHolds(spoiler);
  await expect(composer.count).toHaveText("0 characters left");

  const id = await composer.postedId();
  const kept = await fresh.source(writer, id);
  expect(kept).toEqual({ text, spoiler });
});
