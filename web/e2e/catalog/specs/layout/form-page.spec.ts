// SPDX-License-Identifier: AGPL-3.0-only

// The frame of a page of its own and the parts that lay out its screen (form-page.tsx): FormPage, Form, FormActions,
// FormChoice, FormLinks and FormList. Under `desktop` the page is one column under the server's mark; from `desktop`
// the server's introduction stands beside it and stays in view; its front page shows the introduction at every width.

import { expectNoComputedViolations } from "../axe";
import { boxesAtRest, boxOf, expectInside, expectMirrored, expectNoSidewaysScroll, viewportBox } from "../boxes";
import { expect, from, minimum, test } from "../test";

const story = "design-system-layout-parts--form-page-screen";
const server = "A server by the sea";

test("a page of its own at the minimum width and at each side of `desktop`, in both directions", async ({
  page,
  open,
}) => {
  const mark = page.getByRole("banner");
  const introduction = page.getByRole("complementary", { name: server, exact: true });
  const main = page.getByRole("main");
  const footer = page.getByRole("contentinfo");
  const parts = {
    main,
    email: main.getByRole("textbox", { name: "Email address", exact: true }),
    request: main.getByRole("button", { name: "Request an export", exact: true }),
    download: main.getByRole("button", { name: "Download your follows as CSV", exact: true }),
    deny: main.getByRole("button", { name: "Deny", exact: true }),
    allow: main.getByRole("button", { name: "Allow", exact: true }),
    firstLink: main.getByRole("link", { name: "Sign in with another account", exact: true }),
    secondLink: main.getByRole("link", { name: "Forgot your password?", exact: true }),
    firstItem: main.getByRole("listitem").filter({ hasText: /^Read your posts$/ }),
    longItem: main.getByRole("listitem").filter({ hasText: /^Write posts as you\. .+$/ }),
  };

  await open(story);
  await expectNoSidewaysScroll(page);
  const phone = await boxesAtRest(parts);

  await test.step("under `desktop`: the server's mark over the one column, no introduction beside it", async () => {
    await expect(mark).toBeVisible();
    await expect(introduction).toBeHidden();
    await expect.poll(async () => (await boxOf(mark)).bottom - (await boxOf(main)).top).toBeLessThanOrEqual(0);
  });

  await test.step("the actions: a column, each as wide as the fields", () => {
    expect(phone.download.top).toBeGreaterThanOrEqual(phone.request.bottom);
    for (const action of [phone.request, phone.download]) {
      expect(action.left).toBe(phone.email.left);
      expect(action.right).toBe(phone.email.right);
    }
  });

  await test.step("a choice: the refusal at the start beside the confirmation, each half the column", () => {
    expect(phone.allow.top).toBe(phone.deny.top);
    expect(phone.deny.left).toBe(phone.email.left);
    expect(phone.allow.right).toBe(phone.email.right);
    expect(phone.deny.right).toBeLessThanOrEqual(phone.allow.left);
    expect(Math.abs(phone.deny.width - phone.allow.width)).toBeLessThan(1);
  });

  await test.step("the links and a list's items: one under another, a long item wrapping inside the column", async () => {
    expect(phone.secondLink.top).toBeGreaterThanOrEqual(phone.firstLink.bottom);
    expect(phone.longItem.top).toBeGreaterThanOrEqual(phone.firstItem.bottom);
    expect(phone.longItem.height).toBeGreaterThan(phone.firstItem.height);
    await expectInside(parts.longItem, main);
  });

  await test.step("the footer ends the page, also when the screen is shorter than the viewport", async () => {
    await expect.poll(async () => (await boxOf(footer)).top - (await boxOf(main)).bottom).toBeGreaterThanOrEqual(0);
    await page.setViewportSize({ width: minimum, height: 2400 });
    await expect.poll(async () => (await viewportBox(page)).bottom - (await boxOf(footer)).bottom).toBeLessThan(48);
    await expect
      .poll(async () => (await boxOf(footer)).top - (await boxOf(parts.secondLink)).bottom)
      .toBeGreaterThan(400);
  });
  await expectNoComputedViolations(page);

  await test.step("one px under `desktop` the page is still one column", async () => {
    await page.setViewportSize({ width: from.desktop - 1, height: 640 });
    await expect(mark).toBeVisible();
    await expect(introduction).toBeHidden();
    await expectNoSidewaysScroll(page);
  });

  const wide = { introduction, main, deny: parts.deny, allow: parts.allow };
  await page.setViewportSize({ width: from.desktop, height: 640 });
  await expect(introduction).toBeVisible();
  const ltr = await boxesAtRest(wide);

  await test.step("from `desktop`: the introduction at the start beside the column, as tall as the view, in view while the page scrolls", async () => {
    await expect(mark).toBeHidden();
    await expectNoSidewaysScroll(page);
    expect(ltr.introduction.left).toBe(0);
    expect(ltr.introduction.top).toBe(0);
    expect(ltr.introduction.height).toBe(640);
    expect(ltr.main.left).toBeGreaterThanOrEqual(ltr.introduction.right);
    await footer.scrollIntoViewIfNeeded();
    await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThan(200);
    await expect.poll(async () => (await boxOf(introduction)).top).toBe(0);
    await expect.poll(async () => (await boxOf(introduction)).bottom).toBe(640);
    await page.evaluate(() => {
      window.scrollTo(0, 0);
    });
  });

  await open(story, { locale: "ar-XB", width: from.desktop });
  await expectNoSidewaysScroll(page);
  await expectMirrored(page, ltr, wide);
  await page.setViewportSize({ width: minimum, height: 640 });
  await expectNoSidewaysScroll(page);
  await expectMirrored(page, phone, { deny: parts.deny, allow: parts.allow, request: parts.request });
});

test("with no banner, the introduction sits in the middle of its side", async ({ page, open }) => {
  await open(story, { width: from.desktop, args: { banner: false } });
  const introduction = page.getByRole("complementary", { name: server, exact: true });
  const name = introduction.getByText(server, { exact: true });
  const description = introduction.getByText(/^A small server for people who like the sea.+$/);
  await expect
    .poll(async () => {
      const [side, first, last] = [await boxOf(introduction), await boxOf(name), await boxOf(description)];
      return Math.abs((first.top + last.bottom) / 2 - (side.top + side.bottom) / 2);
    })
    .toBeLessThan(24);
});

test("the server's front page shows its introduction at every width, ended by its link onward", async ({
  page,
  open,
}) => {
  await open(story, { args: { front: true } });
  const introduction = page.getByRole("banner");
  const name = introduction.getByRole("heading", { level: 1, name: server, exact: true });
  const more = introduction.getByRole("link", { name: "About this server", exact: true });
  const main = page.getByRole("main");
  for (const width of [minimum, from.desktop - 1, from.desktop]) {
    await page.setViewportSize({ width, height: 640 });
    await expect(name).toBeVisible();
    await expect(more).toBeVisible();
    await expect(page.getByRole("complementary")).toHaveCount(0);
    await expectNoSidewaysScroll(page);
    await expect.poll(async () => (await boxOf(more)).top - (await boxOf(name)).bottom).toBeGreaterThanOrEqual(0);
  }
  // On a phone the introduction is over the screen; from `desktop` it is beside it.
  await page.setViewportSize({ width: minimum, height: 640 });
  await expect.poll(async () => (await boxOf(main)).top - (await boxOf(more)).bottom).toBeGreaterThanOrEqual(0);
  await page.setViewportSize({ width: from.desktop, height: 640 });
  await expect.poll(async () => (await boxOf(main)).left - (await boxOf(introduction)).right).toBeGreaterThanOrEqual(0);
});

test("a page keeps its width when a name in it is a word no line holds", async ({ page, open }) => {
  test.fail(true, "FormHead, FormList, FormLink: a word no line holds does not wrap and widens the page at 320 px");
  await open(story, { args: { long: "word" } });
  await expectNoSidewaysScroll(page);
});

test("a page keeps its width when an action's label is longer than a line", async ({ page, open }) => {
  test.fail(true, "FormActions: a button's label longer than the column does not wrap and widens the page at 320 px");
  await open(story, { args: { long: "label" } });
  await expectNoSidewaysScroll(page);
});
