// SPDX-License-Identifier: AGPL-3.0-only

// The finance screens (ADR-0067 §8, §17, §22, ADR-0070 §5; FR-902, FR-903, FR-905, FR-916, FR-917), on the fake server
// (app.samples.tsx), whose costs are in yen and dollars (a quarterly and a yearly cost among them, AWS imported by its
// billing month) and whose income holds yearly payments, tips and a refund. The page everyone reads, at /about/costs:
// where the server publishes nothing; the funding meter alone; the costs' totals alone; the breakdown and the income
// beside them with the balance, and the meter's amounts; the income alone with the members of each plan; an estimate
// in this month's costs; a visitor; loading and failing. The page of a holder of `view_finance`, at /finance: the
// totals alone; the items' names and notes; their notes alone; the members of each plan; an estimate; the
// administrator, who sees every item; a reader who may not read it; a visitor; loading and failing. The funding meter
// stands in the column beside the timeline too, wherever the server shows it; the choices of the period and of how a
// payment counts, and the earlier periods, are the route tests' (finance.dom.test.tsx).

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta, type Fake } from "./app.samples";

type Finance = NonNullable<NonNullable<Fake["server"]>["finance"]>;
type More = Omit<Fake, "at" | "server"> & { supporters?: boolean };

const meta = {
  title: "Finance",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/about/costs" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The fake's server with `finance`, and its supporter program where `supporters` (the members of its plans). */
const serverOf = (finance: Finance, { supporters = false, ...more }: More) => ({
  server: { finance, ...(supporters ? { supporters: {} } : {}) },
  ...more,
});

const publicAt = (finance: Finance, more: More = {}) => screen({ at: "/about/costs", ...serverOf(finance, more) });

/** The server publishes neither its costs nor its income, nor a meter: the page says so alone. */
export const PublicPrivate: Story = { parameters: publicAt({ meter: "none" }) };
/** Neither side published, the meter's share alone: this month's funding, then that the totals are not published. */
export const PublicMeterOnly: Story = { parameters: publicAt({ meter: "percent" }) };
/** The costs' totals alone, by month, a figure a currency, the rules under them; the income is not published. */
export const PublicTotals: Story = { parameters: publicAt({ costs: "totals", meter: "none" }) };
/**
 * The costs with their breakdown and the income beside them, the balance, the month's costs by provider (a yearly and
 * a quarterly cost spread over their periods, AWS by its billing month), and the meter with its amounts against the
 * operators' target in yen, the currency it counts in named.
 */
export const PublicBreakdown: Story = {
  parameters: publicAt({ costs: "breakdown", income: "totals", meter: "amounts" }),
};
/** The income alone, with the members of each plan and the tips it holds; the costs are not published. */
export const PublicIncomeOnly: Story = {
  parameters: publicAt({ income: "plans", meter: "none" }, { supporters: true }),
};
/**
 * A server that pays in dollars alone, whose target is the month's costs: this month's AWS bill is not closed, so its
 * costs, its balance and its line are estimates, and the meter, counted in dollars, says so.
 */
export const PublicEstimated: Story = {
  parameters: publicAt({
    costs: "breakdown",
    income: "totals",
    estimated: true,
    meter: "amounts",
    meterTarget: "costs",
    currencies: ["USD"],
  }),
};
/** A visitor reads the page as anyone does. */
export const PublicVisitor: Story = {
  parameters: publicAt({ costs: "breakdown", income: "totals", meter: "amounts" }, { signedOut: true }),
};
export const PublicLoading: Story = {
  parameters: publicAt({ costs: "breakdown", income: "totals", meter: "none" }, { state: "loading" }),
};
export const PublicError: Story = {
  parameters: publicAt({ costs: "breakdown", income: "totals", meter: "none" }, { state: "error" }),
};

const financeAt = (finance: Finance, more: More = {}) => screen({ at: "/finance", ...serverOf(finance, more) });

/** A holder of `view_finance` with the settings' defaults: the totals, the balance, the breakdown; no items. */
export const Finance: Story = { parameters: financeAt({ viewer: {}, meter: "none" }) };
/** The settings show the items' names and notes: the month's items, each its name, its provider, its note. */
export const FinanceNamesNotes: Story = {
  parameters: financeAt({ viewer: { itemNames: true, itemMemos: true }, meter: "none" }),
};
/** The settings show the notes alone: the items headed by their provider, a note under those that have one. */
export const FinanceNotesOnly: Story = { parameters: financeAt({ viewer: { itemMemos: true }, meter: "none" }) };
/** The settings show the members of each plan: a row a plan, its headcount. */
export const FinancePlans: Story = {
  parameters: financeAt({ viewer: { income: "plans" }, meter: "none" }, { supporters: true }),
};
/** The settings show the members of each plan, and no plan has members: one line says so. */
export const FinancePlansNone: Story = { parameters: financeAt({ viewer: { income: "plans" }, meter: "none" }) };
/** This month's AWS bill is not closed: its figures are estimates. */
export const FinanceEstimated: Story = { parameters: financeAt({ viewer: {}, estimated: true, meter: "none" }) };
/** The administrator, who holds `manage_finance`: every item's name and note, and the members of each plan. */
export const FinanceAdmin: Story = { parameters: financeAt({ meter: "none" }, { viewer: "hana", supporters: true }) };
/** A reader who may not read the server's finances: the page says so alone. */
export const FinanceForbidden: Story = { parameters: financeAt({ meter: "none" }) };
/** A visitor is asked to sign in. */
export const FinanceVisitor: Story = { parameters: financeAt({ meter: "none" }, { signedOut: true }) };
export const FinanceLoading: Story = { parameters: financeAt({ viewer: {}, meter: "none" }, { state: "loading" }) };
export const FinanceError: Story = { parameters: financeAt({ viewer: {}, meter: "none" }, { state: "error" }) };
