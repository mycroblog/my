// SPDX-License-Identifier: AGPL-3.0-only

// The emoji of a post and of the composer are drawn with the self-hosted Noto Color Emoji, and a page fetches the
// WOFF2 of the splits of the emoji it shows and no other; a page with no emoji fetches none; the build serves the
// font's license (ADR-0091 §8–§13). Chromium draws COLRv1, the first source of each face.

import { instance } from "../src/scenario";
import { EmojiFonts, Frame, homeTimeline, Landing, selfHostedNoto, SignIn } from "./screens";
import { expect, test } from "./test";

test("the landing page and the sign-in screen, with no emoji, fetch none of the emoji fonts", async ({ page }) => {
  const fonts = new EmojiFonts(page);
  const landing = new Landing(page);
  await landing.open();
  await expect(landing.heading(instance["name"] ?? "")).toBeVisible();
  // emoji.css is on the page, a face per split, and none of them is loaded.
  await expect.poll(() => fonts.faces()).toEqual({ declared: 11, loaded: 0 });
  await landing.signIn.click();
  await expect(new SignIn(page).heading).toBeVisible();
  await expect.poll(() => fonts.faces()).toEqual({ declared: 11, loaded: 0 });
  await fonts.settled();
  expect(fonts.fetched).toEqual([]);
});

test("an emoji in a post is drawn with the self-hosted Noto Color Emoji, which fetches its split alone", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const post = await fresh.post(author, { text: "Bread from the morning bake 🍞" });
  const fonts = new EmojiFonts(page);
  await signIn(author);
  await expect(homeTimeline(page).post(author.displayName, post.text).article).toBeVisible();
  await expect.poll(() => fonts.drawing("🍞")).toContainEqual(selfHostedNoto);
  await fonts.settled();
  expect(fonts.fetched).toEqual(["noto-emoji-colrv1-food-drink.woff2"]);
});

test("an emoji written in the composer is drawn with the self-hosted Noto Color Emoji", async ({
  page,
  signIn,
  fresh,
}) => {
  const writer = await fresh.person();
  const fonts = new EmojiFonts(page);
  await signIn(writer);
  const composer = await new Frame(page).newPost();
  await composer.write("Lunch on the ridge 🍙");
  await expect(composer.text).toHaveValue("Lunch on the ridge 🍙");
  await expect.poll(() => fonts.drawing("🍙")).toContainEqual(selfHostedNoto);
  await fonts.settled();
  expect(fonts.fetched).toEqual(["noto-emoji-colrv1-food-drink.woff2"]);
});

test("the build serves the license of the emoji font", async ({ request }) => {
  const response = await request.get("/licenses/OFL.txt");
  await expect(response).toBeOK();
  const license = await response.text();
  expect(license).toContain("SIL OPEN FONT LICENSE Version 1.1");
});
