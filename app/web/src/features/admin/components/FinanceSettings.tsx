// SPDX-License-Identifier: AGPL-3.0-only
// The costs screen below its heading (ADR-0067 §7, ADR-0068 §9, manage_finance): the totals by week, month or year
// under tabs, amortized (the default) or in cash, with the rules that cut their periods next to them; the form that
// records a cost paid once or every N weeks, months or years; the recorded items, newest first, each with its
// schedule, dates and prices and the dialogs that change its label, revise its price and set its end, and canceling
// it after an alert dialog; the state of the billing imports with importing now; and the income that the payment
// provider records (ADR-0073 §6): its totals under the same tabs, with the gross, the fees, the net, the balance and
// the members of each plan, and its money movements, newest first, with their subscribers by username, narrowed to a
// kind and a range of dates, which only this screen shows. The granularity, the basis and the movements' filter are
// in the URL (web-client/README.md S3). Amounts and dates are formatted with Intl (ADR-0067 §10); names, providers and
// memos are the operator's words, shown with dir="auto". What a change did stays in a StatusMessage above the list;
// a failure stays there too (role="alert", web-client/README.md E2).
import {
  Box,
  Button,
  Card,
  Cluster,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  Dialog,
  DialogTrigger,
  EmptyState,
  Heading,
  Hint,
  InlineError,
  Link,
  Radio,
  RadioGroup,
  Select,
  SelectItem,
  Stack,
  StatusMessage,
  TableCell,
  TableColumnHeader,
  TableRow,
  TableRowHeader,
  Text,
  TextArea,
  TextField,
} from "@my/ui";
import { type ReactNode, useId, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { shellQueryReader } from "../../../lib/query";
import { useFocusAfterRemoval, useFocusHeadingAfterMove } from "../../../lib/useFocusAfterRemoval";
import {
  categoryLabels,
  type CostBasis,
  costCategories,
  type CostGranularity,
  CostTotalsPanel,
  CostTotalsView,
  financeCostTotalsQueryOptions,
  IncomeTotalsView,
  useFormatMoney,
  useFormatTotals,
  utcDate,
} from "../../costs";
import { DateTime, Section, RecordList, RecordTable, RequirePermission, useHasPermission } from "../../moderation";
import { instanceQueryOptions } from "../api/documents";
import {
  type BillingImport,
  type CostItem,
  costMemoLimit,
  costNameLimit,
  type CostSchedule,
  costSchedules,
  incomeKinds,
  type IncomeKind,
  type IncomeMovement,
  type IncomeMovementFilter,
  maxCostInterval,
  priceOn,
} from "../api/finance";
import {
  type CostItemActions,
  type LabelDraft,
  type PriceDraft,
  useAdminIncomeTotals,
  useBillingImports,
  useCostItemActions,
  useCostItems,
  useCostTotals,
  useFormatDay,
  useIncomeMovements,
  useMovementDates,
  useNewCostItemForm,
  useRunBillingImport,
} from "../hooks/useFinance";

const scheduleLabels: Readonly<Record<CostSchedule, ClientMessageKey>> = {
  once: "client:admin.finance.schedule.once",
  weeks: "client:admin.finance.schedule.weeks",
  months: "client:admin.finance.schedule.months",
  years: "client:admin.finance.schedule.years",
};

const monthKindLabels: Readonly<Record<BillingImport["months"][number]["kind"], ClientMessageKey>> = {
  estimate: "client:admin.finance.imports.kind.estimate",
  final: "client:admin.finance.imports.kind.final",
  correction: "client:admin.finance.imports.kind.correction",
};

const incomeKindLabels: Readonly<Record<IncomeKind, ClientMessageKey>> = {
  payment: "client:admin.finance.movements.kind.payment",
  refund: "client:admin.finance.movements.kind.refund",
  chargeback: "client:admin.finance.movements.kind.chargeback",
  chargeback_reversal: "client:admin.finance.movements.kind.chargeback_reversal",
  fee: "client:admin.finance.movements.kind.fee",
};

/** The value of the "every kind" choice of the movements' filter. */
const anyKind = "any";

const errorKindLabels: Readonly<Record<NonNullable<BillingImport["last_run"]>["error_kind"], ClientMessageKey>> = {
  none: "client:admin.finance.imports.error.none",
  permission: "client:admin.finance.imports.error.permission",
  not_enabled: "client:admin.finance.imports.error.not_enabled",
  not_found: "client:admin.finance.imports.error.not_found",
  throttled: "client:admin.finance.imports.error.throttled",
  request_cap: "client:admin.finance.imports.error.request_cap",
  bytes_cap: "client:admin.finance.imports.error.bytes_cap",
  invalid_response: "client:admin.finance.imports.error.invalid_response",
  unavailable: "client:admin.finance.imports.error.unavailable",
};

export type FinanceSettingsProps = {
  /** The granularity and the basis of the totals, from the URL. */
  granularity: CostGranularity;
  basis: CostBasis;
  /** Today's date on the viewer's device, which ends the shown periods (ADR-0067 §5). */
  today: string;
  /** Selects another granularity or basis: the route puts it into the URL. */
  onTotalsChange: (next: { granularity: CostGranularity; basis: CostBasis }) => void;
  /** The kind and the dates of the money movements shown, from the URL. */
  movements: IncomeMovementFilter;
  /** Narrows the money movements another way: the route puts it into the URL. */
  onMovementsChange: (next: IncomeMovementFilter) => void;
};

export function FinanceSettings(props: FinanceSettingsProps) {
  const t = useT();
  return (
    <RequirePermission permission="manage_finance">
      <Stack gap="8">
        <Stack gap="2">
          <Hint size="inherit">{t("client:admin.finance.description")}</Hint>
          <Publication />
          <ViewerSettings granularity={props.granularity} basis={props.basis} today={props.today} />
        </Stack>
        <Section surface="card" title={t("client:admin.finance.totals.title")}>
          <Card>
            <TotalsSection {...props} />
          </Card>
        </Section>
        <Section surface="card" title={t("client:admin.finance.new.title")}>
          <Card>
            <NewCostItemForm />
          </Card>
        </Section>
        <Section surface="card" title={t("client:admin.finance.list.title")}>
          <Card>
            <CostItemList today={props.today} />
          </Card>
        </Section>
        <Section surface="card" title={t("client:admin.finance.imports.title")}>
          <Card>
            <BillingImports />
          </Card>
        </Section>
        <Section surface="card" title={t("client:admin.finance.movements.title")}>
          <IncomeMovements filter={props.movements} onFilterChange={props.onMovementsChange} />
        </Section>
      </Stack>
    </RequirePermission>
  );
}

/**
 * What the instance publishes of these costs and of the income (the instance settings `finance_visibility` and
 * `income_visibility`, ADR-0067 §8, ADR-0073 §7), read from GET /instance. They are server settings (manage_settings),
 * not ones of the costs: a viewer who may change them gets a link to the server settings.
 */
function Publication() {
  const t = useT();
  const api = useApi();
  const { data: instance } = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  const maySet = useHasPermission("manage_settings");
  if (instance === undefined) return null;
  return (
    <Cluster as="p" gap="2" rowGap="0">
      <span>{t("client:admin.finance.visibility", { visibility: instance.finance_visibility })}</span>
      <span>{t("client:admin.finance.income_visibility", { visibility: instance.income_visibility })}</span>
      {maySet && (
        <Link href="/admin/settings" variant="standalone">
          {t("client:admin.finance.visibility.change")}
        </Link>
      )}
    </Cluster>
  );
}

/**
 * What the holders of view_finance read of these costs on the finances page (the instance settings
 * `finance_viewer_*`, ADR-0070 §5), read from GET /finance/cost_totals with the totals of the screen. They are server
 * settings (manage_settings) too, changed through the link above.
 */
function ViewerSettings({
  granularity,
  basis,
  today,
}: {
  granularity: CostGranularity;
  basis: CostBasis;
  today: string;
}) {
  const t = useT();
  const api = useApi();
  const { data } = useQuery({ ...financeCostTotalsQueryOptions(api, granularity, basis, today), ...shellQueryReader });
  if (data === undefined) return null;
  const { viewer } = data;
  return (
    <p>
      {t("client:admin.finance.viewer", {
        names: viewer.item_names ? "shown" : "hidden",
        memos: viewer.item_memos ? "shown" : "hidden",
        income: viewer.income,
      })}
    </p>
  );
}

function TotalsSection({ granularity, basis, today, onTotalsChange }: FinanceSettingsProps) {
  return (
    <CostTotalsPanel granularity={granularity} basis={basis} onTotalsChange={onTotalsChange}>
      <Totals granularity={granularity} basis={basis} today={today} />
    </CostTotalsPanel>
  );
}

/** The costs of the selected periods, then their income with the gross, the fees and the balance under the same basis. */
function Totals({ granularity, basis, today }: { granularity: CostGranularity; basis: CostBasis; today: string }) {
  const t = useT();
  const totals = useCostTotals(granularity, basis, today);
  const income = useAdminIncomeTotals(granularity, basis, today);
  const incomeId = useId();
  return (
    <Stack gap="8">
      <CostTotalsView totals={totals} breakdown />
      <Stack as="section" gap="3" aria-labelledby={incomeId}>
        <Heading level={3} id={incomeId}>
          {t("client:income.title")}
        </Heading>
        <IncomeTotalsView totals={income} detailed headingLevel={4} />
      </Stack>
    </Stack>
  );
}

/** The fields of a label: name, provider, category and memo. */
function LabelFields({
  label,
  onChange,
  errors,
  autoFocus = false,
}: {
  label: LabelDraft;
  onChange: (patch: Partial<LabelDraft>) => void;
  errors: { name?: string; provider?: string; memo?: string };
  autoFocus?: boolean;
}) {
  const t = useT();
  const categories = costCategories.map((id) => ({ id, name: t(categoryLabels[id]) }));
  return (
    <>
      <TextField
        label={t("client:admin.finance.name")}
        description={t("client:admin.finance.name.description", { max: String(costNameLimit) })}
        value={label.name}
        onChange={(name) => {
          onChange({ name });
        }}
        isRequired
        autoFocus={autoFocus}
        {...(errors.name !== undefined && { isInvalid: true, errorMessage: errors.name })}
      />
      <TextField
        label={t("client:admin.finance.provider")}
        description={t("client:admin.finance.provider.description")}
        value={label.provider}
        onChange={(provider) => {
          onChange({ provider });
        }}
        isRequired
        {...(errors.provider !== undefined && { isInvalid: true, errorMessage: errors.provider })}
      />
      <Select
        label={t("client:costs.category")}
        value={label.category}
        onChange={(key) => {
          const choice = categories.find((item) => item.id === key);
          if (choice !== undefined) onChange({ category: choice.id });
        }}
        items={categories}
      >
        {(choice) => (
          <SelectItem id={choice.id} textValue={choice.name}>
            {choice.name}
          </SelectItem>
        )}
      </Select>
      <TextArea
        label={t("client:admin.finance.memo")}
        description={t("client:admin.finance.memo.description")}
        value={label.memo}
        onChange={(memo) => {
          onChange({ memo });
        }}
        rows={3}
        characterLimit={costMemoLimit}
        formatRemaining={(remaining) =>
          remaining < 0
            ? t("client:admin.settings.characters_over", { count: -remaining })
            : t("client:admin.settings.characters_left", { count: remaining })
        }
        {...(errors.memo !== undefined && { isInvalid: true, errorMessage: errors.memo })}
      />
    </>
  );
}

/** The fields of a price: the amount in the major unit and the ISO 4217 code of its currency. */
function PriceFields({
  price,
  onChange,
  errors,
}: {
  price: PriceDraft;
  onChange: (patch: Partial<PriceDraft>) => void;
  errors: { amount?: string; currency?: string };
}) {
  const t = useT();
  return (
    <Cluster gap="4">
      <TextField
        label={t("client:admin.finance.amount")}
        description={t("client:admin.finance.amount.description")}
        value={price.amount}
        onChange={(amount) => {
          onChange({ amount });
        }}
        inputMode="decimal"
        isRequired
        grow
        {...(errors.amount !== undefined && { isInvalid: true, errorMessage: errors.amount })}
      />
      <TextField
        label={t("client:admin.finance.currency")}
        description={t("client:admin.finance.currency.description")}
        value={price.currency}
        onChange={(currency) => {
          onChange({ currency: currency.toUpperCase() });
        }}
        maxLength={3}
        spellCheck="false"
        isRequired
        {...(errors.currency !== undefined && { isInvalid: true, errorMessage: errors.currency })}
      />
    </Cluster>
  );
}

function NewCostItemForm() {
  const t = useT();
  const items = useCostItems();
  const form = useNewCostItemForm(items.items[0]?.prices[0]?.price.currency ?? "USD");
  const { draft, errors } = form;
  return (
    <Stack as="form" gap="6" onSubmit={form.submit} noValidate>
      <LabelFields label={draft} onChange={form.change} errors={errors} />
      <RadioGroup
        label={t("client:admin.finance.schedule")}
        description={t("client:admin.finance.schedule.description")}
        value={draft.schedule}
        onChange={(value) => {
          const schedule = costSchedules.find((item) => item === value);
          if (schedule !== undefined) form.change({ schedule });
        }}
      >
        {costSchedules.map((item) => (
          <Radio key={item} value={item}>
            {t(scheduleLabels[item])}
          </Radio>
        ))}
      </RadioGroup>
      {draft.schedule !== "once" && (
        <TextField
          label={t("client:admin.finance.interval")}
          description={t("client:admin.finance.interval.description", { max: String(maxCostInterval) })}
          value={draft.interval}
          onChange={(interval) => {
            form.change({ interval });
          }}
          inputMode="numeric"
          isRequired
          {...(errors.interval !== undefined && { isInvalid: true, errorMessage: errors.interval })}
        />
      )}
      <TextField
        type="date"
        label={t(draft.schedule === "once" ? "client:admin.finance.paid_on" : "client:admin.finance.starts_on")}
        description={t(
          draft.schedule === "once"
            ? "client:admin.finance.paid_on.description"
            : "client:admin.finance.starts_on.description",
        )}
        value={draft.startsOn}
        onChange={(startsOn) => {
          form.change({ startsOn });
        }}
        isRequired
        {...(errors.startsOn !== undefined && { isInvalid: true, errorMessage: errors.startsOn })}
      />
      {draft.schedule !== "once" && (
        <TextField
          type="date"
          label={t("client:admin.finance.ends_on")}
          description={t("client:admin.finance.ends_on.description")}
          value={draft.endsOn}
          onChange={(endsOn) => {
            form.change({ endsOn });
          }}
          {...(errors.endsOn !== undefined && { isInvalid: true, errorMessage: errors.endsOn })}
        />
      )}
      <PriceFields price={draft} onChange={form.change} errors={errors} />
      {form.failure !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.finance.create_failed", { reason: form.failure })}
        </InlineError>
      )}
      <div>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:admin.finance.create")}
        </Button>
      </div>
      <StatusMessage>{form.status}</StatusMessage>
    </Stack>
  );
}

function CostItemList({ today }: { today: string }) {
  const t = useT();
  const list = useCostItems();
  const listRef = useRef<HTMLUListElement>(null);
  const [status, setStatus] = useState<string | undefined>(undefined);
  // A change keeps the row and may close its dialog: focus moves to the row's heading. A canceled item leaves the
  // list: focus moves to the next row's button.
  const changed = useFocusHeadingAfterMove(listRef);
  const removed = useFocusAfterRemoval(
    listRef,
    list.items.map((item) => item.id),
  );
  const actions = useCostItemActions((item, done, gone) => {
    if (gone) removed(item.id);
    else changed(item.id);
    setStatus(done);
  });
  return (
    <Stack gap="3">
      <StatusMessage>{status}</StatusMessage>
      {actions.error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.finance.change_failed", { reason: actions.error })}
        </InlineError>
      )}
      <RecordList
        list={list}
        getId={(item) => item.id}
        listRef={listRef}
        empty={
          <EmptyState
            headingLevel={3}
            heading={t("client:admin.finance.empty")}
            description={t("client:admin.finance.empty.description")}
          />
        }
      >
        {(item) => <CostItemRow item={item} today={today} actions={actions} />}
      </RecordList>
    </Stack>
  );
}

/** How often an item is paid, in words: "Once", "Every 3 months". */
function useScheduleText() {
  const t = useT();
  return (item: Pick<CostItem, "schedule" | "interval">): string => {
    switch (item.schedule) {
      case "once":
        return t("client:admin.finance.every.once");
      case "weeks":
        return t("client:admin.finance.every.weeks", { count: item.interval });
      case "months":
        return t("client:admin.finance.every.months", { count: item.interval });
      case "years":
        return t("client:admin.finance.every.years", { count: item.interval });
    }
  };
}

function CostItemRow({ item, today, actions }: { item: CostItem; today: string; actions: CostItemActions }) {
  const t = useT();
  const formatDay = useFormatDay();
  const format = useFormatMoney();
  const scheduleText = useScheduleText();
  // Before the start date, the price of the first payment.
  const price = priceOn(item, today) ?? item.prices[0]?.price;
  return (
    <Stack gap="3">
      <Heading level={3} wrap="anywhere" focusable dir="auto">
        {item.name}
      </Heading>
      <DescriptionList>
        <DescriptionItem term={t("client:admin.finance.provider")}>
          <span dir="auto">{item.provider}</span>
        </DescriptionItem>
        <DescriptionItem term={t("client:costs.category")}>{t(categoryLabels[item.category])}</DescriptionItem>
        <DescriptionItem term={t("client:admin.finance.schedule")}>{scheduleText(item)}</DescriptionItem>
        <DescriptionItem
          term={t(item.schedule === "once" ? "client:admin.finance.paid_on" : "client:admin.finance.starts_on")}
        >
          {formatDay(item.starts_on)}
        </DescriptionItem>
        {item.schedule !== "once" && (
          <DescriptionItem term={t("client:admin.finance.ends_on")}>
            {item.ends_on === undefined ? t("client:admin.finance.no_end") : formatDay(item.ends_on)}
          </DescriptionItem>
        )}
        {price !== undefined && (
          <DescriptionItem term={t("client:admin.finance.price")}>
            <Text numeric>{format(price)}</Text>
          </DescriptionItem>
        )}
        {item.prices.length > 1 && (
          <DescriptionItem term={t("client:admin.finance.prices")}>
            <Stack as="ul">
              {item.prices.map((revision) => (
                <Text as="li" numeric key={`${revision.effective_on} ${revision.revised_at}`}>
                  {t("client:admin.finance.price_from", {
                    date: formatDay(revision.effective_on),
                    amount: format(revision.price),
                  })}
                </Text>
              ))}
            </Stack>
          </DescriptionItem>
        )}
        {item.memo !== "" && (
          <DescriptionItem term={t("client:admin.finance.memo")}>
            <Text wrap="anywhere" whitespace="preWrap" dir="auto" lang="">
              {item.memo}
            </Text>
          </DescriptionItem>
        )}
        <DescriptionItem term={t("client:admin.finance.created_at")}>
          <DateTime value={item.created_at} />
        </DescriptionItem>
      </DescriptionList>
      <Cluster gap="2">
        <EditLabel item={item} actions={actions} />
        <RevisePrice item={item} actions={actions} />
        {item.schedule !== "once" && <SetEnd item={item} actions={actions} />}
        <CancelItem item={item} actions={actions} />
      </Cluster>
    </Stack>
  );
}

type ItemActionProps = { item: CostItem; actions: CostItemActions };

/** A dialog's form that closes once `save` worked and keeps a failure inside. */
function DialogForm({
  save,
  close,
  actions,
  children,
}: {
  save: () => Promise<boolean> | undefined;
  close: () => void;
  actions: CostItemActions;
  children: ReactNode;
}) {
  const t = useT();
  const [isSaving, setIsSaving] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <Stack
      as="form"
      gap="4"
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        if (isSaving) return;
        const saving = save();
        if (saving === undefined) return;
        setIsSaving(true);
        setFailed(false);
        void saving.then((saved) => {
          setIsSaving(false);
          if (saved) close();
          else setFailed(true);
        });
      }}
    >
      {children}
      {failed && actions.error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.finance.change_failed", { reason: actions.error })}
        </InlineError>
      )}
      <Cluster gap="2" justify="end">
        <Button onPress={close}>{t("client:admin.finance.cancel_dialog")}</Button>
        <Button type="submit" variant="primary" isPending={isSaving}>
          {t("client:admin.finance.save")}
        </Button>
      </Cluster>
    </Stack>
  );
}

function EditLabel({ item, actions }: ItemActionProps) {
  const t = useT();
  return (
    <DialogTrigger>
      <Button size="sm" aria-label={t("client:admin.finance.edit_one", { name: item.name })}>
        {t("client:admin.finance.edit")}
      </Button>
      <Dialog
        title={t("client:admin.finance.edit.title")}
        closeLabel={t("client:admin.finance.close")}
        isDismissable={false}
        size="md"
      >
        {({ close }) => <EditLabelForm item={item} actions={actions} close={close} />}
      </Dialog>
    </DialogTrigger>
  );
}

function EditLabelForm({ item, actions, close }: ItemActionProps & { close: () => void }) {
  const [label, setLabel] = useState<LabelDraft>({
    name: item.name,
    provider: item.provider,
    category: item.category,
    memo: item.memo,
  });
  const [submitted, setSubmitted] = useState(false);
  const errors = submitted ? actions.labelErrors(label) : {};
  return (
    <DialogForm
      close={close}
      actions={actions}
      save={() => {
        setSubmitted(true);
        if (Object.keys(actions.labelErrors(label)).length > 0) return undefined;
        return actions.relabel(item, label);
      }}
    >
      <LabelFields
        label={label}
        onChange={(patch) => {
          setLabel((current) => ({ ...current, ...patch }));
        }}
        errors={errors}
        autoFocus
      />
    </DialogForm>
  );
}

function RevisePrice({ item, actions }: ItemActionProps) {
  const t = useT();
  return (
    <DialogTrigger>
      <Button size="sm" aria-label={t("client:admin.finance.revise_one", { name: item.name })}>
        {t("client:admin.finance.revise")}
      </Button>
      <Dialog
        title={t("client:admin.finance.revise.title", { name: item.name })}
        closeLabel={t("client:admin.finance.close")}
        isDismissable={false}
        size="md"
      >
        {({ close }) => <RevisePriceForm item={item} actions={actions} close={close} />}
      </Dialog>
    </DialogTrigger>
  );
}

function RevisePriceForm({ item, actions, close }: ItemActionProps & { close: () => void }) {
  const t = useT();
  const last = item.prices.at(-1)?.price;
  const [effectiveOn, setEffectiveOn] = useState("");
  const [price, setPrice] = useState<PriceDraft>({ amount: "", currency: last?.currency ?? "USD" });
  const [submitted, setSubmitted] = useState(false);
  const dateError = (value: string) => {
    if (value === "") return t("client:admin.finance.required");
    if (value < item.starts_on) return t("client:admin.finance.effective_on.before_start");
    return undefined;
  };
  const priceErrors = submitted ? actions.priceErrors(price) : {};
  const effectiveError = submitted ? dateError(effectiveOn) : undefined;
  return (
    <DialogForm
      close={close}
      actions={actions}
      save={() => {
        setSubmitted(true);
        if (dateError(effectiveOn) !== undefined || Object.keys(actions.priceErrors(price)).length > 0)
          return undefined;
        return actions.revise(item, effectiveOn, price);
      }}
    >
      <TextField
        type="date"
        label={t("client:admin.finance.effective_on")}
        description={t("client:admin.finance.effective_on.description")}
        value={effectiveOn}
        onChange={setEffectiveOn}
        isRequired
        autoFocus
        {...(effectiveError !== undefined && { isInvalid: true, errorMessage: effectiveError })}
      />
      <PriceFields
        price={price}
        onChange={(patch) => {
          setPrice((current) => ({ ...current, ...patch }));
        }}
        errors={priceErrors}
      />
    </DialogForm>
  );
}

function SetEnd({ item, actions }: ItemActionProps) {
  const t = useT();
  return (
    <DialogTrigger>
      <Button size="sm" aria-label={t("client:admin.finance.end_one", { name: item.name })}>
        {t("client:admin.finance.end")}
      </Button>
      <Dialog
        title={t("client:admin.finance.end.title", { name: item.name })}
        closeLabel={t("client:admin.finance.close")}
        isDismissable={false}
        size="md"
      >
        {({ close }) => <SetEndForm item={item} actions={actions} close={close} />}
      </Dialog>
    </DialogTrigger>
  );
}

function SetEndForm({ item, actions, close }: ItemActionProps & { close: () => void }) {
  const t = useT();
  const [endsOn, setEndsOn] = useState(item.ends_on ?? "");
  const [submitted, setSubmitted] = useState(false);
  const error = (value: string) =>
    value !== "" && value < item.starts_on ? t("client:admin.finance.ends_on.before_start") : undefined;
  const endError = submitted ? error(endsOn) : undefined;
  return (
    <DialogForm
      close={close}
      actions={actions}
      save={() => {
        setSubmitted(true);
        if (error(endsOn) !== undefined) return undefined;
        return actions.setEnd(item, endsOn === "" ? undefined : endsOn);
      }}
    >
      <TextField
        type="date"
        label={t("client:admin.finance.ends_on")}
        description={t("client:admin.finance.end.description")}
        value={endsOn}
        onChange={setEndsOn}
        autoFocus
        {...(endError !== undefined && { isInvalid: true, errorMessage: endError })}
      />
    </DialogForm>
  );
}

/** Canceling an item, after an alert dialog: it leaves every period, past ones included, and cannot be undone. */
function CancelItem({ item, actions }: ItemActionProps) {
  const t = useT();
  const [isCanceling, setIsCanceling] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <DialogTrigger>
      <Button size="sm" variant="danger" aria-label={t("client:admin.finance.cancel_one", { name: item.name })}>
        {t("client:admin.finance.cancel_item")}
      </Button>
      <ConfirmDialog
        title={t("client:admin.finance.cancel.title", { name: item.name })}
        description={t("client:admin.finance.cancel.description")}
        confirmLabel={t("client:admin.finance.cancel_item")}
        cancelLabel={t("client:admin.finance.cancel_dialog")}
        isPending={isCanceling}
        {...(failed &&
          actions.error !== undefined && {
            errorMessage: t("client:admin.finance.change_failed", { reason: actions.error }),
          })}
        onConfirm={(close) => {
          setIsCanceling(true);
          setFailed(false);
          void actions.cancel(item).then((canceled) => {
            setIsCanceling(false);
            if (canceled) close();
            else setFailed(true);
          });
        }}
      />
    </DialogTrigger>
  );
}

function BillingImports() {
  const t = useT();
  const sources = useBillingImports();
  const run = useRunBillingImport();
  if (sources.length === 0) {
    return (
      <EmptyState
        headingLevel={3}
        heading={t("client:admin.finance.imports.none")}
        description={t("client:admin.finance.imports.none.description")}
      />
    );
  }
  return (
    <Stack gap="3">
      <StatusMessage>{run.status}</StatusMessage>
      {run.error !== undefined && (
        <InlineError size="inherit" alert>
          {run.error}
        </InlineError>
      )}
      <Stack as="ul" divided>
        {sources.map((source) => (
          <Box as="li" container paddingY="3" key={source.source}>
            <BillingImportRow source={source} isPending={run.pendingSource === source.source} onRun={run.run} />
          </Box>
        ))}
      </Stack>
    </Stack>
  );
}

function BillingImportRow({
  source,
  isPending,
  onRun,
}: {
  source: BillingImport;
  isPending: boolean;
  onRun: (source: BillingImport) => void;
}) {
  const t = useT();
  const intl = useIntl();
  const formatTotals = useFormatTotals();
  const count = (used: number, max: number | undefined) =>
    max === undefined
      ? intl.formatNumber(used)
      : t("client:admin.finance.imports.used_of", { used: intl.formatNumber(used), max: intl.formatNumber(max) });
  return (
    <Stack gap="3">
      <Heading level={3}>{source.provider}</Heading>
      {source.method === "budget" && <Hint size="inherit">{t("client:admin.finance.imports.method.budget")}</Hint>}
      <DescriptionList>
        <DescriptionItem term={t("client:admin.finance.imports.time_zone")}>{source.time_zone}</DescriptionItem>
        <DescriptionItem term={t("client:admin.finance.imports.last_success")}>
          {source.last_success_at === undefined ? (
            t("client:admin.finance.imports.never")
          ) : (
            <DateTime value={source.last_success_at} />
          )}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.finance.imports.last_run")}>
          {source.last_run === undefined ? (
            t("client:admin.finance.imports.never")
          ) : (
            <Stack as="span">
              <DateTime value={source.last_run.occurred_at} />
              <span>{t(errorKindLabels[source.last_run.error_kind])}</span>
            </Stack>
          )}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.finance.imports.requests_today")}>
          {count(source.requests_today, source.max_requests_per_day)}
        </DescriptionItem>
        <DescriptionItem term={t("client:admin.finance.imports.requests_month")}>
          {count(source.requests_this_month, source.max_requests_per_month)}
        </DescriptionItem>
        {source.method === "bigquery" && (
          <DescriptionItem term={t("client:admin.finance.imports.bytes")}>
            {intl.formatNumber(source.bytes_billed_this_month, {
              style: "unit",
              unit: "byte",
              notation: "compact",
              unitDisplay: "narrow",
            })}
          </DescriptionItem>
        )}
      </DescriptionList>
      {source.months.length > 0 && (
        <Stack as="ul" gap="1" aria-label={t("client:admin.finance.imports.months", { provider: source.provider })}>
          {source.months.map((month) => (
            <Cluster as="li" gap="2" rowGap="0" key={month.month}>
              <span>
                {intl.formatDate(utcDate(`${month.month}-01`), { year: "numeric", month: "long", timeZone: "UTC" })}
              </span>
              <Text tone="muted">{t(monthKindLabels[month.kind])}</Text>
              <Text numeric>{formatTotals(month.totals)}</Text>
            </Cluster>
          ))}
        </Stack>
      )}
      <div>
        <Button
          size="sm"
          isPending={isPending}
          aria-label={t("client:admin.finance.imports.run_one", { provider: source.provider })}
          onPress={() => {
            onRun(source);
          }}
        >
          {t("client:admin.finance.imports.run")}
        </Button>
      </div>
    </Stack>
  );
}

/**
 * The payment provider's money movements of the income, newest first (ADR-0073 §6): what each brought or took back,
 * and whose subscription it belongs to, narrowed to a kind (which applies at once) and to the dates they are counted on
 * (applied together, since a date field changes while it is typed). Only this screen shows them; the payment provider
 * records them, and nothing here changes them.
 */
function IncomeMovements({
  filter,
  onFilterChange,
}: {
  filter: IncomeMovementFilter;
  onFilterChange: (filter: IncomeMovementFilter) => void;
}) {
  const t = useT();
  const list = useIncomeMovements(filter);
  const dates = useMovementDates(filter, onFilterChange);
  const filtersRef = useRef<HTMLDivElement>(null);
  const filtered = filter.kind !== undefined || filter.from !== undefined || filter.to !== undefined;
  const kinds = [
    { id: anyKind, name: t("client:admin.finance.movements.filter.any_kind") },
    ...incomeKinds.map((kind) => ({ id: kind, name: t(incomeKindLabels[kind]) })),
  ];
  const clear = () => {
    // The pressed button goes away with the filters: focus moves to their first control, which stays.
    filtersRef.current?.querySelector<HTMLElement>("button")?.focus();
    onFilterChange({});
  };
  const clearButton = (
    <Button size="sm" onPress={clear}>
      {t("client:admin.finance.movements.filter.clear")}
    </Button>
  );
  return (
    <Stack gap="3">
      <Hint size="inherit">{t("client:admin.finance.movements.description")}</Hint>
      <Stack gap="4" ref={filtersRef}>
        <Select
          label={t("client:admin.finance.movements.filter.kind")}
          value={filter.kind ?? anyKind}
          onChange={(key) => {
            onFilterChange({ ...filter, kind: incomeKinds.find((kind) => kind === key) });
          }}
          items={kinds}
        >
          {(item) => (
            <SelectItem id={item.id} textValue={item.name}>
              {item.name}
            </SelectItem>
          )}
        </Select>
        <Stack as="form" gap="4" align="start" onSubmit={dates.submit} noValidate>
          <Cluster gap="4" align="start">
            <TextField
              type="date"
              label={t("client:admin.finance.movements.filter.from")}
              value={dates.draft.from}
              onChange={(from) => {
                dates.change({ from });
              }}
            />
            <TextField
              type="date"
              label={t("client:admin.finance.movements.filter.to")}
              value={dates.draft.to}
              onChange={(to) => {
                dates.change({ to });
              }}
              {...(dates.toError !== undefined && { isInvalid: true, errorMessage: dates.toError })}
            />
          </Cluster>
          <Button type="submit">{t("client:admin.finance.movements.filter.apply")}</Button>
        </Stack>
        {filtered && <div>{clearButton}</div>}
      </Stack>
      <RecordTable
        key={`${filter.kind ?? anyKind}:${filter.from ?? ""}:${filter.to ?? ""}`}
        list={list}
        caption={t("client:admin.finance.movements.title")}
        getId={(movement) => movement.id}
        head={
          <>
            <TableColumnHeader>{t("client:admin.finance.movements.filter.kind")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.finance.movements.occurred_at")}</TableColumnHeader>
            <TableColumnHeader>{t("client:income.totals.gross")}</TableColumnHeader>
            <TableColumnHeader>{t("client:income.totals.fee")}</TableColumnHeader>
            <TableColumnHeader>{t("client:income.totals.net")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.finance.movements.subscriber")}</TableColumnHeader>
            <TableColumnHeader>{t("client:admin.finance.movements.provider_id.header")}</TableColumnHeader>
          </>
        }
        empty={
          filtered ? (
            <EmptyState
              headingLevel={3}
              heading={t("client:admin.finance.movements.empty.filtered")}
              description={t("client:admin.finance.movements.empty.filtered.description")}
              action={clearButton}
            />
          ) : (
            <EmptyState
              headingLevel={3}
              heading={t("client:admin.finance.movements.empty")}
              description={t("client:admin.finance.movements.empty.description")}
            />
          )
        }
      >
        {(movement) => <IncomeMovementRow movement={movement} />}
      </RecordTable>
    </Stack>
  );
}

/**
 * Whose subscription a movement belongs to: the subscriber's username, linked to the account's moderation screen for
 * a viewer with manage_accounts; "deleted account" when the account is gone; "none" for a movement of no
 * subscription (the provider's fee).
 */
function Subscriber({ movement }: { movement: IncomeMovement }) {
  const t = useT();
  const mayOpenAccount = useHasPermission("manage_accounts");
  if (movement.account_id === undefined || movement.username === undefined) {
    return t(
      movement.subscription_id === undefined
        ? "client:admin.finance.movements.no_subscriber"
        : "client:admin.finance.movements.deleted_account",
    );
  }
  const handle = `@${movement.username}`;
  return mayOpenAccount ? (
    <Link href={`/moderation/accounts/${movement.account_id}`}>
      <Text wrap="anywhere" dir="ltr">
        {handle}
      </Text>
    </Link>
  ) : (
    <Text wrap="anywhere" dir="ltr">
      {handle}
    </Text>
  );
}

function IncomeMovementRow({ movement }: { movement: IncomeMovement }) {
  const t = useT();
  const intl = useIntl();
  const format = useFormatMoney();
  const formatDay = useFormatDay();
  return (
    <TableRow data-row-id={movement.id}>
      <TableRowHeader wrap="anywhere">
        <Stack gap="1">
          <span>{t(incomeKindLabels[movement.kind])}</span>
          {movement.covers_starts_on !== undefined && movement.covers_ends_on !== undefined && (
            <Cluster as="span" gap="1" rowGap="0">
              <Text size="sm" tone="muted">
                {t("client:admin.finance.movements.covers")}
              </Text>
              <Text size="sm" tone="muted">
                {intl.formatDateTimeRange(utcDate(movement.covers_starts_on), utcDate(movement.covers_ends_on), {
                  dateStyle: "medium",
                  timeZone: "UTC",
                })}
              </Text>
            </Cluster>
          )}
        </Stack>
      </TableRowHeader>
      <TableCell>
        <Stack gap="1">
          <DateTime value={movement.occurred_at} />
          <Text size="sm" tone="muted">
            {formatDay(movement.occurred_on)}
          </Text>
        </Stack>
      </TableCell>
      <TableCell numeric wrap="anywhere">
        {format(movement.gross)}
      </TableCell>
      <TableCell numeric wrap="anywhere">
        {format(movement.fee)}
      </TableCell>
      <TableCell numeric wrap="anywhere">
        {format(movement.net)}
      </TableCell>
      <TableCell wrap="anywhere">
        <Subscriber movement={movement} />
      </TableCell>
      <TableCell wrap="anywhere">
        <Stack gap="1">
          <Text size="sm" mono wrap="anywhere" dir="ltr">
            {movement.provider_id}
          </Text>
          {movement.parent_provider_id !== undefined && (
            <Text size="sm" tone="muted" mono wrap="anywhere" dir="ltr">
              {t("client:admin.finance.movements.parent")} {movement.parent_provider_id}
            </Text>
          )}
        </Stack>
      </TableCell>
    </TableRow>
  );
}
