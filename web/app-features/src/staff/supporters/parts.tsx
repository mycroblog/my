// SPDX-License-Identifier: AGPL-3.0-only

// What the screens of Supporters share (ADR-0072, ADR-0162): their addresses, the currencies of a price, and the
// words of a subscription's state.

import type { AdminSubscription, SubscriptionState } from "@my/app-data";
import type { RecordState } from "@my/design-system";
import type { T } from "../parts";

/** The addresses of the screens of Supporters. */
export const supportersPaths = {
  program: "/admin/supporters",
  plans: "/admin/supporters/plans",
  newPlan: "/admin/supporters/plans/new",
  plan: (id: string) => `/admin/supporters/plans/${encodeURIComponent(id)}`,
  subscriptions: "/admin/supporters/subscriptions",
  subscription: (id: string) => `/admin/supporters/subscriptions/${encodeURIComponent(id)}`,
} as const;

/** The currencies a price or a tip is written in: those the server takes (yen and US dollars). */
export const supporterCurrencies = ["JPY", "USD"] as const;

/**
 * The amounts of the tips as the reader wrote them: one a line, or apart by a semicolon or 、, each with its spaces and
 * its thousands separators (a comma, a space) taken away, so that "1,000" is one amount.
 */
export function tipAmountsOf(text: string): string[] {
  return text
    .split(/[\n;；、]+/)
    .map((a) => a.replace(/[,\s\u00a0\u202f]/g, ""))
    .filter((a) => a !== "");
}

/** How a subscription's state weighs as a record's: what holds the role, what waits, what is over. */
export const subscriptionMark: Readonly<Record<SubscriptionState, RecordState>> = {
  active: "active",
  // Its perks stay to the period's end, which it waits for: not the look of one that renews.
  canceling: "pending",
  past_due: "failed",
  pending: "pending",
  lapsed: "failed",
  expired: "expired",
  revoked: "expired",
  abandoned: "expired",
};

/** A subscription's state in words. */
export const stateWords = (t: T, state: SubscriptionState): string => t("staff_supporters.state", { state });

/** The subscriber of a subscription as a sentence names them: the handle, or that the account was erased. */
export const subscriberOf = (t: T, s: Pick<AdminSubscription, "username">): string =>
  s.username === undefined ? t("staff_supporters.subscriber_gone") : `@${s.username}`;
