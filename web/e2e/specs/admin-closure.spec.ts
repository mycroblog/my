// SPDX-License-Identifier: AGPL-3.0-only

// The closure of the service on the real server (#3266, FR-808): an administrator announces the closure with its date
// and a message, every screen shows its notice, and they withdraw it. An announcement closes the registration, rejects
// the sign-ups that wait and ends the supporter subscriptions of the whole server, so this spec runs in the project of
// the specs that change the whole server, after every other spec and one test at a time (playwright.config.ts), and
// withdraws what it announced. Closing the server is never done here: it would end the server for every spec after
// it; the check, the typed domain and the progress are the stories' and the route tests' on the fake.

import { randomUUID } from "node:crypto";
import { ClosureAdmin } from "./screens";
import { expect, test } from "./test";

/** The closing date the spec announces, far ahead: the announcement is withdrawn before any spec reads it. */
const closesAt = "2099-12-31T12:00";

test("an administrator announces the closure of the service, sees its notice, and withdraws it", async ({
  page,
  signIn,
  fresh,
}) => {
  const admin = await fresh.person({ admin: true });
  await signIn(admin);
  const closure = new ClosureAdmin(page);
  const message = `Thank you for the years together ${randomUUID().slice(0, 8)}.`;
  try {
    await closure.open();

    await closure.openAnnouncement();
    await closure.closes.fill(closesAt);
    await closure.message.fill(message);
    await closure.announce();
    await expect(closure.announcement.getByText(message, { exact: true })).toBeVisible();
    await expect(closure.banner).toBeVisible();

    await closure.withdraw();
    await expect(page.getByRole("main").getByText("No closure is announced.", { exact: true })).toBeVisible();
    await expect(closure.banner).toBeHidden();
  } finally {
    await fresh.closureWithdrawn(admin);
  }
});
