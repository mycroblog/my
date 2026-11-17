// SPDX-License-Identifier: AGPL-3.0-only

// The frame of the app (shell.tsx): Frame, FrameSide, FrameDock, FrameMain, FrameAside, FrameBanners, FrameActions,
// NavList and SectionList. On a phone the tab bar is docked at the bottom; from `tablet` a rail stands at the start,
// which grows into the sidebar from `desktop`; from `wide` a column stands beside the main one, and the section
// folded above the feed goes. The page scrolls, never a region of the frame.

import { expectNoComputedViolations } from "../axe";
import {
  boxesAtRest,
  boxOf,
  covered,
  expectInside,
  expectInViewport,
  expectMirrored,
  expectNoSidewaysScroll,
  linesOf,
} from "../boxes";
import { expect, from, minimum, pc, test } from "../test";

const story = "design-system-shell--closure-announced";

test("the frame at the minimum width and at each side of every breakpoint", async ({ page, open }) => {
  const navigation = page.getByRole("navigation", { name: "Main", exact: true });
  const main = page.getByRole("main");
  const beside = page.getByRole("complementary", { name: "Server activity", exact: true });
  const fold = main.getByRole("link", { name: "2 unread announcements", exact: true });
  const banners = main.getByRole("region", { name: "Server and connection status", exact: true });
  const home = navigation.getByRole("link", { name: "Home", exact: true });
  const local = navigation.getByRole("link", { name: "Local", exact: true });
  const post = page.getByRole("button", { name: "New post", exact: true });
  const article = page.getByRole("button", { name: "New article", exact: true });
  const entry = main.getByRole("article", { name: "Mika", exact: true });
  const height = 640;
  const at = async (width: number) => {
    await page.setViewportSize({ width, height });
    await expectNoSidewaysScroll(page);
  };

  await open(story, { height });

  await test.step("on a phone: the tab bar docked at the bottom edge to edge, New post floating over it at the end", async () => {
    for (const width of [minimum, from.tablet - 1]) {
      await at(width);
      await expect(beside).toBeHidden();
      await expect(fold).toBeVisible();
      await expect(article).toBeHidden();
      const phone = await boxesAtRest({ navigation, main, home, local, post, banners, entry });
      expect(phone.navigation.left).toBe(0);
      expect(phone.navigation.right).toBe(width);
      expect(phone.navigation.bottom).toBe(height);
      // Its places share one row, in the order of the page.
      expect(phone.local.top).toBe(phone.home.top);
      expect(phone.local.left).toBeGreaterThanOrEqual(phone.home.right);
      expect(phone.post.bottom).toBeLessThanOrEqual(phone.navigation.top);
      expect(phone.post.left).toBeGreaterThan(width / 2);
      // The main column takes the page's width, its notices at its top.
      expect(phone.main.left).toBe(0);
      expect(phone.main.right).toBe(width);
      expect(phone.banners.bottom).toBeLessThanOrEqual(phone.entry.top);
      await expectInside(banners, main);
    }
    await expectNoComputedViolations(page);
  });

  await test.step("from `tablet`: a rail at the start, from the top, the main column beside it", async () => {
    for (const width of [from.tablet, from.desktop - 1]) {
      await at(width);
      await expect(beside).toBeHidden();
      await expect(fold).toBeVisible();
      const rail = await boxesAtRest({ navigation, main, home, local, post, article, banners });
      expect(rail.navigation.top).toBe(0);
      expect(rail.main.left).toBeGreaterThanOrEqual(rail.navigation.right);
      // Its places stand one under another; its actions are inside it, the second under the first.
      expect(rail.local.left).toBe(rail.home.left);
      expect(rail.local.top).toBeGreaterThanOrEqual(rail.home.bottom);
      expect(rail.article.top).toBeGreaterThanOrEqual(rail.post.bottom);
      expect(rail.home.top).toBeGreaterThanOrEqual(rail.article.bottom);
      await expectInside(post, navigation);
      await expectInside(article, navigation);
      await expectInside(banners, main);
    }
  });

  await test.step("from `desktop`: the rail grows into the sidebar, which names its places", async () => {
    await at(from.desktop - 1);
    const rail = await boxesAtRest({ navigation, home });
    for (const width of [from.desktop, from.wide - 1]) {
      await at(width);
      await expect(beside).toBeHidden();
      await expect(fold).toBeVisible();
      const sidebar = await boxesAtRest({ navigation, main, home });
      expect(sidebar.navigation.width).toBeGreaterThan(rail.navigation.width);
      expect(sidebar.home.width).toBeGreaterThan(rail.home.width);
      expect(sidebar.main.left).toBeGreaterThanOrEqual(sidebar.navigation.right);
    }
  });

  await test.step("from `wide`: a column beside the main one, at the end, and no section folded above the feed", async () => {
    for (const width of [from.wide, pc]) {
      await at(width);
      await expect(beside).toBeVisible();
      await expect(fold).toBeHidden();
      const wide = await boxesAtRest({ navigation, main, beside });
      expect(wide.main.left).toBeGreaterThanOrEqual(wide.navigation.right);
      expect(wide.beside.left).toBeGreaterThanOrEqual(wide.main.right);
      expect(wide.beside.right).toBeLessThanOrEqual(width);
    }
  });

  await test.step("a section's lines stand one under another, a long one on two lines at most inside the column", async () => {
    const rooms = beside.getByRole("region", { name: "Voice chat rooms", exact: true });
    const first = rooms.getByRole("link", { name: "Morning coffee chat", exact: true });
    const second = rooms.getByRole("link", { name: "Release party for v0.2", exact: true });
    const long = rooms.getByRole("link", { name: /^A room with a name long enough to wrap .+$/ });
    const lines = await boxesAtRest({ first, second, long });
    expect(lines.second.top).toBeGreaterThanOrEqual(lines.first.bottom);
    expect(lines.long.top).toBeGreaterThanOrEqual(lines.second.bottom);
    expect(lines.second.left).toBe(lines.first.left);
    await expect.poll(() => linesOf(long)).toBe(2);
    await expectInside(long, beside);
  });

  await test.step("no region of the frame scrolls by itself: the page does", async () => {
    // A view shorter than the column beside the main one.
    await page.setViewportSize({ width: pc, height: 320 });
    const scrolls = (region: typeof beside) =>
      region.evaluate((element) => {
        const { overflowY } = getComputedStyle(element);
        return (overflowY === "auto" || overflowY === "scroll") && element.scrollHeight > element.clientHeight;
      });
    await expect
      .poll(async () => [await scrolls(navigation), await scrolls(main), await scrolls(beside)])
      .toEqual([false, false, false]);
    await expect.poll(async () => (await boxOf(beside)).height).toBeGreaterThan(320);
    await expect
      .poll(() => page.evaluate(() => document.documentElement.scrollHeight - document.documentElement.clientHeight))
      .toBeGreaterThan(0);
  });
});

test("right to left, the frame is the mirror of itself: the navigation at the right, the column beside at the left", async ({
  page,
  open,
}) => {
  const parts = {
    navigation: page.getByRole("navigation", { name: "Main", exact: true }),
    main: page.getByRole("main"),
    beside: page.getByRole("complementary", { name: "Server activity", exact: true }),
    banners: page.getByRole("region", { name: "Server and connection status", exact: true }),
    post: page.getByRole("button", { name: "New post", exact: true }),
  };
  await open(story, { width: pc });
  const ltr = await boxesAtRest(parts);
  const { beside: _, ...onAPhone } = parts;
  await page.setViewportSize({ width: minimum, height: 640 });
  const phone = await boxesAtRest(onAPhone);

  await open(story, { width: pc, locale: "ar-XB" });
  await expectNoSidewaysScroll(page);
  await expectMirrored(page, ltr, parts);
  await page.setViewportSize({ width: minimum, height: 640 });
  await expectNoSidewaysScroll(page);
  await expectMirrored(page, phone, onAPhone);
});

test("on a phone the dock stacks a call's bar over the tab bar, and New post over both", async ({ page, open }) => {
  await open("design-system-shell--in-call");
  const tabs = page.getByRole("navigation", { name: "Main", exact: true });
  const call = page.getByRole("group", { name: "Morning coffee chat", exact: true });
  const post = page.getByRole("button", { name: "New post", exact: true });
  const dock = await boxesAtRest({ tabs, call, post });
  expect(dock.call.bottom).toBeLessThanOrEqual(dock.tabs.top);
  expect(dock.post.bottom).toBeLessThanOrEqual(dock.call.top);
  expect(dock.call.left).toBe(0);
  expect(dock.call.right).toBe(minimum);
  await expectInViewport(page, call);
  await expectInside(call.getByRole("button", { name: "Leave", exact: true }), call);
  await expectNoSidewaysScroll(page);

  // From `tablet` the bar ends the navigation.
  await page.setViewportSize({ width: from.tablet, height: 640 });
  await expectInside(call, tabs);
  await expectNoSidewaysScroll(page);
});

test("a toast is over the tab bar on a phone, and in the middle of the bottom edge from `tablet`", async ({
  page,
  open,
}) => {
  await open("design-system-shell--update-ready");
  const tabs = page.getByRole("navigation", { name: "Main", exact: true });
  const toast = page.getByRole("status").filter({ hasText: "A newer version of the app is ready." });
  const post = page.getByRole("button", { name: "New post", exact: true });
  const dock = await boxesAtRest({ tabs, toast, post });
  expect(dock.toast.bottom).toBeLessThanOrEqual(dock.tabs.top);
  expect(dock.toast.bottom).toBeLessThanOrEqual(dock.post.top);
  await expectInViewport(page, toast);
  await expectNoSidewaysScroll(page);

  await page.setViewportSize({ width: pc, height: 640 });
  await expectInViewport(page, toast);
  await expect
    .poll(async () => {
      const box = await boxOf(toast.getByRole("button", { name: "Update", exact: true }));
      return box.bottom > 320 && box.left > pc / 4 && box.right < (3 * pc) / 4;
    })
    .toBe(true);
});

test("on a phone the Menu's sheet holds New article, which the tab bar has no room for, over the page", async ({
  page,
  open,
}) => {
  await open("design-system-shell--home");
  const tabs = page.getByRole("navigation", { name: "Main", exact: true });
  await expect(page.getByRole("button", { name: "New article", exact: true })).toBeHidden();
  await tabs.getByRole("button", { name: /^Menu .+$/ }).click();
  const menu = page.getByRole("menu", { name: "Menu", exact: true });
  const write = menu.getByRole("menuitem", { name: "New article", exact: true });
  await expect(write).toBeVisible();
  await expectInViewport(page, menu);
  await expect.poll(() => covered(write)).toEqual([]);
  // The sheet rises over the tab bar it opened from.
  await expect.poll(async () => (await covered(tabs.getByRole("link", { name: "Home", exact: true }))).length).toBe(1);
  await expectNoSidewaysScroll(page);
});

test("on a phone the press that opens the Menu's sheet does not follow the place the sheet raises under the pointer", async ({
  page,
  open,
}) => {
  await open("design-system-shell--home");
  const at = page.url();
  const button = await boxOf(
    page.getByRole("navigation", { name: "Main", exact: true }).getByRole("button", { name: /^Menu .+$/ }),
  );
  await page.mouse.move((button.left + button.right) / 2, (button.top + button.bottom) / 2);
  await page.mouse.down();
  const menu = page.getByRole("menu", { name: "Menu", exact: true });
  await expect(menu).toBeVisible();
  // The sheet has put a place where the pointer went down; the pointer goes up over one.
  const place = (await boxesAtRest({ profile: menu.getByRole("menuitem", { name: "Profile", exact: true }) })).profile;
  await page.mouse.move((place.left + place.right) / 2, (place.top + place.bottom) / 2);
  await page.mouse.up();
  await expect(menu).toBeVisible();
  await expect(page).toHaveURL(at);
});
