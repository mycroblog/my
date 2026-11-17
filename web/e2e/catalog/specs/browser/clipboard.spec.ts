// SPDX-License-Identifier: AGPL-3.0-only

// A post's Copy link, on the app's screen story: the browser's clipboard holds the post's address, which no DOM
// simulation has.

import { Notice } from "../../../specs/screens";
import { expect, pc, test } from "../test";

test("Copy link copies the post's address and says so", async ({ page, context, open }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await open("screens-timeline--normal", { width: pc, height: 800 });
  const post = page
    .getByRole("feed", { name: "Home timeline", exact: true })
    .getByRole("article", { name: "Sora そら", exact: true })
    .filter({ hasText: "Developed the first roll from the coast." });
  await post.getByRole("button", { name: "More", exact: true }).click();
  await page
    .getByRole("menu", { name: "More", exact: true })
    .getByRole("menuitem", { name: "Copy link", exact: true })
    .click();
  await expect(new Notice(page).text("Link copied")).toBeVisible();
  // The address ends with the post's ID, as the link of its time does.
  await expect
    .poll(async () => {
      const id = await post
        .getByRole("link", { name: /^\d+h ago$/ })
        .evaluate((time) => new URL((time as HTMLAnchorElement).href).pathname.split("/").at(-1));
      const copied = await page.evaluate(() => navigator.clipboard.readText());
      return id !== undefined && id !== "" && /^https?:\/\//.test(copied) && copied.endsWith(`/${id}`);
    })
    .toBe(true);
});
