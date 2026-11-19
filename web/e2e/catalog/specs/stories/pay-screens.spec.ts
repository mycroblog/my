// SPDX-License-Identifier: AGPL-3.0-only

// The stories of the payment pages (web/app/src/pay-*.stories.tsx) each show the state they are named by: PAY.JP's
// checkout live, in test mode and with a card the server refused, the fake's checkout and its failure, a tip, the
// portal with its card, after a change, paused and without a card, the fake's portal, and the error page. What the
// pages do from each state is the feature's tests' (web/app-features/src/pay/screen.dom.test.tsx); this is that the
// catalog shows each state for a review and a shot.

import type { Page } from "@playwright/test";
import { expect, pc, test } from "../test";

const main = (page: Page) => page.getByRole("main");

/** The page's band of a test: `says`, or none at all. */
const band = async (page: Page, says?: string) => {
  if (says === undefined) await expect(page.getByRole("note")).toHaveCount(0);
  else await expect(page.getByRole("note")).toHaveText(says);
};

const testBuild = "Test build: this page stands in for a payment provider, and no money moves.";

/** PAY.JP's checkout of the monthly plan: its card form, the line of what follows, and its button. */
const payjpCheckout = async (page: Page) => {
  await expect(main(page).getByRole("group", { name: "Card", exact: true })).toBeVisible();
  await expect(main(page).getByText("Then ¥1,200 a month.", { exact: true })).toBeVisible();
  await expect(main(page).getByRole("button", { name: "Pay ¥1,200 and subscribe", exact: true })).toBeVisible();
};

/** The fake's three choices, and no other way out. */
const fakeChoices = async (page: Page) => {
  for (const name of ["Pay", "Make the payment fail", "Cancel"])
    await expect(main(page).getByRole("button", { name, exact: true })).toBeVisible();
  await expect(main(page).getByRole("link")).toHaveCount(0);
};

/** A portal of PAY.JP: the card form that `button` sends, and the payments as rows. */
const portalOf = async (page: Page, button: string, rows: number) => {
  await expect(main(page).getByRole("button", { name: button, exact: true })).toBeVisible();
  await expect(
    main(page).getByRole("region", { name: "Recent payments", exact: true }).getByRole("listitem"),
  ).toHaveCount(rows);
};

const states: readonly (readonly [string, (page: Page) => Promise<void>])[] = [
  [
    "screens-pay-checkout--payjp",
    async (page) => {
      await band(page);
      await payjpCheckout(page);
      await expect(main(page).getByRole("alert")).toHaveCount(0);
    },
  ],
  [
    "screens-pay-checkout--test-mode",
    async (page) => {
      await band(page, "Test mode: nothing is charged.");
      await payjpCheckout(page);
    },
  ],
  [
    "screens-pay-checkout--refused",
    async (page) => {
      await payjpCheckout(page);
      await expect(main(page).getByRole("alert")).toHaveText(/^The card could not be verified/);
    },
  ],
  [
    "screens-pay-checkout--fake",
    async (page) => {
      await band(page, testBuild);
      await expect(main(page).getByText("¥500 a month", { exact: true })).toBeVisible();
      await fakeChoices(page);
      await expect(main(page).getByRole("alert")).toHaveCount(0);
    },
  ],
  [
    "screens-pay-checkout--fake-failed",
    async (page) => {
      await band(page, testBuild);
      await expect(main(page).getByRole("alert")).toHaveText(/^The payment failed/);
      await fakeChoices(page);
    },
  ],
  [
    "screens-pay-checkout--tip",
    async (page) => {
      await band(page, testBuild);
      await expect(main(page).getByText("¥500, once", { exact: true })).toBeVisible();
      await fakeChoices(page);
    },
  ],
  [
    "screens-pay-portal--payjp",
    async (page) => {
      await expect(main(page).getByText("Next payment: 2026-12-01", { exact: true })).toBeVisible();
      await expect(main(page).getByRole("region", { name: "Card in use", exact: true })).toBeVisible();
      await expect(main(page).getByRole("status")).toHaveCount(0);
      await portalOf(page, "Change the card", 2);
    },
  ],
  [
    "screens-pay-portal--changed",
    async (page) => {
      await expect(main(page).getByRole("status")).toHaveText("The card was changed.");
      await portalOf(page, "Change the card", 2);
    },
  ],
  [
    "screens-pay-portal--paused",
    async (page) => {
      await expect(main(page).getByText(/^The last payment failed, so the subscription is paused\b.*$/)).toBeVisible();
      await expect(main(page).getByText(/^Next payment: .*$/)).toHaveCount(0);
      await portalOf(page, "Change the card", 2);
    },
  ],
  [
    "screens-pay-portal--empty",
    async (page) => {
      await expect(main(page).getByRole("region", { name: "Add a card", exact: true })).toBeVisible();
      await expect(main(page).getByText("No payments yet.", { exact: true })).toBeVisible();
      await portalOf(page, "Add a card", 0);
    },
  ],
  [
    "screens-pay-portal--fake",
    async (page) => {
      await band(page, testBuild);
      await expect(
        main(page).getByText("This test build has no payment methods or invoices to manage.", { exact: true }),
      ).toBeVisible();
      await expect(main(page).getByRole("button")).toHaveCount(0);
    },
  ],
  [
    "screens-pay-error--expired",
    async (page) => {
      await expect(main(page).getByRole("alert")).toHaveText(/^This page has expired\./);
      await expect(main(page).getByRole("link", { name: "Back to the app", exact: true })).toHaveAttribute("href", "/");
    },
  ],
  [
    "screens-pay-error--unavailable",
    async (page) => {
      await expect(main(page).getByRole("alert")).toHaveText(/^PAY\.JP is not answering\./);
      await expect(main(page).getByRole("link", { name: "Back to the app", exact: true })).toHaveAttribute(
        "href",
        "/settings/supporter",
      );
    },
  ],
];

// One page walks the stories, a step each: a page of its own per story would cost the catalog's run its time limit.
test("each story of the payment pages shows its state", async ({ page, open }) => {
  for (const [id, shows] of states)
    await test.step(id, async () => {
      await open(id, { width: pc });
      await shows(page);
    });
});
