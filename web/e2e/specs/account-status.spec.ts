// SPDX-License-Identifier: AGPL-3.0-only

// An account a moderator restricted in part, on the real server (#3204, #3115): a moderator keeps a person from
// posting; that person, who can still use the app, finds the action in the settings, from the count on the menu's
// row down to the action's own screen, and appeals it there; the server keeps the appeal, which shows after a reload;
// and a post the person sends is refused with the sentence that names the place they appealed at, and a link to it. Both people are
// fresh. What each screen shows of every kind of action and appeal, and what the form says of an appeal it could not
// send, is a component test on the fake (app-features, settings/account-status*.dom.test.tsx).

import { AccountStatus, Frame, Settings } from "./screens";
import { expect, test } from "./test";

test("a person a moderator kept from posting sees the action in the settings and appeals it there; the appeal stays after a reload, and a post they send is refused with where to appeal", async ({
  page,
  signIn,
  fresh,
}) => {
  const moderator = await fresh.person({ admin: true });
  const reader = await fresh.person();
  // The reason, which the reader finds the action by: the reader's own name makes it this test's.
  const reason = `The same link was posted many times by ${reader.username}.`;
  await fresh.moderate(moderator, reader, { kind: "restrict_posting", publicReason: reason, expiresIn: 14 * 86_400 });
  await signIn(reader);

  // The menu counts the action on the entry's row, and the entry says that one is in effect.
  const settings = new Settings(page);
  const status = new AccountStatus(page);
  await new Frame(page).goTo("Settings");
  await expect(settings.heading).toBeVisible();
  await expect(status.counts(status.row(settings.menu))).toHaveText("1 item to check");
  await status.open(settings.menu);
  await expect(status.summary).toBeVisible();

  // Account status lists it with the moderator's reason, and its screen says what it does and why.
  await status.seeStatus();
  await expect(page).toHaveURL("/settings/account/status");
  await expect(status.inEffect(reason)).toHaveCount(1);
  await status.openAction(reason);
  await expect(
    status.section("What this action does").getByText("No posting, replying, boosting, or editing", { exact: true }),
  ).toBeVisible();
  await expect(status.section("The moderators' reason").getByText(reason, { exact: true })).toBeVisible();
  // The menu beside the screen counts it too, until it is appealed.
  await expect(status.counts(status.menus)).toHaveCount(1);

  await status.sendAppeal("I posted the link once, in one thread. Please look at it again.");

  await expect(status.appeal).toHaveCount(0);
  await expect(status.counts(status.menus)).toHaveCount(0);
  await status.backToStatus();
  await expect(status.inEffect(reason)).toHaveText(/^.+Appeal under review$/);

  // The server kept the appeal: a reload reads the same.
  const counted = page.waitForResponse(
    (response) =>
      response.request().method() === "GET" && new URL(response.url()).pathname.endsWith("/moderation/actions"),
  );
  await page.reload();
  await expect(status.heading("Account status")).toBeVisible();
  await expect(status.inEffect(reason)).toHaveText(/^.+Appeal under review$/);
  await counted;
  await expect(status.counts(status.menus)).toHaveCount(0);

  // The action is still in effect: a post is refused, and the refusal names the place the reader was at.
  const composer = await new Frame(page).newPost();
  await composer.write("Hello again");
  const refused = page.waitForResponse(
    (response) => response.request().method() === "POST" && new URL(response.url()).pathname.endsWith("/statuses"),
  );
  await composer.postButton.click();
  const answer = await refused;
  expect(answer.status()).toBe(403);
  const alert = composer.dialog.getByRole("alert");
  await expect(alert).toHaveText(
    "A moderation action keeps your account from posting for now. To see why or to appeal, open “Account status” under “Account and data” in the settings. Account status",
  );
  await expect(alert.getByRole("link", { name: "Account status", exact: true })).toHaveAttribute(
    "href",
    "/settings/account/status",
  );
});
