// SPDX-License-Identifier: AGPL-3.0-only

// What a post's media need a browser for: a swipe moves between a post's pictures in the media viewer, and sensitive
// media are covered by the colours of their blurhash and ask for no file until shown. Every post is the test's own.

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

test("sensitive media are covered by their blurhash and load nothing until shown", async ({ page, signIn, fresh }) => {
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
});
