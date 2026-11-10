// SPDX-License-Identifier: AGPL-3.0-only
// The tips on the first page of the supporters screen (ADR-0162 §4, §6, §17, manage_supporters): enabling them with
// the amounts users choose from, changing the amounts, and disabling them, in one form with one save. The form checks
// what it can before sending (1 to 6 amounts, written as numbers); the server keeps the rest (the range of each
// currency and the provider's smallest one-time charge), and its refusal is said in its words next to the field it is
// about. What a save did is the result of the form's action bar.
import type { FormActionResult } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  cacheSupporterTips,
  setSupporterTipsMutation,
  type SupporterTips,
  supporterTipsQueryOptions,
  type TipAmountsProblemReason,
  tipAmountsProblemReason,
} from "../api/supporters";

/** The most amounts the tips offer (AdminSupporterTipsRequest). */
export const maxTipAmounts = 6;

/** The fields of the form of the tips that a refusal can be about. */
type TipsField = "currency" | "amounts" | "form";

/** Why the tips cannot be saved, next to its field. */
export type TipsFieldError = { field: TipsField; message: string };

const reasonFields: Readonly<Record<TipAmountsProblemReason, TipsField>> = {
  count: "amounts",
  currency: "currency",
  range: "amounts",
};

const amountPattern = /^[0-9]+(\.[0-9]+)?$/u;

/** The amounts typed in the form, separated by commas or spaces (also the Japanese comma), as typed. */
function splitTipAmounts(text: string): string[] {
  return text
    .split(/[\s,、，]+/u)
    .map((part) => part.trim())
    .filter((part) => part !== "");
}

/** The amounts of the tips as the form shows them: "500, 1000, 3000". */
function amountsText(tips: SupporterTips): string {
  return tips.amounts.map((money) => money.amount).join(", ");
}

/** The currency the form starts with: that of the amounts, or the first one the provider takes for the tips. */
function startCurrency(tips: SupporterTips): string {
  return tips.amounts[0]?.currency ?? tips.limits[0]?.minimum.currency ?? "JPY";
}

/**
 * The form of the tips: whether they are enabled, the currency and the amounts. A save sends the tips enabled with the
 * amounts (sorted and without repeats), or disabled without them; saving what the server has changes nothing.
 */
export function useSupporterTipsForm() {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const tips = useSuspenseQuery(supporterTipsQueryOptions(api)).data;
  const [enabledDraft, setEnabledDraft] = useState<boolean | undefined>(undefined);
  const [currencyDraft, setCurrencyDraft] = useState<string | undefined>(undefined);
  const [amountsDraft, setAmountsDraft] = useState<string | undefined>(undefined);
  const [error, setError] = useState<TipsFieldError | undefined>(undefined);
  const [result, setResult] = useState<FormActionResult | undefined>(undefined);
  const save = useMutation(setSupporterTipsMutation(api));
  const enabled = enabledDraft ?? tips.enabled;
  const currency = currencyDraft ?? startCurrency(tips);
  const amounts = amountsDraft ?? amountsText(tips);
  const edited = () => {
    setResult(undefined);
    setError(undefined);
  };
  // A refusal about a field is said next to it; any other in the action bar.
  const fail = (next: TipsFieldError) => {
    if (next.field !== "form") {
      setError(next);
      return;
    }
    setResult({ tone: "danger", message: t("client:admin.supporters.tips.failed", { reason: next.message }) });
  };
  return {
    tips,
    enabled,
    setEnabled: (next: boolean) => {
      edited();
      setEnabledDraft(next);
    },
    currency,
    setCurrency: (next: string) => {
      edited();
      setCurrencyDraft(next);
    },
    amounts,
    setAmounts: (next: string) => {
      edited();
      setAmountsDraft(next);
    },
    /** The smallest and largest amount of the chosen currency on the provider; undefined for a currency it does not take. */
    limits: tips.limits.find((limit) => limit.minimum.currency === currency),
    /** Why the last save was refused, next to the field it is about. */
    error,
    submit: async (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (save.isPending) return;
      edited();
      let body: { enabled: false } | { enabled: true; amounts: { amount: string; currency: string }[] };
      if (!enabled) {
        if (!tips.enabled) {
          setResult({ tone: "success", message: t("client:admin.supporters.tips.unchanged") });
          return;
        }
        body = { enabled: false };
      } else {
        const typed = splitTipAmounts(amounts);
        if (typed.some((amount) => !amountPattern.test(amount))) {
          fail({ field: "amounts", message: t("client:admin.supporters.tips.amounts.invalid") });
          return;
        }
        const unique = [...new Set(typed)].toSorted((a, b) => Number(a) - Number(b));
        if (unique.length === 0 || unique.length > maxTipAmounts) {
          fail({
            field: "amounts",
            message: t("client:admin.supporters.tips.amounts.count", { max: String(maxTipAmounts) }),
          });
          return;
        }
        const same =
          tips.enabled &&
          unique.length === tips.amounts.length &&
          tips.amounts.every((money, i) => money.currency === currency && Number(money.amount) === Number(unique[i]));
        if (same) {
          setResult({ tone: "success", message: t("client:admin.supporters.tips.unchanged") });
          return;
        }
        body = { enabled: true, amounts: unique.map((amount) => ({ amount, currency })) };
      }
      try {
        const saved = await save.mutateAsync(body);
        await cacheSupporterTips(api, queryClient, saved);
        setEnabledDraft(undefined);
        setCurrencyDraft(undefined);
        setAmountsDraft(undefined);
        setResult({ tone: "success", message: t("client:admin.supporters.tips.saved") });
      } catch (failure) {
        const reason = tipAmountsProblemReason(failure);
        fail({ field: reason === undefined ? "form" : reasonFields[reason], message: errorMessage(failure) });
      }
    },
    isSaving: save.isPending,
    /** The result of the last save, for the form's action bar. */
    result,
  };
}
