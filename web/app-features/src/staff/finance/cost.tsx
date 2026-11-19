// SPDX-License-Identifier: AGPL-3.0-only

// One cost item as a holder of `manage_finance` keeps it, at /admin/finance/costs/{id} (ADR-0067 §2, §3; FR-901):
// its facts (its provider, its category, how often it is paid, from when to when, its price today, its memo); its
// prices, each said with the date it applies from, and a new price from a date; what names it, changed; its end, set,
// moved or taken away (a repeating item alone); and deleting it, from every period, past ones too, asked first. What a
// press did is said in the screen's status, which takes the focus; a refusal stands under the field it is about.

import {
  costCategories,
  costMemoLimit,
  costNameLimit,
  loadCostItem,
  useCostItem,
  useCostItemActions,
  type AppData,
  type CostCategory,
  type CostItem,
} from "@my/app-data";
import {
  Box,
  Button,
  ConfirmDialog,
  DateTimeField,
  FactList,
  FeedNotice,
  focusRefusedField,
  FormSection,
  LogEntry,
  LogList,
  ModalPresence,
  Row,
  ScreenStack,
  ScreenTitle,
  Select,
  Stack,
  TextField,
  TextLink,
  type Fact,
} from "@my/design-system";
import { useId, useRef, useState, type ReactNode } from "react";
import { fieldError, placeRefusal, type Refused } from "../../field-errors";
import { moneyText } from "../../finance-totals";
import type { StaffUnderProps } from "../entries";
import { errorOf, Field, ReadNotice, StatusRegion, Titled, useMoment, useStaffStrings, useStatus } from "../parts";
import { amountFits, FinanceBack, financePaths, scheduleWords, useDay, useTodayDate } from "./parts";

type Say = (words: string, alert?: boolean) => void;

/** The title of the screen of the cost item of `id`, its name; undefined until it is read. */
export function useCostItemTitle(id: string): string | undefined {
  return useCostItem(id).value?.name;
}

/** Reads the cost item of `id`, for the route's loader. */
export const loadCostItemScreen = (data: AppData, id: string): Promise<void> => loadCostItem(data, id);

/** The cost item of `id`, or why it shows none. */
export function CostItemScreen({ id, go }: StaffUnderProps & { id: string }): ReactNode {
  const { t } = useStaffStrings();
  const read = useCostItem(id);
  if (read.missing)
    return (
      <FeedNotice
        alert
        lead
        action={<TextLink href={financePaths.costs}>{t("staff_finance.cost.missing.all")}</TextLink>}
      >
        {t("staff_finance.cost.missing")}
      </FeedNotice>
    );
  if (read.forbidden) return <FeedNotice lead>{t("staff_finance.forbidden")}</FeedNotice>;
  if (read.value === undefined)
    return (
      <ReadNotice
        status={read.status === "error" ? "error" : "pending"}
        loading={t("staff_finance.cost.loading")}
        failed={t("staff_finance.cost.failed")}
        retry={read.refetch}
      />
    );
  return <CostItemView item={read.value} go={go} />;
}

function CostItemView({ item, go }: { item: CostItem; go: (href: string) => void }) {
  const region = useRef<HTMLDivElement>(null);
  const status = useStatus(region);
  return (
    <ScreenStack>
      <Stack gap="entry-line">
        <ScreenTitle back={<FinanceBack to="costs" go={go} />}>{item.name}</ScreenTitle>
        <StatusRegion said={status.said} region={region} />
      </Stack>
      <CostFacts item={item} />
      <Prices item={item} say={status.say} />
      <Label item={item} say={status.say} />
      {item.schedule !== "once" && <End item={item} say={status.say} />}
      <Delete
        item={item}
        onDeleted={() => {
          go(financePaths.costs);
        }}
        say={status.say}
      />
    </ScreenStack>
  );
}

/** The facts of an item. */
function CostFacts({ item }: { item: CostItem }) {
  const { t } = useStaffStrings();
  const day = useDay();
  const moment = useMoment();
  const facts: Fact[] = [
    { term: t("staff_finance.cost.provider"), value: item.provider },
    { term: t("staff_finance.cost.category"), value: t("finance.category", { category: item.category }) },
    { term: t("staff_finance.cost.schedule"), value: scheduleWords(t, item) },
    item.schedule === "once"
      ? { term: t("staff_finance.cost.paid_on"), value: day(item.starts_on) }
      : { term: t("staff_finance.cost.starts"), value: day(item.starts_on) },
    ...(item.schedule === "once"
      ? []
      : [
          {
            term: t("staff_finance.cost.ends"),
            value: item.ends_on === undefined ? t("staff_finance.cost.no_end") : day(item.ends_on),
          },
        ]),
    ...(item.memo === "" ? [] : [{ term: t("staff_finance.cost.memo"), value: item.memo }]),
    { term: t("staff_finance.cost.recorded"), value: moment(item.created_at) },
  ];
  return <FactList label={t("staff_finance.cost.facts")} items={facts} />;
}

/** Runs `write` once at a time; says `done` of its answer, or that it failed, unless `refuse` places the failure. */
function useWrite(say: Say) {
  const { t } = useStaffStrings();
  const [busy, setBusy] = useState<string | undefined>(undefined);
  const run = <A,>(
    kind: string,
    write: () => Promise<A>,
    done: (answer: A) => string,
    refuse?: (e: unknown) => boolean,
  ): void => {
    if (busy !== undefined) return;
    setBusy(kind);
    write().then(
      (answer) => {
        setBusy(undefined);
        say(done(answer));
      },
      (e: unknown) => {
        setBusy(undefined);
        if (refuse?.(e) !== true) say(t("staff_finance.cost.write_failed"), true);
      },
    );
  };
  return { busy, run };
}

/**
 * The prices of an item, the latest effective date first, each with the date it applies from; and a new price from a
 * date, which applies to the payments on or after it.
 */
function Prices({ item, say }: { item: CostItem; say: Say }) {
  const { t, locale } = useStaffStrings();
  const day = useDay();
  const today = useTodayDate();
  const { revise } = useCostItemActions();
  const { busy, run } = useWrite(say);
  const ids = useId();
  const dateId = `${ids}-date`;
  const amountId = `${ids}-amount`;
  const currency = item.prices.at(-1)?.price.currency ?? "USD";
  const [date, setDate] = useState(today);
  const [amount, setAmount] = useState("");
  const [round, setRound] = useState(0);
  const [errors, setErrors] = useState<{ date?: string; amount?: string }>({});
  const shown = item.prices
    .map((revision, i) => ({ revision, i }))
    .toSorted((a, b) => b.revision.effective_on.localeCompare(a.revision.effective_on) || b.i - a.i);
  const send = () => {
    const found = {
      ...(date === "" || date < item.starts_on ? { date: t("staff_finance.cost.revise.date_before") } : {}),
      ...(amountFits(amount, currency) ? {} : { amount: t("staff_finance.cost.amount.invalid") }),
    };
    setErrors(found);
    if (found.date !== undefined || found.amount !== undefined) {
      requestAnimationFrame(() => {
        focusRefusedField(found.date !== undefined ? dateId : amountId);
      });
      return;
    }
    run(
      "revise",
      () => revise(item.id, { effective_on: date, price: { amount: amount.trim(), currency } }),
      () => {
        setAmount("");
        setRound((n) => n + 1);
        return t("staff_finance.cost.revise.done", {
          price: moneyText(locale, { amount: amount.trim(), currency }),
          date: day(date),
        });
      },
      (e) => {
        const placed = placeRefusal(e, { effective_on: "date", price: "amount", "price.amount": "amount" } as const);
        if (placed.field === undefined) return false;
        setErrors({ [placed.field.input]: placed.field.text });
        requestAnimationFrame(() => {
          focusRefusedField(placed.field?.input === "date" ? dateId : amountId);
        });
        return true;
      },
    );
  };
  return (
    <Titled title={t("staff_finance.cost.prices")}>
      <Stack gap="entry-room">
        <LogList label={t("staff_finance.cost.prices")}>
          {shown.map(({ revision, i }) => (
            <LogEntry key={`${revision.effective_on}-${String(i)}`} icon="finance" date={revision.revised_at}>
              {t("staff_finance.cost.price_from", {
                price: moneyText(locale, revision.price),
                date: day(revision.effective_on),
              })}
            </LogEntry>
          ))}
        </LogList>
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
          <Row wrap align="start" gap="screen-tab-gap">
            <Field id={dateId} label={t("staff_finance.cost.revise.date")} error={errors.date}>
              <DateTimeField
                id={dateId}
                date
                label={t("staff_finance.cost.revise.date")}
                value={date}
                min={item.starts_on}
                invalid={errors.date !== undefined}
                describedBy={errorOf(dateId, errors.date)}
                onChange={(value) => {
                  setDate(value);
                  setErrors({});
                }}
              />
            </Field>
            <TextField
              key={round}
              id={amountId}
              name="amount"
              label={t("staff_finance.cost.revise.amount", { currency })}
              inputMode="decimal"
              error={errors.amount}
              onChange={(value) => {
                setAmount(value);
                setErrors({});
              }}
            />
          </Row>
          <Row>
            <Button type="submit" busy={busy === "revise"}>
              {t("staff_finance.cost.revise")}
            </Button>
          </Row>
        </Stack>
      </Stack>
    </Titled>
  );
}

type LabelInput = "name" | "provider" | "category" | "memo";

/** What names an item, changed as a whole. */
function Label({ item, say }: { item: CostItem; say: Say }) {
  const { t } = useStaffStrings();
  const { relabel } = useCostItemActions();
  const { busy, run } = useWrite(say);
  const ids = useId();
  const idOf = (input: LabelInput) => `${ids}-${input}`;
  const [name, setName] = useState(item.name);
  const [provider, setProvider] = useState(item.provider);
  const [category, setCategory] = useState<CostCategory>(item.category);
  const [memo, setMemo] = useState(item.memo);
  const [refused, setRefused] = useState<Refused<LabelInput> | undefined>(undefined);
  const [blank, setBlank] = useState<LabelInput | undefined>(undefined);
  const errorAt = (input: LabelInput) =>
    blank === input
      ? t(`staff_finance.cost.${input === "name" ? "name" : "provider"}.blank`)
      : fieldError(refused, input);
  const send = () => {
    const empty: LabelInput | undefined = name.trim() === "" ? "name" : provider.trim() === "" ? "provider" : undefined;
    setBlank(empty);
    setRefused(undefined);
    if (empty !== undefined) {
      requestAnimationFrame(() => {
        focusRefusedField(idOf(empty));
      });
      return;
    }
    run(
      "label",
      () => relabel(item.id, { name: name.trim(), provider: provider.trim(), category, memo }),
      () => t("staff_finance.cost.label.done"),
      (e) => {
        const placed = placeRefusal(e, {
          name: "name",
          provider: "provider",
          category: "category",
          memo: "memo",
        } as const);
        if (placed.field === undefined) return false;
        setRefused(placed);
        const input = placed.field.input;
        requestAnimationFrame(() => {
          focusRefusedField(idOf(input));
        });
        return true;
      },
    );
  };
  const clear = () => {
    setBlank(undefined);
    setRefused(undefined);
  };
  return (
    <FormSection id={`${ids}-heading`} heading={t("staff_finance.cost.label")}>
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
          defaultValue={item.name}
          maxLength={costNameLimit}
          error={errorAt("name")}
          onChange={(value) => {
            setName(value);
            clear();
          }}
        />
        <TextField
          id={idOf("provider")}
          name="provider"
          label={t("staff_finance.cost.provider")}
          defaultValue={item.provider}
          maxLength={costNameLimit}
          error={errorAt("provider")}
          onChange={(value) => {
            setProvider(value);
            clear();
          }}
        />
        <Field id={idOf("category")} label={t("staff_finance.cost.category")} error={errorAt("category")}>
          <Select
            id={idOf("category")}
            describedBy={errorOf(idOf("category"), errorAt("category"))}
            options={costCategories.map((c) => ({ value: c, label: t("finance.category", { category: c }) }))}
            value={category}
            onChange={(value) => {
              setCategory(costCategories.find((c) => c === value) ?? "other");
              clear();
            }}
          />
        </Field>
        <TextField
          id={idOf("memo")}
          name="memo"
          label={t("staff_finance.cost.memo")}
          hint={t("staff_finance.cost.memo.hint")}
          defaultValue={item.memo}
          multiline
          rows={3}
          maxLength={costMemoLimit}
          error={errorAt("memo")}
          onChange={(value) => {
            setMemo(value);
            clear();
          }}
        />
        <Row>
          <Button type="submit" busy={busy === "label"}>
            {t("staff_finance.cost.label.save")}
          </Button>
        </Row>
      </Stack>
    </FormSection>
  );
}

/** The end of a repeating item: its last payment date, set or moved, or taken away so that it goes on. */
function End({ item, say }: { item: CostItem; say: Say }) {
  const { t } = useStaffStrings();
  const day = useDay();
  const { end } = useCostItemActions();
  const { busy, run } = useWrite(say);
  const ids = useId();
  const dateId = `${ids}-date`;
  const [date, setDate] = useState(item.ends_on ?? "");
  const [error, setError] = useState<string | undefined>(undefined);
  const send = () => {
    if (date === "" || date < item.starts_on) {
      setError(t("staff_finance.cost.ends.before"));
      requestAnimationFrame(() => {
        focusRefusedField(dateId);
      });
      return;
    }
    run(
      "end",
      () => end(item.id, date),
      () => t("staff_finance.cost.end.done", { date: day(date) }),
      (e) => {
        const placed = placeRefusal(e, { ends_on: "date" } as const);
        if (placed.field === undefined) return false;
        setError(placed.field.text);
        requestAnimationFrame(() => {
          focusRefusedField(dateId);
        });
        return true;
      },
    );
  };
  return (
    <FormSection id={`${ids}-heading`} heading={t("staff_finance.cost.end")}>
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
        <Field id={dateId} label={t("staff_finance.cost.end.date")} error={error}>
          <DateTimeField
            id={dateId}
            date
            label={t("staff_finance.cost.end.date")}
            value={date}
            min={item.starts_on}
            invalid={error !== undefined}
            describedBy={errorOf(dateId, error)}
            onChange={(value) => {
              setDate(value);
              setError(undefined);
            }}
          />
        </Field>
        <Row wrap align="center" gap="screen-tab-gap">
          <Button type="submit" busy={busy === "end"}>
            {t(item.ends_on === undefined ? "staff_finance.cost.end.set" : "staff_finance.cost.end.move")}
          </Button>
          {item.ends_on !== undefined && (
            <Button
              busy={busy === "endless"}
              onPress={() => {
                run(
                  "endless",
                  () => end(item.id, undefined),
                  () => t("staff_finance.cost.end.removed"),
                );
              }}
            >
              {t("staff_finance.cost.end.remove")}
            </Button>
          )}
        </Row>
      </Stack>
    </FormSection>
  );
}

/** Deleting an item from every period, past ones too, asked first; deleted, the reader goes back to the costs. */
function Delete({ item, onDeleted, say }: { item: CostItem; onDeleted: () => void; say: Say }) {
  const { t } = useStaffStrings();
  const { remove } = useCostItemActions();
  const ids = useId();
  const [asking, setAsking] = useState(false);
  const [busy, setBusy] = useState(false);
  return (
    <FormSection id={`${ids}-heading`} heading={t("staff_finance.cost.delete")} apart>
      <Stack gap="screen-field-gap">
        <Box as="p">{t("staff_finance.cost.delete.about")}</Box>
        <Row>
          <Button
            onPress={() => {
              setAsking(true);
            }}
          >
            {t("staff_finance.cost.delete.ask")}
          </Button>
        </Row>
      </Stack>
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            title={t("staff_finance.cost.delete.confirm", { name: item.name })}
            cancel={t("staff_finance.cancel")}
            confirm={t("staff_finance.cost.delete.do")}
            busy={busy}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={() => {
              setBusy(true);
              remove(item.id).then(onDeleted, () => {
                setBusy(false);
                setAsking(false);
                say(t("staff_finance.cost.write_failed"), true);
              });
            }}
          >
            {t("staff_finance.cost.delete.about")}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </FormSection>
  );
}
