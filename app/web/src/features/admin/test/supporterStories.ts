// SPDX-License-Identifier: AGPL-3.0-only
// The data of the stories of the supporters screen's pages (supporters.stories.tsx, supporterPages.stories.tsx): the
// plans of a server that takes supporters, subscriptions of several states, and the handlers that answer them.
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../../test/fixtures";
import { apiBase, withViewer } from "../../../test/msw/http";
import {
  paymentStatusFixture,
  supporterPlanFixture,
  supporterProgramFixture,
  supporterSubscriptionFixture,
} from "./handlers";

export const supporterPlanId = "0199f0e0-0000-7000-8000-0000000000c1";
const patronPlanId = "0199f0e0-0000-7000-8000-0000000000c2";
export const retiredPlanId = "0199f0e0-0000-7000-8000-0000000000c4";

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
  supporterPlanFixture(retiredPlanId, {
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
export const subscriptions: Schemas["AdminSupporterSubscription"][] = [
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

/** The program of a server that takes supporters, the plans, the subscriptions (each one too) and a payment provider. */
export function supporterHandlers(program: Schemas["AdminSupporterProgram"], payment: Schemas["PaymentStatus"]) {
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
    http.get(`${apiBase}/admin/supporter_subscriptions/:id`, ({ request, params }) =>
      withViewer(request, () => {
        const subscription = subscriptions.find((item) => item.id === params["id"]);
        return subscription === undefined
          ? HttpResponse.json({ type: "about:blank", title: "Not Found", status: 404 }, { status: 404 })
          : HttpResponse.json(subscription);
      }),
    ),
    http.get(`${apiBase}/admin/payment_status`, ({ request }) => withViewer(request, () => HttpResponse.json(payment))),
  ];
}

export const enabled = supporterProgramFixture({ enabled: true, provider: "stripe", live_subscriptions: 47 });
export const live = paymentStatusFixture({
  mode: "live",
  last_webhook_at: "2026-10-28T23:40:00Z",
  last_reconciliation: {
    outcome: "succeeded",
    error_kind: "none",
    requests: 4,
    checked_subscriptions: 47,
    corrected_subscriptions: 0,
    occurred_at: "2026-10-28T03:00:00Z",
  },
});
