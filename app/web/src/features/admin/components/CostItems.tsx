// SPDX-License-Identifier: AGPL-3.0-only
// The recorded costs of the costs screen (`/admin/finance/costs`, ADR-0067 §7, manage_finance;
// web-client/screens/admin-finance-costs.md): the items that are not canceled, newest first, one page at a time
// (web-client §14 一覧の表), each row its name linking to its page, how often it is paid, its price today and its first
// payment date. The page's one primary action, "Record a cost", leads to the new item's page; changing an item is on
// its own page. Names are the operator's words, shown with dir="auto"; amounts and dates are formatted with Intl
// (ADR-0067 §10).
import { ButtonLink, EmptyState, Link, Text } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { useFormatMoney } from "../../costs";
import { type RecordColumns, RecordTable, Section } from "../../moderation";
import { type CostItem, priceOn } from "../api/finance";
import { useCostItems, useFormatDay } from "../hooks/useFinance";
import { FinanceScreen, useScheduleText } from "./FinanceParts";

export function CostItems({ today }: { today: string }) {
  const t = useT();
  const list = useCostItems();
  const formatDay = useFormatDay();
  const format = useFormatMoney();
  const scheduleText = useScheduleText();
  const create = (
    <ButtonLink href="/admin/finance/costs/new" variant="primary">
      {t("client:admin.finance.new.title")}
    </ButtonLink>
  );
  const columns: RecordColumns<CostItem> = [
    {
      id: "name",
      header: t("client:admin.finance.name"),
      wrap: "anywhere",
      cell: (item) => (
        <Link href={`/admin/finance/costs/${item.id}`}>
          <span dir="auto">{item.name}</span>
        </Link>
      ),
    },
    {
      id: "schedule",
      header: t("client:admin.finance.schedule"),
      cell: (item) => scheduleText(item),
    },
    {
      id: "price",
      header: t("client:admin.finance.price"),
      numeric: true,
      wrap: "anywhere",
      cell: (item) => {
        // Before the start date, the price of the first payment.
        const price = priceOn(item, today) ?? item.prices[0]?.price;
        return price === undefined ? null : <Text numeric>{format(price)}</Text>;
      },
    },
    {
      id: "starts_on",
      header: t("client:admin.finance.starts_on"),
      cell: (item) => formatDay(item.starts_on),
    },
  ];
  return (
    <FinanceScreen page="costs">
      <Section title={t("client:admin.finance.list.title")}>
        {list.items.length > 0 && <div>{create}</div>}
        <RecordTable
          list={list}
          caption={t("client:admin.finance.list.title")}
          columns={columns}
          getId={(item) => item.id}
          headingLevel={3}
          empty={
            <EmptyState
              headingLevel={3}
              heading={t("client:admin.finance.empty")}
              description={t("client:admin.finance.empty.description")}
              action={create}
            />
          }
        />
      </Section>
    </FinanceScreen>
  );
}
