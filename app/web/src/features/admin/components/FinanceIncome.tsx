// SPDX-License-Identifier: AGPL-3.0-only
// The payments and refunds of the costs screen (`/admin/finance/income`, ADR-0067 §16, manage_finance;
// web-client/screens/admin-finance-income.md): the payment provider's money movements of the income, newest first,
// one page at a time, narrowed to a kind (applied at once) and to the dates they are counted on (applied together,
// since a date field changes while it is typed), both in the URL (web-client/README.md S3). The table has four
// columns (web-client §14 一覧の表): the date, the kind with the period it covers and the provider's identifiers, the
// net with the gross and the fee under it, and whose subscription it belongs to. Only this page shows them; the
// payment provider records them, and nothing here changes them.
import { Button, Cluster, EmptyState, Hint, Link, Select, SelectItem, Stack, Text, TextField } from "@my/ui";
import { useRef } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { useFormatMoney, utcDate } from "../../costs";
import { DateTime, type RecordColumns, RecordTable, Section, useHasPermission } from "../../moderation";
import { incomeKinds, type IncomeMovement, type IncomeMovementFilter } from "../api/finance";
import { useFormatDay, useIncomeMovements, useMovementDates } from "../hooks/useFinance";
import { FinanceScreen, incomeKindLabels } from "./FinanceParts";

/** The value of the "every kind" choice of the movements' filter. */
const anyKind = "any";

export type FinanceIncomeProps = {
  /** The kind and the dates of the money movements shown, from the URL. */
  filter: IncomeMovementFilter;
  /** Narrows the money movements another way: the route puts it into the URL. */
  onFilterChange: (next: IncomeMovementFilter) => void;
};

export function FinanceIncome(props: FinanceIncomeProps) {
  const t = useT();
  return (
    <FinanceScreen page="income">
      <Section title={t("client:admin.finance.movements.title")}>
        <IncomeMovements {...props} />
      </Section>
    </FinanceScreen>
  );
}

function IncomeMovements({ filter, onFilterChange }: FinanceIncomeProps) {
  const t = useT();
  const intl = useIntl();
  const format = useFormatMoney();
  const formatDay = useFormatDay();
  const list = useIncomeMovements(filter);
  const dates = useMovementDates(filter, onFilterChange);
  const columns: RecordColumns<IncomeMovement> = [
    {
      id: "occurred_at",
      header: t("client:admin.finance.movements.occurred_at"),
      cell: (movement) => (
        <Stack gap="1">
          <DateTime value={movement.occurred_at} />
          <Text size="sm" tone="muted">
            {formatDay(movement.occurred_on)}
          </Text>
        </Stack>
      ),
    },
    {
      id: "kind",
      header: t("client:admin.finance.movements.filter.kind"),
      wrap: "anywhere",
      cell: (movement) => (
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
          <Text size="sm" tone="muted" mono wrap="anywhere" dir="ltr">
            {movement.provider_id}
          </Text>
          {movement.parent_provider_id !== undefined && (
            <Text size="sm" tone="muted" mono wrap="anywhere" dir="ltr">
              {t("client:admin.finance.movements.parent")} {movement.parent_provider_id}
            </Text>
          )}
        </Stack>
      ),
    },
    {
      id: "net",
      header: t("client:income.totals.net"),
      numeric: true,
      wrap: "anywhere",
      cell: (movement) => (
        <Stack gap="1">
          <span>{format(movement.net)}</span>
          <Text size="sm" tone="muted">
            {t("client:admin.finance.movements.gross_fee", {
              gross: format(movement.gross),
              fee: format(movement.fee),
            })}
          </Text>
        </Stack>
      ),
    },
    {
      id: "subscriber",
      header: t("client:admin.finance.movements.subscriber"),
      wrap: "anywhere",
      cell: (movement) => <Subscriber movement={movement} />,
    },
  ];
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
        columns={columns}
        getId={(movement) => movement.id}
        headingLevel={3}
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
      />
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
