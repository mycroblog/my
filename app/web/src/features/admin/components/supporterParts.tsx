// SPDX-License-Identifier: AGPL-3.0-only
// What the pages of the supporters screen share (web-client §14 画面の分け方): the row of links between the program,
// the plans and the subscriptions under the page's h1 (`LocalNav`), the words of a plan's and a subscription's state,
// and who a subscription is of.
import { LocalNav, Stack } from "@my/ui";
import type { ReactNode } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { RequirePermission } from "../../moderation";
import type { SupporterPlan, SupporterSubscription, SupporterSubscriptionState } from "../api/supporters";

/** The pages of the supporters screen, in the order of its LocalNav. */
export type SupporterPage = "program" | "plans" | "subscriptions";

const pages: readonly { page: SupporterPage; href: string; label: ClientMessageKey }[] = [
  { page: "program", href: "/admin/supporters", label: "client:admin.supporters.pages.program" },
  { page: "plans", href: "/admin/supporters/plans", label: "client:admin.supporters.pages.plans" },
  {
    page: "subscriptions",
    href: "/admin/supporters/subscriptions",
    label: "client:admin.supporters.pages.subscriptions",
  },
];

/** A page of the supporters screen below its h1, for manage_supporters: the screen's LocalNav, then the page. */
export function SupporterPages({ current, children }: { current: SupporterPage; children: ReactNode }) {
  const t = useT();
  return (
    <RequirePermission permission="manage_supporters">
      <Stack gap="6">
        <LocalNav
          label={t("client:admin.supporters.pages")}
          items={pages.map((item) => ({ href: item.href, label: t(item.label), current: item.page === current }))}
        />
        {children}
      </Stack>
    </RequirePermission>
  );
}

export const subscriptionStateLabels: Readonly<Record<SupporterSubscriptionState, ClientMessageKey>> = {
  pending: "client:admin.supporters.state.pending",
  active: "client:admin.supporters.state.active",
  canceling: "client:admin.supporters.state.canceling",
  past_due: "client:admin.supporters.state.past_due",
  lapsed: "client:admin.supporters.state.lapsed",
  expired: "client:admin.supporters.state.expired",
  revoked: "client:admin.supporters.state.revoked",
  abandoned: "client:admin.supporters.state.abandoned",
};

/** Whether a subscription goes on, so that it can be ended at once. */
export function isLive(state: SupporterSubscriptionState): boolean {
  return state !== "expired" && state !== "revoked" && state !== "abandoned";
}

/** Who a subscription is of: the subscriber's username, or "deleted account". */
export function subscriberName(t: ReturnType<typeof useT>, subscription: SupporterSubscription): string {
  return subscription.username === undefined
    ? t("client:admin.supporters.subscription.deleted_account")
    : `@${subscription.username}`;
}

/** The state of a plan in words: retired, preparing its prices, hidden, or in effect. */
export function planStateLabel(plan: SupporterPlan): ClientMessageKey {
  if (plan.retired) return "client:admin.supporters.plan.state.retired";
  if (!plan.prices_ready) return "client:admin.supporters.plan.preparing";
  if (plan.visibility === "hidden") return "client:admin.supporters.plan.state.hidden";
  return "client:admin.supporters.plan.state.in_effect";
}
