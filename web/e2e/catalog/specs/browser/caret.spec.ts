// SPDX-License-Identifier: AGPL-3.0-only

// The composer's text under the keys that move the caret (#3097, ADR-0092 §4), on the app's screen story: typing at
// the caret, Home, End and the arrows, and Backspace over a custom emoji, which no DOM simulation moves a caret for.

import { Frame } from "../../../specs/screens";
import { expect, pc, test } from "../test";

test.beforeEach(async ({ open }) => {
  await open("screens-timeline--normal", { width: pc, height: 800 });
});

test("the text is a named text box of several lines; typed and edited in the middle, it holds what was written", async ({
  page,
}) => {
  const composer = await new Frame(page).newPost();
  await expect(composer.text).toBeFocused();
  await expect(composer.text).toHaveAttribute("aria-multiline", "true");

  await composer.write("Hello world");
  await composer.holds("Hello world");
  await composer.text.press("Home");
  for (const _ of "Hello") await composer.text.press("ArrowRight");
  await page.keyboard.type(",");
  await composer.holds("Hello, world");

  // Enter writes a line break.
  await composer.text.press("End");
  await page.keyboard.press("Enter");
  await page.keyboard.type("Second line");
  await composer.holds("Hello, world\nSecond line");
});

test("a custom emoji shows as its image, is one unit for the caret and Backspace, and is read as its shortcode", async ({
  page,
}) => {
  const composer = await new Frame(page).newPost();
  await composer.write("Tea :teacup: ");
  await expect(composer.text.getByRole("img", { name: ":teacup:", exact: true })).toBeVisible();
  await composer.holds("Tea :teacup: ");

  // Backspace takes the space, then the whole emoji.
  await page.keyboard.press("Backspace");
  await composer.holds("Tea :teacup:");
  await page.keyboard.press("Backspace");
  await composer.holds("Tea ");
  await expect(composer.text.getByRole("img", { name: ":teacup:", exact: true })).toHaveCount(0);

  // One ArrowLeft moves the caret over the whole emoji.
  await page.keyboard.type(":teacup:");
  await expect(composer.text.getByRole("img", { name: ":teacup:", exact: true })).toBeVisible();
  await page.keyboard.press("ArrowLeft");
  await page.keyboard.type("x");
  await composer.holds("Tea x:teacup:");
});
