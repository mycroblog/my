// SPDX-License-Identifier: AGPL-3.0-only
// The instance's costs (ADR-0067, manage_finance): the cost items, newest first, and the form that records one paid
// once or every N weeks, months or years; changing an item's label, revising its price from a date on, setting or
// removing its end, and canceling it; the totals by week, month or year, amortized (the default) or in cash, with
// the rules that cut their periods; and the state of the billing imports (ADR-0068), which an administrator can run
// now; and the income that the payment provider records (ADR-0073 §6): its totals and its money movements, which
// nobody changes here. Amounts are decimal strings in the API and are formatted with Intl.NumberFormat in the item's currency
// (ADR-0067 §10). Every change goes to the audit log.
import { useMutation, useQueryClient, useSuspenseInfiniteQuery, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { type CostBasis, type CostCategory, type CostGranularity, type CostTotals, utcDate } from "../../costs";
import { type PagedList, type PagedPages, toPagedList } from "../../moderation";
import {
  type AdminIncomeTotals,
  adminIncomeTotalsQueryOptions,
  type BillingImport,
  billingImportsQueryOptions,
  cancelCostItemMutation,
  type CostItem,
  costItemsQueryOptions,
  costMemoLimit,
  costNameLimit,
  type CostSchedule,
  costTotalsQueryOptions,
  createCostItemMutation,
  type IncomeMovement,
  type IncomeMovementFilter,
  incomeMovementsQueryOptions,
  maxCostInterval,
  refreshBillingImports,
  refreshCosts,
  reviseCostItemPriceMutation,
  runBillingImportMutation,
  setCostItemEndMutation,
  updateCostItemMutation,
} from "../api/finance";
import { characterCount } from "./languages";

/** The cost items that are not canceled, newest first. */
export function useCostItems(): PagedList<CostItem> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(costItemsQueryOptions(api)));
}

/** The totals of the shown periods up to `today`. */
export function useCostTotals(granularity: CostGranularity, basis: CostBasis, today: string): CostTotals {
  const api = useApi();
  return useSuspenseQuery(costTotalsQueryOptions(api, granularity, basis, today)).data;
}

/** The income totals of the shown periods up to `today`, with the gross, the fees, the balance and the members of each plan. */
export function useAdminIncomeTotals(granularity: CostGranularity, basis: CostBasis, today: string): AdminIncomeTotals {
  const api = useApi();
  return useSuspenseQuery(adminIncomeTotalsQueryOptions(api, granularity, basis, today)).data;
}

/** The payment provider's money movements of the income, newest first, narrowed by the filter. */
export function useIncomeMovements(filter: IncomeMovementFilter): PagedPages<IncomeMovement> {
  const api = useApi();
  return toPagedList(useSuspenseInfiniteQuery(incomeMovementsQueryOptions(api, filter)));
}

/** The dates of the movements' filter as typed: empty is no date. */
export type MovementDatesDraft = { from: string; to: string };

/**
 * The dates of the movements' filter, typed and applied at once (a date field changes its value while the year is
 * typed, digit by digit). The draft follows the filter in the URL when it changes there (back and forward, clearing
 * the filters), as react.dev's "Adjusting some state when a prop changes" does; a last date before the first is not
 * applied and says why.
 */
export function useMovementDates(filter: IncomeMovementFilter, onFilterChange: (filter: IncomeMovementFilter) => void) {
  const t = useT();
  const [shown, setShown] = useState({ from: filter.from, to: filter.to });
  const [draft, setDraft] = useState<MovementDatesDraft>({ from: filter.from ?? "", to: filter.to ?? "" });
  if (shown.from !== filter.from || shown.to !== filter.to) {
    setShown({ from: filter.from, to: filter.to });
    setDraft({ from: filter.from ?? "", to: filter.to ?? "" });
  }
  const toError =
    draft.from !== "" && draft.to !== "" && draft.to < draft.from
      ? t("client:admin.finance.movements.filter.to_before_from")
      : undefined;
  return {
    draft,
    change: (patch: Partial<MovementDatesDraft>) => {
      setDraft((current) => ({ ...current, ...patch }));
    },
    toError,
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (toError !== undefined) return;
      onFilterChange({
        ...filter,
        from: draft.from === "" ? undefined : draft.from,
        to: draft.to === "" ? undefined : draft.to,
      });
    },
  };
}

/** The state of the imports of every billing source, in the order of billing_sources. */
export function useBillingImports(): BillingImport[] {
  const api = useApi();
  return useSuspenseQuery(billingImportsQueryOptions(api)).data.items;
}

/** Formats a calendar date of the API (`YYYY-MM-DD`): the same day in every time zone. */
export function useFormatDay(): (date: string) => string {
  const intl = useIntl();
  return (date) => intl.formatDate(utcDate(date), { dateStyle: "medium", timeZone: "UTC" });
}

const amountPattern = /^[0-9]+(\.[0-9]+)?$/u;
const currencyPattern = /^[A-Z]{3}$/u;

/** A price as the forms take it: the amount as typed and the ISO 4217 code. */
export type PriceDraft = { amount: string; currency: string };

/** Why a price cannot be sent, localized; undefined when it can. The server checks the digits of the currency. */
function usePriceErrors() {
  const t = useT();
  return (price: PriceDraft): { amount?: string; currency?: string } => ({
    ...(amountPattern.test(price.amount.trim()) ? {} : { amount: t("client:admin.finance.amount.invalid") }),
    ...(currencyPattern.test(price.currency.trim()) ? {} : { currency: t("client:admin.finance.currency.invalid") }),
  });
}

/** The label of an item as the forms take it. */
export type LabelDraft = { name: string; provider: string; category: CostCategory; memo: string };

/** Why a label cannot be sent, localized, by field. */
function useLabelErrors() {
  const t = useT();
  return (label: LabelDraft): { name?: string; provider?: string; memo?: string } => {
    const line = (value: string) => {
      if (value.trim() === "") return t("client:admin.finance.required");
      if (characterCount(value.trim()) > costNameLimit)
        return t("client:admin.finance.too_long", { max: String(costNameLimit) });
      return undefined;
    };
    const name = line(label.name);
    const provider = line(label.provider);
    const memo =
      characterCount(label.memo) > costMemoLimit
        ? t("client:admin.finance.too_long", { max: String(costMemoLimit) })
        : undefined;
    return {
      ...(name === undefined ? {} : { name }),
      ...(provider === undefined ? {} : { provider }),
      ...(memo === undefined ? {} : { memo }),
    };
  };
}

type NewCostDraft = LabelDraft &
  PriceDraft & { schedule: CostSchedule; interval: string; startsOn: string; endsOn: string };

function emptyDraft(currency: string): NewCostDraft {
  return {
    name: "",
    provider: "",
    category: "other",
    memo: "",
    schedule: "months",
    interval: "1",
    startsOn: "",
    endsOn: "",
    amount: "",
    currency,
  };
}

/**
 * The form of a new cost item; the currency starts as that of the newest item. It empties once the list shows the item,
 * and gives what it did to `onCreated` (the list's result).
 */
export function useNewCostItemForm(defaultCurrency: string, onCreated: (message: string) => void) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const labelErrors = useLabelErrors();
  const priceErrors = usePriceErrors();
  const [draft, setDraft] = useState<NewCostDraft>(() => emptyDraft(defaultCurrency));
  const [submitted, setSubmitted] = useState(false);
  const create = useMutation({
    ...createCostItemMutation(api),
    onSuccess: async (item) => {
      await refreshCosts(queryClient);
      setDraft((current) => emptyDraft(current.currency));
      setSubmitted(false);
      onCreated(t("client:admin.finance.created", { name: item.name }));
    },
  });
  const errorsOf = (value: NewCostDraft) => {
    const interval = Number(value.interval);
    const intervalError =
      value.schedule !== "once" && !(Number.isInteger(interval) && interval >= 1 && interval <= maxCostInterval)
        ? t("client:admin.finance.interval.invalid", { max: String(maxCostInterval) })
        : undefined;
    const startsOn = value.startsOn === "" ? t("client:admin.finance.required") : undefined;
    const endsOn =
      value.endsOn !== "" && value.startsOn !== "" && value.endsOn < value.startsOn
        ? t("client:admin.finance.ends_on.before_start")
        : undefined;
    return {
      ...labelErrors(value),
      ...priceErrors(value),
      ...(intervalError === undefined ? {} : { interval: intervalError }),
      ...(startsOn === undefined ? {} : { startsOn }),
      ...(endsOn === undefined ? {} : { endsOn }),
    };
  };
  const errors = submitted ? errorsOf(draft) : {};
  return {
    draft,
    change: (patch: Partial<NewCostDraft>) => {
      setDraft((current) => ({ ...current, ...patch }));
    },
    errors,
    /** Records the cost once the draft is valid; `onAdded` runs once it is recorded (to close the add panel). */
    submit: (event: SyntheticEvent<HTMLFormElement>, onAdded?: () => void) => {
      event.preventDefault();
      setSubmitted(true);
      if (create.isPending || Object.keys(errorsOf(draft)).length > 0) return;
      create.mutate(
        {
          schedule: draft.schedule,
          interval: draft.schedule === "once" ? 1 : Number(draft.interval),
          starts_on: draft.startsOn,
          ...(draft.endsOn === "" || draft.schedule === "once" ? {} : { ends_on: draft.endsOn }),
          name: draft.name.trim(),
          provider: draft.provider.trim(),
          category: draft.category,
          memo: draft.memo.trim(),
          price: { amount: draft.amount.trim(), currency: draft.currency.trim() },
        },
        { onSuccess: () => onAdded?.() },
      );
    },
    isPending: create.isPending,
    /** Why recording failed, localized; the form keeps what was typed. */
    failure: create.isError ? errorMessage(create.error) : undefined,
  };
}

/** What the list does to an item; each resolves whether it worked. */
export type CostItemActions = {
  relabel: (item: CostItem, label: LabelDraft) => Promise<boolean>;
  revise: (item: CostItem, effectiveOn: string, price: PriceDraft) => Promise<boolean>;
  /** Sets the last payment date, or removes the end with undefined. */
  setEnd: (item: CostItem, endsOn: string | undefined) => Promise<boolean>;
  cancel: (item: CostItem) => Promise<boolean>;
  labelErrors: (label: LabelDraft) => { name?: string; provider?: string; memo?: string };
  priceErrors: (price: PriceDraft) => { amount?: string; currency?: string };
  /** Why the last change failed, localized. */
  error: string | undefined;
};

/**
 * The changes of the items in the list. Each says what it did through `onDone(item, status, removed)` once the list
 * shows it (the items and the totals are fetched again first), so that the screen can move focus and keep the status
 * next to the list.
 */
export function useCostItemActions(
  onDone: (item: CostItem, status: string, removed: boolean) => void,
): CostItemActions {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const labelErrors = useLabelErrors();
  const priceErrors = usePriceErrors();
  const [error, setError] = useState<string | undefined>(undefined);
  const update = useMutation({ ...updateCostItemMutation(api) });
  const revise = useMutation({ ...reviseCostItemPriceMutation(api) });
  const end = useMutation({ ...setCostItemEndMutation(api) });
  const cancel = useMutation({ ...cancelCostItemMutation(api) });
  const run = async (
    item: CostItem,
    work: () => Promise<unknown>,
    status: string,
    removed = false,
  ): Promise<boolean> => {
    setError(undefined);
    try {
      await work();
      await refreshCosts(queryClient);
      onDone(item, status, removed);
      return true;
    } catch (failure) {
      setError(errorMessage(failure));
      return false;
    }
  };
  return {
    relabel: (item, label) =>
      run(
        item,
        () =>
          update.mutateAsync({
            id: item.id,
            body: {
              name: label.name.trim(),
              provider: label.provider.trim(),
              category: label.category,
              memo: label.memo.trim(),
            },
          }),
        t("client:admin.finance.relabeled", { name: label.name.trim() }),
      ),
    revise: (item, effectiveOn, price) =>
      run(
        item,
        () =>
          revise.mutateAsync({
            id: item.id,
            body: {
              effective_on: effectiveOn,
              price: { amount: price.amount.trim(), currency: price.currency.trim() },
            },
          }),
        t("client:admin.finance.revised", { name: item.name }),
      ),
    setEnd: (item, endsOn) =>
      run(
        item,
        () => end.mutateAsync({ id: item.id, endsOn }),
        t(endsOn === undefined ? "client:admin.finance.end_removed" : "client:admin.finance.ended", {
          name: item.name,
        }),
      ),
    cancel: (item) =>
      run(item, () => cancel.mutateAsync(item.id), t("client:admin.finance.canceled", { name: item.name }), true),
    labelErrors,
    priceErrors,
    error,
  };
}

/** Running the import of a billing source now: at most once an hour (429), which the screen explains. */
export function useRunBillingImport() {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [result, setResult] = useState<{ status: string } | { error: string } | undefined>(undefined);
  const { mutationKey, mutationFn } = runBillingImportMutation(api);
  const run = useMutation({
    mutationKey,
    mutationFn: ({ source }: Pick<BillingImport, "source" | "provider">) => mutationFn(source),
    onMutate: () => {
      setResult(undefined);
    },
    onSuccess: async (_, { provider }) => {
      await refreshBillingImports(queryClient);
      setResult({ status: t("client:admin.finance.imports.queued", { provider }) });
    },
    onError: (failure) => {
      const tooSoon = failure instanceof ApiError && failure.status === 429;
      setResult({
        error: tooSoon
          ? t("client:admin.finance.imports.too_soon")
          : t("client:admin.finance.imports.failed", { reason: errorMessage(failure) }),
      });
    },
  });
  return {
    run: (source: BillingImport) => {
      run.mutate(source);
    },
    pendingSource: run.isPending ? run.variables.source : undefined,
    status: result !== undefined && "status" in result ? result.status : undefined,
    error: result !== undefined && "error" in result ? result.error : undefined,
  };
}
