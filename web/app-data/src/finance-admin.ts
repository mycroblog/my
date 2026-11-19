// SPDX-License-Identifier: AGPL-3.0-only
// The finance as a holder of `manage_finance` keeps it (ADR-0067, ADR-0068; FR-901–FR-905, FR-917): the cost items,
// once or every N weeks, months or years, with their price revisions, each effective from its date, their end and
// their deletion; the totals of the costs and of the income (gross, fee, net and the balance) by week, month or year;
// the income's money movements with who paid each; the billing imports of each cloud provider, with running one now;
// and what the server publishes of its finance, the instance settings a holder of `manage_settings` changes. A write
// puts the item the server answered into the cache and reads again every total it changes, the public ones and the
// funding meter of the instance among them.
import { ProblemError, type components } from "@mycroblog/mykit";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import type { FinanceQuery } from "./finance";
import { instanceQuery } from "./instance";
import { pagedQuery, usePagedList, type PagedList } from "./paged";
import { useAppData } from "./provider";

type Schemas = components["schemas"];

/** A cost item: what is paid, to whom, how often from when to when, and every revision of its price. */
export type CostItem = Schemas["CostItem"];
/** How often a cost item is paid: once, or every `interval` weeks, months or years. */
export type CostSchedule = Schemas["CostSchedule"];
/** A cost item as it is recorded. */
export type CostItemDraft = Schemas["CostItemCreateRequest"];
/** What names a cost item: its name, provider, category and memo; an omitted member keeps its value. */
export type CostItemLabel = Schemas["CostItemLabelRequest"];
/** A price of a cost item and the date the payments on or after it take it. */
export type CostPriceRevision = Schemas["CostPriceRevision"];
/** The totals of the income as a holder of `manage_finance` reads them: the gross, the fee, the net, the balance. */
export type AdminIncomeTotals = Schemas["AdminIncomeTotals"];
/** A period of the administrators' income totals. */
export type AdminIncomePeriodTotal = Schemas["AdminIncomePeriodTotal"];
/** A money movement of the payment provider, with who paid it. */
export type IncomeMovement = Schemas["IncomeMovement"];
/** What a money movement is: a payment, a refund, a chargeback, its reversal, or a fee. */
export type IncomeKind = Schemas["IncomeKind"];
/** The state of the imports of one billing source. */
export type BillingImport = Schemas["BillingImport"];
/** A billing source: a cloud provider whose usage charges the server imports. */
export type BillingSourceName = Schemas["BillingSourceName"];
/** An instance setting as a holder of `manage_settings` reads it. */
export type AdminSetting = Schemas["AdminSetting"];

/** The schedules of a cost item, once first. */
export const costSchedules = ["once", "weeks", "months", "years"] as const satisfies readonly CostSchedule[];
/** The kinds of a money movement, in the server's order. */
export const incomeKinds = [
  "payment",
  "refund",
  "chargeback",
  "chargeback_reversal",
  "fee",
] as const satisfies readonly IncomeKind[];
/** The most every N of a repeating cost item. */
export const costIntervalLimit = 99;
/** The most characters of a cost item's name and provider, and of its memo. */
export const costNameLimit = 100;
export const costMemoLimit = 10_000;

/** Whether `error` is the server's answer of `status`. */
export const answered = (error: unknown, status: number) => error instanceof ProblemError && error.status === status;

/** What a screen reads of one thing: it, once read, and where its read is. */
export type OneRead<T> = {
  readonly value: T | undefined;
  readonly status: "pending" | "error" | "success";
  /** The server has no such thing (404). */
  readonly missing: boolean;
  /** The server refused the reader (403): their roles no longer have the permission. */
  readonly forbidden: boolean;
  readonly refetch: () => void;
};

/** What a query gives a screen of one thing (a list read whole, a set of totals). */
export function oneRead<T>(query: {
  data: T | undefined;
  status: "pending" | "error" | "success";
  error: unknown;
  refetch: () => unknown;
}): OneRead<T> {
  return {
    value: query.data,
    status: query.status,
    missing: answered(query.error, 404),
    forbidden: answered(query.error, 403),
    refetch: () => {
      void query.refetch();
    },
  };
}

// ---- The cost items ---------------------------------------------------------------------------------------------------

/** The cost items that are not deleted, newest first. */
export function costItemsQuery({ api, queries }: AppData) {
  return pagedQuery<CostItem>(
    queries.queryOptions("get", "/admin/cost_items").queryKey,
    async (cursor, signal) =>
      (await api.GET("/admin/cost_items", { params: { query: cursor === undefined ? {} : { cursor } }, signal })).data,
  );
}

/** One cost item. */
export function costItemQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/admin/cost_items/{id}", { params: { path: { id } } });
}

/** The cost items, newest first, a page at a time. */
export function useCostItems(): PagedList<CostItem> {
  return usePagedList(costItemsQuery(useAppData()));
}

/** The cost item of `id`. */
export function useCostItem(id: string): OneRead<CostItem> {
  return oneRead(useQuery(costItemQuery(useAppData(), id)));
}

/** Reads the first page of the cost items, for a route's loader. */
export async function loadCostItems(data: AppData): Promise<void> {
  await data.queryClient.infiniteQuery(costItemsQuery(data)).catch(() => undefined);
}

/** Reads the cost item of `id`, for a route's loader. */
export async function loadCostItem(data: AppData, id: string): Promise<void> {
  await data.queryClient.query(costItemQuery(data, id)).catch(() => undefined);
}

/** Every key of the totals and of the meter, which a change of the costs or of what is published changes. */
const totalsPaths = new Set([
  "/admin/cost_totals",
  "/admin/income_totals",
  "/finance/cost_totals",
  "/finance/income_totals",
  "/instance/cost_totals",
  "/instance/income_totals",
]);

/** Reads again every total of the costs and the income, and the instance with its funding meter. */
function totalsChanged(data: AppData): void {
  void data.queryClient.invalidateQueries({ predicate: ({ queryKey }) => totalsPaths.has(String(queryKey[1])) });
  void data.queryClient.invalidateQueries({ queryKey: instanceQuery(data).queryKey });
}

/** Puts the item a write answered into the cache, and reads again the list and every total it changes. */
async function itemWritten(data: AppData, item: CostItem | undefined): Promise<CostItem> {
  if (item === undefined) throw new Error("app-data: a write of a cost item answered without the item");
  const key = costItemQuery(data, item.id).queryKey;
  await data.queryClient.cancelQueries({ queryKey: key });
  data.queryClient.setQueryData(key, item);
  void data.queryClient.invalidateQueries({ queryKey: costItemsQuery(data).queryKey });
  totalsChanged(data);
  return item;
}

/**
 * What a holder of `manage_finance` does to the cost items; each resolves with the item as the server answered it, and
 * rejects with the server's problem, a 422 naming the member it refused.
 */
export type CostItemActions = {
  /** Records a cost item. */
  readonly create: (draft: CostItemDraft) => Promise<CostItem>;
  /** Changes what names the item of `id`. */
  readonly relabel: (id: string, label: CostItemLabel) => Promise<CostItem>;
  /** Records the price of the payments of the item of `id` on or after `effective_on`. */
  readonly revise: (id: string, revision: Schemas["CostPriceRevisionRequest"]) => Promise<CostItem>;
  /** Ends the item of `id` on `endsOn` (its last payment date, included), or makes it endless again (undefined). */
  readonly end: (id: string, endsOn: string | undefined) => Promise<CostItem>;
  /** Deletes the item of `id` from every period, past ones included; one the server no longer has counts as deleted. */
  readonly remove: (id: string) => Promise<void>;
};

export function costItemActions(data: AppData): CostItemActions {
  const path = (id: string) => ({ params: { path: { id } } });
  return {
    create: async (draft) => itemWritten(data, (await data.api.POST("/admin/cost_items", { body: draft })).data),
    relabel: async (id, label) =>
      itemWritten(data, (await data.api.PATCH("/admin/cost_items/{id}", { ...path(id), body: label })).data),
    revise: async (id, revision) =>
      itemWritten(
        data,
        (await data.api.POST("/admin/cost_items/{id}/price_revisions", { ...path(id), body: revision })).data,
      ),
    end: async (id, endsOn) =>
      itemWritten(
        data,
        (
          await data.api.PUT("/admin/cost_items/{id}/end", {
            ...path(id),
            body: endsOn === undefined ? {} : { ends_on: endsOn },
          })
        ).data,
      ),
    remove: async (id) => {
      await data.api.DELETE("/admin/cost_items/{id}", path(id)).catch((e: unknown) => {
        if (!answered(e, 404)) throw e;
      });
      data.queryClient.removeQueries({ queryKey: costItemQuery(data, id).queryKey, exact: true });
      void data.queryClient.invalidateQueries({ queryKey: costItemsQuery(data).queryKey });
      totalsChanged(data);
    },
  };
}

/** What a holder of `manage_finance` does to the cost items, for a screen. */
export function useCostItemActions(): CostItemActions {
  return costItemActions(useAppData());
}

/**
 * The price of `item` on `date` (a calendar date): the revision of the latest effective date on or before it, the one
 * recorded last among those of one date; the first price before the item's start.
 */
export function costPriceOn(item: Pick<CostItem, "prices">, date: string): CostPriceRevision | undefined {
  let found: CostPriceRevision | undefined;
  for (const revision of item.prices)
    if (revision.effective_on <= date && (found === undefined || revision.effective_on >= found.effective_on))
      found = revision;
  return found ?? item.prices[0];
}

// ---- The totals -------------------------------------------------------------------------------------------------------

const totalsQueryOf = ({ granularity, from, to, basis }: FinanceQuery) => ({
  granularity,
  from,
  to,
  ...(basis === undefined ? {} : { basis }),
});

/** The totals of the costs as a holder of `manage_finance` reads them. */
export function adminCostTotalsQuery({ queries }: AppData, q: FinanceQuery) {
  return queries.queryOptions("get", "/admin/cost_totals", { params: { query: totalsQueryOf(q) } });
}

/** The totals of the income as a holder of `manage_finance` reads them. */
export function adminIncomeTotalsQuery({ queries }: AppData, q: FinanceQuery) {
  return queries.queryOptions("get", "/admin/income_totals", { params: { query: totalsQueryOf(q) } });
}

/** The totals of the costs of `q`. */
export function useAdminCostTotals(q: FinanceQuery): OneRead<Schemas["CostTotals"]> {
  return oneRead(useQuery({ ...adminCostTotalsQuery(useAppData(), q), placeholderData: keepPreviousData }));
}

/** The totals of the income of `q`. */
export function useAdminIncomeTotals(q: FinanceQuery): OneRead<AdminIncomeTotals> {
  return oneRead(useQuery({ ...adminIncomeTotalsQuery(useAppData(), q), placeholderData: keepPreviousData }));
}

// ---- The money movements ----------------------------------------------------------------------------------------------

/** Which money movements a list shows: of one kind, or of every kind. */
export type IncomeMovementQuery = { readonly kind?: IncomeKind | undefined };

/** The money movements of `q`, newest first. */
export function incomeMovementsQuery({ api, queries }: AppData, q: IncomeMovementQuery) {
  const query = q.kind === undefined ? {} : { kind: q.kind };
  return pagedQuery<IncomeMovement>(
    queries.queryOptions("get", "/admin/income_movements", { params: { query } }).queryKey,
    async (cursor, signal) =>
      (
        await api.GET("/admin/income_movements", {
          params: { query: cursor === undefined ? query : { ...query, cursor } },
          signal,
        })
      ).data,
  );
}

/** The money movements of `q`, newest first, a page at a time. */
export function useIncomeMovements(q: IncomeMovementQuery): PagedList<IncomeMovement> {
  return usePagedList(incomeMovementsQuery(useAppData(), q));
}

// ---- The billing imports ----------------------------------------------------------------------------------------------

/** The state of the imports of every billing source. */
export function billingImportsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/billing_imports");
}

/** The state of the imports of every billing source, in the order of the configuration. */
export function useBillingImports(): OneRead<readonly BillingImport[]> {
  const read = oneRead(useQuery(billingImportsQuery(useAppData())));
  return { ...read, value: read.value?.items };
}

/** Reads the state of the billing imports, for a route's loader. */
export async function loadBillingImports(data: AppData): Promise<void> {
  await data.queryClient.query(billingImportsQuery(data)).catch(() => undefined);
}

/**
 * What came of asking for an import now: queued; refused because one ran in the last UTC hour (`too-soon`, a 429); or
 * refused because the configuration no longer enables the source (`unknown`, a 404). Any other failure rejects.
 */
export type ImportRun = "queued" | "too-soon" | "unknown";

/** Asks for an import of `source` now, and reads the state of the imports again. */
export async function runBillingImport(data: AppData, source: BillingSourceName): Promise<ImportRun> {
  const run: ImportRun = await data.api
    .POST("/admin/billing_imports/{source}/runs", { params: { path: { source } } })
    .then(
      () => "queued" as const,
      (e: unknown) => {
        if (answered(e, 429)) return "too-soon";
        if (answered(e, 404)) return "unknown";
        throw e;
      },
    );
  void data.queryClient.invalidateQueries({ queryKey: billingImportsQuery(data).queryKey });
  return run;
}

/** Asks for an import of a source now, for a screen. */
export function useBillingImportRun(): (source: BillingSourceName) => Promise<ImportRun> {
  const data = useAppData();
  return (source) => runBillingImport(data, source);
}

// ---- What the server publishes ----------------------------------------------------------------------------------------

/** Every instance setting, for a holder of `manage_settings`. */
export function adminSettingsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/settings");
}

/**
 * The instance settings of `keys`, by key, for a holder of `manage_settings`; nothing is read while `enabled` is false
 * (a reader without it).
 */
export function useAdminSettings<K extends AdminSetting["key"]>(
  keys: readonly K[],
  enabled = true,
): OneRead<Readonly<Record<K, AdminSetting>>> {
  const read = oneRead(useQuery({ ...adminSettingsQuery(useAppData()), enabled }));
  const items = read.value?.items;
  const found = items === undefined ? undefined : keys.map((key) => items.find((s) => s.key === key));
  return {
    ...read,
    value:
      found === undefined || found.some((s) => s === undefined)
        ? undefined
        : (Object.fromEntries(found.map((s) => [s?.key, s])) as Record<K, AdminSetting>),
  };
}

/** Reads every instance setting, for a route's loader. */
export async function loadAdminSettings(data: AppData): Promise<void> {
  await data.queryClient.query(adminSettingsQuery(data)).catch(() => undefined);
}

/**
 * Changes the settings of `values` together, all of them or none: the cache then holds what the server answered, and
 * the instance (what it publishes, its funding meter) and every total are read again. Rejects with the server's
 * problem, a 422 naming the key it refused.
 */
export async function saveAdminSettings(data: AppData, values: Readonly<Record<string, string>>): Promise<void> {
  const { data: list } = await data.api.PATCH("/admin/settings", { body: { values: { ...values } } });
  const key = adminSettingsQuery(data).queryKey;
  await data.queryClient.cancelQueries({ queryKey: key });
  if (list !== undefined) data.queryClient.setQueryData(key, list);
  totalsChanged(data);
}

/** Changes instance settings together, for a screen. */
export function useSaveAdminSettings(): (values: Readonly<Record<string, string>>) => Promise<void> {
  const data = useAppData();
  return (values) => saveAdminSettings(data, values);
}
