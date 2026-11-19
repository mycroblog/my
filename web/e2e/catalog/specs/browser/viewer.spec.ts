// SPDX-License-Identifier: AGPL-3.0-only

// The media viewer (viewer.tsx) under a finger: a swipe from the picture moves to the next or the previous one, by
// where the touch went.

import { boxesAtRest } from "../boxes";
import { expect, test } from "../test";

test.describe("on a touch screen", () => {
  test.use({ hasTouch: true });

  test("a swipe on the picture moves to the next picture, and one back to the previous", async ({ page, open }) => {
    await open("design-system-media-viewer--default", { width: 800 });
    const viewer = page.getByRole("dialog", { name: "Media", exact: true });
    const position = (text: string) => viewer.getByText(text, { exact: true });
    await expect(position("2 of 3")).toBeVisible();
    /**
     * A touch that starts on the shown picture, once it is drawn, and moves sideways by two fifths of the viewer's
     * width: "left" shows the next in a left-to-right page.
     */
    const swipe = async (direction: "left" | "right") => {
      const { viewer: box, picture } = await boxesAtRest({ viewer, picture: viewer.getByRole("presentation") });
      const y = (picture.top + picture.bottom) / 2;
      const from = (picture.left + picture.right) / 2;
      const cdp = await page.context().newCDPSession(page);
      await cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x: from, y }] });
      for (const part of [0.1, 0.2, 0.3, 0.4]) {
        const x = from + box.width * (direction === "left" ? -part : part);
        await cdp.send("Input.dispatchTouchEvent", { type: "touchMove", touchPoints: [{ x, y }] });
      }
      await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
      await cdp.detach();
    };
    await swipe("left");
    await expect(position("3 of 3")).toBeVisible();
    await swipe("right");
    await expect(position("2 of 3")).toBeVisible();
    await swipe("right");
    await expect(position("1 of 3")).toBeVisible();
  });
});
