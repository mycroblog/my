// SPDX-License-Identifier: AGPL-3.0-only

// A real press on a tab and on a choice's row (#3358): while the mouse holds it down, it takes another shade than at
// rest and moves down 1px, and the tab row keeps the moved tab whole; the current tab, pressed, takes a shade of its
// own too; a choice that is held while an answer is in flight takes no press at all; an off switch's track steps
// through the raised surface's shades (#3369). Only a browser's mouse makes the state (`:active`, React Aria's press),
// which no DOM simulation paints.

import { colorOf, cutAtBottom, holdDown, lookOf, slack } from "../press";
import { expect, test } from "../test";

test("a tab the mouse holds down takes the pressed shade and moves down 1px, the current one too, kept whole", async ({
  page,
  open,
}) => {
  /** How the tab looks at rest and while the mouse holds it down; it looks pressed and stands whole. */
  const press = async (name: RegExp) => {
    const tab = page.getByRole("tab", { name });
    const rest = await lookOf(tab);
    await holdDown(page, tab);
    const pressed = await lookOf(tab);
    await page.mouse.up();
    expect(pressed.shades).not.toBe(rest.shades);
    expect(Math.abs(pressed.top - rest.top - 1)).toBeLessThanOrEqual(slack);
    expect(cutAtBottom(rest)).toEqual([]);
    expect(cutAtBottom(pressed)).toEqual([]);
    return pressed;
  };

  for (const theme of ["light", "dark"] as const) {
    await open("design-system-follow-requests--in-notifications-rows", { theme });
    // The current tab first: the press on the other one makes it current once the mouse lets go.
    await test.step(`the current tab, in the ${theme} theme`, async () => {
      const pressed = await press(/^All$/);
      expect(pressed.own).toBe(await colorOf(page, "--color-surface-raised-pressed"));
    });
    await test.step(`the tab that is not current, in the ${theme} theme`, async () => {
      // Its name holds the count of what waits under it.
      await press(/^Mentions(\s.*)?$/);
    });
  }
});

test("a choice's row the mouse holds down takes the pressed shade and moves down 1px", async ({ page, open }) => {
  await open("design-system-choice--one");
  // A row is the label around its option's radio.
  const option = page.getByRole("radio", { name: "Baguette", exact: true });
  const rest = await lookOf(option, "its label");

  await holdDown(page, option, "its label");
  const pressed = await lookOf(option, "its label");
  await page.mouse.up();

  expect(pressed.shades).not.toBe(rest.shades);
  expect(Math.abs(pressed.top - rest.top - 1)).toBeLessThanOrEqual(slack);
});

test("a choice held while an answer is in flight shows no press where the mouse holds a row down", async ({
  page,
  open,
}) => {
  await open("design-system-choice--held");

  for (const name of ["Sourdough", "Baguette"])
    await test.step(name, async () => {
      const option = page.getByRole("radio", { name, exact: true });
      const rest = await lookOf(option, "its label");
      await holdDown(page, option, "its label");
      const held = await lookOf(option, "its label");
      await page.mouse.up();

      expect(held).toEqual(rest);
    });
});

test("an off switch's track rests on the raised surface and takes its hovered and pressed steps, on a raised surface too", async ({
  page,
  open,
}) => {
  /** The color of the track of `control`, the switch's input inside its label. */
  const trackOf = (control: ReturnType<typeof page.getByRole>) =>
    control.evaluate((input) => {
      const track = input.closest("label")?.querySelector('[aria-hidden="true"]');
      return track === null || track === undefined ? "" : getComputedStyle(track).backgroundColor;
    });

  for (const theme of ["light", "dark"] as const) {
    await open("design-system-switch--off", { theme });
    const steps = await Promise.all(
      (["--color-surface-raised", "--color-surface-raised-hover", "--color-surface-raised-pressed"] as const).map(
        (token) => colorOf(page, token),
      ),
    );
    // The switch at rest on the page, then on a raised surface (a section).
    for (const [at, control] of [
      ["the page", page.getByRole("switch", { name: "At rest", exact: true })],
      ["a raised surface", page.getByRole("switch", { name: "At rest, on a section", exact: true })],
    ] as const)
      await test.step(`on ${at}, in the ${theme} theme`, async () => {
        const rest = await trackOf(control);
        const box = await control.evaluate((input) => {
          const r = input.closest("label")?.querySelector('[aria-hidden="true"]')?.getBoundingClientRect();
          return r === undefined ? { x: 0, y: 0 } : { x: r.left + r.width / 2, y: r.top + r.height / 2 };
        });
        await page.mouse.move(box.x, box.y);
        const hovered = await trackOf(control);
        await page.mouse.down();
        const pressed = await trackOf(control);
        await page.mouse.up();
        await page.mouse.move(0, 0);
        expect([rest, hovered, pressed]).toEqual(steps);
      });
  }
});
