// SPDX-License-Identifier: AGPL-3.0-only

// The pressed state of a tab and of a choice's row, as a story previews it (#3358): the pressed one takes another
// shade than at rest and moves down 1px, at every width and in both themes, while the others stay as they were; and
// the tab row keeps the moved tab whole. The resting story of each is the same part with nothing pressed.

import { setTheme } from "../axe";
import { cutAtBottom, lookOf, slack } from "../press";
import { expect, minimum, pc, test } from "../test";

const widths = [minimum, pc] as const;
const themes = ["light", "dark"] as const;

test("a tab a story presses takes the pressed shade and moves down 1px, kept whole by its row", async ({
  page,
  open,
}) => {
  // Its name holds the count of what waits under it.
  const mentions = page.getByRole("tab", { name: /^Mentions(\s.*)?$/ });
  const all = page.getByRole("tab", { name: "All", exact: true });
  const read = async () => ({ mentions: await lookOf(mentions), all: await lookOf(all) });

  const rest: Record<string, Awaited<ReturnType<typeof read>>> = {};
  await open("design-system-follow-requests--in-notifications-rows");
  for (const width of widths)
    for (const theme of themes) {
      await page.setViewportSize({ width, height: 640 });
      await setTheme(page, theme);
      rest[`${String(width)} ${theme}`] = await read();
    }

  await open("design-system-follow-requests--in-notifications-tab-pressed");
  for (const width of widths)
    for (const theme of themes)
      await test.step(`at ${String(width)} px in the ${theme} theme`, async () => {
        await page.setViewportSize({ width, height: 640 });
        await setTheme(page, theme);
        const before = rest[`${String(width)} ${theme}`];
        if (before === undefined) throw new Error("test: no resting look was read");
        const pressed = await read();

        expect(pressed.mentions.shades).not.toBe(before.mentions.shades);
        expect(Math.abs(pressed.mentions.top - before.mentions.top - 1)).toBeLessThanOrEqual(slack);
        expect(cutAtBottom(before.mentions)).toEqual([]);
        expect(cutAtBottom(pressed.mentions)).toEqual([]);
        // The current tab is not pressed: it looks and stands as it did.
        expect(pressed.all).toEqual(before.all);
      });
});

test("a choice's row a story presses takes the pressed shade and moves down 1px, and the other rows rest", async ({
  page,
  open,
}) => {
  // A row is the label around its option's radio.
  const row = (name: string) => lookOf(page.getByRole("radio", { name, exact: true }), "its label");
  const read = async () => ({ baguette: await row("Baguette"), sourdough: await row("Sourdough") });

  const rest: Record<string, Awaited<ReturnType<typeof read>>> = {};
  // The same list, the same option chosen, nothing pressed.
  await open("design-system-choice--one");
  for (const width of widths)
    for (const theme of themes) {
      await page.setViewportSize({ width, height: 640 });
      await setTheme(page, theme);
      rest[`${String(width)} ${theme}`] = await read();
    }

  await open("design-system-choice--pressed");
  for (const width of widths)
    for (const theme of themes)
      await test.step(`at ${String(width)} px in the ${theme} theme`, async () => {
        await page.setViewportSize({ width, height: 640 });
        await setTheme(page, theme);
        const before = rest[`${String(width)} ${theme}`];
        if (before === undefined) throw new Error("test: no resting look was read");
        const pressed = await read();

        expect(pressed.baguette.shades).not.toBe(before.baguette.shades);
        expect(Math.abs(pressed.baguette.top - before.baguette.top - 1)).toBeLessThanOrEqual(slack);
        expect(pressed.sourdough).toEqual(before.sourdough);
      });
});
