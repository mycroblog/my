// SPDX-License-Identifier: AGPL-3.0-only
// The supporter program (/admin/supporters; web-client/screens/admin-supporters.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import {
  paymentStatusFixture,
  supporterPlanFixture,
  supporterProgramFixture,
  supporterSubscriptionFixture,
} from "./test/handlers";
import {
  adminAnswering,
  adminFailingOn,
  adminLoadingOn,
  administrator,
  adminScreen,
  withoutPermission,
} from "./test/stories";

const supporterPlanId = "0199f0e0-0000-7000-8000-0000000000c1";
const patronPlanId = "0199f0e0-0000-7000-8000-0000000000c2";

/** A plan for each rank, one whose prices are being prepared, and a retired one. */
const plans: Schemas["AdminSupporterPlan"][] = [
  supporterPlanFixture(supporterPlanId, { members: 38 }),
  supporterPlanFixture(patronPlanId, {
    name: "Patron",
    description: "For those who keep the lights on: a badge, twenty custom emoji and our heartfelt thanks every month.",
    rank: 2,
    monthly_price: { amount: "1500", currency: "JPY" },
    yearly_price: { amount: "15000", currency: "JPY" },
    annual_discount_percent: 16,
    emoji_limit: 20,
    members: 9,
  }),
  supporterPlanFixture("0199f0e0-0000-7000-8000-0000000000c3", {
    name: "Server sustainer with a deliberately long plan name",
    rank: 3,
    monthly_price: { amount: "5000", currency: "JPY" },
    yearly_price: { amount: "50000", currency: "JPY" },
    prices_ready: false,
  }),
  supporterPlanFixture("0199f0e0-0000-7000-8000-0000000000c4", {
    name: "Early bird",
    rank: 0,
    retired: true,
    retired_at: "2026-09-30T00:00:00Z",
    members: 4,
  }),
];

const usernames = ["bob", "carol", "dave_the_very_long_username", "erin", "frank", "grace"];
const states: Schemas["AdminSupporterSubscription"]["state"][] = [
  "active",
  "active",
  "canceling",
  "past_due",
  "pending",
  "lapsed",
];

/** Subscriptions of several states, monthly and yearly, and one of an account that was deleted since. */
const subscriptions: Schemas["AdminSupporterSubscription"][] = [
  ...usernames.map((username, index) =>
    supporterSubscriptionFixture(`0199f0e0-0000-7000-8000-0000000000e${String(index)}`, {
      username,
      account_id: `0192f0c4-7a8e-7000-8000-00000000010${String(index)}`,
      state: states[index] ?? "active",
      ...(index % 2 === 1 && {
        plan_id: patronPlanId,
        plan_name: "Patron",
        interval: "year",
        price: { amount: "15000", currency: "JPY" },
      }),
    }),
  ),
  (() => {
    const {
      account_id: _account,
      username: _username,
      ...erased
    } = supporterSubscriptionFixture("0199f0e0-0000-7000-8000-0000000000e9", { state: "expired" });
    return erased;
  })(),
];

/** The program of a server that takes supporters, the plans, the subscriptions and a payment provider in live mode. */
function supporterHandlers(program: Schemas["AdminSupporterProgram"], payment: Schemas["PaymentStatus"]) {
  return [
    http.get(`${apiBase}/admin/supporter_program`, ({ request }) =>
      withViewer(request, () => HttpResponse.json(program)),
    ),
    http.get(`${apiBase}/admin/supporter_plans`, ({ request }) =>
      withViewer(request, () => HttpResponse.json({ items: plans } satisfies Schemas["AdminSupporterPlanList"])),
    ),
    http.get(`${apiBase}/admin/supporter_subscriptions`, ({ request }) =>
      withViewer(request, () => HttpResponse.json({ items: subscriptions })),
    ),
    http.get(`${apiBase}/admin/payment_status`, ({ request }) => withViewer(request, () => HttpResponse.json(payment))),
  ];
}

const enabled = supporterProgramFixture({ enabled: true, provider: "stripe", live_subscriptions: 47 });
const live = paymentStatusFixture({
  mode: "live",
  last_webhook_at: "2026-10-28T23:40:00Z",
  last_reconcile: {
    outcome: "succeeded",
    error_kind: "none",
    requests: 4,
    checked_subscriptions: 47,
    corrected_subscriptions: 0,
    occurred_at: "2026-10-28T03:00:00Z",
  },
});

const meta = {
  title: "Screens/Admin/Supporters",
  ...adminScreen("/admin/supporters", supporterHandlers(enabled, live)),
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** An enabled program in live mode: plans by rank with a retired one apart, and subscriptions of several states. */
export const Normal: Story = {};

/** Warnings: no commerce disclosure, no webhook delivery, and a reconciliation that the provider refused. */
export const Warnings: Story = adminAnswering(
  ...supporterHandlers(
    supporterProgramFixture({
      enabled: true,
      provider: "stripe",
      live_subscriptions: 47,
      commerce_disclosure: false,
      warnings: ["no_commerce_disclosure"],
    }),
    (() => {
      const { last_webhook_at: _webhook, ...status } = paymentStatusFixture({
        mode: "live",
        last_reconcile: {
          outcome: "failed",
          error_kind: "permission",
          requests: 3,
          checked_subscriptions: 12,
          corrected_subscriptions: 2,
          occurred_at: "2026-10-22T00:00:00Z",
        },
        warnings: ["no_webhook", "reconcile_corrected"],
      });
      return status;
    })(),
  ),
);

/** A disabled program on a server without a payment provider: the switch stays off and says why. */
export const Disabled: Story = {
  parameters: {
    screen: {
      handlers: [
        administrator,
        http.get(`${apiBase}/admin/supporter_program`, ({ request }) => {
          const { configured_provider: _provider, ...program } = supporterProgramFixture({
            can_enable: false,
            warnings: ["no_provider"],
          });
          return withViewer(request, () => HttpResponse.json(program));
        }),
      ],
    },
  },
};

/** A disabled program without plans or subscriptions (the default handlers). */
export const Empty: Story = adminAnswering();

/** A moderator without manage_supporters. */
export const MissingPermission: Story = withoutPermission;

/** The subscriptions have not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/supporter_subscriptions");

/** The server failed to answer the plans. */
export const ServerError: Story = adminFailingOn("get", "/admin/supporter_plans");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px, where the subscriptions' table scrolls. */
export const Narrow: Story = narrow;
