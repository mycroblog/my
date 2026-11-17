// SPDX-License-Identifier: AGPL-3.0-only

// What opens over a page (menu.tsx, dialog.tsx, modal.tsx): on a phone a menu and a small form are a sheet along the
// bottom edge, headed by its name and Close, painted over its backdrop; from `tablet` they are a popover by their
// trigger, with no head. A question stands in the middle of the view, and a panel is a screen of its own on a phone.
// The test presses the triggers itself.

import type { Locator, Page } from "@playwright/test";
import { expectNoComputedViolations } from "../axe";
import {
  boxesAtRest,
  boxOf,
  covered,
  expectInside,
  expectInViewport,
  expectNoSidewaysScroll,
  outside,
  viewportBox,
} from "../boxes";
import { expect, pc, test } from "../test";

const story = "design-system-layout-parts--overlays";

const trigger = (page: Page, name: string): Locator => page.getByRole("button", { name, exact: true });
/** A line of the page under the overlays. */
const under = (page: Page): Locator => page.getByText(/^Line 1 of the page under the overlays\. .+$/);

/** The box reaches the viewport's bottom edge and its sides, give or take the sheet's margin. */
async function expectAlongBottomEdge(page: Page, sheet: Locator): Promise<void> {
  await expect
    .poll(async () => {
      const [box, view] = [await boxOf(sheet), await viewportBox(page)];
      return box.width > view.width * 0.8 && view.bottom - box.bottom < 48 && box.top > view.height / 4;
    })
    .toBe(true);
}

test("on a phone a menu and a small form are sheets along the bottom edge, headed, over their backdrop", async ({
  page,
  open,
}) => {
  await open(story);

  await test.step("a menu", async () => {
    await trigger(page, "Menu at the end").click();
    const menu = page.getByRole("menu", { name: "Menu at the end", exact: true });
    const heading = page.getByRole("heading", { name: "Menu at the end", exact: true });
    const close = page.getByRole("button", { name: "Close", exact: true });
    await expect(menu).toBeVisible();
    await expectAlongBottomEdge(page, menu);
    await expectInViewport(page, menu);
    await expectInViewport(page, heading);
    // Its head: the name, with Close at its end, over the items.
    const head = await boxesAtRest({ heading, close, menu });
    expect(head.close.left).toBeGreaterThanOrEqual(head.heading.right);
    expect(head.heading.bottom).toBeLessThanOrEqual(head.menu.top + 1);
    // Each item, the name and Close are on top at their centre; the page is under the backdrop.
    await expect.poll(() => covered(menu.getByRole("menuitem"))).toEqual([]);
    await expect.poll(() => covered(heading)).toEqual([]);
    await expect.poll(() => covered(close)).toEqual([]);
    await expect.poll(async () => (await covered(under(page))).length).toBe(1);
    await expectNoSidewaysScroll(page);
    await expectNoComputedViolations(page);
    await close.click();
    await expect(menu).toBeHidden();
    await expect(trigger(page, "Menu at the end")).toBeFocused();
  });

  await test.step("a small form", async () => {
    await trigger(page, "Open the form").click();
    const form = page.getByRole("dialog", { name: "Schedule", exact: true });
    await expect(form).toBeVisible();
    await expectAlongBottomEdge(page, form);
    await expectInViewport(page, form);
    const heading = form.getByRole("heading", { name: "Schedule", exact: true });
    const close = form.getByRole("button", { name: "Close", exact: true });
    const field = form.getByRole("textbox", { name: "Date and time", exact: true });
    const head = await boxesAtRest({ heading, close, field });
    expect(head.close.left).toBeGreaterThanOrEqual(head.heading.right);
    expect(head.field.top).toBeGreaterThanOrEqual(head.heading.bottom);
    await expect.poll(() => covered(form.getByRole("button", { name: "Set time", exact: true }))).toEqual([]);
    await expect.poll(async () => (await covered(under(page))).length).toBe(1);
    await close.click();
    await expect(form).toBeHidden();
    await expect(trigger(page, "Open the form")).toBeFocused();
  });
});

test("on a phone a question stands in the middle of the view, and a panel is a screen of its own that scrolls inside", async ({
  page,
  open,
}) => {
  await open(story);

  await test.step("a question", async () => {
    await trigger(page, "Open the question").click();
    const question = page.getByRole("alertdialog", { name: "Delete this list?", exact: true });
    const cancel = question.getByRole("button", { name: "Cancel", exact: true });
    const confirm = question.getByRole("button", { name: "Delete", exact: true });
    await expect(question).toBeVisible();
    await expectInViewport(page, question);
    await expect
      .poll(async () => {
        const [box, view] = [await boxOf(question), await viewportBox(page)];
        return Math.abs((box.left + box.right) / 2 - view.width / 2);
      })
      .toBeLessThan(1);
    // The refusal beside the confirmation, which is at the end, both inside it.
    const buttons = await boxesAtRest({ cancel, confirm });
    expect(buttons.cancel.right).toBeLessThanOrEqual(buttons.confirm.left);
    expect(buttons.confirm.top).toBe(buttons.cancel.top);
    await expectInside(confirm, question);
    await expect.poll(() => covered(confirm)).toEqual([]);
    await expect.poll(async () => (await covered(under(page))).length).toBe(1);
    await expectNoComputedViolations(page);
    await cancel.click();
    await expect(question).toBeHidden();
  });

  await test.step("a panel", async () => {
    await trigger(page, "Open the panel").click();
    const panel = page.getByRole("dialog", { name: "Edit history", exact: true });
    const close = panel.getByRole("button", { name: "Close", exact: true });
    await expect(panel).toBeVisible();
    // The whole view, and no more: what it shows scrolls inside it, the page under it does not.
    await expect
      .poll(async () => {
        const [box, view] = [await boxOf(panel), await viewportBox(page)];
        return (
          [box.left, box.top, box.right, box.bottom].map(Math.round).join() === [0, 0, view.right, view.bottom].join()
        );
      })
      .toBe(true);
    await expectInViewport(page, close);
    const last = panel.getByText(/^Line 30 of the panel\. .+$/);
    await last.scrollIntoViewIfNeeded();
    await expectInViewport(page, last);
    await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(0);
    await expectNoSidewaysScroll(page);
    await close.click();
    await expect(panel).toBeHidden();
  });
});

for (const locale of ["en", "ar-XB"] as const)
  test(`from \`tablet\` a menu and a small form are popovers by their triggers, with no head (${locale === "en" ? "left to right" : "right to left"})`, async ({
    page,
    open,
  }) => {
    await open(story, { width: pc, height: 800, locale });
    /** How far the popover's edges are from its trigger's: its start, its end, and the gap under the trigger. */
    const by = async (popover: Locator, button: Locator) => {
      const [box, of] = [await boxOf(popover), await boxOf(button)];
      const [start, end] = [Math.abs(box.left - of.left), Math.abs(box.right - of.right)];
      return { start: locale === "en" ? start : end, end: locale === "en" ? end : start, under: box.top - of.bottom };
    };
    // Within a step of the trigger's edge: the popover's own edge and the menu's inside it.
    const step = 16;

    for (const side of ["start", "end"] as const) {
      const button = trigger(page, `Menu at the ${side}`);
      await button.click();
      const menu = page.getByRole("menu", { name: `Menu at the ${side}`, exact: true });
      await expect(menu).toBeVisible();
      await expect.poll(async () => (await boxOf(menu)).width).toBeLessThan(pc / 2);
      await expect.poll(async () => (await by(menu, button))[side]).toBeLessThan(step);
      await expect.poll(async () => (await by(menu, button)).under).toBeGreaterThanOrEqual(0);
      await expect.poll(async () => (await by(menu, button)).under).toBeLessThan(step);
      await expectInViewport(page, menu);
      await expect.poll(() => covered(menu.getByRole("menuitem"))).toEqual([]);
      await expect(page.getByRole("heading", { name: `Menu at the ${side}`, exact: true })).toBeHidden();
      await page.keyboard.press("Escape");
      await expect(menu).toBeHidden();
      await expect(button).toBeFocused();
    }

    const button = trigger(page, "Open the form");
    await button.click();
    const form = page.getByRole("dialog", { name: "Schedule", exact: true });
    await expect(form).toBeVisible();
    await expect(form.getByRole("heading", { name: "Schedule", exact: true })).toBeHidden();
    await expect.poll(async () => (await boxOf(form)).width).toBeLessThan(pc / 2);
    await expect.poll(async () => (await by(form, button)).start).toBeLessThan(step);
    await expect.poll(async () => (await by(form, button)).under).toBeGreaterThanOrEqual(0);
    await expect.poll(async () => (await by(form, button)).under).toBeLessThan(step);
    await page.keyboard.press("Escape");
    await expect(form).toBeHidden();
    await expect(button).toBeFocused();
  });

test("from `tablet` a question and a panel stand in the middle of the view, over the dimmed page", async ({
  page,
  open,
}) => {
  await open(story, { width: pc, height: 800 });
  const middle = async (dialog: Locator) => {
    const [box, view] = [await boxOf(dialog), await viewportBox(page)];
    return Math.abs((box.left + box.right) / 2 - view.width / 2);
  };

  await trigger(page, "Open the question").click();
  const question = page.getByRole("alertdialog", { name: "Delete this list?", exact: true });
  await expect(question).toBeVisible();
  await expect.poll(() => middle(question)).toBeLessThan(1);
  await expectInViewport(page, question);
  await expect.poll(async () => (await boxOf(question)).width).toBeLessThan(pc / 2);
  await expect.poll(async () => (await covered(under(page))).length).toBe(1);
  await question.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(question).toBeHidden();

  await trigger(page, "Open the panel").click();
  const panel = page.getByRole("dialog", { name: "Edit history", exact: true });
  await expect(panel).toBeVisible();
  await expect.poll(() => middle(panel)).toBeLessThan(1);
  await expect.poll(async () => (await boxOf(panel)).width).toBeLessThan(pc / 2);
  await expectInViewport(page, panel.getByRole("button", { name: "Close", exact: true }));
  // What it shows is longer than the view: its end comes into view, and the page under it stays where it was.
  const last = panel.getByText(/^Line 30 of the panel\. .+$/);
  await last.scrollIntoViewIfNeeded();
  await expectInViewport(page, last);
  await expectInside(last, panel);
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(0);
});

test("a question keeps its words and its buttons inside it when its title holds a word no line holds", async ({
  page,
  open,
}) => {
  test.fail(
    true,
    "ConfirmDialog: a word no line holds in the title does not wrap; the text and the buttons leave the panel",
  );
  await open(story, { args: { long: true } });
  await trigger(page, "Open the question").click();
  const question = page.getByRole("alertdialog", { name: /^Delete .+\?$/ });
  const at = await boxesAtRest({
    question,
    heading: question.getByRole("heading"),
    confirm: question.getByRole("button", { name: "Delete", exact: true }),
  });
  expect(outside(at.heading, await viewportBox(page))).toEqual([]);
  expect(outside(at.confirm, at.question)).toEqual([]);
});

test("a menu's sheet keeps an item inside it when its label holds a word no line holds", async ({ page, open }) => {
  test.fail(true, "MenuItem: a word no line holds in the label does not wrap and runs out of the sheet at 320 px");
  await open(story, { args: { long: true } });
  await trigger(page, "Menu at the end").click();
  const menu = page.getByRole("menu", { name: "Menu at the end", exact: true });
  const at = await boxesAtRest({ label: menu.getByText(/^Bookmark .+$/) });
  expect(outside(at.label, await viewportBox(page))).toEqual([]);
});
