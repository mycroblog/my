// SPDX-License-Identifier: AGPL-3.0-only

// The finance of the server as a holder of `manage_finance` keeps it, at /admin/finance (ADR-0067 §7, §16; FR-902,
// FR-903, FR-917): the doors to the cost items, the income, the billing imports and what is published; then the totals
// of the costs and of the income per week, month or year, and the balance, a figure a currency, never converted, with
// the rules that cut the periods; the latest period's costs by provider; and the members of each plan now.

import { useAdminCostTotals, useAdminIncomeTotals, useInstance } from "@my/app-data";
import { FeedNotice, ScreenStack, ScreenTitle, SettingsGroup, SettingsItem, SettingsMenu } from "@my/design-system";
import type { ReactNode } from "react";
import {
  CostBreakdown,
  FinanceChoices,
  FinancePages,
  PlanMembers,
  ReadNotice,
  TotalsTable,
  useFinanceView,
} from "../../finance-totals";
import { useStaffStrings } from "../parts";
import { financePaths } from "./parts";

/** The doors to the screens under Finance, the publication's with what is published now. */
function Doors() {
  const { t } = useStaffStrings();
  const instance = useInstance();
  const published =
    instance === undefined
      ? undefined
      : t("staff_finance.publication.now", {
          costs: instance.finance_visibility,
          income: instance.income_visibility,
        });
  return (
    <SettingsMenu label={t("staff_finance.doors")}>
      <SettingsGroup heading={t("staff_finance.doors")}>
        <SettingsItem
          icon="finance"
          label={t("staff_finance.costs")}
          hint={t("staff_finance.costs.hint")}
          count={undefined}
          href={financePaths.costs}
        />
        <SettingsItem
          icon="funding"
          label={t("staff_finance.income")}
          hint={t("staff_finance.income.hint")}
          count={undefined}
          href={financePaths.income}
        />
        <SettingsItem
          icon="update"
          label={t("staff_finance.imports")}
          hint={t("staff_finance.imports.hint")}
          count={undefined}
          href={financePaths.imports}
        />
        <SettingsItem
          icon="public"
          label={t("staff_finance.publication")}
          hint={published ?? t("staff_finance.publication.hint")}
          count={undefined}
          href={financePaths.publication}
        />
      </SettingsGroup>
    </SettingsMenu>
  );
}

/** The first page of Finance. */
export function FinanceAdminScreen(): ReactNode {
  const { t } = useStaffStrings();
  const view = useFinanceView();
  const costs = useAdminCostTotals(view.query);
  const income = useAdminIncomeTotals(view.query);
  const title = <ScreenTitle>{t("staff_finance.entry")}</ScreenTitle>;
  if (costs.forbidden || income.forbidden)
    return (
      <ScreenStack>
        {title}
        <FeedNotice lead>{t("staff_finance.forbidden")}</FeedNotice>
      </ScreenStack>
    );
  const spent = costs.value;
  const got = income.value;
  const pages = <FinancePages view={view} />;
  return (
    <ScreenStack>
      {title}
      <Doors />
      <FinanceChoices view={view} about={false} />
      {spent === undefined || got === undefined ? (
        <ReadNotice reads={[costs, income]} />
      ) : (
        <TotalsTable
          view={view}
          costs={spent.periods}
          income={got.periods}
          balance
          sources={spent.rules.sources}
          incomeRules={got.rules}
          rules={false}
          coded
        />
      )}
      {pages}
      {spent !== undefined && <CostBreakdown view={view} period={spent.periods.at(-1)} sources={spent.rules.sources} />}
      {got !== undefined && <PlanMembers plans={got.plans} tips={got.tips} />}
    </ScreenStack>
  );
}
