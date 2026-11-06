// SPDX-License-Identifier: AGPL-3.0-only
// The pages of the supporters screen after the program (web-client/screens/admin-supporters-*.md) in the states of
// ADR-0079 §13: the plans, a new plan, one plan (in effect and retired), the subscriptions and one subscription. Each
// story opens its page's path.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { inLocale, narrow, settled } from "../../test/stories";
import {
  enabled,
  live,
  retiredPlanId,
  subscriptions,
  supporterHandlers,
  supporterPlanId,
} from "./test/supporterStories";
import { adminAnswering, adminFailingOn, adminLoadingOn, adminScreen, withoutPermission } from "./test/stories";

const subscriptionId = subscriptions[0]?.id ?? "";

const meta = {
  title: "Screens/Admin/Supporter pages",
  ...adminScreen("/admin/supporters/plans", supporterHandlers(enabled, live)),
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** `story` (by default, the meta's) at `path`; Storybook merges the screen's parameters with the meta's. */
function at(path: string, story?: { parameters?: { screen?: object } }) {
  return {
    ...story,
    parameters: { ...story?.parameters, screen: { ...story?.parameters?.screen, path } },
  };
}

/** The plans in effect by rank, then the retired one, with the one link that creates another. */
export const Plans: Story = {
  play: async (context) => {
    await settled(context);
    await expect(context.canvas.getAllByRole("link", { name: "Create a plan" })).toHaveLength(1);
  },
};

/** The plans at 320 CSS px: the table becomes a list of cards. */
export const PlansNarrow: Story = narrow;

/** No plans yet (the default handlers). */
export const PlansEmpty: Story = adminAnswering();

/** The server failed to answer the plans. */
export const PlansServerError: Story = adminFailingOn("get", "/admin/supporter_plans");

/** The form of a new plan on its own page. */
export const NewPlan: Story = at("/admin/supporters/plans/new");

/** A plan in effect: its facts, the form that changes it and retiring it. */
export const Plan: Story = at(`/admin/supporters/plans/${supporterPlanId}`);

/** A retired plan: its facts only. */
export const RetiredPlan: Story = at(`/admin/supporters/plans/${retiredPlanId}`);

/** A plan in Japanese. */
export const PlanJapanese: Story = {
  ...at(`/admin/supporters/plans/${supporterPlanId}`),
  ...inLocale("ja"),
};

/** The subscriptions of several states, in four columns. */
export const Subscriptions: Story = at("/admin/supporters/subscriptions");

/** The subscriptions at 320 CSS px: the table becomes a list of cards. */
export const SubscriptionsNarrow: Story = { ...at("/admin/supporters/subscriptions"), ...narrow };

/** The subscriptions have not arrived yet. */
export const SubscriptionsLoading: Story = at(
  "/admin/supporters/subscriptions",
  adminLoadingOn("get", "/admin/supporter_subscriptions"),
);

/** One subscription and ending it at once. */
export const Subscription: Story = at(`/admin/supporters/subscriptions/${subscriptionId}`);

/** Right to left, in the pseudo-locale ar-XB. */
export const SubscriptionRightToLeft: Story = {
  ...at(`/admin/supporters/subscriptions/${subscriptionId}`),
  ...inLocale("ar-XB"),
};

/** A moderator without manage_supporters. */
export const MissingPermission: Story = withoutPermission;
