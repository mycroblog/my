// SPDX-License-Identifier: AGPL-3.0-only

// An announcement from its writing to its readers, on the real server (#3263, FR-726, ADR-0125): an administrator finds
// Announcements through Admin in the navigation and its list of screens, writes one with a title of its own and
// publishes it, answering the question; a person then finds it in the server's announcements and opens it. Every local
// user is notified of it and sees it above their home timeline, so the administrator withdraws it from its page, and
// it is withdrawn by its title at the end whatever happened (a window that ends soon would read the clock, which the
// specs do not); nothing here counts what other specs could change. What each screen shows in every state is the
// stories' and the route tests' on the fake (web/app, admin-announcements.stories.tsx).

import { randomUUID } from "node:crypto";
import type { Page } from "@playwright/test";
import { AnnouncementsAdmin, Frame, SignIn } from "./screens";
import { expect, test, type FreshPerson } from "./test";

/** Signs in as `p` on `page`, a browser of its own, and waits for the home screen. */
async function signInOn(page: Page, p: FreshPerson): Promise<void> {
  const screen = new SignIn(page);
  await screen.open();
  await screen.withPassword(p.email, p.password);
  await expect(new Frame(page).heading("Home")).toBeVisible();
}

test("an administrator publishes an announcement, and a person reads it among the server's announcements", async ({
  page,
  browser,
  baseURL,
  signIn,
  fresh,
}) => {
  const admin = await fresh.person({ admin: true });
  const reader = await fresh.person();
  const title = `Maintenance notice ${randomUUID().slice(0, 8)}`;
  const text = "The server restarts tonight for a few minutes.";

  // The administrator, in a browser of their own.
  const theirs = await browser.newContext({
    ...(baseURL === undefined ? {} : { baseURL }),
    extraHTTPHeaders: { "X-Forwarded-For": "198.19.252.1" },
  });
  const staff = await theirs.newPage();
  const screen = new AnnouncementsAdmin(staff);
  try {
    await signInOn(staff, admin);
    await screen.place.click();
    await screen.entry.click();
    await screen.write();
    await screen.fill({ title, text });
    await screen.publish();
    await expect(staff).toHaveURL(/\/admin\/announcements\/[^/]+$/);
    const published = staff.url();
    await expect(screen.heading(title)).toBeVisible();

    // The person finds it among the server's announcements, and opens it.
    await signIn(reader);
    await page.goto("/announcements");
    await expect(screen.listedOn(page, title)).toBeVisible();
    await screen.listedOn(page, title).click();
    await expect(page.getByRole("main").getByText(text)).toBeVisible();
    // The administrator withdraws it from its page.
    await staff.goto(published);
    await screen.withdraw();
    await expect(screen.said("Withdrawn")).toBeVisible();
  } finally {
    // Withdrawn whatever happened, found by its title, so that no other spec's reader keeps seeing it.
    await fresh.withdrawAnnouncement(title);
    await theirs.close();
  }
});
