// SPDX-License-Identifier: AGPL-3.0-only
// The totals of the instance's costs as the administrators' costs screen, the public costs page and the finances page
// show them (ADR-0067 §4, §5, FR-902, FR-903, ADR-0070 §5): the choice of the basis and the tabs of the granularity,
// the table of the periods, newest first, with their totals by currency, the breakdown of the current period by
// provider and category when it may be shown, the lines of its items on the finances page, and the rules that cut the
// periods. Providers, names and memos are the operators' words, shown with dir="auto".
import {
  Cluster,
  Heading,
  Hint,
  Radio,
  RadioGroup,
  Stack,
  Tab,
  Table,
  TableBody,
  TableCell,
  TableColumnHeader,
  TableHead,
  TabList,
  TableRow,
  TableRowHeader,
  TabPanel,
  Tabs,
  Tag,
  Text,
} from "@my/ui";
import { type ReactNode, useId } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import {
  type CostBasis,
  costBases,
  type CostCategory,
  type CostGranularity,
  costGranularities,
  type CostPeriodTotal,
  type CostTotals,
  type FinanceCostItemLine,
} from "../api/costs";
import { useFormatMoney, useFormatTotals, usePeriodName } from "../hooks/useCostFormat";

const granularityLabels: Readonly<Record<CostGranularity, ClientMessageKey>> = {
  week: "client:costs.granularity.week",
  month: "client:costs.granularity.month",
  year: "client:costs.granularity.year",
};

const basisLabels: Readonly<Record<CostBasis, { label: ClientMessageKey; description: ClientMessageKey }>> = {
  amortized: { label: "client:costs.basis.amortized", description: "client:costs.basis.amortized.description" },
  cash: { label: "client:costs.basis.cash", description: "client:costs.basis.cash.description" },
};

/** The words of each category (ADR-0067 §2). */
export const categoryLabels: Readonly<Record<CostCategory, ClientMessageKey>> = {
  compute: "client:costs.category.compute",
  database: "client:costs.category.database",
  storage: "client:costs.category.storage",
  network: "client:costs.category.network",
  domain: "client:costs.category.domain",
  email: "client:costs.category.email",
  software: "client:costs.category.software",
  other: "client:costs.category.other",
};

function isGranularity(key: unknown): key is CostGranularity {
  return costGranularities.some((item) => item === key);
}

function isBasis(key: unknown): key is CostBasis {
  return costBases.some((item) => item === key);
}

export type CostTotalsPanelProps = {
  /** The granularity and the basis of the totals, from the URL. */
  granularity: CostGranularity;
  basis: CostBasis;
  /** Selects another granularity or basis: the route puts it into the URL. */
  onTotalsChange: (next: { granularity: CostGranularity; basis: CostBasis }) => void;
  /** The totals of the selected granularity and basis, in the panel of the selected tab. */
  children: ReactNode;
};

/** The choice of the basis and the tabs of the granularity, around the totals they select. */
export function CostTotalsPanel({ granularity, basis, onTotalsChange, children }: CostTotalsPanelProps) {
  const t = useT();
  return (
    <Stack gap="4">
      <RadioGroup
        label={t("client:costs.basis")}
        orientation="horizontal"
        value={basis}
        onChange={(value) => {
          if (isBasis(value)) onTotalsChange({ granularity, basis: value });
        }}
      >
        {costBases.map((item) => (
          <Radio key={item} value={item} description={t(basisLabels[item].description)}>
            {t(basisLabels[item].label)}
          </Radio>
        ))}
      </RadioGroup>
      <Tabs
        selectedKey={granularity}
        onSelectionChange={(key) => {
          if (isGranularity(key)) onTotalsChange({ granularity: key, basis });
        }}
      >
        <TabList aria-label={t("client:costs.granularity")}>
          {costGranularities.map((item) => (
            <Tab key={item} id={item}>
              {t(granularityLabels[item])}
            </Tab>
          ))}
        </TabList>
        {/* One panel per tab: React Aria mounts only the selected one, and each keeps the id its tab's aria-controls names. */}
        {costGranularities.map((item) => (
          <TabPanel key={item} id={item}>
            {children}
          </TabPanel>
        ))}
      </Tabs>
    </Stack>
  );
}

/** A period of the totals, with the lines of its items when the finances page reads them. */
type ShownPeriod = CostPeriodTotal & { items?: readonly FinanceCostItemLine[] };

/** The totals of any of the screens: the finances page's periods also have the lines of their items. */
type ShownTotals = Omit<CostTotals, "periods"> & { periods: readonly ShownPeriod[] };

export type CostTotalsViewProps = {
  totals: ShownTotals;
  /** Shows the breakdown of the current period by provider and category: the public page only when the instance publishes it. */
  breakdown: boolean;
  /** Shows the lines of the current period's items: the finances page while the instance shows their names or memos. */
  items?: boolean;
};

/** The totals of the shown periods, newest first, and the rules that cut them. */
export function CostTotalsView({ totals, breakdown, items = false }: CostTotalsViewProps) {
  const t = useT();
  const periodName = usePeriodName();
  const formatTotals = useFormatTotals();
  const { granularity, basis } = totals;
  const newestFirst = totals.periods.toReversed();
  const current = newestFirst[0];
  return (
    <Stack gap="4">
      <Table
        caption={t("client:costs.totals.caption", {
          granularity: t(granularityLabels[granularity]),
          basis: t(basisLabels[basis].label),
        })}
      >
        <TableHead>
          <TableColumnHeader>{t("client:costs.totals.period")}</TableColumnHeader>
          <TableColumnHeader>{t("client:costs.totals.total")}</TableColumnHeader>
        </TableHead>
        <TableBody>
          {newestFirst.map((period) => (
            <TableRow key={period.label}>
              <TableRowHeader>{periodName(granularity, period)}</TableRowHeader>
              <TableCell numeric wrap="anywhere">
                <Cluster as="span" align="center" gap="2">
                  {formatTotals(period.totals)}
                  {period.estimated && <Tag>{t("client:costs.totals.estimated")}</Tag>}
                </Cluster>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {breakdown && current !== undefined && <Breakdown name={periodName(granularity, current)} period={current} />}
      {items && current !== undefined && (
        <ItemLines name={periodName(granularity, current)} lines={current.items ?? []} />
      )}
      <PeriodRules totals={totals} />
    </Stack>
  );
}

/** The breakdown of the current period by provider and category. */
function Breakdown({ name, period }: { name: string; period: CostPeriodTotal }) {
  const t = useT();
  const format = useFormatMoney();
  const headingId = useId();
  return (
    <Stack as="section" aria-labelledby={headingId} gap="2">
      <Heading level={3} id={headingId}>
        {t("client:costs.breakdown.title", { period: name })}
      </Heading>
      {period.breakdown.length === 0 ? (
        <Hint size="inherit">{t("client:costs.breakdown.empty")}</Hint>
      ) : (
        <Stack as="ul" gap="1">
          {period.breakdown.map((line) => (
            <Cluster as="li" key={`${line.provider} ${line.category} ${line.amount.currency}`} gap="2" rowGap="0">
              <Text dir="auto" wrap="anywhere">
                {line.provider}
              </Text>
              <Text tone="muted">{t(categoryLabels[line.category])}</Text>
              <Text numeric>{format(line.amount)}</Text>
            </Cluster>
          ))}
        </Stack>
      )}
    </Stack>
  );
}

/**
 * The items of the current period, each cost item and each service of the imported costs with its amount: the name
 * and the memo only where the instance shows them, so a line may have neither. The lines carry no ID, and two of them
 * may show the same words, so they are keyed by their place in the answer, which is replaced whole.
 */
function ItemLines({ name, lines }: { name: string; lines: readonly FinanceCostItemLine[] }) {
  const t = useT();
  const format = useFormatMoney();
  const headingId = useId();
  return (
    <Stack as="section" aria-labelledby={headingId} gap="2">
      <Heading level={3} id={headingId}>
        {t("client:costs.items.title", { period: name })}
      </Heading>
      {lines.length === 0 ? (
        <Hint size="inherit">{t("client:costs.items.empty")}</Hint>
      ) : (
        <Stack as="ul" gap="2">
          {lines.map((line, index) => (
            <Stack as="li" key={`${String(index)} ${line.provider} ${line.category} ${line.amount.currency}`}>
              <Cluster as="span" gap="2" rowGap="0">
                {line.name !== undefined && (
                  <Text dir="auto" weight="medium" wrap="anywhere">
                    {line.name}
                  </Text>
                )}
                <Text dir="auto" wrap="anywhere">
                  {line.provider}
                </Text>
                <Text tone="muted">{t(categoryLabels[line.category])}</Text>
                <Text numeric>{format(line.amount)}</Text>
              </Cluster>
              {line.memo !== undefined && (
                <Hint as="span" dir="auto" whitespace="preLine" wrap="anywhere">
                  {line.memo}
                </Hint>
              )}
            </Stack>
          ))}
        </Stack>
      )}
    </Stack>
  );
}

/** How the totals cut their periods and place the payments (ADR-0067 §5, FR-902). */
function PeriodRules({ totals }: { totals: ShownTotals }) {
  const t = useT();
  const headingId = useId();
  return (
    <Stack as="section" aria-labelledby={headingId} gap="2" tone="muted">
      <Heading level={3} id={headingId} tone="fg">
        {t("client:costs.rules.title")}
      </Heading>
      <Stack as="ul" marker="disc" gap="1">
        <li>{t("client:costs.rules.dates")}</li>
        <li>{t("client:costs.rules.week_start")}</li>
        <li>{t(totals.basis === "amortized" ? "client:costs.rules.amortized" : "client:costs.rules.cash")}</li>
        <li>{t("client:costs.rules.currencies")}</li>
        {totals.rules.sources.map((source) => (
          <li key={source.source}>
            {t("client:costs.rules.source", { provider: source.provider, timeZone: source.time_zone })}
          </li>
        ))}
        {/* Without the sources (the public totals only), an estimated period still needs the explanation of its badge. */}
        {(totals.rules.sources.length > 0 || totals.periods.some((period) => period.estimated)) && (
          <li>{t("client:costs.rules.estimated")}</li>
        )}
      </Stack>
    </Stack>
  );
}
