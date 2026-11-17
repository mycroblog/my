// SPDX-License-Identifier: AGPL-3.0-only

// The parts that lay out a screen's lists (list.tsx): ScreenStack, ScreenHead, RowList, RowItem and ButtonPair, with
// what only a layout engine decides of the rows they hold and of a screen's title: a long name is cut short or breaks
// where it may, what ends a row stays at its end, and a row's onward glyph turns with the page.

import { expectNoComputedViolations } from "../axe";
import {
  boxesAtRest,
  cutShort,
  expectInside,
  expectMirrored,
  expectNoSidewaysScroll,
  glyphsOf,
  textLines,
} from "../boxes";
import { expect, test } from "../test";

const name = "Bartholomew-Maximilian-Featherstonehaugh-Cholmondeley-Warner";

test("a screen's head, its rows and a pair of buttons at the minimum width, in both directions", async ({
  page,
  open,
}) => {
  const story = "design-system-layout-parts--screen-rows";
  const people = page.getByRole("list", { name: "People", exact: true });
  const row = (link: string | RegExp) =>
    people.getByRole("listitem").filter({ has: page.getByRole("link", { name: link, exact: true }) });
  const parts = {
    back: page.getByRole("button", { name: "Held-back notifications", exact: true }),
    heading: page.getByRole("heading", { name, exact: true }),
    accept: page.getByRole("button", { name: "Accept", exact: true }),
    people,
    requests: row(/^Follow requests .+$/),
    layla: row(/^Layla .+$/),
    sam: row("Sam"),
    follow: row("Sam").getByRole("button", { name: "Follow", exact: true }),
    refuse: page.getByRole("button", { name: "Not now", exact: true }),
    confirm: page.getByRole("button", { name: "Accept every request", exact: true }),
  };

  await open(story);
  await expectNoSidewaysScroll(page);
  const ltr = await boxesAtRest(parts);

  await test.step("the head: the way back over the account's row, whose long name is cut short", async () => {
    expect(ltr.back.bottom).toBeLessThanOrEqual(ltr.heading.top);
    await expect.poll(() => cutShort(parts.heading.getByText(name, { exact: true }))).toBe(true);
    await expectInside(parts.accept, page);
  });

  await test.step("the rows: one under another at the list's width, what ends a row at its end", async () => {
    for (const item of [ltr.requests, ltr.layla, ltr.sam]) {
      expect(item.left).toBe(ltr.people.left);
      expect(item.right).toBe(ltr.people.right);
    }
    expect(ltr.layla.top).toBeGreaterThanOrEqual(ltr.requests.bottom);
    expect(ltr.sam.top).toBeGreaterThanOrEqual(ltr.layla.bottom);
    expect(ltr.people.right - ltr.follow.right).toBeLessThan(16);
    await expect.poll(() => cutShort(parts.layla.getByRole("link").getByText(/^Layla .+$/))).toBe(true);
  });

  await test.step("the pair: the refusal beside the confirmation, which is at the end", () => {
    expect(ltr.refuse.right).toBeLessThanOrEqual(ltr.confirm.left);
    expect(ltr.confirm.top).toBe(ltr.refuse.top);
    expect(ltr.people.right - ltr.confirm.right).toBeLessThan(16);
  });

  await test.step("a row that opens elsewhere: its glyph at the start, the way onward at the end", async () => {
    const glyphs = await glyphsOf(parts.requests);
    expect(glyphs).toHaveLength(2);
    const [glyph, onward] = glyphs;
    expect(glyph?.right).toBeLessThan(onward?.left ?? 0);
    expect(glyphs.map((g) => g.mirrored)).toEqual([false, false]);
  });

  await expectNoComputedViolations(page);

  await open(story, { locale: "ar-XB" });
  await expectNoSidewaysScroll(page);
  await expectMirrored(page, ltr, parts);
  // The way onward points the other way; the glyph of what the row opens is no direction.
  await expect.poll(async () => (await glyphsOf(parts.requests)).map((g) => g.mirrored)).toEqual([false, true]);
});

test("a screen's title is one row: a long name breaks between its words, and its action ends the row", async ({
  page,
  open,
}) => {
  const story = "design-system-layout-parts--long-screen-title";
  const title = "#TheLongestHashtagSomebodyEverWroteAboutSlowMorningsByTheSea2026";
  const heading = page.getByRole("heading", { name: `${title} 1,234 posts`, exact: true });
  const parts = {
    back: page.getByRole("button", { name: "Back", exact: true }),
    heading,
    follow: page.getByRole("button", { name: "Follow", exact: true }),
  };

  await open(story);
  await expectNoSidewaysScroll(page);
  const ltr = await boxesAtRest(parts);
  expect(ltr.back.bottom).toBeLessThanOrEqual(ltr.heading.top);
  // The action is beside the name, at the row's end, not under it.
  expect(ltr.follow.left).toBeGreaterThanOrEqual(ltr.heading.right);
  expect(ltr.follow.top).toBeLessThan(ltr.heading.bottom);
  expect(ltr.follow.bottom).toBeGreaterThan(ltr.heading.top);
  await expectInside(parts.follow, page);
  // The name takes several lines, each after the first starting at a capital or a figure: no word of small letters
  // is broken.
  const name = heading.getByText(title, { exact: true });
  await expect.poll(async () => (await textLines(name)).length).toBeGreaterThan(1);
  await expect.poll(async () => (await textLines(name)).join("")).toBe(title);
  await expect.poll(async () => (await textLines(name)).slice(1).filter((line) => !/^[A-Z0-9]/.test(line))).toEqual([]);
  await expectNoComputedViolations(page);

  await open(story, { locale: "ar-XB" });
  await expectNoSidewaysScroll(page);
  await expectMirrored(page, ltr, { back: parts.back, follow: parts.follow });
});

test("the glyphs that are directions turn with the page, and no other does", async ({ page, open }) => {
  await open("design-system-glyphs--all", { locale: "ar-XB" });
  // Each row of the sheet: the glyph's name, the glyph, and what it is, which says whether it flips.
  const rows = () =>
    page.getByRole("code").evaluateAll((names) =>
      names.map((name) => {
        const row = name.parentElement;
        const drawn = [...(row?.querySelectorAll("svg") ?? [])].map((svg) => (svg.getScreenCTM()?.a ?? 1) < 0);
        return {
          name: name.textContent,
          flips: row?.textContent.includes("flips in RTL") ?? false,
          mirrored: drawn.length > 0 && drawn.every(Boolean),
          straight: drawn.every((m) => !m),
        };
      }),
    );
  await expect.poll(async () => (await rows()).length).toBeGreaterThan(100);
  await expect.poll(async () => (await rows()).filter((r) => (r.flips ? !r.mirrored : !r.straight))).toEqual([]);
  await expect.poll(async () => (await rows()).filter((r) => r.flips).length).toBeGreaterThan(5);
});
