// SPDX-License-Identifier: AGPL-3.0-only

// The digits of a currency as the server keeps them (ISO 4217, internal/domain/finance/currency.go), where Intl's own
// (CLDR) differ: an amount the server takes is one the client takes, and a code the server refuses is not offered.

import { expect, test } from "bun:test";
import { amountFits, currencies, digitsOf } from "./money";

test.each([
  ["JPY", 0],
  ["USD", 2],
  ["HUF", 2],
  ["IDR", 2],
  ["COP", 2],
  ["IQD", 3],
  ["BHD", 3],
  ["CLF", 4],
  ["ISK", 0],
] as const)("%s has %d digits after the point, as the server keeps it", (currency, digits) => {
  expect(digitsOf(currency)).toBe(digits);
});

test("an amount fits its currency with as many decimals as it has at most, digits alone", () => {
  expect(amountFits("1650", "JPY")).toBe(true);
  expect(amountFits("1650.5", "JPY")).toBe(false);
  expect(amountFits("1000.50", "HUF")).toBe(true);
  expect(amountFits("12.345", "IQD")).toBe(true);
  expect(amountFits("12.3456", "IQD")).toBe(false);
  expect(amountFits("1,000", "USD")).toBe(false);
});

test("the codes the server refuses are not offered", () => {
  for (const refused of ["CUC", "HRK", "SLL", "XDR", "XSU", "ZWL"]) expect(currencies).not.toContain(refused);
  expect(currencies).toContain("JPY");
  expect(currencies).toEqual(currencies.toSorted());
});
