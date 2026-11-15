// SPDX-License-Identifier: AGPL-3.0-only

// A person opens a post's pictures whole in the media viewer, from a timeline and from a thread, moves between them by
// the keyboard and by swipe, and closes it back to the picture that opened it; plays a video and a sound in place;
// and shows sensitive media, which loads nothing until shown. A broken link card leaves its post drawn. Every post is
// the test's own: the demo server's videos and sounds are the few-kilobyte files of src/av.ts, and a link card, which
// the server makes only for a page it fetched, is given to the client's timeline by the test.

import type { Page } from "@playwright/test";
import type { Drawing, RGB } from "../src/drawing";
import { homeTimeline, MediaViewer, Thread } from "./screens";
import { expect, test } from "./test";

/** A picture of one color, whose blurhash is that color. */
const plain = (color: RGB): Drawing => ({ width: 64, height: 40, top: color, bottom: color, shapes: [] });

const long =
  "A wide valley at first light, three ridges one behind the other, the nearest dark with pines and the farthest a pale" +
  " blue line under a peach sky";

const three = [
  { name: "ridge.png", description: "Morning ridge", drawing: plain([200, 120, 90]) },
  { name: "sea.png", description: "", drawing: plain([90, 150, 200]) },
  { name: "valley.png", description: long, drawing: plain([120, 170, 110]) },
];

const escaped = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

test("a picture opens whole from the home timeline, the arrow keys move and announce, and Escape returns to it", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const { text } = await fresh.post(author, { text: "Three from the walk.", images: three });
  await signIn(await fresh.person({ follows: [author] }));
  const post = homeTimeline(page).post(author.displayName, text);
  const opener = post.picture("Open image: Morning ridge");
  await opener.click();
  const viewer = new MediaViewer(page);
  await expect(viewer.dialog).toBeVisible();
  await expect(viewer.position("1 of 3")).toBeVisible();
  await expect(viewer.caption("Morning ridge")).toBeVisible();
  await page.keyboard.press("ArrowRight");
  await expect(viewer.position("2 of 3")).toBeVisible();
  await expect(viewer.announced("2 of 3, image")).toBeAttached();
  await page.keyboard.press("ArrowRight");
  await expect(viewer.position("3 of 3")).toBeVisible();
  await expect(viewer.caption(long)).toBeVisible();
  // The start of the description, cut short at 100 characters.
  await expect(viewer.announced(new RegExp(`^3 of 3, ${escaped(long.slice(0, 60))}.{0,40}…$`))).toBeAttached();
  await page.keyboard.press("ArrowRight");
  await expect(viewer.position("3 of 3")).toBeVisible();
  await page.keyboard.press("ArrowLeft");
  await page.keyboard.press("ArrowLeft");
  await expect(viewer.position("1 of 3")).toBeVisible();
  await expect(viewer.announced("1 of 3, Morning ridge")).toBeAttached();
  await page.keyboard.press("Escape");
  await expect(viewer.dialog).toHaveCount(0);
  await expect(opener).toBeFocused();
});

test("Previous, Next and Zoom act as the keys do, and Close returns to the picture that opened it", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const { text } = await fresh.post(author, { text: "Three from the walk, again.", images: three });
  await signIn(await fresh.person({ follows: [author] }));
  const opener = homeTimeline(page).post(author.displayName, text).picture("Open image");
  await opener.click();
  const viewer = new MediaViewer(page);
  await expect(viewer.position("2 of 3")).toBeVisible();
  await viewer.button("Next").click();
  await expect(viewer.position("3 of 3")).toBeVisible();
  await expect(viewer.button("Next")).toHaveCount(0);
  await viewer.button("Previous").click();
  await viewer.button("Previous").click();
  await expect(viewer.position("1 of 3")).toBeVisible();
  await expect(viewer.button("Previous")).toHaveCount(0);
  await viewer.button("Zoom in").click();
  await expect(viewer.button("Zoom out")).toBeVisible();
  await viewer.button("Zoom out").click();
  await expect(viewer.button("Zoom in")).toBeVisible();
  await viewer.button("Close").click();
  await expect(viewer.dialog).toHaveCount(0);
  await expect(opener).toBeFocused();
});

test.describe("on a touch screen", () => {
  test.use({ hasTouch: true });

  test("a picture opens whole from a thread, a swipe moves between them, and Close returns to it", async ({
    page,
    signIn,
    fresh,
  }) => {
    const author = await fresh.person();
    const { text } = await fresh.post(author, { text: "Three from the walk, in a thread.", images: three });
    await signIn(await fresh.person({ follows: [author] }));
    await homeTimeline(page).post(author.displayName, text).open();
    const thread = new Thread(page);
    await expect(thread.heading).toBeVisible();
    const opener = thread.post(author.displayName, text).picture("Open image: Morning ridge");
    await opener.click();
    const viewer = new MediaViewer(page);
    await expect(viewer.position("1 of 3")).toBeVisible();
    await viewer.swipe("left");
    await expect(viewer.position("2 of 3")).toBeVisible();
    await viewer.swipe("left");
    await expect(viewer.position("3 of 3")).toBeVisible();
    await viewer.swipe("right");
    await expect(viewer.position("2 of 3")).toBeVisible();
    await viewer.button("Close").click();
    await expect(viewer.dialog).toHaveCount(0);
    await expect(opener).toBeFocused();
  });
});

test("a video plays in place and keeps its description, and Open video opens it whole", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const { text } = await fresh.post(author, {
    text: "The tide coming in.",
    files: [{ kind: "video", description: "Waves over the rocks", poster: plain([40, 70, 120]) }],
  });
  await signIn(await fresh.person({ follows: [author] }));
  const post = homeTimeline(page).post(author.displayName, text);
  await post.openVideo.click();
  const viewer = new MediaViewer(page);
  await expect(viewer.caption("Waves over the rocks")).toBeVisible();
  await expect(viewer.position("1 of 1")).toHaveCount(0);
  await viewer.button("Close").click();
  await expect(post.openVideo).toBeFocused();
  await post.playVideo("Play video: Waves over the rocks").click();
  await expect(post.playVideo("Play video: Waves over the rocks")).toHaveCount(0);
  await expect(post.video("Waves over the rocks")).toBeVisible();
});

test("a sound plays in place and pauses, its time read as where it is of its length", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const { text } = await fresh.post(author, {
    text: "Three seconds of quiet.",
    files: [{ kind: "audio", description: "A quiet room" }],
  });
  await signIn(await fresh.person({ follows: [author] }));
  const post = homeTimeline(page).post(author.displayName, text);
  await expect(post.article.getByText("0:00 of 0:03", { exact: true })).toBeAttached();
  await post.audioButton("Play audio").click();
  await expect(post.audioButton("Pause audio")).toBeVisible();
  await post.audioButton("Pause audio").click();
  await expect(post.audioButton("Play audio")).toBeVisible();
});

test("sensitive media are covered by their blurhash and load nothing until shown; the line shows and hides them", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const color: RGB = [63, 111, 134];
  const { text, media } = await fresh.post(author, {
    text: "A graze from the trail.",
    sensitive: true,
    images: [{ name: "graze.png", description: "A grazed knee", drawing: plain(color) }],
  });
  const file = media[0]?.preview_url ?? "";
  const requested = new Set<string>();
  page.on("request", (r) => requested.add(r.url()));
  await signIn(await fresh.person({ follows: [author] }));
  const post = homeTimeline(page).post(author.displayName, text);
  await expect(post.sensitiveLine("Sensitive image", "Show")).toBeVisible();
  await expect(post.image("A grazed knee")).toHaveCount(0);
  // The blurhash of a picture of one color decodes to that color, give or take its rounding.
  await expect
    .poll(async () => {
      const drawn = await post.coveredColor();
      return drawn.length === 3 ? Math.max(...drawn.map((v, i) => Math.abs(v - (color[i] ?? 0)))) : undefined;
    })
    .toBeLessThanOrEqual(2);
  expect(requested.has(file)).toBe(false);
  await post.sensitiveLine("Sensitive image", "Show").click();
  await expect(post.image("A grazed knee")).toBeVisible();
  await expect(post.sensitiveLine("Sensitive image", "Hide")).toBeVisible();
  await expect.poll(() => requested.has(file)).toBe(true);
  await post.sensitiveLine("Sensitive image", "Hide").click();
  await expect(post.image("A grazed knee")).toHaveCount(0);
  await expect(post.sensitiveLine("Sensitive image", "Show")).toBeVisible();
});

test("a press on covered media shows them, named by kind and count", async ({ page, signIn, fresh }) => {
  const author = await fresh.person();
  const { text } = await fresh.post(author, {
    text: "Two more from the trail.",
    sensitive: true,
    images: three.slice(0, 2),
  });
  await signIn(await fresh.person({ follows: [author] }));
  const post = homeTimeline(page).post(author.displayName, text);
  await expect(post.sensitiveLine("2 sensitive images", "Show")).toBeVisible();
  await post.covered("2 sensitive images").click();
  await expect(post.image("Morning ridge")).toBeVisible();
  await expect(post.sensitiveLine("2 sensitive images", "Hide")).toBeVisible();
  await expect(post.picture("Open image: Morning ridge")).toBeVisible();
});

/** Gives the home timeline's copy of the post `id` the link card `card`. */
async function withCard(page: Page, id: string, card: Record<string, unknown>) {
  await page.route("**/api/v1/timelines/home*", async (route) => {
    const response = await route.fetch();
    const page = (await response.json()) as { items: { id: string }[] };
    await route.fulfill({
      response,
      json: { ...page, items: page.items.map((s) => (s.id === id ? { ...s, card } : s)) },
    });
  });
}

test("a post's link card is a link to the page, in a new tab", async ({ page, signIn, fresh }) => {
  const author = await fresh.person();
  const { id, text } = await fresh.post(author, { text: "Worth a read." });
  await withCard(page, id, {
    url: "https://blog.example/posts/rye",
    title: "Baking rye at home",
    description: "What three weeks of starters taught me.",
    site_name: "Field Notes",
  });
  await signIn(await fresh.person({ follows: [author] }));
  const card = homeTimeline(page)
    .post(author.displayName, text)
    .card(/^Field Notes Baking rye at home What three weeks of starters taught me\. \(opens in a new tab\)$/);
  await expect(card).toHaveAttribute("href", "https://blog.example/posts/rye");
  await expect(card).toHaveAttribute("target", "_blank");
});

test("a link card that cannot be drawn leaves its post drawn", async ({ page, signIn, fresh }) => {
  const author = await fresh.person();
  const { id, text } = await fresh.post(author, { text: "This card is broken." });
  // A title that is no text: the card throws as it draws.
  await withCard(page, id, { url: "https://blog.example/", title: { broken: true }, description: "", site_name: "x" });
  const errors: string[] = [];
  page.on("console", (m) => {
    if (m.type() === "error") errors.push(m.text());
  });
  await signIn(await fresh.person({ follows: [author] }));
  const post = homeTimeline(page).post(author.displayName, text);
  await expect(post.paragraph(text)).toBeVisible();
  await expect(post.favouriteButton).toBeVisible();
  // React's error 31, an object drawn as text: the card did throw, and its boundary caught it.
  await expect
    .poll(() => errors.some((e) => /Minified React error #31|Objects are not valid as a React child/.test(e)))
    .toBe(true);
  await expect(post.card(/^x .*\(opens in a new tab\)$/)).toHaveCount(0);
});
