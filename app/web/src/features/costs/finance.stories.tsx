// SPDX-License-Identifier: AGPL-3.0-only
// The finances of the server for the holders of view_finance (/finance; web-client/screens/finance.md) in the states of
// ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { RequestHandler } from "msw";
import {
  answering,
  failingOn,
  inLocale,
  loadingOn,
  narrow,
  renderScreen,
  screenLoader,
  settled,
  viewerWith,
} from "../../test/stories";
import { financeCostsOverSixMonths, incomeOverSixMonths } from "./test/storyFixtures";

/** A viewer who may read the finances, of a server that shows the names and memos of the items and the plans. */
const reader = [
  viewerWith({ permissions: ["view_finance"] }),
  financeCostsOverSixMonths,
  incomeOverSixMonths("/finance/income_totals"),
];

/** `story`, whose own handlers answer first, for the reader of `reader`. */
function forReader<S extends { parameters: { screen: { handlers: RequestHandler[] } } }>(story: S): S {
  return { ...story, parameters: { screen: { handlers: [...story.parameters.screen.handlers, ...reader] } } };
}

const meta = {
  title: "Screens/About/Finance",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/finance", signedIn: true, handlers: reader } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Six months of costs with the items of the last one, the income, the balance and the members of each plan. */
export const Normal: Story = {
  play: async (context) => {
    await settled(context);
    const table = await context.canvas.findByRole("table", { name: /^Totals by Months/u });
    // The header row and six periods.
    await expect(within(table).getAllByRole("row")).toHaveLength(7);
  },
};

/** A user without the permission is told why, and nothing is read. */
export const NoPermission: Story = answering(viewerWith({}));

/** The totals have not arrived yet. */
export const Loading: Story = forReader(loadingOn("get", "/finance/cost_totals"));

/** The server failed to answer the costs. */
export const ServerError: Story = forReader(failingOn("get", "/finance/cost_totals"));

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
