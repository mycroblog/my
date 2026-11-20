// SPDX-License-Identifier: AGPL-3.0-only

// The stories of the supporter's screens (web/app/src/supporters.stories.tsx) each show the state they are named by, on
// the fake API: the supporters' page for a reader without a plan, a member, a visitor, a reader who may not subscribe
// nor tip (each reason, with the program and on a server of tips alone), a server of tips alone, the native shell, a
// server that takes no support, and its loading, failure, empty list and long names; the settings' Supporter plans and
// tips in each state of a subscription and back from the checkout; and the tip, its returns, a reader who may not tip
// and a server that takes none. What the screens do from each state is the app's route tests'
// (web/app/src/supporters-routes.dom.test.tsx); this is that the catalog shows each state for a review and a shot.

import type { Page } from "@playwright/test";
import { expect, pc, test } from "../test";

/** The story of the supporter's screens named `name`. */
const story = (name: string) => `screens-supporters--${name}`;

const main = (page: Page) => page.getByRole("main");

/** The plans of the page, each an item of their list. */
const plans = (page: Page) => main(page).getByRole("list", { name: "Plans", exact: true }).getByRole("listitem");

/** Whether the main says `text`, whole. */
const says = (page: Page, text: string | RegExp) =>
  expect(
    typeof text === "string" ? main(page).getByText(text, { exact: true }) : main(page).getByText(text),
  ).toBeVisible();

const states: readonly (readonly [string, (page: Page) => Promise<void>])[] = [
  [
    "page",
    async (page) => {
      await expect(plans(page)).toHaveCount(2);
      await expect(main(page).getByRole("button", { name: "Choose Supporter", exact: true })).toBeVisible();
      await expect(main(page).getByRole("link", { name: "Tip", exact: true })).toBeVisible();
      await expect(main(page).getByRole("list", { name: "Patron", exact: true })).toBeVisible();
    },
  ],
  [
    "page-member",
    async (page) => {
      await expect(main(page).getByRole("listitem", { name: "Supporter", exact: true })).toHaveAttribute(
        "aria-current",
        "true",
      );
      await expect(main(page).getByRole("button", { name: "Switch Patron", exact: true })).toBeVisible();
      await says(page, /^You support this server on Supporter\..*Your subscription$/);
    },
  ],
  [
    "page-lapsed",
    async (page) => {
      await says(page, /^Your payment for Supporter failed, and its perks have ended\..*Your subscription$/);
      for (const name of ["Supporter", "Patron"])
        await expect(main(page).getByRole("listitem", { name, exact: true })).not.toHaveAttribute("aria-current");
    },
  ],
  [
    "page-hidden-plan",
    async (page) => {
      await expect(main(page).getByRole("button", { name: "Switch Supporter", exact: true })).toBeVisible();
      await expect(main(page).getByRole("button", { name: "Switch Patron", exact: true })).toBeVisible();
    },
  ],
  [
    "page-visitor",
    async (page) => {
      await expect(plans(page)).toHaveCount(2);
      await says(page, /^Subscribing needs an account on this server\..*Sign in$/);
      // The plans' line alone leads to the sign-in.
      await says(page, "Only people signed in to this server can see the supporters list.");
      await expect(main(page).getByRole("button", { name: "Choose Supporter", exact: true })).toHaveCount(0);
    },
  ],
  [
    "page-unverified",
    async (page) => {
      await says(page, "Confirm your email address to subscribe.");
      await says(page, "Confirm your email address to tip.");
    },
  ],
  [
    "page-tips-only",
    async (page) => {
      await expect(plans(page)).toHaveCount(0);
      await expect(main(page).getByRole("link", { name: "Tip", exact: true })).toBeVisible();
    },
  ],
  [
    "page-not-local",
    async (page) => {
      await says(page, "Only people with an account on this server can subscribe.");
      await says(page, "Only people with an account on this server can tip.");
    },
  ],
  [
    "page-restricted",
    async (page) => {
      await says(page, "Your account cannot subscribe while it is restricted.");
      await says(page, "Your account cannot tip while it is restricted.");
    },
  ],
  ...(
    [
      ["unverified", "Confirm your email address to tip."],
      ["not-local", "Only people with an account on this server can tip."],
      ["restricted", "Your account cannot tip while it is restricted."],
    ] as const
  ).map(
    ([reason, why]) =>
      [
        `page-tips-only-${reason}`,
        async (page: Page) => {
          await says(page, why);
          await expect(main(page).getByRole("link", { name: "Tip", exact: true })).toHaveCount(0);
        },
      ] as const,
  ),
  [
    "page-native-shell",
    async (page) => {
      await expect(plans(page)).toHaveCount(0);
      await expect(main(page).getByRole("link", { name: "Tip", exact: true })).toHaveCount(0);
      await expect(main(page).getByRole("list", { name: "Supporter", exact: true })).toBeVisible();
    },
  ],
  ["page-off", (page) => says(page, "This server takes no support here.")],
  ["page-loading", (page) => says(page, "Loading the plans…")],
  ["page-error", (page) => says(page, /^The plans could not be loaded\./)],
  ["page-empty", (page) => says(page, "Nobody chose to be listed yet.")],
  ["page-long", async (page) => expect(plans(page)).toHaveCount(2)],
  [
    "settings",
    async (page) => {
      await says(page, "You do not support this server with a plan.");
      await expect(main(page).getByRole("link", { name: /^See the plans.*$/ })).toBeVisible();
    },
  ],
  [
    "settings-pending",
    (page) => says(page, "Your subscription to Supporter, monthly, waits for its first payment of ¥500."),
  ],
  [
    "settings-active",
    async (page) => {
      await says(page, /^Renews on .+$/);
      await expect(main(page).getByRole("button", { name: "Cancel subscription…", exact: true })).toBeVisible();
      await expect(main(page).getByRole("switch", { name: "List me among the supporters", exact: true })).toBeChecked();
    },
  ],
  ["settings-yearly", (page) => says(page, /^Supporter, yearly · .+$/)],
  [
    "settings-canceling",
    async (page) => {
      await says(page, /^Ends on .+$/);
      await expect(main(page).getByRole("button", { name: "Resume", exact: true })).toBeVisible();
    },
  ],
  [
    "settings-scheduled",
    async (page) => {
      await says(page, /^Changes to Supporter, monthly, on .+$/);
      await expect(main(page).getByRole("button", { name: "Keep my plan", exact: true })).toBeVisible();
    },
  ],
  [
    "settings-past-due",
    async (page) => {
      await says(page, /^Your payment failed: update your payment method before .+, or your perks end then$/);
      await expect(main(page).getByRole("button", { name: "Update payment", exact: true })).toBeVisible();
      await expect(main(page).getByRole("button", { name: "Cancel subscription…", exact: true })).toHaveCount(0);
    },
  ],
  ["settings-kept-price", (page) => says(page, /^Supporter, monthly · ¥400 a month$/)],
  ["settings-lapsed", (page) => says(page, "Your payment failed, and your perks have ended")],
  ["settings-unverified", (page) => says(page, "Confirm your email address to subscribe.")],
  ["settings-returned-paid", (page) => says(page, "Thank you. You support this server on Supporter, monthly.")],
  [
    "settings-returned-settling",
    (page) => says(page, "Your payment is being confirmed. This page updates once it goes through."),
  ],
  ["settings-returned-canceled", (page) => says(page, "You left the payment page. Nothing was charged.")],
  ["settings-loading", (page) => says(page, "Loading your subscription…")],
  ["settings-error", (page) => says(page, /^Your subscription could not be loaded\./)],
  [
    "tip",
    async (page) => {
      await expect(main(page).getByRole("radio", { name: "¥500", exact: true })).toBeChecked();
      await expect(page.getByRole("button", { name: "Tip ¥500", exact: true })).toBeVisible();
    },
  ],
  ["tip-paid", (page) => says(page, "Thank you for your tip of ¥1,000")],
  ["tip-settling", (page) => says(page, "Your tip of ¥500 is being confirmed")],
  ["tip-unpaid", (page) => says(page, "Your tip was not paid")],
  [
    "tip-unverified",
    async (page) => {
      await says(page, "Confirm your email address to tip.");
      await expect(page.getByRole("button", { name: "Tip ¥500", exact: true })).toHaveCount(0);
    },
  ],
  ...(
    [
      ["tip-not-local", "Only people with an account on this server can tip."],
      ["tip-restricted", "Your account cannot tip while it is restricted."],
      ["tip-only-unverified", "Confirm your email address to tip."],
      ["tip-only-not-local", "Only people with an account on this server can tip."],
      ["tip-only-restricted", "Your account cannot tip while it is restricted."],
    ] as const
  ).map(
    ([name, why]) =>
      [
        name,
        async (page: Page) => {
          await says(page, why);
          await expect(page.getByRole("button", { name: "Tip ¥500", exact: true })).toHaveCount(0);
        },
      ] as const,
  ),
  ["tip-off", (page) => says(page, "This server takes no tips now.")],
  ["tip-loading", (page) => says(page, "Loading the amounts…")],
  ["tip-error", (page) => says(page, /^The amounts could not be loaded\./)],
];

// One page walks the stories, a step each: a page of its own per story would cost the catalog's run its time limit.
test("each story of the supporter's screens shows its state", async ({ page, open }) => {
  for (const [name, shows] of states)
    await test.step(name, async () => {
      await open(story(name), { width: pc });
      await shows(page);
    });
});
