// SPDX-License-Identifier: AGPL-3.0-only

// The text is drawn with the self-hosted Noto Sans for Latin, Greek and Cyrillic and Noto Sans CJK of its language's
// region, and a page fetches the WOFF2 of the slices of the characters it shows and of no other region; a word in
// letters that Noto Sans CJK lacks draws with one font; a theme's own font (--font-sans-override) draws the Latin it
// has, while the emoji and the Han characters it lacks stay with the self-hosted fonts; the build serves the fonts'
// licenses (ADR-0009 §5, ADR-0059 §2). Chromium reads which font drew a piece of text over the DevTools protocol.

import { instance } from "../src/scenario";
import {
  applyTheme,
  CjkFonts,
  homeTimeline,
  Landing,
  selfHostedCjk,
  selfHostedNoto,
  selfHostedNotoSans,
  type DrawnWith,
} from "./screens";
import { expect, test } from "./test";

/** The fonts, each once, in a stable order: a text node lists a font once per run of glyphs it drew. */
const distinct = (fonts: readonly DrawnWith[]) =>
  [...new Map(fonts.map((f) => [`${f.familyName}|${String(f.isCustomFont)}`, f])).values()].toSorted((a, b) =>
    a.familyName.localeCompare(b.familyName),
  );

test("the landing page in English fetches the slices of its own characters alone, of the page's region", async ({
  page,
}) => {
  const fonts = new CjkFonts(page);
  const landing = new Landing(page);
  const name = instance["name"] ?? "";
  await landing.open();
  await expect(landing.heading(name)).toBeVisible();
  // Its Latin draws with Noto Sans.
  await expect.poll(async () => distinct(await fonts.drawing(name))).toEqual([selfHostedNotoSans]);
  await fonts.settled();
  // The page's default region is JP; a page in English shows no Han, kana or Hangul of another region.
  expect(fonts.fetched.filter((s) => s.region !== "jp")).toEqual([]);
  await expect.poll(() => fonts.unclaimed()).toEqual([]);
});

test("a post in en, ja, zh-CN, zh-TW or ko draws its Han characters with Noto Sans CJK of its language's region", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  // The same Han characters, whose glyphs differ by region; a post in English takes the page's default region, JP.
  // The Latin, digits and spaces of a post (each ends in a tag of its own) draw with Noto Sans.
  const languages = [
    ["en", "Straight bone: 直 骨 角 写 誤", "jp"],
    ["ja", "直線の骨と角を写す誤り", "jp"],
    ["zh-CN", "直线的骨与角写错了", "sc"],
    ["zh-TW", "直線的骨與角寫錯了", "tc"],
    ["ko", "직선 直 骨 角 寫 誤 한국어", "kr"],
  ] as const;
  const posts = [];
  for (const [language, text, region] of languages)
    posts.push({ language, region, post: await fresh.post(author, { text, language }) });
  const fonts = new CjkFonts(page);
  await signIn(author);
  for (const { language, region, post } of posts) {
    await expect(homeTimeline(page).post(author.displayName, post.text).article).toBeVisible();
    // Every other character of the post, its kana, Hangul and Han, is drawn with that region's Noto alone.
    await expect
      .poll(async () => ({ language, fonts: distinct(await fonts.drawing(post.text)) }))
      .toEqual({ language, fonts: distinct([selfHostedCjk(region), selfHostedNotoSans]) });
  }
});

test("a word in letters that Noto Sans CJK lacks draws with one font, also in a post in Japanese", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  // Polish and Ukrainian, whose ż ó ł ć ę ś ą ń ї Noto Sans CJK lacks: each word drew with two fonts before.
  const posts = [];
  for (const [language, text] of [
    ["pl", "Zażółć gęślą jaźń"],
    ["uk", "їжак"],
    ["ja", "Zażółć gęślą jaźń їжак"],
  ] as const)
    posts.push({ language, post: await fresh.post(author, { text, language }) });
  const fonts = new CjkFonts(page);
  await signIn(author);
  for (const { language, post } of posts) {
    await expect(homeTimeline(page).post(author.displayName, post.text).article).toBeVisible();
    await expect
      .poll(async () => ({ language, fonts: distinct(await fonts.drawing(post.text)) }))
      .toEqual({ language, fonts: [selfHostedNotoSans] });
  }
});

test("a post in Traditional Chinese is drawn with Noto Sans CJK TC, which fetches its slices alone", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const post = await fresh.post(author, { text: "我們一起喝茶", language: "zh-TW" });
  await signIn(author);
  // The home screen on a page of its own, so that the sign-in screen's characters and slices are not counted.
  const home = await page.context().newPage();
  const fonts = new CjkFonts(home);
  await home.goto(page.url());
  await expect(homeTimeline(home).post(author.displayName, post.text).article).toBeVisible();
  await expect
    .poll(async () => distinct(await fonts.drawing(post.text)))
    .toEqual(distinct([selfHostedCjk("tc"), selfHostedNotoSans]));
  await fonts.settled();
  // The slice of TC that holds 茶 is fetched; no slice of SC or KR, which nothing on the page is in; and each slice
  // fetched claims a character the page shows.
  const tea = await fonts.sliceOf("tc", "茶");
  expect(tea).toBeDefined();
  expect(fonts.fetched.filter((s) => s.region === "tc").map((s) => s.slice)).toContain(tea);
  expect(fonts.fetched.filter((s) => s.region === "sc" || s.region === "kr")).toEqual([]);
  await expect.poll(() => fonts.unclaimed()).toEqual([]);
});

test("a theme's --font-sans-override draws the Latin, and the emoji and the Han stay with the self-hosted fonts", async ({
  page,
  signIn,
  fresh,
}) => {
  const admin = await fresh.person({ admin: true });
  // A font of the system with Latin alone, which Playwright's Chromium installs with its dependencies.
  const theme = await fresh.themeDraftStylesheet(admin, [{ token: "font-sans-override", value: '"Liberation Sans"' }]);
  const author = await fresh.person();
  const post = await fresh.post(author, { text: "Tea time 茶 🍵" });
  const fonts = new CjkFonts(page);
  await signIn(author);
  await applyTheme(page, theme);
  await expect(homeTimeline(page).post(author.displayName, post.text).article).toBeVisible();
  await expect
    .poll(async () => distinct(await fonts.drawing(post.text)))
    .toEqual(distinct([{ familyName: "Liberation Sans", isCustomFont: false }, selfHostedCjk("jp"), selfHostedNoto]));
});

test("the build serves the license of Noto Sans CJK", async ({ request }) => {
  const response = await request.get("/licenses/OFL-NotoSansCJK.txt");
  await expect(response).toBeOK();
  const license = await response.text();
  expect(license).toContain("SIL OPEN FONT LICENSE Version 1.1");
});

test("the build serves the license of Noto Sans", async ({ request }) => {
  const response = await request.get("/licenses/OFL-NotoSans.txt");
  await expect(response).toBeOK();
  const license = await response.text();
  expect(license).toContain("SIL OPEN FONT LICENSE Version 1.1");
});
