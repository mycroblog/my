// SPDX-License-Identifier: AGPL-3.0-only

// The finance views (ADR-0067, FR-902, FR-916, FR-917), each in every state, and the finance screen they make
// together: the totals by month, costs beside income and the balance, in one currency with their bars, the month of
// today so far and from an estimate; by week, each its dates; in two currencies, a figure a currency and no bars;
// costs alone, where income is not published; a month's breakdown by provider, category and currency, with its
// total; and the two ways of counting a cost longer than a month and the period's length, each a choice. The
// providers and the amounts are samples, formatted as the app would.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { SegmentChoice, type SegmentOption } from "./choice";
import { FinanceRow, FinanceTable, type FinanceColumn } from "./finance";
import { Box, Stack } from "./layout";
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

/** The totals by month, in yen: each figure's bar its share of the largest, ¥18,400. */
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

/** A month's costs by provider, category and currency, and their total. */
function Breakdown() {
  return (
    <FinanceTable
      label="Costs of November 2026 by provider"
      head="Provider"
      columns={[{ label: "Category" }, { label: "Amount", figures: true }]}
      estimated="Estimate"
      total={<FinanceRow head="Total" cells={["", { figures: ["¥12,300", "US$45.67"], estimated: true }]} />}
    >
      <FinanceRow head="AWS" cells={["Servers", { figures: ["US$38.20"], estimated: true }]} />
      <FinanceRow head="AWS" cells={["Storage", { figures: ["US$7.47"], estimated: true }]} />
      <FinanceRow head="さくらインターネット" cells={["Servers", { figures: ["¥10,650"] }]} />
      <FinanceRow
        head="Example Registrar"
        caption="Paid yearly, spread over its year"
        cells={["Domain", { figures: ["¥150"] }]}
      />
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
  amortized: "A cost paid for more than a month counts a share each day it covers: what a month costs.",
  cash: "Each cost counts on the day it is paid: what left the account, to check against a statement.",
};

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Finance", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/**
 * The finance screen, composed of the parts: the period's length and the way of counting, each a choice, the second
 * saying what it means; the totals by month; the rules of the periods under them; and the month's breakdown.
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
              <ByMonth />
              <Box as="p" text="caption" tone="muted" breaks="phrase">
                Months are calendar months, by date, with no time zone. AWS bills in UTC; an estimate turns final once
                the month closes.
              </Box>
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
 * The totals by month, latest first: costs, income and the balance, in one currency, so each figure has its bar, its
 * share of the table's largest; the month of today so far, its costs and its balance from an estimate. A phone puts
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

/** The totals by year in two currencies, never converted: a figure a currency, one under another, and no bars. */
export const TotalsInCurrencies: Story = {
  render: () => (
    <Column>
      <FinanceTable label="Totals by year" head="Year" columns={flow} estimated="Estimate">
        <FinanceRow
          head="2026"
          caption="So far"
          cells={[
            { figures: ["¥131,400", "US$512.30"], estimated: true },
            { figures: ["¥118,080", "US$96.00"] },
            { figures: ["−¥13,320", "−US$416.30"], estimated: true },
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

/** Costs alone, where income is not published: one column of figures, and so no balance. */
export const CostsOnly: Story = {
  render: () => (
    <Column>
      <FinanceTable
        label="Costs by month"
        head="Month"
        columns={[{ label: "Costs", figures: true }]}
        estimated="Estimate"
      >
        <FinanceRow
          head="November 2026"
          caption="So far"
          cells={[{ figures: ["¥12,300"], estimated: true, share: 12300 / 14950 }]}
        />
        <FinanceRow head="October 2026" cells={[{ figures: ["¥14,950"], share: 1 }]} />
        <FinanceRow head="September 2026" cells={[{ figures: ["¥14,820"], share: 14820 / 14950 }]} />
      </FinanceTable>
    </Column>
  ),
};

/**
 * A month's breakdown: a provider a row, with its category and its amount in its currency, a provider's name the
 * operator's, in any script; a cost paid yearly, spread over its year; and the total, a figure a currency, on the
 * raised surface.
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
