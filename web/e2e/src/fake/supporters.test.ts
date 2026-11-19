// SPDX-License-Identifier: AGPL-3.0-only

// The fake's supporters (supporters.ts), as the screens of the supporters rely on them: the program off and on, the
// reader's subscription in each of its states and what the server lets them do with it, the checkout and the tips
// whose provider's page is the return to the app, and the supporters list by its visibility. Unit tests, without a DOM.

import { describe, expect, test } from "bun:test";
import type { components } from "@mycroblog/mykit";
import { createFakeApi, fakeOrigin, type FakeApi, type FakeOptions, type FakeSupporterOptions } from "./index";

type Schemas = components["schemas"];
type Own = Schemas["OwnSupporterSubscription"];
type Subscription = Schemas["SupporterSubscription"];
type Answered<T> = { status: number; body: T; headers: Headers };

async function ask<T>(fake: FakeApi, method: string, path: string, body?: unknown): Promise<Answered<T>> {
  const response = await fake.serve(
    new Request(`${fakeOrigin}/api/v1${path}`, {
      method,
      headers: body === undefined ? {} : { "Content-Type": "application/json" },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    }),
  );
  const text = await response.text();
  return {
    status: response.status,
    body: (text === "" ? undefined : JSON.parse(text)) as T,
    headers: response.headers,
  };
}

const problemOf = (a: Answered<unknown>) => [a.status, (a.body as Schemas["Problem"]).type];
const own = async (fake: FakeApi) => (await ask<Own>(fake, "GET", "/supporter/subscription")).body;
const fakeWith = (supporters: FakeSupporterOptions, more: FakeOptions = {}) => createFakeApi({ ...more, supporters });

describe("the supporter program", () => {
  test("off, the instance says so and every operation of it is a 404", async () => {
    const fake = createFakeApi();
    const instance = (await ask<Schemas["Instance"]>(fake, "GET", "/instance")).body;
    expect([instance.supporters_enabled, instance.tips_enabled, instance.supporter_list_visibility]).toEqual([
      false,
      false,
      "off",
    ]);
    for (const path of ["/instance/supporter_plans", "/supporter/subscription", "/instance/tip_amounts"])
      expect((await ask(fake, "GET", path)).status).toBe(404);
  });

  test("on, two plans by rank, each monthly and yearly, the year cheaper than twelve months", async () => {
    const fake = fakeWith({});
    const plans = (await ask<Schemas["SupporterPlanList"]>(fake, "GET", "/instance/supporter_plans")).body.items;
    expect(plans.map((p) => [p.name, p.rank, p.id])).toEqual([
      ["Supporter", 1, fake.supporters.planId("supporter")],
      ["Patron", 2, fake.supporters.planId("patron")],
    ]);
    for (const p of plans) expect(Number(p.yearly_price.amount)).toBeLessThan(12 * Number(p.monthly_price.amount));
    expect((await own(fake)).subscription).toBeUndefined();
  });

  test("the reader's subscription in each state the options name, and why they may not subscribe", async () => {
    const states = await Promise.all(
      (["pending", "active", "yearly", "canceling", "scheduled", "past-due", "lapsed"] as const).map(
        async (subscription) => (await own(fakeWith({ subscription }))).subscription,
      ),
    );
    expect(states.map((s) => s?.state)).toEqual([
      "pending",
      "active",
      "active",
      "canceling",
      "active",
      "past_due",
      "lapsed",
    ]);
    expect([
      states[2]?.interval,
      states[4]?.scheduled_change?.plan_name,
      states[5]?.grace_ends_at !== undefined,
    ]).toEqual(["year", "Supporter", true]);
    expect((await own(fakeWith({ ineligible: "email_unverified" }))).ineligible_reason).toBe("email_unverified");
  });
});

describe("the checkout", () => {
  test("goes back to the app paid, pending until the sync, then active", async () => {
    const fake = fakeWith({});
    const started = await ask<Schemas["SupporterCheckout"]>(fake, "POST", "/supporter/checkout", {
      plan_id: fake.supporters.planId("patron"),
      interval: "year",
    });
    const back = new URL(started.body.redirect_url);
    expect([back.origin, back.pathname, back.searchParams.get("checkout")]).toEqual([
      fakeOrigin,
      "/settings/supporter",
      "success",
    ]);
    expect((await own(fake)).subscription?.state).toBe("pending");
    const synced = await ask<Own>(fake, "POST", "/supporter/subscription/sync");
    expect(synced.body.subscription?.state).toBe("pending");
    const after = (await own(fake)).subscription;
    expect([after?.state, after?.plan_name, after?.interval, after?.price.amount]).toEqual([
      "active",
      "Patron",
      "year",
      "15000",
    ]);
  });

  test("left unpaid it returns canceled and stays pending; started again it continues", async () => {
    const fake = fakeWith({ checkout: "canceled" });
    const choice = { plan_id: fake.supporters.planId("supporter"), interval: "month" };
    const first = await ask<Schemas["SupporterCheckout"]>(fake, "POST", "/supporter/checkout", choice);
    expect(new URL(first.body.redirect_url).searchParams.get("checkout")).toBe("canceled");
    await ask(fake, "POST", "/supporter/subscription/sync");
    expect((await own(fake)).subscription?.state).toBe("pending");
    const again = await ask<Schemas["SupporterCheckout"]>(fake, "POST", "/supporter/checkout", choice);
    expect(again.body.subscription_id).toBe(first.body.subscription_id);
  });

  test("refused to a reader who may not, who subscribes already, and once the closure is announced", async () => {
    const choice = (fake: FakeApi) => ({ plan_id: fake.supporters.planId("supporter"), interval: "month" });
    const ineligible = fakeWith({ ineligible: "email_unverified" });
    const subscribed = fakeWith({ subscription: "active" });
    const closing = fakeWith({}, { closure: "announced" });
    expect(problemOf(await ask(ineligible, "POST", "/supporter/checkout", choice(ineligible)))).toEqual([
      422,
      "/api/v1/problems/supporter-ineligible",
    ]);
    expect(problemOf(await ask(subscribed, "POST", "/supporter/checkout", choice(subscribed)))).toEqual([
      409,
      "/api/v1/problems/supporter-subscribed",
    ]);
    expect(problemOf(await ask(closing, "POST", "/supporter/checkout", choice(closing)))).toEqual([
      409,
      "/api/v1/problems/service-closing",
    ]);
  });
});

describe("the subscription's changes", () => {
  const change = (fake: FakeApi, plan: "supporter" | "patron", interval: string) =>
    ask<Subscription>(fake, "POST", "/supporter/subscription/change", {
      plan_id: fake.supporters.planId(plan),
      interval,
    });

  test("a higher plan changes at once; a lower one waits for the period's end, and its wait is cancelled", async () => {
    const fake = fakeWith({ subscription: "active" });
    const up = await change(fake, "patron", "month");
    expect([up.status, up.body.plan_name, up.body.scheduled_change]).toEqual([200, "Patron", undefined]);
    const down = await change(fake, "supporter", "month");
    expect([down.body.plan_name, down.body.scheduled_change?.plan_name]).toEqual(["Patron", "Supporter"]);
    const kept = await ask<Subscription>(fake, "POST", "/supporter/subscription/cancel_scheduled_change");
    expect([kept.body.plan_name, kept.body.scheduled_change]).toEqual(["Patron", undefined]);
    expect(problemOf(await change(fake, "patron", "month"))).toEqual([422, "/api/v1/problems/supporter-same-plan"]);
  });

  test("the yearly price of the same plan is higher; the monthly one of a yearly plan waits", async () => {
    const monthly = fakeWith({ subscription: "active" });
    expect((await change(monthly, "supporter", "year")).body.interval).toBe("year");
    const yearly = fakeWith({ subscription: "yearly" });
    const waits = await change(yearly, "supporter", "month");
    expect([waits.body.interval, waits.body.scheduled_change?.interval]).toEqual(["year", "month"]);
  });

  test("an upgrade whose payment fails or has not settled keeps the plan", async () => {
    for (const [upgrade, status, type] of [
      ["failed", 402, "/api/v1/problems/supporter-payment-failed"],
      ["processing", 409, "/api/v1/problems/supporter-payment-processing"],
    ] as const) {
      const fake = fakeWith({ subscription: "active", upgrade });
      expect(problemOf(await change(fake, "patron", "month"))).toEqual([status, type]);
      expect((await own(fake)).subscription?.plan_name).toBe("Supporter");
    }
  });

  test("cancelled it stays to the period's end and is resumed; out of its state each is refused", async () => {
    const fake = fakeWith({ subscription: "scheduled" });
    const cancelled = await ask<Subscription>(fake, "POST", "/supporter/subscription/cancel");
    expect([cancelled.body.state, cancelled.body.scheduled_change]).toEqual(["canceling", undefined]);
    expect(problemOf(await change(fake, "supporter", "year"))).toEqual([
      409,
      "/api/v1/problems/supporter-subscription-state",
    ]);
    expect((await ask<Subscription>(fake, "POST", "/supporter/subscription/resume")).body.state).toBe("active");
    expect((await ask(fake, "POST", "/supporter/subscription/resume")).status).toBe(409);
    const pastDue = fakeWith({ subscription: "past-due" });
    expect((await ask<Subscription>(pastDue, "POST", "/supporter/subscription/cancel")).body.state).toBe("canceling");
  });

  test("the portal returns to the app, for a subscription the provider knows", async () => {
    const fake = fakeWith({ subscription: "lapsed" });
    const portal = await ask<Schemas["SupporterPortal"]>(fake, "POST", "/supporter/portal");
    expect(new URL(portal.body.url).searchParams.get("portal")).toBe("returned");
    expect(problemOf(await ask(fakeWith({}), "POST", "/supporter/portal"))).toEqual([
      409,
      "/api/v1/problems/subscription-ended",
    ]);
  });
});

describe("the tips", () => {
  const start = (fake: FakeApi, amount = "1000") =>
    ask<Schemas["SupporterTipCheckout"]>(fake, "POST", "/supporter/tips", { amount: { amount, currency: "JPY" } });
  const tip = async (fake: FakeApi, id: string) =>
    (await ask<Schemas["SupporterTip"]>(fake, "GET", `/supporter/tips/${id}`)).body;

  test("the operator's amounts; a tip returns to the app, open on the first read and paid on the next", async () => {
    const fake = fakeWith({ program: false, tips: {} });
    const amounts = (await ask<Schemas["TipAmounts"]>(fake, "GET", "/instance/tip_amounts")).body.amounts;
    expect(amounts.map((a) => a.amount)).toEqual(["500", "1000", "3000"]);
    const started = await start(fake);
    const back = new URL(started.body.redirect_url);
    expect([back.pathname, back.searchParams.get("tip"), back.searchParams.get("checkout")]).toEqual([
      "/settings/supporter/tip",
      started.body.tip_id,
      "success",
    ]);
    expect((await tip(fake, started.body.tip_id)).state).toBe("open");
    expect((await tip(fake, started.body.tip_id)).state).toBe("paid");
    expect(fake.supporters.tipId("last")).toBe(started.body.tip_id);
  });

  test("one open tip at a time: a new start ends the open one; a paid one of the seed stays paid", async () => {
    const fake = fakeWith({ tips: { open: true, paid: true } });
    expect((await tip(fake, fake.supporters.tipId("paid"))).state).toBe("paid");
    expect((await tip(fake, fake.supporters.tipId("open"))).state).toBe("open");
    await start(fake);
    expect((await tip(fake, fake.supporters.tipId("open"))).state).toBe("expired");
  });

  test("an amount not of the operator's, a reader who may not, and the rate limit are refused", async () => {
    expect(problemOf(await start(fakeWith({ tips: {} }), "700"))).toEqual([
      422,
      "/api/v1/problems/supporter-tip-amount",
    ]);
    expect(problemOf(await start(fakeWith({ tips: {}, ineligible: "email_unverified" })))).toEqual([
      422,
      "/api/v1/problems/supporter-ineligible",
    ]);
    const limited = await start(fakeWith({ tips: { limited: true } }));
    expect([limited.status, limited.headers.get("Retry-After")]).toEqual([429, "1800"]);
    const fake = fakeWith({ tips: { checkout: "canceled" } });
    for (let i = 0; i < 5; i += 1) expect((await start(fake)).status).toBe(200);
    expect((await start(fake)).status).toBe(429);
  });
});

describe("the supporters list", () => {
  type Page = Schemas["SupporterListPage"];
  const list = (fake: FakeApi, query = "") => ask<Page>(fake, "GET", `/instance/supporters${query}`);

  test("off, a 404; for signed-in readers, a 401 to a visitor; to everyone, the listed by plan, newest first", async () => {
    expect((await list(fakeWith({}))).status).toBe(404);
    const visitor = await list(fakeWith({ list: "signed_in" }, { visitor: true }));
    expect(problemOf(visitor)).toEqual([401, "/api/v1/problems/unauthenticated"]);
    const everyone = await list(fakeWith({ list: "everyone" }, { visitor: true }));
    expect(everyone.body.items.map((e) => e.account.username)).toEqual(["sam", "hana", "mio"]);
    expect(everyone.body.plans.map((p) => p.name)).toEqual(["Supporter", "Patron"]);
    expect(everyone.headers.get("X-Robots-Tag")).toBe("noindex, noarchive");
    const fake = fakeWith({ list: "everyone" });
    const patrons = await list(fake, `?plan_id=${fake.supporters.planId("patron")}`);
    expect(patrons.body.items.map((e) => e.account.username)).toEqual(["mio"]);
  });

  test("the reader's choice: a member's, kept, which puts them on the list; a 404 to one who is no member", async () => {
    const fake = fakeWith({ subscription: "active", list: "signed_in" });
    expect((await ask<{ listed: boolean }>(fake, "GET", "/preferences/supporter_listing")).body.listed).toBe(false);
    await ask(fake, "PUT", "/preferences/supporter_listing", { listed: true });
    expect((await list(fake)).body.items.map((e) => e.account.username)).toEqual(["yuki", "sam", "hana", "mio"]);
    const listedAlready = fakeWith({ subscription: "canceling", listed: true, list: "everyone" });
    expect((await ask<{ listed: boolean }>(listedAlready, "GET", "/preferences/supporter_listing")).body.listed).toBe(
      true,
    );
    expect((await ask(fakeWith({}), "GET", "/preferences/supporter_listing")).status).toBe(404);
    expect((await ask(fakeWith({}), "PUT", "/preferences/supporter_listing", { listed: true })).status).toBe(404);
  });
});
