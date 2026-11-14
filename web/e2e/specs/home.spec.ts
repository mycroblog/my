// SPDX-License-Identifier: AGPL-3.0-only

// The home timeline shows the scenario's posts as the server returns them: text in its own direction, images with
// their alt text, boosts with who boosted, and a content warning before the text it hides.

import { homeTimeline } from "./screens";
import { expect, person, test } from "./test";

const yuki = person("yuki");
const sam = person("sam");
const layla = person("layla");
const hana = person("hana");

test.beforeEach(async ({ signIn }) => {
  await signIn(yuki);
});

test("a post in Arabic reads right to left, by the direction of its own text", async ({ page }) => {
  const text = homeTimeline(page).post(layla.displayName, "صباح الخير").paragraph("صباح الخير");
  await expect(text).toHaveAttribute("dir", "auto");
  await expect(text).toHaveCSS("direction", "rtl");
});

test("the images of posts show with their alt text", async ({ page }) => {
  const home = homeTimeline(page);
  await expect(
    home.post(yuki.displayName, "今朝の尾根").image("朝焼けの山並み。手前に三つの尾根が重なっている"),
  ).toBeVisible();
  await expect(
    home
      .post(sam.displayName, "Day 3 of the sourdough")
      .image("A round loaf of sourdough on a wooden board, seen from above"),
  ).toBeVisible();
});

test("a boost shows the boosted post with who boosted it", async ({ page }) => {
  await expect(
    homeTimeline(page).boostOf(hana.displayName, sam.displayName, "Day 3 of the sourdough").article,
  ).toBeVisible();
});

test("a content warning hides the text until the person shows it", async ({ page }) => {
  const post = homeTimeline(page).post(sam.displayName, "The ending of the film we watched");
  const hidden = post.paragraph("The phone call in the first scene");
  await expect(hidden).toBeHidden();
  await post.show();
  await expect(hidden).toBeVisible();
});
