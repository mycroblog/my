// SPDX-License-Identifier: AGPL-3.0-only

// What every payment page shares: its strings, its money, and whether what the server refused takes the focus.

import { useLocale } from "@my/design-system";
import { createContext } from "react";
import { stringsIn } from "../locale";
import type { PayPrice } from "./data";

/** The payment pages' strings in the reader's locale. */
export const usePayStrings = () => stringsIn(useLocale().locale).t;

/**
 * An amount in the minor units of `currency` as the reader's locale writes money: with the currency's symbol and its
 * own decimals (¥1,200, $12.00).
 */
export function moneyIn(locale: string, amount: number, currency: string): string {
  const format = new Intl.NumberFormat(locale, { style: "currency", currency });
  return format.format(amount / 10 ** (format.resolvedOptions().maximumFractionDigits ?? 0));
}

/** How the payment pages write money and prices in the reader's locale. */
export function useMoney() {
  const { locale } = useLocale();
  const t = stringsIn(locale).t;
  const money = (amount: number, currency: string) => moneyIn(locale, amount, currency);
  const price = (p: PayPrice) => {
    const amount = money(p.amount, p.currency);
    switch (p.interval) {
      case "month":
        return t("pay.price.month", { amount });
      case "year":
        return t("pay.price.year", { amount });
      case "once":
        return t("pay.price.once", { amount });
    }
  };
  return { money, price };
}

/**
 * Whether what the server refused takes the focus when the page shows: on the page itself, not where many pages show
 * at once (a story).
 */
export const FocusOnLoadContext = createContext(true);
