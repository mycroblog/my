// SPDX-License-Identifier: AGPL-3.0-only

// The composer's text and content warning under an input method (FR-119, ADR-0092), on the app's screen story: a
// composition is the browser's own (CDP Input.imeSetComposition), which no DOM simulation runs. Android's input
// (Gboard) cannot run here: it is checked on a device or an emulator.

import { Frame } from "../../../specs/screens";
import { expect, pc, test } from "../test";

test.beforeEach(async ({ open }) => {
  await open("screens-timeline--normal", { width: pc, height: 800 });
});

test("the suggestions open while an input method composes, and a press picks one over the composition", async ({
  page,
}) => {
  const composer = await new Frame(page).newPost();
  await composer.write("Lunch #");
  const hashtags = composer.suggestions("Hashtags");

  // The input method composes 写 after #, as an Android keyboard composes every word: the hashtags for it open.
  await composer.compose(["しゃ", "写"]);
  await composer.holds("Lunch #写");
  const photo = hashtags.getByRole("option", { name: /^#写真, \d+ posts?$/ });
  await expect(photo).toHaveAttribute("aria-selected", "true");

  // A press writes it in place of the composition, which the input method does not write again; what is typed next
  // goes after its space.
  await photo.click();
  await expect(hashtags).toBeHidden();
  await expect(composer.text).toBeFocused();
  await composer.holds("Lunch #写真 ");
  await page.keyboard.type("today");
  await composer.holds("Lunch #写真 today");
});

test("the word an input method commits keeps its suggestions, and Enter then picks one", async ({ page }) => {
  const composer = await new Frame(page).newPost();
  await composer.write("#");
  const hashtags = composer.suggestions("Hashtags");
  await composer.compose(["しゃ", "写"]);
  await composer.commit("写");
  await composer.holds("#写");
  const photo = hashtags.getByRole("option", { name: /^#写真, \d+ posts?$/ });
  await expect(photo).toHaveAttribute("aria-selected", "true");
  await composer.text.press("Enter");
  await expect(hashtags).toBeHidden();
  await composer.holds("#写真 ");
});

test("an input method composes in the content warning too", async ({ page }) => {
  const composer = await new Frame(page).newPost();
  await composer.addWarning();
  await page.keyboard.type("Spoilers ");
  await composer.compose(["ね", "ねた", "ネタ"]);
  await composer.commit("ネタ");
  await composer.warningHolds("Spoilers ネタ");
});

test("a Japanese input method composes in the text, also inside a hashtag, and the caret stays after what it wrote", async ({
  page,
}) => {
  const composer = await new Frame(page).newPost();
  await composer.write("今日は");
  await composer.compose(["や", "やま", "山"]);
  await composer.holds("今日は山");
  await composer.commit("山");
  await composer.holds("今日は山");
  await page.keyboard.type("!");
  await composer.holds("今日は山!");

  // Inside the hashtag #山, after its 山: the composition and its commit go at the caret, and so does what follows.
  await composer.replace("#山 walk");
  await composer.holds("#山 walk");
  await composer.text.press("Home");
  for (const _ of "#山") await composer.text.press("ArrowRight");
  await composer.compose(["の", "のう", "のうえ"]);
  await composer.holds("#山のうえ walk");
  await composer.compose(["の上"]);
  await composer.commit("の上");
  await composer.holds("#山の上 walk");
  await page.keyboard.type("の小屋");
  await composer.holds("#山の上の小屋 walk");
  await expect.poll(() => composer.linesOf("#山の上の小屋")).toEqual([1]);
});
