// SPDX-License-Identifier: AGPL-3.0-only

// ChipRow (chip.tsx): a row of chips that wraps; and a chip link whose word no line holds, which breaks inside it.

import { expectNoComputedViolations } from "../axe";
import { boxesAtRest, expectInside, expectMirrored, expectNoSidewaysScroll } from "../boxes";
import { expect, test } from "../test";

const story = "design-system-layout-parts--chip-row-wrapping";

test("more chips than a row holds wrap onto further rows inside it, from the start side in either direction, and a chip link's word no line holds breaks inside it", async ({
  page,
  open,
}) => {
  const row = page.getByRole("group", { name: "Reactions", exact: true });
  const chip = (name: string) => row.getByRole("button", { name, exact: true });
  const parts = { row, first: chip("🍞 1"), second: chip("🌊 2"), last: chip("🪴 14") };

  await open(story);
  await expectNoSidewaysScroll(page);
  await expectInside(parts.last, row);
  const ltr = await boxesAtRest(parts);
  // The first two share the first row, which starts at the row's start; the last is on a row below it.
  expect(Math.abs(ltr.first.left - ltr.row.left)).toBeLessThan(1);
  expect(ltr.second.top).toBe(ltr.first.top);
  expect(ltr.second.left).toBeGreaterThan(ltr.first.right);
  expect(ltr.last.top).toBeGreaterThanOrEqual(ltr.first.bottom);
  await expectNoComputedViolations(page);

  await open(story, { locale: "ar-XB" });
  await expectNoSidewaysScroll(page);
  await expectMirrored(page, ltr, parts);

  // The same page opens the chip links' story.
  await test.step("a chip link whose name is a word no line holds stays inside its list, the word broken inside the chip, in both directions", async () => {
    const list = page.getByRole("list");
    const long = list.getByRole("link", { name: /^#Donaudampfschifffahrts\S+ 24$/ });
    for (const locale of ["en", "ar-XB"] as const) {
      await open("design-system-links--chips", { locale });
      await expectNoSidewaysScroll(page);
      await expectInside(long, list);
      await expectInside(long.getByText(/^#Donaudampfschifffahrts\S+$/), long);
      await expectInside(long.getByText("24", { exact: true }), long);
    }
  });
});
