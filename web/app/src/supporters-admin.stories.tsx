// SPDX-License-Identifier: AGPL-3.0-only

// Supporters in the server's administration (ADR-0072, ADR-0162; FR-910, FR-912, FR-914, FR-918, FR-919), on the fake
// server (app.samples.tsx) as its administrator (hana) reads it: the program on Stripe in test mode, the tips at
// ¥500, ¥1,000 and ¥3,000, the plans Supporter and Patron, a hidden Founder and a retired Early bird, and the
// subscriptions of the seed's people. The first page: as it is; the program off and ready; no payment provider in the
// configuration, which it cannot be turned on without; an empty commerce disclosure; the tips off, and a provider that
// takes no one-time payment; webhooks that never arrived and a reconciliation that failed; loading and failing; and
// for a reader without the permission. The plans: the list, none, one preparing its prices; a new plan; a plan; a
// retired one; an address that names none. The subscriptions: the list, filtered, past one page, none; one active, one
// whose payment failed, one of an erased account. What follows a press (saving, a refusal, a confirmation) is the
// route tests'.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta, type Fake } from "./app.samples";

type Admin = NonNullable<NonNullable<Fake["server"]>["supportersAdmin"]>;

const meta = {
  title: "Administration/Supporters",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/admin/supporters", viewer: "hana" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The screen at `path` as the administrator reads it, the fake's supporter program as `admin` says. */
const at = (path: string, admin: Admin = {}, more: Omit<Fake, "at" | "server"> = {}) =>
  screen({ at: path, viewer: "hana", server: { supportersAdmin: admin }, ...more });

/** The program on, its provider, the tips, who may read the list, and the provider's state. */
export const Program: Story = { parameters: at("/admin/supporters") };
/** The program off, ready to turn on. */
export const ProgramOff: Story = { parameters: at("/admin/supporters", { program: "off", subscriptions: "none" }) };
/** No payment provider in the configuration: the warning, and why the program cannot be turned on. */
export const ProgramNoProvider: Story = {
  parameters: at("/admin/supporters", { program: "no-provider", subscriptions: "none", plans: "none" }),
};
/** An empty commerce disclosure, which the program warns of. */
export const ProgramNoDisclosure: Story = { parameters: at("/admin/supporters", { noDisclosure: true }) };
/** The tips off. */
export const TipsOff: Story = { parameters: at("/admin/supporters", { tips: "off" }) };
/** A provider that takes no one-time payment: users cannot tip. */
export const TipsNoOneTime: Story = { parameters: at("/admin/supporters", { tips: "no-one-time" }) };
/** No webhook delivery recorded, and the last reconciliation failed on the key's permissions. */
export const PaymentsWarned: Story = {
  parameters: at("/admin/supporters", { webhook: "missing", reconciliation: "failed" }),
};
export const ProgramLoading: Story = { parameters: at("/admin/supporters", {}, { state: "loading" }) };
export const ProgramError: Story = { parameters: at("/admin/supporters", {}, { state: "error" }) };
/** A reader without the permission: the address names no screen open to them. */
export const ProgramNotOpen: Story = {
  parameters: screen({ at: "/admin/supporters", viewer: "marco", server: { supportersAdmin: {} } }),
};

/** Every plan by order: taking subscriptions, hidden, and retired. */
export const Plans: Story = { parameters: at("/admin/supporters/plans") };
/** No plan yet. */
export const PlansNone: Story = { parameters: at("/admin/supporters/plans", { plans: "none", subscriptions: "none" }) };
/** Patron's prices not made at the provider yet. */
export const PlansPreparing: Story = { parameters: at("/admin/supporters/plans", { plans: "preparing" }) };
export const PlansLoading: Story = { parameters: at("/admin/supporters/plans", {}, { state: "loading" }) };
export const PlansError: Story = { parameters: at("/admin/supporters/plans", {}, { state: "error" }) };
/** A new plan's form, its role chosen among the roles below the moderators with member permissions alone. */
export const NewPlan: Story = { parameters: at("/admin/supporters/plans/new") };
/** Patron: its facts, its change and Retire the plan…. */
export const Plan: Story = { parameters: at("/admin/supporters/plans/{plan:patron}") };
/** A plan preparing its prices. */
export const PlanPreparing: Story = { parameters: at("/admin/supporters/plans/{plan:patron}", { plans: "preparing" }) };
/** A retired plan: its facts alone. */
export const PlanRetired: Story = { parameters: at("/admin/supporters/plans/{plan:early}") };
/** An address that names no plan. */
export const PlanMissing: Story = { parameters: at("/admin/supporters/plans/none") };

/** The subscriptions, the newest first. */
export const Subscriptions: Story = { parameters: at("/admin/supporters/subscriptions") };
/** The subscriptions whose payment failed. */
export const SubscriptionsFiltered: Story = { parameters: at("/admin/supporters/subscriptions?state=past_due") };
/** More subscriptions than a page holds, on the second page: Previous and Next, and which page it is. */
export const SubscriptionsMany: Story = {
  parameters: at("/admin/supporters/subscriptions", { subscriptions: "many" }, { next: true }),
};
export const SubscriptionsNone: Story = {
  parameters: at("/admin/supporters/subscriptions", { subscriptions: "none" }),
};
export const SubscriptionsLoading: Story = {
  parameters: at("/admin/supporters/subscriptions", {}, { state: "loading" }),
};
export const SubscriptionsError: Story = { parameters: at("/admin/supporters/subscriptions", {}, { state: "error" }) };
/** An active subscription, with End now…. */
export const Subscription: Story = { parameters: at("/admin/supporters/subscriptions/{subscription:hana}") };
/** A subscription whose payment failed, in its grace. */
export const SubscriptionPastDue: Story = { parameters: at("/admin/supporters/subscriptions/{subscription:marco}") };
/** A subscription of an erased account, ended. */
export const SubscriptionErased: Story = {
  parameters: at("/admin/supporters/subscriptions/{subscription:erased}"),
};
/** An address that names no subscription. */
export const SubscriptionMissing: Story = { parameters: at("/admin/supporters/subscriptions/none") };
