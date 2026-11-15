// SPDX-License-Identifier: AGPL-3.0-only

// A reader of English translates a post written in Japanese through the server's provider (the demo server's fake,
// "Fake", which puts "[en] " before each text it translates) and goes back to the original, in the home timeline and
// in the post's thread; the content warning, the poll's options and the alt text are translated with the text. The
// provider down, a followers-only post the provider may not receive, and the reader's monthly limit (1000 characters
// on the demo server) each say why. Every post is the test's own, and so is every reader: the limit counts per person.

import { homeTimeline, line, Thread } from "./screens";
import type { Image } from "../src/scenario";
import { expect, test, type FreshPost } from "./test";

const note = "Post translated from Japanese by Fake · Show original";

const soup: Image = {
  name: "soup.png",
  description: "湯気の立つスープ",
  drawing: { width: 64, height: 64, top: [200, 120, 80], bottom: [120, 60, 40], shapes: [] },
};

test("a reader translates a post in the home timeline, the request reaching the server, and goes back", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const { text } = await fresh.post(author, { text: "今朝は尾根まで歩きました。", language: "ja" });
  await signIn(await fresh.person({ follows: [author] }));
  const post = homeTimeline(page).post(author.displayName, text);
  await post.translate();
  await expect(post.paragraph(`[en] ${text}`)).toBeVisible();
  await expect(post.translationLine("Post translated")).toHaveText(line(note));
  await expect(post.translateButton).toHaveCount(0);
  await post.originalButton.click();
  await expect(post.paragraph(text)).toHaveText(text);
  await expect(post.translateButton).toBeVisible();
  await expect(post.originalButton).toHaveCount(0);
});

test("a reader translates a post in its thread, and goes back", async ({ page, signIn, fresh }) => {
  const author = await fresh.person();
  const { text } = await fresh.post(author, { text: "駐車場から二時間くらいです。", language: "ja" });
  await signIn(await fresh.person({ follows: [author] }));
  await homeTimeline(page).post(author.displayName, text).open();
  const post = new Thread(page).post(author.displayName, text);
  await post.translate();
  await expect(post.paragraph(`[en] ${text}`)).toBeVisible();
  await expect(post.translationLine("Post translated")).toHaveText(line(note));
  await post.originalButton.click();
  await expect(post.paragraph(text)).toHaveText(text);
  await expect(post.translateButton).toBeVisible();
});

test("a post in the reader's own language offers no translation", async ({ page, signIn, fresh }) => {
  const author = await fresh.person();
  const { text } = await fresh.post(author, { text: "Up on the ridge before dawn." });
  await signIn(await fresh.person({ follows: [author] }));
  const post = homeTimeline(page).post(author.displayName, text);
  await expect(post.paragraph(text)).toBeVisible();
  await expect(post.translateButton).toHaveCount(0);
});

test("behind a content warning, the warning, the poll's options and the alt text are translated with the text", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const polled = await fresh.post(author, {
    text: "土曜のパンはどれにしますか？",
    language: "ja",
    spoiler: "パンの話",
    poll: { options: ["ライ麦", "サワー種"], days: 1 },
  });
  const pictured = await fresh.post(author, {
    text: "今日の昼ごはんです。",
    language: "ja",
    spoiler: "食べ物の写真",
    image: soup,
  });
  await signIn(await fresh.person({ follows: [author] }));
  const timeline = homeTimeline(page);

  // The translate line sits in the warning's band: it translates with the text still folded.
  const poll = timeline.post(author.displayName, "パンの話");
  await poll.translate();
  await expect(poll.warning("[en] パンの話")).toBeVisible();
  await expect(poll.translationLine("Post translated")).toHaveText(line(note));
  await poll.show();
  await expect(poll.paragraph(`[en] ${polled.text}`)).toBeVisible();
  await expect(poll.option("[en] ライ麦")).toBeVisible();
  await expect(poll.option("[en] サワー種")).toBeVisible();

  const image = timeline.post(author.displayName, "食べ物の写真");
  await image.translate();
  await expect(image.warning("[en] 食べ物の写真")).toBeVisible();
  await image.show();
  await expect(image.paragraph(`[en] ${pictured.text}`)).toBeVisible();
  await expect(image.image(`[en] ${soup.description}`)).toBeVisible();
  await image.originalButton.click();
  await expect(image.warning("食べ物の写真")).toBeVisible();
  await expect(image.image(soup.description)).toBeVisible();
});

test("the provider down says so, and Try again asks it again", async ({ page, signIn, fresh }) => {
  const author = await fresh.person();
  const { text } = await fresh.post(author, { text: "[translation-fails] 今日は雨です。", language: "ja" });
  await signIn(await fresh.person({ follows: [author] }));
  const post = homeTimeline(page).post(author.displayName, text);
  await post.translate();
  const refusal = post.translationLine("This server's translation service");
  await expect(refusal).toHaveText(line("This server's translation service is not answering. · Try again"));
  await post.translate(post.translateAgainButton);
  await expect(refusal).toHaveText(line("This server's translation service is not answering. · Try again"));
  await expect(post.paragraph(text)).toHaveText(text);
});

test("a followers-only post, which the provider may not receive, offers no translation", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const { text } = await fresh.post(author, {
    text: "フォロワーだけに: 集合は二階です。",
    language: "ja",
    visibility: "private",
  });
  await signIn(await fresh.person({ follows: [author] }));
  const post = homeTimeline(page).post(author.displayName, text);
  await expect(post.paragraph(text)).toBeVisible();
  await expect(post.translateButton).toHaveCount(0);
});

test("over the reader's monthly limit, the line says so and when translation comes back", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  // Two posts of 489 characters each use 978 of the reader's 1000; a third does not fit.
  const long = "山の朝は静かです。".repeat(53).slice(0, 480);
  const posts: FreshPost[] = [];
  for (const n of ["一", "二", "三"])
    posts.push(await fresh.post(author, { text: `${n}${long.slice(1)}`, language: "ja" }));
  await signIn(await fresh.person({ follows: [author] }));
  const timeline = homeTimeline(page);
  const [first, second, third] = posts.map((p) => timeline.post(author.displayName, p.text));
  if (first === undefined || second === undefined || third === undefined) throw new Error("e2e: three posts");
  for (const post of [first, second]) {
    await post.translate();
    await expect(post.translationLine("Post translated")).toHaveText(line(note));
  }
  await third.translate();
  await expect(third.translationLine("This server's monthly")).toHaveText(
    line("This server's monthly translation limit is reached. Translation is available again next month."),
  );
  await expect(third.translateButton).toHaveCount(0);
  await expect(third.translateAgainButton).toHaveCount(0);
});
