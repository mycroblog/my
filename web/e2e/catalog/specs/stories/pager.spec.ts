// SPDX-License-Identifier: AGPL-3.0-only

// The pager (web/design-system/src/pager.tsx, #3279) where a press disables the button it was on, in a browser: the
// browser drops the focus of a button that turns disabled to the page's body, and the pager hands it to the other way
// instead. On the stories of the app that page on the fake (web/app/src/operations.stories.tsx, finance.stories.tsx):
// the error groups' second page, one a page and the last of two, where Previous reaches the first page and Next the
// last; and the public finance's window ending this month, where Later comes back to it. A simulation cannot tell
// whether the focus fell (pager.dom.test.tsx drops it by hand); this is that a browser keeps it on the pager.

import { expect, pc, test } from "../test";

test("Previous to the first page and Next to the last leave the focus on the other way, not the page", async ({
  page,
  open,
}) => {
  await open("screens-operations--errors-second-page", { width: pc });
  const pages = page.getByRole("main").getByRole("navigation", { name: "Pages", exact: true });
  const status = pages.getByRole("status");
  const previous = pages.getByRole("button", { name: "Previous", exact: true });
  const next = pages.getByRole("button", { name: "Next", exact: true });
  await expect(status).toHaveText("Page 2");
  await expect(next).toBeDisabled();

  await previous.click();
  await expect(status).toHaveText("Page 1");
  await expect(previous).toBeDisabled();
  await expect(next).toBeFocused();

  await next.click();
  await expect(status).toHaveText("Page 2");
  await expect(next).toBeDisabled();
  await expect(previous).toBeFocused();
});

test("Later back to the window ending this month leaves the focus on Earlier, not the page", async ({ page, open }) => {
  await open("screens-finance--public-totals", { width: pc });
  const pages = page.getByRole("main").getByRole("navigation", { name: "Pages", exact: true });
  const later = pages.getByRole("button", { name: "Later months", exact: true });
  const earlier = pages.getByRole("button", { name: "Earlier months", exact: true });
  await expect(later).toBeDisabled();
  const current = pages.getByRole("status");
  await expect(current).toHaveText(/^Showing .+ to .+\.$/);

  await earlier.click();
  await expect(later).toBeEnabled();
  await later.click();
  await expect(later).toBeDisabled();
  await expect(earlier).toBeFocused();
});
