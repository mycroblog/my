// SPDX-License-Identifier: AGPL-3.0-only

// What the finance screens compute from the API's answer before they draw it: today in the reader's zone, the periods
// the server answered for, whether a line is an imported provider's however an operator spelled it, the income beside
// the costs of a period, and which amounts draw a bar. Unit tests, without a DOM.

import { expect, test } from "bun:test";
import { financePeriodAfter } from "@my/app-data";
import {
  answeredPeriods,
  barOf,
  importedProvider,
  incomeBeside,
  moneyText,
  todayIn,
  type FinanceView,
} from "./finance-totals";

test("today is the date of the reader's zone, ahead of UTC's east of it", () => {
  const at = new Date("2026-11-30T16:00:00Z");
  expect(todayIn("UTC", at)).toBe("2026-11-30");
  expect(todayIn("Asia/Tokyo", at)).toBe("2026-12-01");
});

/** A window of months ending with the one that holds `today`. */
const months = (today: string, count: number): FinanceView =>
  ({
    granularity: "month",
    periods: Array.from({ length: count }, (_, i) => financePeriodAfter("month", today, -i)).filter(
      (p) => p !== undefined,
    ),
  }) as unknown as FinanceView;

test("a period after the last the server answered for is left out, not shown as having nothing", () => {
  // Tokyo is in December, the server's UTC date still in November.
  const view = months("2026-12-01", 3);
  const answered = [{ label: "2026-10" }, { label: "2026-11" }];
  expect(answeredPeriods(view, answered, undefined).map((p) => p.label)).toEqual(["2026-11", "2026-10"]);
  expect(answeredPeriods(view, undefined, undefined).map((p) => p.label)).toEqual(["2026-12", "2026-11", "2026-10"]);
});

test("a line is an imported provider's in any spelling the server groups with it (NFKC, lower case)", () => {
  const sources = [{ source: "aws" as const, provider: "AWS", time_zone: "UTC" }];
  expect(importedProvider(sources, "aws")).toBe(true);
  expect(importedProvider(sources, "ＡＷＳ")).toBe(true);
  expect(importedProvider(sources, "Sakura Cloud")).toBe(false);
});

test("the income of a period has a zero in each currency it has costs in, sorted by currency", () => {
  expect(
    incomeBeside(
      [{ amount: "4520", currency: "JPY" }],
      [
        { amount: "3300", currency: "JPY" },
        { amount: "45.67", currency: "USD" },
      ],
    ),
  ).toEqual([
    { amount: "4520", currency: "JPY" },
    { amount: "0", currency: "USD" },
  ]);
});

test("only an amount above zero draws a bar", () => {
  expect(barOf({ amount: "12.5", currency: "USD" })).toBe(12.5);
  expect(barOf({ amount: "-12.5", currency: "USD" })).toBeUndefined();
  expect(barOf({ amount: "0", currency: "USD" })).toBeUndefined();
});

test("an amount is written with its currency's digits as the server keeps them, not Intl's", () => {
  // Intl (CLDR) writes the Iraqi dinar and the Hungarian forint with no decimals; ISO 4217 gives them 3 and 2.
  expect(moneyText("en", { amount: "12.345", currency: "IQD" })).toContain("12.345");
  expect(moneyText("en", { amount: "1000.50", currency: "HUF" })).toContain("1,000.50");
  expect(moneyText("en", { amount: "1650", currency: "JPY" })).toBe("¥1,650");
});

test("a negative amount is written with the minus sign, not the hyphen-minus", () => {
  expect(moneyText("en", { amount: "-1650", currency: "JPY" })).toBe("\u2212¥1,650");
  expect(moneyText("ja", { amount: "-1650", currency: "JPY" })).toBe("\u2212￥1,650");
});
