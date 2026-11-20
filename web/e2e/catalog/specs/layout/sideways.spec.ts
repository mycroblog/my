// SPDX-License-Identifier: AGPL-3.0-only

// What the check of every story says of a page that scrolls sideways (sidewaysScroll of boxes.ts): the element that
// reaches past the viewport, where its JSX is, its direction and its parent's, and its box, so that the cause is read
// from the code and not reproduced.

import { sidewaysScroll } from "../boxes";
import { expect, minimum, test } from "../test";

test("a page that scrolls sideways right to left names the element past its edge, its direction and its box", async ({
  page,
}) => {
  await page.setViewportSize({ width: minimum, height: 640 });
  await page.setContent(`
    <html dir="rtl"><body style="margin: 0">
      <main data-source="web/design-system/src/frame.tsx:12">
        <div role="group" aria-label="Actions" class="flex gap-2" style="inline-size: 400px" dir="rtl"
          data-source="web/design-system/src/row.tsx:34"><span>One</span><span>Two</span></div>
        <p class="note" style="inline-size: 360px">A note</p>
      </main>
    </body></html>
  `);

  await expect
    .poll(() => sidewaysScroll(page))
    .toEqual([
      "the page is 80 px wider than its 320 px viewport",
      '<div.flex.gap-2> role group "Actions" at web/design-system/src/row.tsx:34; dir rtl (own), its parent\'s rtl (inherited); from -80 to 320, 400 px wide in a 320 px viewport',
      '<p.note> "A note" inside web/design-system/src/frame.tsx:12; dir rtl (inherited), its parent\'s rtl (inherited); from -40 to 320, 360 px wide in a 320 px viewport',
    ]);
});

test("a page no wider than its viewport says nothing", async ({ page }) => {
  await page.setViewportSize({ width: minimum, height: 640 });
  await page.setContent(`<html dir="rtl"><body style="margin: 0"><p style="inline-size: 320px">Fits</p></body></html>`);

  await expect.poll(() => sidewaysScroll(page)).toEqual([]);
});
