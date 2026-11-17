// SPDX-License-Identifier: AGPL-3.0-only

// ArticlePane (article-editor.tsx): a part of the article editor that shows at some widths.

import { expectNoSidewaysScroll } from "../boxes";
import { expect, from, test } from "../test";

test("a pane shows at every width, from `desktop` alone, under it alone, or never", async ({ page, open }) => {
  await open("design-system-layout-parts--article-panes", { width: from.desktop - 1 });
  const pane = (shown: string) => page.getByText(`A pane shown ${shown}.`, { exact: true });
  await expect(pane("always")).toBeVisible();
  await expect(pane("narrow")).toBeVisible();
  await expect(pane("desktop")).toBeHidden();
  await expect(pane("never")).toBeHidden();
  await expectNoSidewaysScroll(page);

  await page.setViewportSize({ width: from.desktop, height: 640 });
  await expect(pane("always")).toBeVisible();
  await expect(pane("desktop")).toBeVisible();
  await expect(pane("narrow")).toBeHidden();
  await expect(pane("never")).toBeHidden();
});
