// SPDX-License-Identifier: AGPL-3.0-only
// What the pages of the costs screen share (ADR-0067 §7, manage_finance; web-client §14 画面の分け方): the frame of a
// page, which checks the permission and puts the screen's pages (`LocalNav`) under the heading, the words of the
// schedules and of the kinds of the money movements, and the fields of an item's label and price that the new item's
// form and the item's page both use.
import { Cluster, LocalNav, Select, SelectItem, Stack, TextArea, TextField } from "@my/ui";
import type { ReactNode } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { categoryLabels, costCategories } from "../../costs";
import { RequirePermission } from "../../moderation";
import { type CostItem, costMemoLimit, costNameLimit, type CostSchedule, type IncomeKind } from "../api/finance";
import type { LabelDraft, PriceDraft } from "../hooks/useFinance";

/** The pages of the costs screen, in the order of its navigation. */
type FinancePage = "overview" | "income" | "costs" | "imports" | "publication";

const pages: readonly { page: FinancePage; href: string; label: ClientMessageKey }[] = [
  { page: "overview", href: "/admin/finance", label: "client:admin.finance.pages.overview" },
  { page: "income", href: "/admin/finance/income", label: "client:admin.finance.pages.income" },
  { page: "costs", href: "/admin/finance/costs", label: "client:admin.finance.pages.costs" },
  { page: "imports", href: "/admin/finance/imports", label: "client:admin.finance.pages.imports" },
  { page: "publication", href: "/admin/finance/publication", label: "client:admin.finance.pages.publication" },
];

/**
 * A page of the costs screen below its heading: for a viewer with manage_finance, the screen's pages with `page` as
 * the current one, then the page; for another viewer, why it shows nothing.
 */
export function FinanceScreen({ page, children }: { page: FinancePage; children: ReactNode }) {
  const t = useT();
  return (
    <RequirePermission permission="manage_finance">
      <Stack gap="6">
        <LocalNav
          label={t("client:admin.finance.pages")}
          items={pages.map((item) => ({ href: item.href, label: t(item.label), current: item.page === page }))}
        />
        {children}
      </Stack>
    </RequirePermission>
  );
}

export const scheduleLabels: Readonly<Record<CostSchedule, ClientMessageKey>> = {
  once: "client:admin.finance.schedule.once",
  weeks: "client:admin.finance.schedule.weeks",
  months: "client:admin.finance.schedule.months",
  years: "client:admin.finance.schedule.years",
};

export const incomeKindLabels: Readonly<Record<IncomeKind, ClientMessageKey>> = {
  payment: "client:admin.finance.movements.kind.payment",
  refund: "client:admin.finance.movements.kind.refund",
  chargeback: "client:admin.finance.movements.kind.chargeback",
  chargeback_reversal: "client:admin.finance.movements.kind.chargeback_reversal",
  fee: "client:admin.finance.movements.kind.fee",
};

/** How often an item is paid, in words: "Once", "Every 3 months". */
export function useScheduleText() {
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

/** The fields of a label: name, provider, category and memo. */
export function LabelFields({
  label,
  onChange,
  errors,
}: {
  label: LabelDraft;
  onChange: (patch: Partial<LabelDraft>) => void;
  errors: { name?: string; provider?: string; memo?: string };
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
export function PriceFields({
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
