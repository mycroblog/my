// SPDX-License-Identifier: AGPL-3.0-only

// The finance views (ADR-0067, FR-902, FR-916, FR-917), each in every state, and the finance screen they make
// together: the totals by month, costs beside income and the balance, in one currency, a bar comparing each month's
// costs and income, the month of today so far and from an estimate; by week, each its dates; in two currencies, a
// figure and a balance a currency and no bars; costs alone, where income is not published, with no bars; a month's
// breakdown, a provider named once over its categories, an estimate marked on its figure, with its total; and the
// two ways of counting a cost longer than a month and the period's length, each a choice. The providers and the
// amounts are samples, formatted as the app would; the rules under a table are the screen's, short.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { SegmentChoice, type SegmentOption } from "./choice";
import { FinanceRow, FinanceTable, type FinanceColumn } from "./finance";
import { Stack } from "./layout";
import { GroupHeading } from "./list";

const rem = (px: number) => `${px / rootPx}rem`;

/** The column of a screen: as wide as the app's main column at most, the frame's edge around it. */
function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(600), padding: rem(16) }}>{children}</div>;
}

const flow: readonly FinanceColumn[] = [
  { label: "Costs", figures: true },
  { label: "Income", figures: true },
  { label: "Balance", figures: true },
];

/** How the screen says the periods are cut and a long cost is counted (FR-902), short. */
const rules = "Calendar months, by date, with no time zone; AWS bills by the month in UTC.";

/** The totals by month of a server that pays in yen alone: each bar its share of the largest figure, ¥18,400. */
function ByMonth() {
  const share = (amount: number) => amount / 18400;
  return (
    <FinanceTable label="Totals by month" head="Month" columns={flow} estimated="Estimate">
      <FinanceRow
        head="November 2026"
        caption="So far"
        cells={[
          { figures: ["¥12,300"], estimated: true, share: share(12300) },
          { figures: ["¥9,840"], share: share(9840) },
          { figures: ["−¥2,460"], estimated: true },
        ]}
      />
      <FinanceRow
        head="October 2026"
        cells={[
          { figures: ["¥14,950"], share: share(14950) },
          { figures: ["¥18,400"], share: share(18400) },
          { figures: ["¥3,450"] },
        ]}
      />
      <FinanceRow
        head="September 2026"
        cells={[
          { figures: ["¥14,820"], share: share(14820) },
          { figures: ["¥11,200"], share: share(11200) },
          { figures: ["−¥3,620"] },
        ]}
      />
      <FinanceRow
        head="August 2026"
        cells={[
          { figures: ["¥13,100"], share: share(13100) },
          { figures: ["¥0"], share: 0 },
          { figures: ["−¥13,100"] },
        ]}
      />
    </FinanceTable>
  );
}

/**
 * The totals by month of a server that pays in yen and in dollars: a figure a currency, never added, and a balance a
 * currency; the dollars of this month from AWS's estimate. No bars: amounts in two currencies do not compare.
 */
function ByMonthInCurrencies() {
  return (
    <FinanceTable label="Totals by month" head="Month" columns={flow} estimated="Estimate" about={rules}>
      <FinanceRow
        head="November 2026"
        caption="So far"
        cells={[
          { figures: ["¥12,300", { shown: "US$45.67", estimated: true }] },
          { figures: ["¥9,840"] },
          { figures: ["−¥2,460", { shown: "−US$45.67", estimated: true }] },
        ]}
      />
      <FinanceRow
        head="October 2026"
        cells={[{ figures: ["¥12,300", "US$44.10"] }, { figures: ["¥18,400"] }, { figures: ["¥6,100", "−US$44.10"] }]}
      />
      <FinanceRow
        head="September 2026"
        cells={[{ figures: ["¥12,300", "US$43.85"] }, { figures: ["¥11,200"] }, { figures: ["−¥1,100", "−US$43.85"] }]}
      />
    </FinanceTable>
  );
}

/** A month's costs, a provider named once over its categories, an amount in its currency; and their total. */
function Breakdown() {
  return (
    <FinanceTable
      label="Costs of November 2026 by provider"
      head="Provider"
      columns={[{ label: "Category" }, { label: "Amount", figures: true }]}
      estimated="Estimate"
      total={<FinanceRow head="Total" cells={["", { figures: ["¥12,300", { shown: "US$45.67", estimated: true }] }]} />}
    >
      <FinanceRow head="AWS" cells={["Servers", { figures: ["US$38.20"], estimated: true }]} />
      <FinanceRow head="AWS" continued cells={["Storage", { figures: ["US$7.47"], estimated: true }]} />
      <FinanceRow head="さくらインターネット" cells={["Servers", { figures: ["¥10,650"] }]} />
      <FinanceRow head="Example Registrar" cells={["Domain", { figures: ["¥150"] }]} />
      <FinanceRow head="Mail Relay Co." cells={["Email", { figures: ["¥1,500"] }]} />
    </FinanceTable>
  );
}

type Granularity = "week" | "month" | "year";
type Basis = "amortized" | "cash";

const granularities: readonly SegmentOption<Granularity>[] = [
  { value: "week", label: "Week" },
  { value: "month", label: "Month" },
  { value: "year", label: "Year" },
];

const bases: readonly SegmentOption<Basis>[] = [
  { value: "amortized", label: "Spread over its period" },
  { value: "cash", label: "When paid" },
];

const basisAbout: Record<Basis, string> = {
  amortized: "A cost for more than a month counts a share each day it covers.",
  cash: "Each cost counts on the day it is paid.",
};

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Finance", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/**
 * The finance screen, composed of the parts: the period's length and the way of counting, each a choice, the second
 * saying what it means; the totals by month in both the server's currencies, the rules of the periods under them; and
 * the month's breakdown, whose total is the month's costs.
 */
export const FinanceScreen: Story = {
  render: () => {
    function Screen() {
      const [granularity, setGranularity] = useState<Granularity>("month");
      const [basis, setBasis] = useState<Basis>("amortized");
      return (
        <Column>
          <Stack gap="entry-room">
            <Stack gap="screen-row-gap">
              <SegmentChoice label="Period" options={granularities} value={granularity} onChange={setGranularity} />
              <SegmentChoice
                label="Count costs"
                options={bases}
                value={basis}
                onChange={setBasis}
                about={basisAbout[basis]}
              />
            </Stack>
            <Stack gap="entry-line">
              <GroupHeading>Totals</GroupHeading>
              <ByMonthInCurrencies />
            </Stack>
            <Stack gap="entry-line">
              <GroupHeading>November 2026 by provider</GroupHeading>
              <Breakdown />
            </Stack>
          </Stack>
        </Column>
      );
    }
    return <Screen />;
  },
};

/**
 * The totals by month, latest first, of a server that pays in yen alone: costs, income and the balance, a bar under
 * the costs and the income, so that a month's two compare at a glance, none under the balance; the month of today so
 * far, its costs and its balance from an estimate, the glyph on each figure, said once under the table. A phone puts
 * each month on a line of its own over its figures.
 */
export const TotalsByMonth: Story = {
  render: () => (
    <Column>
      <ByMonth />
    </Column>
  ),
};

/** The totals by week, each week's dates under its number, the week of today so far. */
export const TotalsByWeek: Story = {
  render: () => {
    const share = (amount: number) => amount / 4100;
    return (
      <Column>
        <FinanceTable label="Totals by week" head="Week" columns={flow} estimated="Estimate">
          <FinanceRow
            head="2026-W47"
            caption="Nov 16 – 22 · So far"
            cells={[
              { figures: ["¥2,870"], estimated: true, share: share(2870) },
              { figures: ["¥1,960"], share: share(1960) },
              { figures: ["−¥910"], estimated: true },
            ]}
          />
          <FinanceRow
            head="2026-W46"
            caption="Nov 9 – 15"
            cells={[
              { figures: ["¥2,870"], share: share(2870) },
              { figures: ["¥4,100"], share: share(4100) },
              { figures: ["¥1,230"] },
            ]}
          />
          <FinanceRow
            head="2026-W45"
            caption="Nov 2 – 8"
            cells={[
              { figures: ["¥2,870"], share: share(2870) },
              { figures: ["¥2,400"], share: share(2400) },
              { figures: ["−¥470"] },
            ]}
          />
        </FinanceTable>
      </Column>
    );
  },
};

/**
 * The totals by year in two currencies, never converted: a figure a currency, one under another, a balance a
 * currency, the dollars of this year from an estimate, and no bars.
 */
export const TotalsInCurrencies: Story = {
  render: () => (
    <Column>
      <FinanceTable label="Totals by year" head="Year" columns={flow} estimated="Estimate">
        <FinanceRow
          head="2026"
          caption="So far"
          cells={[
            { figures: ["¥131,400", { shown: "US$512.30", estimated: true }] },
            { figures: ["¥118,080", "US$96.00"] },
            { figures: ["−¥13,320", { shown: "−US$416.30", estimated: true }] },
          ]}
        />
        <FinanceRow
          head="2025"
          cells={[
            { figures: ["¥142,800", "US$1,204.88"] },
            { figures: ["¥96,000"] },
            { figures: ["−¥46,800", "−US$1,204.88"] },
          ]}
        />
      </FinanceTable>
    </Column>
  ),
};

/** Costs alone, where income is not published: one column of figures, and so no balance and no bars. */
export const CostsOnly: Story = {
  render: () => (
    <Column>
      <FinanceTable
        label="Costs by month"
        head="Month"
        columns={[{ label: "Costs", figures: true }]}
        estimated="Estimate"
      >
        <FinanceRow head="November 2026" caption="So far" cells={[{ figures: ["¥12,300"], estimated: true }]} />
        <FinanceRow head="October 2026" cells={[{ figures: ["¥14,950"] }]} />
        <FinanceRow head="September 2026" cells={[{ figures: ["¥14,820"] }]} />
      </FinanceTable>
    </Column>
  ),
};

/**
 * A month's breakdown: a provider named once, its categories under it, each with its amount in its currency, the
 * estimates marked on their figures; a provider's name the operator's, in any script; and the total, a figure a
 * currency, on the raised surface.
 */
export const BreakdownByProvider: Story = {
  render: () => (
    <Column>
      <Breakdown />
    </Column>
  ),
};

/** The two ways of counting a cost longer than a month, and the period's length: each a choice, the first saying what it means. */
export const Choices: Story = {
  render: () => {
    function Held() {
      const [granularity, setGranularity] = useState<Granularity>("week");
      const [basis, setBasis] = useState<Basis>("cash");
      return (
        <Stack gap="entry-room">
          <SegmentChoice
            label="Count costs"
            options={bases}
            value={basis}
            onChange={setBasis}
            about={basisAbout[basis]}
          />
          <SegmentChoice label="Period" options={granularities} value={granularity} onChange={setGranularity} />
        </Stack>
      );
    }
    return (
      <Column>
        <Held />
      </Column>
    );
  },
};
