// SPDX-License-Identifier: AGPL-3.0-only
// The user's page of the supporter plans (/settings/supporter) in the states of
// ADR-0079 §13: no subscription yet, an active one, one that is cancelling, one whose payment failed.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "storybook/test";
import { http, HttpResponse, type RequestHandler } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem, withViewer } from "../../test/msw/http";
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
import { ownSubscriptionFixture } from "./test/handlers";
import { coffeeId, loafId, offeringPlans, programOn } from "./test/storyFixtures";

/** GET /supporter/subscription answering `own`. */
function owning(own: Schemas["OwnSupporterSubscription"]) {
  return http.get(`${apiBase}/supporter/subscription`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(own)),
  );
}

const program = [programOn("everyone"), offeringPlans];

/** A story whose server has the program on, three plans, and the viewer's subscription `own`. */
function subscribed(own: Schemas["OwnSupporterSubscription"], ...more: RequestHandler[]) {
  return answering(owning(own), ...more, ...program);
}

const meta = {
  title: "Screens/Settings/Supporter",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings/supporter", signedIn: true, handlers: [owning({}), ...program] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Three plans to choose from, without a subscription yet. */
export const Normal: Story = {};

/** An active monthly subscription with a change to a higher plan scheduled. */
export const Active: Story = subscribed({
  subscription: ownSubscriptionFixture(coffeeId, {
    plan_name: "Coffee",
    scheduled_change: {
      plan_id: loafId,
      plan_name: "A whole loaf of sourdough every month",
      interval: "month",
      price: { amount: "1500", currency: "JPY" },
      effective_at: "2026-11-20T00:00:00Z",
    },
  }),
});

/** The payment of a change to a higher plan failed: the plan stays, with the way to update the card. */
export const ChangePaymentFailed: Story = {
  ...subscribed(
    { subscription: ownSubscriptionFixture(coffeeId, { plan_name: "Coffee" }) },
    http.post(`${apiBase}/supporter/subscription/change`, ({ request }) =>
      withViewer(request, () => problem(402, "/api/v1/problems/supporter-payment-failed", "The payment failed")),
    ),
  ),
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    await userEvent.click(canvas.getByRole("radio", { name: "A whole loaf of sourdough every month" }));
    await userEvent.click(canvas.getByRole("button", { name: "Change the plan" }));
    const dialog = await within(document.body).findByRole("alertdialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Change the plan" }));
    await canvas.findByText(/The payment failed, so your plan has not changed/u);
  },
};

/** A subscription that ends at the end of the period, which the user may resume. */
export const Canceling: Story = subscribed({
  subscription: ownSubscriptionFixture(coffeeId, { plan_name: "Coffee", state: "canceling" }),
});

/** A failed payment, with the end of the grace period. */
export const PastDue: Story = subscribed({
  subscription: ownSubscriptionFixture(coffeeId, {
    plan_name: "Coffee",
    state: "past_due",
    grace_ends_at: "2026-10-27T00:00:00Z",
  }),
});

/** A user who must verify their email address first. */
export const Ineligible: Story = subscribed({ ineligible_reason: "email_unverified" });

/** A server that does not offer supporter plans. */
export const Empty: Story = answering(
  http.get(`${apiBase}/instance/supporter_plans`, () => problem(404, "about:blank", "Not Found")),
  http.get(`${apiBase}/supporter/subscription`, () => problem(404, "about:blank", "Not Found")),
);

/** The plans have not arrived yet. */
export const Loading: Story = loadingOn("get", "/instance/supporter_plans");

/** The server failed to answer the subscription. */
export const ServerError: Story = answering(
  ...failingOn("get", "/supporter/subscription").parameters.screen.handlers,
  ...program,
);

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
