// SPDX-License-Identifier: AGPL-3.0-only

// What the reader's subscription allows and how a change applies (ADR-0072 §7, §8, §9), on the subscriptions the fake
// holds: a change applies now when it goes to a plan of a higher rank, or within one rank from monthly to yearly, and at
// the period's end otherwise, ranked by the subscription's own plan, a hidden one too; a member may change and cancel
// while active, resume while ending, keep their plan while a change waits, and only update the payment while it fails.

import { expect, test } from "bun:test";
import type { FakeSupporterOptions } from "@my/e2e/fake";
import {
  planChangeTiming,
  supporterActions,
  supporterStanding,
  type SupporterPlan,
  type SupporterSubscription,
} from "./supporters";
import { fakeData } from "./test/data";

type Held = NonNullable<FakeSupporterOptions["subscription"]>;

/** The reader's subscription of `subscription` on a fake server, and its public plans by name. */
async function held(
  subscription: Held,
): Promise<{ own: SupporterSubscription | undefined; plans: Record<string, SupporterPlan> }> {
  const { fake, data } = fakeData({ supporters: { subscription } });
  const { data: page } = await data.api.GET("/instance/supporter_plans");
  return {
    own: fake.supporters.subscription(),
    plans: Object.fromEntries((page?.items ?? []).map((plan) => [plan.name, plan])),
  };
}

test.each([
  // Supporter (rank 1), monthly: up the ranks or to yearly now.
  ["active", "Patron", "month", "now"],
  ["active", "Patron", "year", "now"],
  ["active", "Supporter", "year", "now"],
  ["active", "Supporter", "month", "same"],
  // Supporter, yearly: to monthly at the period's end; to a higher rank, monthly too, now.
  ["yearly", "Supporter", "month", "period-end"],
  ["yearly", "Patron", "month", "now"],
  // Patron (rank 2), monthly: to a lower rank at the period's end, yearly as well — the rank decides.
  ["scheduled", "Supporter", "month", "period-end"],
  ["scheduled", "Supporter", "year", "period-end"],
  ["scheduled", "Patron", "year", "now"],
  // Founder (rank 3, hidden): every public plan is lower, whatever the list holds.
  ["hidden", "Patron", "month", "period-end"],
  ["hidden", "Patron", "year", "period-end"],
  ["hidden", "Supporter", "year", "period-end"],
] as const)(
  "a change from the %s subscription to %s, %sly, applies %s",
  async (subscription, plan, interval, timing) => {
    const { own, plans } = await held(subscription);
    const to = plans[plan];
    if (own === undefined || to === undefined) throw new Error("test: the fake holds no such subscription or plan");
    expect(planChangeTiming(own, to, interval)).toBe(timing);
  },
);

test.each([
  ["none", { change: false, cancel: false, resume: false, cancelScheduledChange: false, portal: false }],
  ["pending", { change: false, cancel: false, resume: false, cancelScheduledChange: false, portal: false }],
  ["active", { change: true, cancel: true, resume: false, cancelScheduledChange: false, portal: true }],
  ["scheduled", { change: true, cancel: true, resume: false, cancelScheduledChange: true, portal: true }],
  ["canceling", { change: false, cancel: false, resume: true, cancelScheduledChange: false, portal: true }],
  ["past-due", { change: false, cancel: false, resume: false, cancelScheduledChange: false, portal: true }],
  ["lapsed", { change: false, cancel: false, resume: false, cancelScheduledChange: false, portal: true }],
] as const)("the %s subscription allows %j", async (subscription, allowed) => {
  const { own } = await held(subscription);
  expect(supporterActions(own)).toEqual(allowed);
});

test.each([
  ["none", "none", undefined],
  ["pending", "pending", undefined],
  ["active", "active", "period_ends_at"],
  ["canceling", "canceling", "period_ends_at"],
  ["scheduled", "scheduled", "period_ends_at"],
  ["past-due", "past-due", "grace_ends_at"],
  ["lapsed", "lapsed", undefined],
] as const)("the %s subscription stands as %s, dated by its %s", async (subscription, kind, dated) => {
  const { own } = await held(subscription);
  const standing = supporterStanding(own);
  expect(standing.kind).toBe(kind);
  if (dated !== undefined) {
    const day = own?.[dated];
    expect(day).toBeDefined();
    expect(Object.values(standing)).toContain(day);
  }
});
