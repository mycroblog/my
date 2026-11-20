// SPDX-License-Identifier: AGPL-3.0-only

// A domain suspended and read in the audit log, on the real server (#3261, FR-702, FR-704, FR-717): a moderator opens
// Domain actions through Moderation in the navigation and its list of screens, fills in a new action on a domain,
// and is asked, before the suspend is taken, how many follows it ends and to type the domain; the action's page then
// says it is in effect. Its Audit log opens the operations on the domain, which say that the moderator took an
// action on it, the domain linked; the domain's link opens its server's screen. What each screen shows in every state
// is the stories' and the route tests' on the fake (web/app, domains.stories.tsx).

import { randomUUID } from "node:crypto";
import { Frame, Moderation } from "./screens";
import { StaffDomains } from "./screens/staff-domains";
import { expect, test } from "./test";

test("a moderator suspends a domain after reading what it ends, and reads it in the audit log", async ({
  page,
  signIn,
  fresh,
}) => {
  const moderator = await fresh.person({ admin: true });
  const domain = `spam-${randomUUID().slice(0, 8)}.example`;
  await signIn(moderator);

  const moderation = new Moderation(page);
  const domains = new StaffDomains(page);
  await moderation.place.click();
  await domains.domainActionsLink.click();
  await expect(new Frame(page).heading("Domain actions")).toBeVisible();

  // The suspend asks first: what it stops, the follows it ends as the server counts them, and the domain to type.
  await domains.newAction();
  await domains.fill(domain, "Spam from every account.");
  await domains.suspend();
  const confirmation = domains.confirmation(domain);
  await expect(confirmation).toBeVisible();
  await expect(confirmation).toContainText("No follow between people here and its accounts ends.");
  await expect(confirmation.getByRole("button", { name: "Suspend", exact: true })).toBeDisabled();
  await domains.confirmSuspend(domain);

  await expect(domains.actionHeading(domain)).toBeVisible();
  await expect(domains.actionFacts).toContainText("In effect");
  await expect(domains.actionFacts).toContainText("Suspend");

  // The audit log, opened from the action's page on the operations on its domain, says who took it.
  await domains.auditLogOfDomain();
  const entry = domains.entry(`@${moderator.username} took an action on the domain ${domain}`);
  await expect(entry).toBeVisible();
  await entry.getByRole("link", { name: domain, exact: true }).click();
  await expect(new Frame(page).heading(domain)).toBeVisible();
});
