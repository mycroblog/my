// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the supporters feature (web-client/README.md §7): the plans, the user's subscription and
// its changes, the checkout and the portal, the supporter list and the user's choice to be on it. By default no plan
// takes subscriptions, the user has no subscription and may subscribe, and nobody is listed; the default instance
// has the program disabled and the list unpublished, so nothing of it shows until a test turns it on. The server takes
// no tips until a test turns them on too.
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../../test/fixtures";
import { apiBase, problem, withViewer } from "../../../test/msw/http";

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

/** The ID of the tip that the default start answers. */
export const tipId = "0192f0c4-7a8e-7000-8000-0000000000f1";

/** A tip of the user of ¥500, open since 2026-10-22, with `overrides`. */
export function ownTipFixture(overrides: Partial<Schemas["SupporterTip"]> = {}): Schemas["SupporterTip"] {
  return {
    id: tipId,
    state: "open",
    amount: { amount: "500", currency: "JPY" },
    created_at: "2026-10-22T00:00:00Z",
    ...overrides,
  };
}

/** The amounts of the tips that a test turns on with the instance's `tips_enabled`: ¥300, ¥500 and ¥1,000. */
export const tipAmounts: Schemas["TipAmounts"] = {
  amounts: [
    { amount: "300", currency: "JPY" },
    { amount: "500", currency: "JPY" },
    { amount: "1000", currency: "JPY" },
  ],
};

export const handlers = [
  // operationIds getInstanceTipAmounts, startSupporterTip, getOwnSupporterTip: the server takes no tips by default
  // (404, as the default instance's tips_enabled says); a test turns them on.
  http.get(`${apiBase}/instance/tip_amounts`, () => problem(404, "about:blank", "Not Found")),
  http.post(`${apiBase}/supporter/tips`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ tip_id: tipId, redirect_url: checkoutUrl })),
  ),
  http.get(`${apiBase}/supporter/tips/:id`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(ownTipFixture())),
  ),
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
