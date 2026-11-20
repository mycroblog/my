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

/**
 * The digits after the point of every currency the server takes (ISO 4217, list one, with minor units: its table in
 * internal/domain/finance/currency.go, which this mirrors), by code. A currency left out is one the server refuses.
 */
export const currencyDigits: Readonly<Record<string, number>> = {
  AED: 2,
  AFN: 2,
  ALL: 2,
  AMD: 2,
  ANG: 2,
  AOA: 2,
  ARS: 2,
  AUD: 2,
  AWG: 2,
  AZN: 2,
  BAM: 2,
  BBD: 2,
  BDT: 2,
  BGN: 2,
  BHD: 3,
  BIF: 0,
  BMD: 2,
  BND: 2,
  BOB: 2,
  BOV: 2,
  BRL: 2,
  BSD: 2,
  BTN: 2,
  BWP: 2,
  BYN: 2,
  BZD: 2,
  CAD: 2,
  CDF: 2,
  CHE: 2,
  CHF: 2,
  CHW: 2,
  CLF: 4,
  CLP: 0,
  CNY: 2,
  COP: 2,
  COU: 2,
  CRC: 2,
  CUP: 2,
  CVE: 2,
  CZK: 2,
  DJF: 0,
  DKK: 2,
  DOP: 2,
  DZD: 2,
  EGP: 2,
  ERN: 2,
  ETB: 2,
  EUR: 2,
  FJD: 2,
  FKP: 2,
  GBP: 2,
  GEL: 2,
  GHS: 2,
  GIP: 2,
  GMD: 2,
  GNF: 0,
  GTQ: 2,
  GYD: 2,
  HKD: 2,
  HNL: 2,
  HTG: 2,
  HUF: 2,
  IDR: 2,
  ILS: 2,
  INR: 2,
  IQD: 3,
  IRR: 2,
  ISK: 0,
  JMD: 2,
  JOD: 3,
  JPY: 0,
  KES: 2,
  KGS: 2,
  KHR: 2,
  KMF: 0,
  KPW: 2,
  KRW: 0,
  KWD: 3,
  KYD: 2,
  KZT: 2,
  LAK: 2,
  LBP: 2,
  LKR: 2,
  LRD: 2,
  LSL: 2,
  LYD: 3,
  MAD: 2,
  MDL: 2,
  MGA: 2,
  MKD: 2,
  MMK: 2,
  MNT: 2,
  MOP: 2,
  MRU: 2,
  MUR: 2,
  MVR: 2,
  MWK: 2,
  MXN: 2,
  MXV: 2,
  MYR: 2,
  MZN: 2,
  NAD: 2,
  NGN: 2,
  NIO: 2,
  NOK: 2,
  NPR: 2,
  NZD: 2,
  OMR: 3,
  PAB: 2,
  PEN: 2,
  PGK: 2,
  PHP: 2,
  PKR: 2,
  PLN: 2,
  PYG: 0,
  QAR: 2,
  RON: 2,
  RSD: 2,
  RUB: 2,
  RWF: 0,
  SAR: 2,
  SBD: 2,
  SCR: 2,
  SDG: 2,
  SEK: 2,
  SGD: 2,
  SHP: 2,
  SLE: 2,
  SOS: 2,
  SRD: 2,
  SSP: 2,
  STN: 2,
  SVC: 2,
  SYP: 2,
  SZL: 2,
  THB: 2,
  TJS: 2,
  TMT: 2,
  TND: 3,
  TOP: 2,
  TRY: 2,
  TTD: 2,
  TWD: 2,
  TZS: 2,
  UAH: 2,
  UGX: 0,
  USD: 2,
  USN: 2,
  UYI: 0,
  UYU: 2,
  UYW: 4,
  UZS: 2,
  VED: 2,
  VES: 2,
  VND: 0,
  VUV: 0,
  WST: 2,
  XAF: 0,
  XCD: 2,
  XCG: 2,
  XOF: 0,
  XPF: 0,
  YER: 2,
  ZAR: 2,
  ZMW: 2,
  ZWG: 2,
};

/** The codes of the currencies the server takes, in alphabetical order. */
export const currencies: readonly string[] = Object.keys(currencyDigits).toSorted();

/** The digits after the point of `currency` as the server keeps them; 2 for a code it does not take. */
export const digitsOf = (currency: string): number => currencyDigits[currency] ?? 2;

/**
 * Whether `amount` is an amount of `currency` as the server takes it: digits, with no more of them after the point
 * than the currency has, and no grouping.
 */
export function amountFits(amount: string, currency: string): boolean {
  const digits = digitsOf(currency);
  return (digits === 0 ? /^\d+$/ : new RegExp(`^\\d+(\\.\\d{1,${String(digits)}})?$`)).test(amount.trim());
}
