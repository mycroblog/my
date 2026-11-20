// SPDX-License-Identifier: AGPL-3.0-only

// The cost items as a holder of `manage_finance` records them (ADR-0067 §2, §3; FR-901): every item that is not
// deleted, newest first, at /admin/finance/costs, each its name and provider, how often it is paid, its price today
// and whether it ends, and the way to record one; and a new item at /admin/finance/costs/new: its name, provider and
// category, paid once on a date or every N weeks, months or years from a first payment to an optional last one, its
// price in a currency, and a memo that is never published. What the form finds wrong before sending, and what the
// server refuses, stands under the field it is about; what names none, beside the button.

import {
  amountFits,
  costCategories,
  costIntervalLimit,
  costMemoLimit,
  costNameLimit,
  costPriceOn,
  costSchedules,
  loadCostItems,
  useCostItemActions,
  useCostItems,
  type AppData,
  type CostCategory,
  type CostItem,
  type CostItemDraft,
  type CostSchedule,
} from "@my/app-data";
import {
  Button,
  DateTimeField,
  FeedNotice,
  FieldError,
  focusRefusedField,
  FormAlert,
  LinkButton,
  MoreRecords,
  NumberField,
  RecordTable,
  Row,
  ScreenStack,
  ScreenTitle,
  Select,
  Stack,
  StateMark,
  TextButton,
  TextField,
  type RecordColumn,
  type RecordRow,
} from "@my/design-system";
import { useId, useState, type ReactNode } from "react";
import { fieldError, placeRefusal, type Refused } from "../../field-errors";
import { moneyText } from "../../finance-totals";
import type { StaffUnderProps } from "../entries";
import { errorOf, Field, ReadNotice, useStaffStrings, type T } from "../parts";
import { FinanceBack, financePaths, scheduleWords, currencyOptions, useDay, useTodayDate } from "./parts";

// ---- The list ---------------------------------------------------------------------------------------------------------

type Column = "schedule" | "price" | "end";

/** Whether `item` has paid its last payment by `today`, or is paid once and was. */
const ended = (item: CostItem, today: string) =>
  item.schedule === "once" ? item.starts_on < today : item.ends_on !== undefined && item.ends_on < today;

/** Where an item stands: paid once (on a date), ended (on its last date), ending, or going on. */
function EndMark({ item, today }: { item: CostItem; today: string }) {
  const { t } = useStaffStrings();
  const day = useDay();
  if (item.schedule === "once")
    return <StateMark state="expired">{t("staff_finance.costs.once_on", { date: day(item.starts_on) })}</StateMark>;
  if (item.ends_on === undefined) return <StateMark state="active">{t("staff_finance.costs.going_on")}</StateMark>;
  return ended(item, today) ? (
    <StateMark state="expired">{t("staff_finance.costs.ended", { date: day(item.ends_on) })}</StateMark>
  ) : (
    <StateMark state="ending">{t("staff_finance.costs.ends", { date: day(item.ends_on) })}</StateMark>
  );
}

/** The cost items, newest first, and Record a cost. */
export function CostItemsScreen({ go }: StaffUnderProps): ReactNode {
  const { t, locale } = useStaffStrings();
  const list = useCostItems();
  const today = useTodayDate();
  const head = (
    <ScreenTitle
      back={<FinanceBack to="root" go={go} />}
      actions={
        <LinkButton kind="primary" size="timeline" href={financePaths.newCost}>
          {t("staff_finance.costs.record")}
        </LinkButton>
      }
    >
      {t("staff_finance.costs")}
    </ScreenTitle>
  );
  if (list.status !== "success" && list.items.length === 0)
    return (
      <ScreenStack>
        {head}
        <ReadNotice
          status={list.status === "error" ? "error" : "pending"}
          loading={t("staff_finance.costs.loading")}
          failed={t("staff_finance.costs.failed")}
          retry={list.refetch}
        />
      </ScreenStack>
    );
  const columns: RecordColumn<Column>[] = [
    { key: "schedule", label: t("staff_finance.costs.column.schedule"), bare: true },
    { key: "end", label: t("staff_finance.costs.column.end"), kind: "fit", beside: true },
    // Last, and beside the head on a phone too, so that the prices line up at the row's end.
    { key: "price", label: t("staff_finance.costs.column.price"), kind: "figures", beside: true },
  ];
  const rows = list.items.map((item): RecordRow<Column> => {
    const price = costPriceOn(item, today)?.price;
    return {
      id: item.id,
      head: item.name,
      caption: item.provider,
      href: financePaths.cost(item.id),
      cells: {
        schedule: scheduleWords(t, item),
        price: price === undefined ? "" : moneyText(locale, price),
        end: <EndMark item={item} today={today} />,
      },
    };
  });
  return (
    <ScreenStack>
      {head}
      <RecordTable
        label={t("staff_finance.costs")}
        head={{ label: t("staff_finance.costs.column.item") }}
        columns={columns}
        rows={rows}
        even
        empty={<FeedNotice start>{t("staff_finance.costs.empty")}</FeedNotice>}
      />
      {list.failedNextPage ? (
        <FeedNotice alert start action={<TextButton onPress={list.fetchNextPage}>{t("staff.retry")}</TextButton>}>
          {t("staff_finance.costs.failed_more")}
        </FeedNotice>
      ) : (
        list.hasNextPage && <MoreRecords onPress={list.fetchNextPage} busy={list.isFetchingNextPage} />
      )}
    </ScreenStack>
  );
}

/** Reads the first page of the cost items, for the route's loader. */
export const loadCostItemsScreen = (data: AppData): Promise<void> => loadCostItems(data);

// ---- A new item -------------------------------------------------------------------------------------------------------

/** The inputs of the form of a new item. */
type Input =
  "name" | "provider" | "category" | "schedule" | "interval" | "starts" | "ends" | "amount" | "currency" | "memo";

/** The members of the request each input answers to. */
const members = {
  name: "name",
  provider: "provider",
  category: "category",
  interval: "interval",
  starts_on: "starts",
  ends_on: "ends",
  price: "amount",
  "price.amount": "amount",
  "price.currency": "currency",
  memo: "memo",
} as const satisfies Record<string, Input>;

/** The order of the inputs on the form, which the focus of a refusal follows. */
const order: readonly Input[] = [
  "name",
  "provider",
  "category",
  "interval",
  "starts",
  "ends",
  "amount",
  "currency",
  "memo",
];

type Draft = {
  name: string;
  provider: string;
  category: CostCategory;
  schedule: CostSchedule;
  interval: number | null;
  startsOn: string;
  endsOn: string;
  amount: string;
  currency: string;
  memo: string;
};

/** What the form finds wrong with `draft` before sending, input by input. */
function faultsOf(t: T, draft: Draft): Partial<Record<Input, string>> {
  const faults: Partial<Record<Input, string>> = {};
  if (draft.name.trim() === "") faults.name = t("staff_finance.cost.name.blank");
  if (draft.provider.trim() === "") faults.provider = t("staff_finance.cost.provider.blank");
  if (draft.schedule !== "once" && (draft.interval === null || draft.interval < 1))
    faults.interval = t("staff_finance.cost.interval.blank", { max: costIntervalLimit });
  if (draft.startsOn === "") faults.starts = t("staff_finance.cost.starts.blank");
  if (draft.schedule !== "once" && draft.endsOn !== "" && draft.startsOn !== "" && draft.endsOn < draft.startsOn)
    faults.ends = t("staff_finance.cost.ends.before");
  if (!amountFits(draft.amount, draft.currency)) faults.amount = t("staff_finance.cost.amount.invalid");
  return faults;
}

/** The request of `draft`, which the form checked. */
const requestOf = (draft: Draft): CostItemDraft => ({
  schedule: draft.schedule,
  interval: draft.schedule === "once" ? 1 : (draft.interval ?? 1),
  starts_on: draft.startsOn,
  ...(draft.schedule !== "once" && draft.endsOn !== "" ? { ends_on: draft.endsOn } : {}),
  name: draft.name.trim(),
  provider: draft.provider.trim(),
  category: draft.category,
  ...(draft.memo.trim() === "" ? {} : { memo: draft.memo }),
  price: { amount: draft.amount.trim(), currency: draft.currency },
});

/** The currency a new item starts with: the one of the newest item recorded, else the reader's language's. */
function useStartCurrency(): string {
  const { locale } = useStaffStrings();
  const newest = useCostItems().items[0];
  if (newest !== undefined) return costPriceOn(newest, newest.starts_on)?.price.currency ?? "USD";
  return locale.startsWith("ja") ? "JPY" : "USD";
}

/** The form of a new cost item; recorded, the reader goes on to its screen. */
export function CostItemCreateScreen({ go }: StaffUnderProps): ReactNode {
  const { t } = useStaffStrings();
  const { create } = useCostItemActions();
  const currencies = currencyOptions();
  const today = useTodayDate();
  const ids = useId();
  const idOf = (input: Input) => `${ids}-${input}`;
  const [draft, setDraft] = useState<Draft>(() => ({
    name: "",
    provider: "",
    category: "compute",
    schedule: "months",
    interval: 1,
    startsOn: today,
    endsOn: "",
    amount: "",
    currency: "",
    memo: "",
  }));
  const startCurrency = useStartCurrency();
  const currency = draft.currency === "" ? startCurrency : draft.currency;
  const [faults, setFaults] = useState<Partial<Record<Input, string>>>({});
  const [refused, setRefused] = useState<Refused<Input> | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const change = (next: Partial<Draft>) => {
    setDraft((d) => ({ ...d, ...next }));
    setFaults({});
    setRefused(undefined);
  };
  const errorAt = (input: Input) => faults[input] ?? fieldError(refused, input);
  const focusFirst = (wrong: (input: Input) => boolean) => {
    const first = order.find(wrong);
    if (first !== undefined)
      requestAnimationFrame(() => {
        focusRefusedField(idOf(first));
      });
  };
  const send = () => {
    if (busy) return;
    const shown = { ...draft, currency };
    const found = faultsOf(t, shown);
    if (Object.keys(found).length > 0) {
      setFaults(found);
      focusFirst((input) => found[input] !== undefined);
      return;
    }
    setBusy(true);
    create(requestOf(shown)).then(
      (item) => {
        go(financePaths.cost(item.id));
      },
      (e: unknown) => {
        setBusy(false);
        const placed = placeRefusal(e, members);
        setRefused(placed);
        const input = placed.field?.input;
        if (input !== undefined) focusFirst((i) => i === input);
      },
    );
  };
  const once = draft.schedule === "once";
  const failed = Object.keys(faults).length > 0 || refused !== undefined;
  return (
    <ScreenStack>
      <ScreenTitle back={<FinanceBack to="costs" go={go} />}>{t("staff_finance.costs.new")}</ScreenTitle>
      <Stack
        gap="screen-field-gap"
        as={
          <form
            noValidate
            onSubmit={(e) => {
              e.preventDefault();
              send();
            }}
          />
        }
      >
        <TextField
          id={idOf("name")}
          name="name"
          label={t("staff_finance.cost.name")}
          maxLength={costNameLimit}
          error={errorAt("name")}
          onChange={(name) => {
            change({ name });
          }}
        />
        <TextField
          id={idOf("provider")}
          name="provider"
          label={t("staff_finance.cost.provider")}
          maxLength={costNameLimit}
          error={errorAt("provider")}
          onChange={(provider) => {
            change({ provider });
          }}
        />
        <Field id={idOf("category")} label={t("staff_finance.cost.category")} error={errorAt("category")}>
          <Select
            id={idOf("category")}
            describedBy={errorOf(idOf("category"), errorAt("category"))}
            options={costCategories.map((c) => ({ value: c, label: t("finance.category", { category: c }) }))}
            value={draft.category}
            onChange={(value) => {
              change({ category: costCategories.find((c) => c === value) ?? "other" });
            }}
          />
        </Field>
        <Field id={idOf("schedule")} label={t("staff_finance.cost.schedule")}>
          <Row align="center" gap="screen-tab-gap">
            {!once && (
              <NumberField
                id={idOf("interval")}
                label={t("staff_finance.cost.interval")}
                describedBy={errorOf(idOf("interval"), errorAt("interval"))}
                invalid={errorAt("interval") !== undefined}
                value={draft.interval}
                min={1}
                max={costIntervalLimit}
                onChange={(interval) => {
                  change({ interval });
                }}
              />
            )}
            <Select
              id={idOf("schedule")}
              options={costSchedules.map((s) => ({
                value: s,
                label: t("staff_finance.cost.schedule.option", { schedule: s, count: draft.interval ?? 1 }),
              }))}
              value={draft.schedule}
              onChange={(value) => {
                change({ schedule: costSchedules.find((s) => s === value) ?? "months" });
              }}
            />
          </Row>
          {!once && errorAt("interval") !== undefined && (
            <FieldError id={`${idOf("interval")}-error`}>{errorAt("interval") ?? ""}</FieldError>
          )}
        </Field>
        <Field
          id={idOf("starts")}
          label={t(once ? "staff_finance.cost.paid_on" : "staff_finance.cost.starts")}
          error={errorAt("starts")}
        >
          <DateTimeField
            id={idOf("starts")}
            date
            label={t(once ? "staff_finance.cost.paid_on" : "staff_finance.cost.starts")}
            value={draft.startsOn}
            invalid={errorAt("starts") !== undefined}
            describedBy={errorOf(idOf("starts"), errorAt("starts"))}
            onChange={(startsOn) => {
              change({ startsOn });
            }}
          />
        </Field>
        {!once && (
          <Field id={idOf("ends")} label={t("staff_finance.cost.ends")} error={errorAt("ends")}>
            <DateTimeField
              id={idOf("ends")}
              date
              label={t("staff_finance.cost.ends")}
              value={draft.endsOn}
              min={draft.startsOn === "" ? undefined : draft.startsOn}
              invalid={errorAt("ends") !== undefined}
              describedBy={errorOf(idOf("ends"), errorAt("ends"))}
              onChange={(endsOn) => {
                change({ endsOn });
              }}
            />
          </Field>
        )}
        <Row wrap align="start" gap="screen-tab-gap">
          <TextField
            id={idOf("amount")}
            name="amount"
            label={t("staff_finance.cost.amount")}
            inputMode="decimal"
            figures
            error={errorAt("amount")}
            onChange={(amount) => {
              change({ amount });
            }}
          />
          <Field id={idOf("currency")} label={t("staff_finance.cost.currency")} error={errorAt("currency")}>
            <Select
              id={idOf("currency")}
              describedBy={errorOf(idOf("currency"), errorAt("currency"))}
              options={currencies}
              value={currency}
              onChange={(value) => {
                change({ currency: value });
              }}
            />
          </Field>
        </Row>
        <TextField
          id={idOf("memo")}
          name="memo"
          label={t("staff_finance.cost.memo")}
          hint={t("staff_finance.cost.memo.hint")}
          multiline
          rows={3}
          maxLength={costMemoLimit}
          error={errorAt("memo")}
          onChange={(memo) => {
            change({ memo });
          }}
        />
        {failed && (
          <FormAlert regular>
            {refused?.field === undefined && refused?.reason !== undefined
              ? refused.reason
              : t("staff_finance.cost.not_recorded")}
          </FormAlert>
        )}
        <Row>
          <Button type="submit" kind="primary" busy={busy}>
            {t("staff_finance.costs.record_it")}
          </Button>
        </Row>
      </Stack>
    </ScreenStack>
  );
}
