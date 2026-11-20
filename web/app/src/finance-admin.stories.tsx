// SPDX-License-Identifier: AGPL-3.0-only

// Finance in the server's administration (ADR-0067, ADR-0068; FR-901–FR-905, FR-917), on the fake server
// (app.samples.tsx) as its administrator (hana) reads it: five cost items in yen and dollars, among them a VPS whose
// price went up in April and a domain that ends, the monthly subscriptions and yearly ones, tips and a refund as the
// income, and AWS as the billing import. The root of the administration with Finance; the first page, its totals by
// month, loading, failing, and for a reader without the permission. The costs: the list, none, loading, failing; a new
// cost; a cost with its two prices, each with the date it applies from; one that ends; an address that names none.
// The income: its totals and its money movements, the refunds alone. The billing imports: as they are, one that failed
// and why, none set up. What is published: nothing, everything with the meter's amounts, one side alone (which leaves
// the meter off). What follows a press (recording, revising, ending, deleting, saving, importing now) is the route
// tests' and the E2E's.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta, type Fake } from "./app.samples";

type Server = NonNullable<Fake["server"]>;

const meta = {
  title: "Administration/Finance",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/admin/finance", viewer: "hana" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The screen at `path` as the administrator reads it, the fake's finance as `server` says. */
const at = (path: string, server: Server = {}, more: Omit<Fake, "at" | "server"> = {}) =>
  screen({ at: path, viewer: "hana", server, ...more });

/** The administration's root, with Finance and Supporters under Funding. */
export const AdminRoot: Story = { parameters: at("/admin") };

/** The first page: the doors, then the totals of the last six months, the latest month's costs, the plans' members. */
export const Finance: Story = { parameters: at("/admin/finance") };
export const FinanceLoading: Story = { parameters: at("/admin/finance", {}, { state: "loading" }) };
export const FinanceError: Story = { parameters: at("/admin/finance", {}, { state: "error" }) };
/** Finance's address, for a moderator, whose permissions do not open it. */
export const FinanceNotOpen: Story = { parameters: screen({ at: "/admin/finance", viewer: "marco" }) };

/** The cost items, newest first: each its provider, how often, its price today, and whether it ends. */
export const Costs: Story = { parameters: at("/admin/finance/costs") };
/** The cost items' second page, two a page. */
export const CostsSecondPage: Story = { parameters: at("/admin/finance/costs", { staffPage: 2 }, { next: true }) };
/** No cost recorded yet. */
export const CostsEmpty: Story = { parameters: at("/admin/finance/costs", { financeAdmin: { costs: "none" } }) };
export const CostsLoading: Story = { parameters: at("/admin/finance/costs", {}, { state: "loading" }) };
export const CostsError: Story = { parameters: at("/admin/finance/costs", {}, { state: "error" }) };

/** The form of a new cost, monthly from today, in the newest item's currency. */
export const NewCost: Story = { parameters: at("/admin/finance/costs/new") };

/** The VPS: its facts, its two prices, each with the date it applies from, and what changes it. */
export const Cost: Story = { parameters: at("/admin/finance/costs/{cost:vps}") };
/** The domain, paid every year, which ends: its last payment date, Move the end and Remove the end. */
export const CostEnding: Story = { parameters: at("/admin/finance/costs/{cost:domain}") };
/** An address that names no cost item. */
export const CostMissing: Story = { parameters: at("/admin/finance/costs/none") };
export const CostLoading: Story = { parameters: at("/admin/finance/costs/{cost:vps}", {}, { state: "loading" }) };
export const CostError: Story = { parameters: at("/admin/finance/costs/{cost:vps}", {}, { state: "error" }) };

/** The income by month (gross, fees, net, balance), the latest month's net by source, and the money movements. */
export const Income: Story = { parameters: at("/admin/finance/income") };
/** The money movements' second page, two a page. */
export const IncomeSecondPage: Story = { parameters: at("/admin/finance/income", { staffPage: 2 }, { next: true }) };
/** The refunds alone among the money movements. */
export const IncomeRefunds: Story = { parameters: at("/admin/finance/income?kind=refund") };
/** No chargeback ever: the filter's notice, with Clear filters. */
export const IncomeNoMatch: Story = { parameters: at("/admin/finance/income?kind=chargeback") };
export const IncomeLoading: Story = { parameters: at("/admin/finance/income", {}, { state: "loading" }) };
export const IncomeError: Story = { parameters: at("/admin/finance/income", {}, { state: "error" }) };

/** AWS: its last import, the requests against their caps, and its billing months, this one an estimate. */
export const Imports: Story = { parameters: at("/admin/finance/imports") };
/** AWS's last import failed: why, and what to do. */
export const ImportsFailing: Story = {
  parameters: at("/admin/finance/imports", { financeAdmin: { imports: "failing" } }),
};
/** No billing source in the configuration. */
export const ImportsNone: Story = { parameters: at("/admin/finance/imports", { financeAdmin: { imports: "none" } }) };
export const ImportsLoading: Story = { parameters: at("/admin/finance/imports", {}, { state: "loading" }) };
export const ImportsError: Story = { parameters: at("/admin/finance/imports", {}, { state: "error" }) };

/** Nothing published, the meter's percentage alone, which both sides private allow. */
export const Publication: Story = { parameters: at("/admin/finance/publication", { finance: { meter: "percent" } }) };
/** Everything published, the meter with its amounts. */
export const PublicationEverything: Story = {
  parameters: at("/admin/finance/publication", { finance: { costs: "breakdown", income: "plans", meter: "amounts" } }),
};
/** The costs' totals alone: the meter cannot be shown, and says why. */
export const PublicationOneSide: Story = {
  parameters: at("/admin/finance/publication", { finance: { costs: "totals", meter: "none" } }),
};
export const PublicationLoading: Story = { parameters: at("/admin/finance/publication", {}, { state: "loading" }) };
export const PublicationError: Story = { parameters: at("/admin/finance/publication", {}, { state: "error" }) };
