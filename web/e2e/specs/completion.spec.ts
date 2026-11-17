// SPDX-License-Identifier: AGPL-3.0-only

// What the composer's completion needs a browser for (FR-119, ADR-0092): the suggestions open while an input method
// composes a word after `#`, a pick writes over the composition, and a hashtag is never broken inside at the end of a
// line.

import { Frame } from "./screens";
import { expect, test } from "./test";

test("the suggestions open while an input method composes, and a press picks one over the composition", async ({
  page,
  signIn,
  fresh,
}) => {
  await signIn(await fresh.person());
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

test("the word an input method commits keeps its suggestions, and Enter then picks one", async ({
  page,
  signIn,
  fresh,
}) => {
  await signIn(await fresh.person());
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

test("a hashtag at the end of a narrow line goes to the next line whole, never broken inside", async ({
  page,
  signIn,
  fresh,
}) => {
  await signIn(await fresh.person());
  const composer = await new Frame(page).newPost();
  await page.setViewportSize({ width: 320, height: 640 });
  const tag = "#川沿いの公園の写真";
  const mark = composer.text.getByText(tag, { exact: true });
  /** The lines the hashtag's boxes stand on, and whether it starts below the text's first line. */
  const lines = () =>
    mark.evaluate((element) => {
      const tops = new Set(Array.from(element.getClientRects(), (r) => Math.round(r.top)));
      const first = element.closest("p")?.getClientRects()[0]?.top ?? 0;
      return { lines: tops.size, wrapped: element.getBoundingClientRect().top > first + 1 };
    });
  // Japanese breaks between any two characters, so some of these lengths put the hashtag across the line's end; the
  // comma lets the hashtag start, as a letter before `#` would not.
  let wrapped = false;
  for (let length = 6; length <= 24; length += 1) {
    const text = `${"あ".repeat(length)}、${tag}`;
    await composer.replace(text);
    await composer.holds(text);
    await expect
      .poll(async () => {
        const seen = await lines();
        wrapped ||= seen.wrapped;
        return seen.lines;
      })
      .toBe(1);
  }
  expect(wrapped).toBe(true);
});
