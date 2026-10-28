// SPDX-License-Identifier: AGPL-3.0-only
// The public costs and income of the server (/about/costs; web-client/screens/about-costs.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { http, HttpResponse, type RequestHandler } from "msw";
import { instanceFixture, type Schemas } from "../../test/fixtures";
import { apiBase, problem } from "../../test/msw/http";
import {
  answering,
  failingOn,
  inLocale,
  loadingOn,
  narrow,
  renderScreen,
  screenLoader,
  settled,
} from "../../test/stories";
import { incomeOverSixMonths, publicCostsOverSixMonths } from "./test/storyFixtures";

/** GET /instance answering a server that publishes its finances as `visibility` says. */
function publishing(visibility: Pick<Schemas["Instance"], "finance_visibility" | "income_visibility">) {
  return http.get(`${apiBase}/instance`, () => HttpResponse.json({ ...instanceFixture, ...visibility }));
}

/** A server that publishes the breakdown of its costs, its income and its plans, over six months. */
const open = [
  publishing({ finance_visibility: "breakdown", income_visibility: "plans" }),
  publicCostsOverSixMonths,
  incomeOverSixMonths("/instance/income_totals"),
];

/** `story`, whose own handlers answer first, on the server of `open`. */
function withOpen<S extends { parameters: { screen: { handlers: RequestHandler[] } } }>(story: S): S {
  return { ...story, parameters: { screen: { handlers: [...story.parameters.screen.handlers, ...open] } } };
}

const meta = {
  title: "Screens/About/Costs",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/about/costs", handlers: open } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Six months of costs with the breakdown of the last one, the income, the balance and the plans. */
export const Normal: Story = {
  play: async (context) => {
    await settled(context);
    const table = await context.canvas.findByRole("table", { name: /^Totals by Months/u });
    // The header row and six periods.
    await expect(within(table).getAllByRole("row")).toHaveLength(7);
  },
};

/** The totals of the costs only, without their breakdown or any income. */
export const TotalsOnly: Story = answering(
  publishing({ finance_visibility: "totals", income_visibility: "private" }),
  publicCostsOverSixMonths,
);

/** A server that publishes nothing: the page says so, without any amount. */
export const Empty: Story = answering(
  publishing({ finance_visibility: "private", income_visibility: "private" }),
  http.get(`${apiBase}/instance/cost_totals`, () => problem(404, "about:blank", "Not Found")),
);

/** The totals have not arrived yet. */
export const Loading: Story = withOpen(loadingOn("get", "/instance/cost_totals"));

/** The server failed to answer the costs. */
export const ServerError: Story = withOpen(failingOn("get", "/instance/cost_totals"));

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
