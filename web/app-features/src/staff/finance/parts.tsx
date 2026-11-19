// SPDX-License-Identifier: AGPL-3.0-only

// What the screens of Finance share (ADR-0067, ADR-0068): their addresses, the way back to the first page, a calendar
// date and a cost item's schedule as the reader's language writes them, today in the reader's zone, and an amount of
// a currency as the API takes it.

import type { CostItem } from "@my/app-data";
import { BackButton, dateFormat, useLocale, useTimeZone } from "@my/design-system";
import { todayIn } from "../../finance-totals";
import { useStaffStrings, type T } from "../parts";

/** The addresses of the screens of Finance. */
export const financePaths = {
  root: "/admin/finance",
  costs: "/admin/finance/costs",
  newCost: "/admin/finance/costs/new",
  cost: (id: string) => `/admin/finance/costs/${encodeURIComponent(id)}`,
  income: "/admin/finance/income",
  imports: "/admin/finance/imports",
  publication: "/admin/finance/publication",
} as const;

/** Back to the first page of Finance, or to the costs. */
export function FinanceBack({ to, go }: { to: "root" | "costs"; go: (href: string) => void }) {
  const { t } = useStaffStrings();
  return (
    <BackButton
      onPress={() => {
        go(financePaths[to]);
      }}
    >
      {t(to === "root" ? "staff_finance.back.root" : "staff_finance.back.costs")}
    </BackButton>
  );
}

/**
 * Writes a calendar date (2026-11-20) as the reader's language does, held together (no-break spaces): a date of the
 * calendar is no moment, so it is read in UTC, where it is written.
 */
export function useDay(): (date: string) => string {
  const { locale } = useLocale();
  const format = dateFormat(locale, "UTC", { dateStyle: "medium" });
  return (date) => format.format(new Date(`${date}T00:00:00Z`)).replace(/ /g, " ");
}

/** Today in the reader's time zone, as a date of the calendar. */
export function useTodayDate(): string {
  return todayIn(useTimeZone());
}

/** How often a cost item is paid, in words: once, or every N weeks, months or years. */
export const scheduleWords = (t: T, item: Pick<CostItem, "schedule" | "interval">): string =>
  t("staff_finance.schedule", { schedule: item.schedule, count: item.interval });

/** The digits after the point of `currency`'s amounts (ISO 4217), as `Intl` knows them; 2 for one it does not. */
function digitsOf(currency: string): number {
  try {
    return new Intl.NumberFormat("en", { style: "currency", currency }).resolvedOptions().maximumFractionDigits ?? 2;
  } catch {
    return 2;
  }
}

/** Whether `amount` is an amount of `currency` as the API takes it: digits, and no more decimals than it has. */
export function amountFits(amount: string, currency: string): boolean {
  const digits = digitsOf(currency);
  return (digits === 0 ? /^\d+$/ : new RegExp(`^\\d+(\\.\\d{1,${String(digits)}})?$`)).test(amount.trim());
}

/** The currencies a cost is written in, by their code, in the reader's language: every one `Intl` knows. */
export function useCurrencyOptions(): { value: string; label: string }[] {
  const { locale } = useLocale();
  const names = new Intl.DisplayNames([locale], { type: "currency" });
  return Intl.supportedValuesOf("currency").map((code) => ({
    value: code,
    label: `${names.of(code) ?? code} (${code})`,
  }));
}
