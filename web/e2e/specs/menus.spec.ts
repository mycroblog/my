// SPDX-License-Identifier: AGPL-3.0-only

// Every menu is one component (#3129): on a phone a sheet along the bottom edge, headed by its name and Close, whose
// submenu takes its place under Back and its title; from a tablet's width a popover by its trigger, with no head. The
// press that opens a sheet over its trigger (the tab bar's Menu) does not run the item it raises under the pointer.

import type { Locator, Page } from "@playwright/test";
import type { Fresh } from "../src/fresh";
import type { Person } from "../src/scenario";
import { Frame, homeTimeline, Menu } from "./screens";
import { expect, person, test } from "./test";

/** A fresh person with a post of their own on their home timeline, signed in; returns the post. */
async function withPost(page: Page, signIn: (p: Person) => Promise<void>, fresh: Fresh) {
  const writer = await fresh.person();
  const { text } = await fresh.post(writer, { text: "A post with a menu." });
  await signIn(writer);
  const post = homeTimeline(page).post(writer.displayName, text);
  await expect(post.article).toBeVisible();
  return post;
}

/** The title of the open menu's head, a heading. */
const heading = (page: Page, title: string): Locator => page.getByRole("heading", { name: title, exact: true });

/**
 * A control of the open menu's head: Back before a submenu's title, Close after the title on a phone. Over a screen
 * without a Close of its own (a timeline), the menu's is the page's only one; over the composer, it is not.
 */
const headControl = (page: Page, control: "Back" | "Close"): Locator =>
  page.getByRole("button", { name: control, exact: true });

/**
 * The items of `menu` that something else is painted over at their centre (by `elementFromPoint`), each with what
 * covers it. Hit testing passes over an inert element (a backdrop that takes no press is one) however it is painted, so
 * every element is hit for the probe and inert again after it: the probe sees the paint order, as the reader's eye does.
 */
const covered = (menu: Locator): Promise<string[]> =>
  menu.getByRole("menuitem").evaluateAll((items) => {
    const inert = [...document.querySelectorAll<HTMLElement>("[inert]")];
    for (const el of inert) el.inert = false;
    try {
      return items.flatMap((item) => {
        const box = item.getBoundingClientRect();
        const top = document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2);
        return top !== null && item.contains(top)
          ? []
          : [`${item.textContent} under ${top?.outerHTML.slice(0, 120) ?? "nothing"}`];
      });
    } finally {
      for (const el of inert) el.inert = true;
    }
  });

/** The width of the viewport, in CSS px. */
const viewportWidth = (page: Page): number => page.viewportSize()?.width ?? 0;
const viewportHeight = (page: Page): number => page.viewportSize()?.height ?? 0;

test.describe("on a phone", () => {
  // The narrowest screen the client supports (ADR-0110 §10).
  test.use({ viewport: { width: 320, height: 640 } });

  test("a post's More menu is a sheet on the bottom edge, headed by its name and Close, which closes it", async ({
    page,
    signIn,
    fresh,
  }) => {
    const post = await withPost(page, signIn, fresh);
    await post.more();
    await expect(post.menu).toBeVisible();
    await expect
      .poll(async () => (await post.menu.boundingBox())?.width ?? 0)
      .toBeGreaterThan(viewportWidth(page) * 0.8);
    await expect
      .poll(async () => {
        const box = await post.menu.boundingBox();
        return box === null ? Infinity : viewportHeight(page) - (box.y + box.height);
      })
      .toBeLessThan(48);
    await expect(heading(page, "More")).toBeVisible();
    await expect(headControl(page, "Back")).toBeHidden();
    await headControl(page, "Close").click();
    await expect(post.menu).toBeHidden();
    await expect(post.article.getByRole("button", { name: "More", exact: true })).toBeFocused();
  });

  test("a post's More sheet is painted above its backdrop: each item is on top at its centre", async ({
    page,
    signIn,
    fresh,
  }) => {
    const post = await withPost(page, signIn, fresh);
    await post.more();
    await expect(post.menu).toBeVisible();
    await expect.poll(() => covered(post.menu)).toEqual([]);
  });

  test("the composer's audience opens a submenu in the same sheet, under Back and its title; Back returns, Escape closes", async ({
    page,
    signIn,
  }) => {
    await signIn(person("yuki"));
    await page.getByRole("button", { name: "New post", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "New post", exact: true });
    const audience = dialog.getByRole("button", { name: /^Who can see it: .+$/ });
    await audience.click();
    const top = page.getByRole("menu", { name: "Who can see and reply", exact: true });
    await expect(top).toBeVisible();
    await top.getByRole("menuitem", { name: /^Who can see it\s*\S.*$/ }).click();
    const sub = page.getByRole("menu", { name: "Who can see it", exact: true });
    // Each audience is a value of one setting: a radio, the chosen one checked.
    await expect(sub.getByRole("menuitemradio", { checked: true })).toHaveCount(1);
    await expect(sub.getByRole("menuitem")).toHaveCount(0);
    await expect(top).toBeHidden();
    await expect(heading(page, "Who can see it")).toBeVisible();
    await headControl(page, "Back").click();
    await expect(top).toBeVisible();
    await expect(sub).toBeHidden();
    await expect(heading(page, "Who can see and reply")).toBeVisible();
    // Escape closes the menu alone, not the composer under it.
    await page.keyboard.press("Escape");
    await expect(top).toBeHidden();
    await expect(dialog).toBeVisible();
    await expect(audience).toBeFocused();
  });

  test("the press that opens the tab bar's Menu does not follow the place the sheet raises under it", async ({
    page,
    signIn,
  }) => {
    await signIn(person("yuki"));
    const frame = new Frame(page);
    const at = page.url();
    const button = await frame.menuButton.boundingBox();
    if (button === null) throw new Error("the tab bar shows no Menu");
    await page.mouse.move(button.x + button.width / 2, button.y + button.height / 2);
    await page.mouse.down();
    const menu = new Menu(page);
    await expect(menu.dialog).toBeVisible();
    const place = await menu.link("Profile").boundingBox();
    if (place === null) throw new Error("the Menu shows no Profile");
    await page.mouse.move(place.x + place.width / 2, place.y + place.height / 2);
    await page.mouse.up();
    await expect(menu.dialog).toBeVisible();
    await expect(page).toHaveURL(at);
  });
});

test.describe("from a tablet's width", () => {
  test("a post's More menu is a popover by its button, with no head, and Escape closes it", async ({
    page,
    signIn,
    fresh,
  }) => {
    const post = await withPost(page, signIn, fresh);
    const more = post.article.getByRole("button", { name: "More", exact: true });
    await post.more();
    await expect(post.menu).toBeVisible();
    await expect
      .poll(async () => (await post.menu.boundingBox())?.width ?? Infinity)
      .toBeLessThan(viewportWidth(page) / 2);
    // By its button: the popover's edge is within a step of the button's, under or over it.
    await expect
      .poll(async () => {
        const [m, b] = [await post.menu.boundingBox(), await more.boundingBox()];
        if (m === null || b === null) return Infinity;
        return Math.min(Math.abs(m.y - (b.y + b.height)), Math.abs(m.y + m.height - b.y));
      })
      .toBeLessThan(32);
    await expect(heading(page, "More")).toBeHidden();
    await expect(page.getByRole("button", { name: "Close", exact: true })).toBeHidden();
    await page.keyboard.press("Escape");
    await expect(post.menu).toBeHidden();
    await expect(more).toBeFocused();
  });

  test("the composer's audience opens its submenu in the popover, headed by Back and its title", async ({
    page,
    signIn,
  }) => {
    await signIn(person("yuki"));
    const composer = await new Frame(page).newPost();
    await composer.dialog.getByRole("button", { name: /^Who can see it: .+$/ }).click();
    const top = page.getByRole("menu", { name: "Who can see and reply", exact: true });
    await expect(top).toBeVisible();
    await expect(heading(page, "Who can see and reply")).toBeHidden();
    await top.getByRole("menuitem", { name: /^Who can see it\s*\S.*$/ }).click();
    const sub = page.getByRole("menu", { name: "Who can see it", exact: true });
    await expect(sub.getByRole("menuitemradio", { checked: true })).toHaveCount(1);
    await expect(heading(page, "Who can see it")).toBeVisible();
    await headControl(page, "Back").click();
    await expect(top).toBeVisible();
    await expect(heading(page, "Who can see and reply")).toBeHidden();
  });
});
