// SPDX-License-Identifier: AGPL-3.0-only

// The supporter's side on the real server with its fake payment provider (#3245, FR-911, FR-918): a fresh person
// chooses a plan on the supporters' page, pays on the provider's checkout and comes back to their subscription in the
// settings; switches to the higher plan, which applies at once; cancels it, its perks kept to the period's end; and
// tips the server once, back with thanks. Each step is the server's: the subscription and the tip are read from it.
// What the screens show in each state is the stories' and the route tests' on the fake (web/app,
// supporters.stories.tsx, supporters-routes.dom.test.tsx).

import type { Page } from "@playwright/test";
import { expect, test } from "./test";

/** What a live region of the page says, whole. */
const said = (page: Page, sentence: string | RegExp) =>
  typeof sentence === "string"
    ? page.getByRole("status").getByText(sentence, { exact: true })
    : page.getByRole("status").getByText(sentence);

/** Presses `name` in the question over the page. */
async function confirm(page: Page, name: string) {
  await page.getByRole("alertdialog").getByRole("button", { name, exact: true }).click();
}

test("a reader subscribes to a plan, sees it, switches it, cancels it, and tips the server", async ({
  page,
  signIn,
  fresh,
}) => {
  await fresh.supporterProgram();
  const reader = await fresh.person();
  await signIn(reader);

  // The plans, monthly: Choose asks what subscribing does, then the provider's checkout takes the payment.
  await page.goto("/about/supporters");
  const main = page.getByRole("main");
  await main.getByRole("button", { name: "Choose Supporter", exact: true }).click();
  await expect(page.getByRole("alertdialog", { name: "Subscribe to Supporter, monthly?", exact: true })).toBeVisible();
  await confirm(page, "Go to payment");
  await page.getByRole("button", { name: "Pay", exact: true }).click();

  // Back in the settings, the server asked the provider: the subscription renews.
  await expect(page).toHaveURL(/^[^?]+\/settings\/supporter$/);
  await expect(said(page, "Thank you. You support this server on Supporter, monthly.")).toBeVisible();
  await expect(main.getByText(/^Renews on .+$/)).toBeVisible();

  // The higher plan applies at once, its difference charged.
  await page.goto("/about/supporters");
  await main.getByRole("button", { name: "Switch Patron", exact: true }).click();
  await confirm(page, "Change now");
  await expect(said(page, "Your plan is now Patron, monthly.")).toBeVisible();

  // Read anew, the subscription is of the higher plan; cancelled, it ends at the period's end, and says so read anew.
  await page.goto("/settings/supporter");
  await expect(main.getByText(/^Patron, monthly · .+ a month$/)).toBeVisible();
  await main.getByRole("button", { name: "Cancel subscription…", exact: true }).click();
  await confirm(page, "Cancel subscription");
  await expect(said(page, /^Your subscription ends on .+\.$/)).toBeVisible();
  await page.reload();
  await expect(main.getByText(/^Ends on .+$/)).toBeVisible();
  await expect(main.getByText(/^Patron, monthly · .+ a month · Your perks stay until then\.$/)).toBeVisible();
  await expect(main.getByRole("button", { name: "Resume", exact: true })).toBeVisible();

  // A tip of ¥1,000, paid once on the provider's checkout, comes back with thanks.
  await page.goto("/settings/supporter/tip");
  await main.getByRole("radio", { name: "¥1,000", exact: true }).click();
  await page.getByRole("button", { name: "Tip ¥1,000", exact: true }).click();
  await page.getByRole("button", { name: "Pay", exact: true }).click();
  await expect(main.getByRole("heading", { name: "Thank you for your tip of ¥1,000", exact: true })).toBeVisible();
  await expect(page).toHaveURL(/^[^?]+\/settings\/supporter\/tip$/);
});
