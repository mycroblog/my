// SPDX-License-Identifier: AGPL-3.0-only

// A feed's pull from its top (pull.tsx) under a finger, in a browser that scrolls the page: pulled down past the
// threshold from the page's top and let go, the feed is read anew, which its live region says until it is read;
// pulled short of it, or with a mouse, nothing is read.

import type { Page } from "@playwright/test";
import { expect, test } from "../test";

const story = "design-system-feed-pull--a-touch-pulls";

/** The first post's text. */
const firstPost = (page: Page) =>
  page.getByText("The first frost turned every leaf on the path silver this morning.", { exact: true });

/** What the live region of the pull says while the feed is read anew. */
const said = (page: Page) => page.getByRole("status").getByText("Loading what’s new…", { exact: true });

/** A touch that starts on the first post and moves down by `by` px, in steps, then lets go. */
async function pull(page: Page, by: number): Promise<void> {
  const box = await firstPost(page).boundingBox();
  if (box === null) throw new Error("test: the first post has no box");
  const x = box.x + box.width / 2;
  const y = box.y + box.height / 2;
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x, y }] });
  for (const part of [0.25, 0.5, 0.75, 1])
    await cdp.send("Input.dispatchTouchEvent", { type: "touchMove", touchPoints: [{ x, y: y + by * part }] });
  await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  await cdp.detach();
}

test.describe("on a touch screen", () => {
  test.use({ hasTouch: true });

  test("a pull past the threshold reads the feed anew, and says so until it is read", async ({ page, open }) => {
    await open(story);
    await pull(page, 200);
    await expect(said(page)).toHaveCount(1);
    await expect(said(page)).toHaveCount(0);
  });

  test("a pull short of the threshold reads nothing", async ({ page, open }) => {
    await open(story);
    await pull(page, 40);
    await expect(said(page)).toHaveCount(0);
    await expect(firstPost(page)).toBeVisible();
  });
});

test("a mouse dragged down from the top reads nothing", async ({ page, open }) => {
  await open(story);
  const box = await firstPost(page).boundingBox();
  if (box === null) throw new Error("test: the first post has no box");
  await page.mouse.move(box.x + 10, box.y + 5);
  await page.mouse.down();
  await page.mouse.move(box.x + 10, box.y + 205, { steps: 4 });
  await page.mouse.up();
  await expect(said(page)).toHaveCount(0);
});
