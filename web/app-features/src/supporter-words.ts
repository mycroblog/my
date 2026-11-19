// SPDX-License-Identifier: AGPL-3.0-only

// What the supporters' page and the settings' Supporter both say (ADR-0072 §6, ADR-0162 §3): an amount as the reader's
// language writes it in its own currency, and why the reader is not offered to subscribe or to tip, in one sentence.

import type { Money, SupporterBar } from "@my/app-data";
import { useLocale } from "@my/design-system";
import type { stringsIn } from "./locale";

type T = ReturnType<typeof stringsIn>["t"];

/** An amount as the reader's language writes it, in its currency ("¥500", "US$4.50"). */
export function useMoney(): (money: Money) => string {
  const { locale } = useLocale();
  return (money) =>
    new Intl.NumberFormat(locale, { style: "currency", currency: money.currency }).format(
      // The API's decimal string, taken as it is: a JSON number would round.
      money.amount as Intl.StringNumericLiteral,
    );
}

/** The languages whose sentences follow each other with no space between them. */
const unspaced: ReadonlySet<string> = new Set(["ja", "zh"]);

/**
 * Sentences of the catalog, one after the other, as the language of `locale` writes them: a space between them where
 * it puts one (en), none where it does not (ja: 「…。…。」).
 */
export function sentences(locale: string, parts: readonly string[]): string {
  const language = locale.split("-")[0]?.toLowerCase() ?? "";
  return parts.join(unspaced.has(language) ? "" : " ");
}

/** The bars of `SupporterBar` a sentence says: the program or the tips off and the native shell say nothing. */
export type SaidBar = Exclude<SupporterBar, "off" | "app">;

/** Whether the reader is told why they are not offered it: never for the program off, nor in the native shell. */
export const barSaid = (bar: SupporterBar | undefined): bar is SaidBar =>
  bar !== undefined && bar !== "off" && bar !== "app";

/** Why the reader is not offered to subscribe (`subscribe`) or to tip (`tip`), in one sentence. */
export function barSentence(bar: SaidBar, of: "subscribe" | "tip", t: T): string {
  const reason = bar === "signed-out" ? "signed_out" : bar;
  return of === "subscribe" ? t("supporters.bar.subscribe", { reason }) : t("supporters.bar.tip", { reason });
}
