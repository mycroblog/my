// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the supporters feature (web-client/README.md §7): the plans, the user's subscription and
// its changes, the checkout and the portal, the supporter list and the user's choice to be on it. By default no plan
// takes subscriptions, the user has no subscription and may subscribe, and nobody is listed; the default instance
// has the program disabled and the list unpublished, so nothing of it shows until a test turns it on.
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../../test/fixtures";
import { apiBase, withViewer } from "../../../test/msw/http";

/** A plan that takes subscriptions, ¥500 a month or ¥5,400 a year, with `overrides`. */
export function supporterPlanFixture(
  id: string,
  overrides: Partial<Schemas["SupporterPlan"]> = {},
): Schemas["SupporterPlan"] {
  return {
    id,
    name: "Supporter",
    description: "",
    rank: 1,
    monthly_price: { amount: "500", currency: "JPY" },
    yearly_price: { amount: "5400", currency: "JPY" },
    ...overrides,
  };
}

/** The user's active monthly subscription to `planId`, renewing on 2026-11-20, with `overrides`. */
export function ownSubscriptionFixture(
  planId: string,
  overrides: Partial<Schemas["SupporterSubscription"]> = {},
): Schemas["SupporterSubscription"] {
  return {
    id: "0192f0c4-7a8e-7000-8000-0000000000e1",
    state: "active",
    plan_id: planId,
    plan_name: "Supporter",
    interval: "month",
    price: { amount: "500", currency: "JPY" },
    period_ends_at: "2026-11-20T00:00:00Z",
    started_at: "2026-10-20T00:00:00Z",
    ...overrides,
  };
}

/** Where the default checkout and portal send the browser. */
export const checkoutUrl = "https://checkout.payments.example/c/session";
export const portalUrl = "https://billing.payments.example/p/session";

export const handlers = [
  http.get(`${apiBase}/instance/supporter_plans`, () => HttpResponse.json({ items: [] })),
  http.get(`${apiBase}/supporter/subscription`, ({ request }) => withViewer(request, () => HttpResponse.json({}))),
  http.post(`${apiBase}/supporter/subscription/sync`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({})),
  ),
  http.post(`${apiBase}/supporter/checkout`, ({ request }) =>
    withViewer(request, () =>
      HttpResponse.json({ subscription_id: "0192f0c4-7a8e-7000-8000-0000000000e1", redirect_url: checkoutUrl }),
    ),
  ),
  http.post(`${apiBase}/supporter/subscription/change`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(ownSubscriptionFixture("plan"))),
  ),
  http.post(`${apiBase}/supporter/subscription/cancel`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(ownSubscriptionFixture("plan", { state: "canceling" }))),
  ),
  http.post(`${apiBase}/supporter/subscription/resume`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(ownSubscriptionFixture("plan"))),
  ),
  http.post(`${apiBase}/supporter/subscription/cancel_scheduled_change`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(ownSubscriptionFixture("plan"))),
  ),
  http.post(`${apiBase}/supporter/portal`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ url: portalUrl })),
  ),
  http.get(`${apiBase}/instance/supporters`, () => HttpResponse.json({ plans: [], items: [] })),
  http.get(`${apiBase}/preferences/supporter_listing`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ listed: false })),
  ),
  http.put(`${apiBase}/preferences/supporter_listing`, async ({ request }) => {
    const body = (await request.json()) as Schemas["SupporterListing"];
    return withViewer(request, () => HttpResponse.json(body));
  }),
];
