// SPDX-License-Identifier: AGPL-3.0-only

// What a browser alone decides of the focus: a folded part is out of the Tab order and takes no room (disclosure.tsx),
// and the focus ring shows on a control the keyboard focused, not on one the pointer pressed (`:focus-visible`), which
// both DOM simulations answer the same for.

import { boxOf } from "../boxes";
import { expect, test } from "../test";

const story = "design-system-layout-parts--folded-disclosure";

test("what a disclosure folds takes no room and no Tab stop until it is shown, and the focus ring shows on a button the keyboard focused alone", async ({
  page,
  open,
}) => {
  await open(story);
  const before = page.getByRole("button", { name: "Before the fold", exact: true });
  const after = page.getByRole("button", { name: "After the fold", exact: true });
  const inside = page.getByRole("link", { name: "a link in the folded part", exact: true });
  const show = page.getByRole("button", { expanded: false });
  await expect(inside).toBeHidden();
  const folded = (await boxOf(after)).top;

  await before.focus();
  await page.keyboard.press("Tab");
  await expect(show).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(after).toBeFocused();

  // Shown, it takes its room, and Tab stops at the link inside it.
  await page.keyboard.press("Shift+Tab");
  await page.keyboard.press("Enter");
  await expect(inside).toBeVisible();
  await expect.poll(async () => (await boxOf(after)).top).toBeGreaterThan(folded);
  await page.keyboard.press("Tab");
  await expect(inside).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(after).toBeFocused();

  // The same page opens the story again, with nothing focused and the part folded.
  await test.step("the focus ring shows on a button the keyboard focused, and not on one the pointer pressed", async () => {
    await open(story);
    /** What draws a ring around the button: its outline and its shadow. */
    const ring = (button: typeof after) =>
      button.evaluate((element) => {
        const { outlineStyle, outlineWidth, boxShadow } = getComputedStyle(element);
        return `${outlineStyle === "none" ? "no outline" : outlineWidth} / ${boxShadow}`;
      });
    const atRest = await ring(after);

    await after.click();
    await expect(after).toBeFocused();
    await expect.poll(() => ring(after)).toBe(atRest);

    await before.focus();
    await page.keyboard.press("Tab");
    await page.keyboard.press("Tab");
    await expect(after).toBeFocused();
    await expect.poll(() => ring(after)).not.toBe(atRest);
  });
});
