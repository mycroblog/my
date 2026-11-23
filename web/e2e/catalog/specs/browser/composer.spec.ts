// SPDX-License-Identifier: AGPL-3.0-only

// What the composer writes at the caret and marks in its text, on the app's screen story: the emoji its picker gives
// goes where the caret is and the text has the focus again (FR-116), and the part of the text over the limit is marked
// with a CSS highlight (#3097); a reply's caret after its mention, Ctrl+Enter that sends, and an image pasted in that is
// attached. No DOM simulation moves a caret, keeps a highlight, or takes a keyboard's or a clipboard's default action.

import { Frame, homeTimeline } from "../../../specs/screens";
import { expect, pc, test } from "../test";

test.beforeEach(async ({ open }) => {
  await open("screens-timeline--normal", { width: pc, height: 800 });
});

test("the composer's Add emoji button writes the emoji at the caret, and gives the text the focus back", async ({
  page,
}) => {
  const composer = await new Frame(page).newPost();
  await composer.write("Good morning");

  // A custom emoji gets a space on each side that has none.
  let picker = await composer.emoji();
  await expect(picker.search).toBeFocused();
  await picker.emoji("Custom emoji", ":teacup:").click();
  await expect(picker.dialog).toBeHidden();
  await expect(composer.text).toBeFocused();
  await composer.holds("Good morning :teacup: ");

  // A Unicode emoji goes at the caret as it is, and the caret follows it.
  await composer.text.press("ControlOrMeta+Home");
  for (const _ of "Good") await composer.text.press("ArrowRight");
  picker = await composer.emoji();
  await picker.emoji("Smileys & emotion", "grinning face").click();
  await expect(composer.text).toBeFocused();
  await page.keyboard.type("!");
  await composer.holds("Good😀! morning :teacup: ");

  // With Shift, the picker stays open for the next emoji, which follows the last at the caret.
  picker = await composer.emoji();
  await picker.emoji("Custom emoji", ":wave:").click({ modifiers: ["Shift"] });
  await composer.holds("Good😀! :wave: morning :teacup: ");
  await expect(picker.dialog).toBeVisible();
  await picker.emoji("Smileys & emotion", "grinning face").click();
  await expect(picker.dialog).toBeHidden();
  await composer.holds("Good😀! :wave:😀 morning :teacup: ");
  await expect(composer.text).toBeFocused();

  // An emoji takes the place of the selection.
  await composer.text.press("ControlOrMeta+a");
  picker = await composer.emoji();
  await picker.emoji("Smileys & emotion", "grinning face").click();
  await composer.holds("😀");
});

test("the count says how far the text and its content warning are over the limit, and the excess is marked", async ({
  page,
}) => {
  // The text and the content warning count together, by code point, 500 at most: a custom emoji's shortcode counts its
  // characters, an astral emoji one.
  const spoiler = "注意 😀";
  const head = "Limit :teacup: 😀 日本語 ";
  const atLimit = head + "a".repeat(500 - Array.from(spoiler).length - Array.from(head).length);

  const composer = await new Frame(page).newPost();
  await composer.addWarning();
  await page.keyboard.type(spoiler);
  await composer.replace(atLimit);
  await composer.holds(atLimit);
  await expect(composer.count).toHaveText("0 characters left");

  await composer.replace(`${atLimit}xyz`);
  await expect(composer.count).toHaveText("3 characters over");
  await expect.poll(() => composer.highlighted()).toContain("xyz");
});

test("a reply opens with its caret after the mention, so that what is typed follows it", async ({ page }) => {
  const composer = await homeTimeline(page).post("Ren 蓮", "来週の集まり").reply();
  await expect(composer.text).toBeFocused();
  await page.keyboard.type("thanks");
  await composer.holds("@ren thanks");
});

test("Ctrl+Enter or ⌘+Enter sends the post, and Enter alone writes a line break", async ({ page }) => {
  const composer = await new Frame(page).newPost();
  await composer.write("first");
  await composer.text.press("Enter");
  await page.keyboard.type("second");
  await composer.holds("first\nsecond");
  await composer.text.press("ControlOrMeta+Enter");
  await expect(composer.dialog).toBeHidden();
});

test("an image pasted into the text is attached, and writes nothing", async ({ page }) => {
  const composer = await new Frame(page).newPost();
  await composer.write("look");
  await composer.text.evaluate((box) => {
    const data = new DataTransfer();
    data.items.add(new File([new Uint8Array([137, 80, 78, 71])], "shot.png", { type: "image/png" }));
    box.dispatchEvent(new ClipboardEvent("paste", { clipboardData: data, bubbles: true, cancelable: true }));
  });
  await expect(composer.dialog.getByRole("textbox", { name: "Description", exact: true })).toHaveCount(1);
  await composer.holds("look");
});
