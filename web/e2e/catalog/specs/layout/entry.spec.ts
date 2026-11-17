// SPDX-License-Identifier: AGPL-3.0-only

// The parts of an entry (entry.tsx): Stack and ActionRow, which place, and what only a layout engine decides of the
// byline and of a one-line caption: a long name is cut short on its one line, and what ends the line keeps its room.

import { expectNoComputedViolations } from "../axe";
import {
  atRest,
  boxesAtRest,
  cutShort,
  expectInside,
  expectMirrored,
  expectNoSidewaysScroll,
  linesOf,
  outside,
  sidewaysScroll,
} from "../boxes";
import { expect, test } from "../test";

const story = "design-system-layout-parts--entry-parts";
const name = "Bartholomew-Maximilian-Featherstonehaugh-Cholmondeley-Warner";
const booster = "Wilhelmina-Alexandrina-Marjoribanks-Featherstonehaugh-Cholmondeley";

test("an entry's parts at the minimum width: a stack, the byline, a one-line caption and the row of actions", async ({
  page,
  open,
}) => {
  const entry = page.getByRole("article", { name, exact: true });
  const parts = {
    first: entry.getByText(/^First part: .+$/),
    second: entry.getByText(/^Second part: .+$/),
    line: entry.getByText("First line of the third part.", { exact: true }),
    next: entry.getByText("Second line of the third part.", { exact: true }),
    reply: entry.getByRole("button", { name: /^Reply, .+$/ }),
    more: entry.getByRole("button", { name: "More", exact: true }),
  };
  await open(story);
  await expectNoSidewaysScroll(page);
  const ltr = await boxesAtRest(parts);

  await test.step("a stack lays its parts one under another at the full width, a line's gap closer than a part's", () => {
    expect(ltr.second.left).toBe(ltr.first.left);
    expect(ltr.second.right).toBe(ltr.first.right);
    expect(ltr.line.right).toBe(ltr.first.right);
    const part = ltr.second.top - ltr.first.bottom;
    const line = ltr.next.top - ltr.line.bottom;
    expect(part).toBeGreaterThan(0);
    expect(ltr.line.top - ltr.second.bottom).toBe(part);
    expect(line).toBeGreaterThanOrEqual(0);
    expect(line).toBeLessThan(part);
  });

  await test.step("the byline cuts a long name and a long handle short on their lines, and the stamp keeps its room", async () => {
    const byName = entry.getByRole("link", { name, exact: true });
    const handle = entry.getByText(/^@bartholomew_maximilian_featherstonehaugh@.+$/);
    const time = entry.getByText("1h ago", { exact: true });
    await expect.poll(() => linesOf(byName)).toBe(1);
    await expect.poll(() => cutShort(byName.getByText(name, { exact: true }))).toBe(true);
    await expect.poll(() => cutShort(handle)).toBe(true);
    await expectInside(byName, entry);
    await expectInside(handle, entry);
    await expectInside(time, entry);
    // The time is whole, on the name's line after the name; the handle is under them.
    await expect.poll(() => cutShort(time)).toBe(false);
    const at = await boxesAtRest({ byName, handle, time });
    expect(at.time.left).toBeGreaterThanOrEqual(at.byName.right);
    expect(at.time.top).toBeLessThan(at.byName.bottom);
    expect(at.handle.top).toBeGreaterThanOrEqual(at.byName.bottom - 1);
  });

  await test.step("a one-line caption stays on one line: its name is cut short, the words after it stay whole", async () => {
    const caption = entry.getByText(new RegExp(`^${booster} boosted$`));
    const who = entry.getByText(booster, { exact: true });
    await expect.poll(() => linesOf(caption)).toBe(1);
    await expect.poll(() => cutShort(who)).toBe(true);
    await expectInside(caption, entry);
    const at = await boxesAtRest({ caption, who });
    expect(at.who.right).toBeLessThan(at.caption.right);
  });

  await test.step("the row of actions is one row from the start, with More at its end", async () => {
    expect(ltr.more.top).toBe(ltr.reply.top);
    expect(ltr.reply.left).toBeLessThanOrEqual(ltr.first.left);
    expect(ltr.more.right).toBeGreaterThanOrEqual(ltr.first.right);
    await expectInside(parts.reply, entry);
    await expectInside(parts.more, entry);
  });

  await expectNoComputedViolations(page);

  // Right to left, the parts whose size no word decides are where a mirror puts them.
  await open(story, { locale: "ar-XB" });
  await expectMirrored(page, ltr, {
    first: parts.first,
    second: parts.second,
    line: parts.line,
    next: parts.next,
  });
});

test("the row of actions keeps inside the entry right to left, where large counts read longer", async ({
  page,
  open,
}) => {
  await open(story, { locale: "ar-XB", args: { counts: "large" } });
  const entry = page.getByRole("article", { name, exact: true });
  const at = await boxesAtRest({ entry, more: entry.getByRole("button", { name: "More", exact: true }) });
  expect(outside(at.more, at.entry)).toEqual([]);
  const overflow = await atRest(() => sidewaysScroll(page));
  expect(overflow).toEqual([]);
});
