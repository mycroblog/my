// SPDX-License-Identifier: AGPL-3.0-only

// DraftSections and DraftList (drafts.tsx): the sections of the drafts' list one under another, and their rows.

import { expectNoComputedViolations } from "../axe";
import { boxesAtRest, expectInside, expectMirrored, expectNoSidewaysScroll } from "../boxes";
import { expect, test } from "../test";

const story = "design-system-layout-parts--drafts";

test("the sections of the drafts and their rows stand one under another, each row's menu at its end", async ({
  page,
  open,
}) => {
  const scheduled = page.getByRole("list", { name: "Scheduled", exact: true });
  const parts = {
    scheduledHeading: page.getByRole("heading", { name: "Scheduled 2", exact: true }),
    scheduled,
    first: scheduled.getByRole("button", { name: /^Open the first scheduled post .+$/ }),
    firstMenu: scheduled.getByRole("button", { name: "More for the first scheduled post", exact: true }),
    second: scheduled.getByRole("button", { name: /^Open the second scheduled post .+$/ }),
    draftsHeading: page.getByRole("heading", { name: "Drafts 1", exact: true }),
    drafts: page.getByRole("list", { name: "Drafts", exact: true }),
  };

  await open(story);
  // A row's text holds a word no line holds.
  await expectNoSidewaysScroll(page);
  const ltr = await boxesAtRest(parts);
  expect(ltr.scheduled.top).toBeGreaterThanOrEqual(ltr.scheduledHeading.bottom);
  expect(ltr.second.top).toBeGreaterThanOrEqual(ltr.first.bottom);
  expect(ltr.draftsHeading.top).toBeGreaterThanOrEqual(ltr.scheduled.bottom);
  expect(ltr.drafts.top).toBeGreaterThanOrEqual(ltr.draftsHeading.bottom);
  // The sections stand further apart than a section's rows.
  expect(ltr.draftsHeading.top - ltr.scheduled.bottom).toBeGreaterThan(ltr.second.top - ltr.first.bottom);
  expect(ltr.drafts.left).toBe(ltr.scheduled.left);
  expect(ltr.drafts.right).toBe(ltr.scheduled.right);
  await expectInside(parts.first, page);
  await expectInside(parts.firstMenu, page);
  expect(ltr.firstMenu.left).toBeGreaterThan((ltr.scheduled.left + ltr.scheduled.right) / 2);
  await expectNoComputedViolations(page);

  await open(story, { locale: "ar-XB" });
  await expectNoSidewaysScroll(page);
  await expectMirrored(page, ltr, parts);
});
