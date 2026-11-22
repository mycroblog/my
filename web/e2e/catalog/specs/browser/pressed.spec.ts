// SPDX-License-Identifier: AGPL-3.0-only

// A real press on a tab and on a choice's row (#3358): while the mouse holds it down, it takes another shade than at
// rest and moves down 1px, and the tab row keeps the moved tab whole; the current tab, pressed, takes a shade of its
// own too; a choice that is held while an answer is in flight takes no press at all; an off switch's track steps
// through the raised surface's shades; words thicken their underline and a picture takes the press shade, neither
// moving (#3369). Only a browser's mouse makes the state (`:active`, React Aria's press),
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

test("words in a line thicken their underline from 1px to 2px hovered and 3px pressed, unbroken, and stay where they are", async ({
  page,
  open,
}) => {
  await open("design-system-button--in-text");
  const line = (name: string, role: "link" | "button") => page.getByRole(role, { name, exact: true });
  for (const [name, role] of [
    ["Sign in", "link"],
    ["Try again", "button"],
  ] as const)
    await test.step(name, async () => {
      const word = line(name, role);
      const read = () =>
        word.evaluate((el) => {
          const style = getComputedStyle(el);
          return {
            thickness: style.textDecorationThickness,
            skip: style.textDecorationSkipInk,
            top: el.getBoundingClientRect().top,
          };
        });
      const rest = await read();
      const box = await word.boundingBox();
      if (box === null) throw new Error(`${name} is not drawn`);
      await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
      const hovered = await read();
      await page.mouse.down();
      const pressed = await read();
      await page.mouse.up();
      await page.mouse.move(0, 0);
      expect([rest.thickness, hovered.thickness, pressed.thickness]).toEqual(["1px", "2px", "3px"]);
      expect([hovered.skip, pressed.skip]).toEqual(["none", "none"]);
      expect(Math.abs(pressed.top - rest.top)).toBeLessThanOrEqual(slack);
    });
});

test("a picture takes the press tokens' shade over it, as a filled control's share, hovered and pressed, and stays", async ({
  page,
  open,
}) => {
  for (const theme of ["light", "dark"] as const) {
    await open("design-system-media--default", { theme });
    const shades = await page.evaluate(() =>
      ["--press-hover", "--press-pressed"].map((share) => {
        const probe = document.createElement("div");
        probe.style.cssText = `position: absolute; visibility: hidden; background-color: color-mix(in srgb, var(--press-shade) var(${share}), transparent)`;
        document.body.append(probe);
        const color = getComputedStyle(probe).backgroundColor;
        probe.remove();
        return color;
      }),
    );
    await test.step(`in the ${theme} theme`, async () => {
      const picture = page.getByRole("button", {
        name: "Open image: Low hills at dawn under a peach sky",
        exact: true,
      });
      const rest = await lookOf(picture);
      await holdDown(page, picture);
      const pressed = await lookOf(picture);
      await page.mouse.up();
      await page.mouse.move(0, 0);
      const box = await picture.boundingBox();
      if (box === null) throw new Error("the picture is not drawn");
      await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
      const hovered = await lookOf(picture);
      await page.mouse.move(0, 0);
      expect([hovered.own, pressed.own]).toEqual(shades);
      expect(Math.abs(pressed.top - rest.top)).toBeLessThanOrEqual(slack);
    });
  }
});
