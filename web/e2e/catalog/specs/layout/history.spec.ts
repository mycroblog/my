// SPDX-License-Identifier: AGPL-3.0-only

// VersionList (history.tsx): the versions of a post one under another.

import { expectNoComputedViolations } from "../axe";
import { boxesAtRest, expectMirrored, expectNoSidewaysScroll } from "../boxes";
import { expect, test } from "../test";

const story = "design-system-layout-parts--versions";

test("the versions stand one under another at the list's width, and hold a word no line holds", async ({
  page,
  open,
}) => {
  const versions = page.getByRole("list", { name: "Versions", exact: true });
  const version = (label: string) =>
    versions.getByRole("listitem").filter({ has: page.getByText(label, { exact: true }) });
  const parts = {
    versions,
    current: version("Current"),
    edit: version("Edit"),
    original: version("Original"),
    text: versions.getByText(/^Original: .+$/),
  };

  await open(story);
  await expectNoSidewaysScroll(page);
  const ltr = await boxesAtRest(parts);
  for (const item of [ltr.current, ltr.edit, ltr.original]) {
    expect(item.left).toBe(ltr.versions.left);
    expect(item.right).toBe(ltr.versions.right);
  }
  expect(ltr.edit.top).toBeGreaterThanOrEqual(ltr.current.bottom);
  expect(ltr.original.top).toBeGreaterThanOrEqual(ltr.edit.bottom);
  expect(ltr.text.right).toBeLessThanOrEqual(ltr.versions.right);
  await expectNoComputedViolations(page);

  await open(story, { locale: "ar-XB" });
  await expectNoSidewaysScroll(page);
  await expectMirrored(page, ltr, parts);
});
