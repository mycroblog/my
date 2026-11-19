// SPDX-License-Identifier: AGPL-3.0-only

// The page's data is the JSON block the server writes into the payment page: each screen comes from it as it was
// written, and a page that holds none, or one that names no screen of this entry or no title, draws nothing.

import { expect, test } from "bun:test";
import { parsePayPage, type PayPage } from "./data";

const card = {
  action: "/payments/pages/payjp/checkout",
  state: "signed",
  publicKey: "pk_test",
  email: "a@example.com",
};

test("each screen is read as the server wrote it", () => {
  for (const page of [
    {
      screen: "checkout",
      server: { name: "Hanami Town" },
      title: "Subscribe to Supporter",
      testMode: true,
      price: { amount: 1200, currency: "JPY", interval: "month" },
      error: "The card was declined.",
      back: "/settings/supporter",
      card,
    },
    {
      screen: "checkout",
      title: "Tip",
      fake: true,
      price: { amount: 500, currency: "JPY", interval: "once" },
      choices: { action: "/payments/pages/fake/checkout/cs_1" },
    },
    {
      screen: "portal",
      title: "Payment method and history",
      notice: "The card was changed.",
      card: { ...card, change: true },
      portal: {
        plan: { name: "Supporter", price: { amount: 12000, currency: "JPY", interval: "year" }, next: "2026-12-01" },
        card: "Visa ending in 4242, expires 12/2030",
        paused: false,
        payments: [{ date: "2026-11-01", amount: 1200, refunded: 200, currency: "JPY" }],
      },
    },
    { screen: "error", title: "Payment", error: "This page has expired." },
  ] as const satisfies readonly PayPage[])
    expect(parsePayPage(JSON.stringify(page))).toEqual(page);
});

test("a page without its data, or with data of no screen or no title, is refused", () => {
  for (const [block, says] of [
    [null, "the page has no data block"],
    ["[]", "no screen"],
    [JSON.stringify({ screen: "invoice", title: "Invoice" }), "no screen invoice"],
    [JSON.stringify({ screen: "error", error: "x" }), "no title"],
  ] as const)
    expect(() => parsePayPage(block)).toThrow(says);
  expect(() => parsePayPage("not json")).toThrow(SyntaxError);
});
