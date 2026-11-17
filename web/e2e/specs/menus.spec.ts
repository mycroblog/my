// SPDX-License-Identifier: AGPL-3.0-only

// The composer's text, whose code comes after its menu closed, does not take the focus from the menu's button
// (#3129): it needs the app's own build, whose chunk of the editor a slow network brings late. Where a menu is drawn (a
// sheet on a phone, a popover from a tablet's width) is a layout test on its story (catalog/).

import { expect, person, test } from "./test";

test.describe("on a phone", () => {
  // The narrowest screen the client supports (ADR-0110 §10).
  test.use({ viewport: { width: 320, height: 640 } });

  test("the composer's text, come after its menu closed, leaves the focus on the menu's button", async ({
    page,
    signIn,
  }) => {
    // The editor's chunk (Lexical) is held until the menu has closed, as a slow network brings it.
    const held: (() => void)[] = [];
    let holding = true;
    await page.route(/\/assets\/editor-[\w-]+\.js$/, async (route) => {
      if (holding) await new Promise<void>((resolve) => held.push(resolve));
      await route.continue();
    });
    await signIn(person("yuki"));
    await page.getByRole("button", { name: "New post", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "New post", exact: true });
    const audience = dialog.getByRole("button", { name: /^Who can see it: .+$/ });
    await audience.click();
    const menu = page.getByRole("menu", { name: "Who can see and reply", exact: true });
    await expect(menu).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(menu).toBeHidden();
    await expect(audience).toBeFocused();
    holding = false;
    for (const resolve of held.splice(0)) resolve();
    const text = dialog.getByRole("textbox", { name: "Text", exact: true });
    await expect(text).toBeVisible();
    await expect(audience).toBeFocused();
  });
});
