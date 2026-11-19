// SPDX-License-Identifier: AGPL-3.0-only
// Money as the API gives it (ADR-0067 §6, §9): a decimal string in the major unit with the digits of its currency and
// the currency's ISO 4217 code, never a JSON number, which would round. The totals hold one amount per currency and
// are never converted. What is computed here is exact, on the decimal strings; the screens write an amount with
// `Intl.NumberFormat` (style `currency`), which takes the string as it is.
import type { components } from "@mycroblog/mykit";

/** An amount of money: `{ amount: "12.34", currency: "USD" }`, `{ amount: "1650", currency: "JPY" }`. */
export type Money = components["schemas"]["Money"];

/** An amount as an integer of `scale` decimal digits: "-12.3" is -123 at scale 1. */
type Scaled = { readonly units: bigint; readonly scale: number };

function scaled(amount: string): Scaled {
  const [whole = "0", fraction = ""] = amount.split(".");
  return { units: BigInt(whole + fraction), scale: fraction.length };
}

/** `a` and `b` at the larger of their scales. */
function aligned(a: Scaled, b: Scaled): [bigint, bigint, number] {
  const scale = Math.max(a.scale, b.scale);
  return [a.units * 10n ** BigInt(scale - a.scale), b.units * 10n ** BigInt(scale - b.scale), scale];
}

/** The decimal string of `units` at `scale`. */
function written(units: bigint, scale: number): string {
  const negative = units < 0n;
  const digits = (negative ? -units : units).toString().padStart(scale + 1, "0");
  const whole = scale === 0 ? digits : digits.slice(0, -scale);
  const fraction = scale === 0 ? "" : `.${digits.slice(-scale)}`;
  return `${negative ? "-" : ""}${whole}${fraction}`;
}

/** Whether `money` is below zero (-1), zero (0), or above it (1): a balance in deficit is below. */
export function moneySign(money: Money): -1 | 0 | 1 {
  const { units } = scaled(money.amount);
  return units < 0n ? -1 : units > 0n ? 1 : 0;
}

/** `a` less `b`, of one currency; undefined for two currencies, which are never converted. */
export function moneyLess(a: Money, b: Money): Money | undefined {
  if (a.currency !== b.currency) return undefined;
  const [x, y, scale] = aligned(scaled(a.amount), scaled(b.amount));
  return { amount: written(x - y, scale), currency: a.currency };
}

/** `money` times the whole number `times`. */
export function moneyTimes(money: Money, times: number): Money {
  const { units, scale } = scaled(money.amount);
  return { amount: written(units * BigInt(times), scale), currency: money.currency };
}

/** The amount of `currency` among `amounts` (a total of one currency each); undefined where it has none. */
export function moneyIn(amounts: readonly Money[], currency: string): Money | undefined {
  return amounts.find((m) => m.currency === currency);
}

/**
 * How much `part` is of `whole`, of one currency, in whole percent cut toward zero; undefined for a `whole` that is
 * not above zero, or two currencies.
 */
export function moneyPercent(part: Money, whole: Money): number | undefined {
  if (part.currency !== whole.currency) return undefined;
  const [x, y] = aligned(scaled(part.amount), scaled(whole.amount));
  if (y <= 0n) return undefined;
  return Number((x * 100n) / y);
}
