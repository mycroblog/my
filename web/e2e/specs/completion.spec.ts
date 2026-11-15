// SPDX-License-Identifier: AGPL-3.0-only

// A writer completes an account after `@`, a hashtag after `#` and an emoji after `:` in the composer, by keyboard and
// by pointer, also while an input method composes (FR-119, ADR-0092): the text keeps the focus, the list is announced,
// and what is picked is written with a space after it. Mentions, hashtags and custom emoji are marked as typed, and a
// hashtag is never broken inside at the end of a line.

import { randomBytes } from "node:crypto";
import { Frame } from "./screens";
import { expect, person, test } from "./test";

/** A word of lowercase letters unique to the run, which no hashtag of the scenario or of another run starts with. */
const uniqueWord = () => Array.from(randomBytes(8), (b) => String.fromCharCode(97 + (b % 26))).join("");

test("a writer completes an account by keyboard: the followed first, the list announced, the caret after it", async ({
  page,
  signIn,
  fresh,
}) => {
  await signIn(await fresh.person({ follows: [person("mio")] }));
  const composer = await new Frame(page).newPost();
  await composer.write("Hi @m");

  const accounts = composer.suggestions("Accounts");
  // Marco and Mio by the start of their names, Sam by the m in sam, a username of this server (GET /accounts/search).
  await expect(accounts.getByRole("option", { name: "Mio 澪, @mio", exact: true })).toBeVisible();
  await expect(accounts.getByRole("option", { name: "Marco, @marco", exact: true })).toBeVisible();
  await expect(accounts.getByRole("option")).toHaveCount(3);
  await expect(composer.announced("3 suggestions")).toHaveCount(1);
  await expect(composer.text).toHaveAttribute("aria-autocomplete", "list");
  // The account the writer follows comes first, and is active as the list opens.
  await expect.poll(() => composer.activeSuggestion()).toBe("Mio 澪, @mio");
  await expect(composer.text).toBeFocused();

  await composer.text.press("ArrowDown");
  await expect.poll(() => composer.activeSuggestion()).toBe("Marco, @marco");
  await expect(accounts.getByRole("option", { name: "Marco, @marco", exact: true })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await composer.text.press("Enter");
  await expect(accounts).toBeHidden();
  await composer.holds("Hi @marco ");
  await expect(composer.text).toBeFocused();

  // The caret follows what was written: what is typed next goes after its space.
  await page.keyboard.type("and hello");
  await composer.holds("Hi @marco and hello");
});

test("Escape closes the suggestions and leaves the composer open; a press on an account writes it", async ({
  page,
  signIn,
  fresh,
}) => {
  await signIn(await fresh.person());
  const composer = await new Frame(page).newPost();
  await composer.write("@m");
  const accounts = composer.suggestions("Accounts");
  await expect(accounts).toBeVisible();

  await composer.text.press("Escape");
  await expect(accounts).toBeHidden();
  await expect(composer.dialog).toBeVisible();
  await expect(composer.text).toBeFocused();
  await composer.holds("@m");

  // A new word opens them again; a press picks one and leaves the focus in the text.
  await page.keyboard.type(" and @ma");
  await accounts.getByRole("option", { name: "Marco, @marco", exact: true }).click();
  await expect(accounts).toBeHidden();
  await expect(composer.text).toBeFocused();
  await composer.holds("@m and @marco ");
});

test("a writer completes a hashtag with Tab, which one undo takes back", async ({ page, signIn, fresh }) => {
  await signIn(await fresh.person());
  const composer = await new Frame(page).newPost();
  await composer.write("Bread #ba");
  const hashtags = composer.suggestions("Hashtags");
  const baking = hashtags.getByRole("option", { name: /^#baking, \d+ posts?$/ });
  await expect(baking).toHaveAttribute("aria-selected", "true");

  await composer.text.press("Tab");
  await expect(hashtags).toBeHidden();
  await expect(composer.text).toBeFocused();
  await composer.holds("Bread #baking ");

  await composer.text.press("ControlOrMeta+z");
  await composer.holds("Bread #ba");
});

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

test("a custom emoji is completed by an alias, by pointer and by keyboard, and shows as its image", async ({
  page,
  signIn,
  fresh,
}) => {
  await signIn(await fresh.person());
  const composer = await new Frame(page).newPost();
  await composer.write("Tea :och");
  const emoji = composer.suggestions("Emoji");

  // ocha is an alias of :teacup:.
  await emoji.getByRole("option", { name: ":teacup:", exact: true }).click();
  await expect(emoji).toBeHidden();
  await expect(composer.text).toBeFocused();
  await expect(composer.text.getByRole("img", { name: ":teacup:", exact: true })).toBeVisible();
  await composer.holds("Tea :teacup: ");

  await page.keyboard.type("and :spark");
  await expect(emoji.getByRole("option", { name: ":sparkle:", exact: true })).toHaveAttribute("aria-selected", "true");
  await composer.text.press("Enter");
  await expect(emoji).toBeHidden();
  await expect(composer.text.getByRole("img", { name: ":sparkle:", exact: true })).toBeVisible();
  await composer.holds("Tea :teacup: and :sparkle: ");
});

test("the hashtags posted lately on the device come first among those the server found", async ({
  page,
  signIn,
  fresh,
}) => {
  const writer = await fresh.person();
  const word = uniqueWord();
  // The server puts the hashtag carried by more posts first: …b, with two, before …a.
  await fresh.post(writer, { text: `First loaf #${word}b` });
  await fresh.post(writer, { text: `Second loaf #${word}b` });
  await signIn(writer);
  const posting = await new Frame(page).newPost();
  await posting.write(`Third loaf #${word}a `);
  await posting.post();

  const composer = await new Frame(page).newPost();
  await composer.write(`More #${word}`);
  const hashtags = composer.suggestions("Hashtags");
  const posted = hashtags.getByRole("option", { name: `#${word}a, 1 post`, exact: true });
  await expect(hashtags.getByRole("option", { name: `#${word}b, 2 posts`, exact: true })).toBeVisible();
  await expect(posted).toHaveAttribute("aria-selected", "true");
  await posted.click();
  await composer.holds(`More #${word}a `);
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
