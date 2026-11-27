// SPDX-License-Identifier: AGPL-3.0-only

// The supporter's side (ADR-0072, ADR-0162, ADR-0070 §13; FR-911, FR-914, FR-918), on the fake server (app.samples.tsx):
// the supporters' page with the plans, the tip and the list, for a reader without a plan, for a member (Switch), for a
// visitor, for a reader who may not subscribe nor tip (each reason the server says, with the program and on a server
// of tips alone), on a server of tips alone, in the native shell (the list
// alone), on a server that takes no support, and loading, failing, with an empty list and long names; the settings'
// Supporter in every state of a subscription (none, its first payment not made, renewing, yearly, ending after a
// cancel, a change scheduled, a failed payment in its grace and past it), with the listing, for a reader who may not
// subscribe, back from the provider's checkout (paid, not settled yet, left unpaid), loading and failing; and the tip,
// its amounts, back from its checkout (paid, not settled yet, left unpaid), for a reader who may not tip, on a server
// that takes none, loading and failing. What follows a press (a question, a refusal, the rate limit) is the route
// tests' (supporters-routes.dom.test.tsx).
// More of the supporters listed than a page holds, before and after Show more (pages.samples.ts, `more`).

import type { FakeSupporterOptions } from "@my/e2e/fake";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta, type Fake } from "./app.samples";
import { manySupporters } from "./pages.samples";

/** The program with its two plans, the tips, and the list shown to everyone, unless a story says otherwise. */
const all: FakeSupporterOptions = { tips: {}, list: "everyone" };

const at = (path: string, supporters: FakeSupporterOptions | undefined, more: Omit<Fake, "at" | "server"> = {}) =>
  screen({ at: path, server: supporters === undefined ? {} : { supporters }, ...more });

const page = "/about/supporters";
const settings = "/settings/supporter";
const tip = "/settings/supporter/tip";

const meta = {
  title: "Supporters",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: page }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

// ---- The supporters' page --------------------------------------------------------------------------------------------

/** A reader without a plan: the plans, monthly, each Choose; the tip; the supporters under their plans. */
export const Page: Story = { parameters: at(page, all) };
/** A member, monthly Supporter: their plan marked, Switch on the others, the way to their subscription. */
export const PageMember: Story = { parameters: at(page, { ...all, subscription: "active", listed: true }) };
/** A reader whose payment failed past its grace: no plan marked, the failure said, the way to their subscription. */
export const PageLapsed: Story = { parameters: at(page, { ...all, subscription: "lapsed" }) };
/** A member of a plan the administrators hid: no card marked, Switch on the public plans. */
export const PageHiddenPlan: Story = { parameters: at(page, { ...all, subscription: "hidden" }) };
/** A visitor: the plans with the way to sign in, and the tip's; the list where it is shown to signed-in readers. */
export const PageVisitor: Story = { parameters: at(page, { ...all, list: "signed_in" }, { signedOut: true }) };
/** A reader whose address is not confirmed: why they may not subscribe nor tip, in a sentence each. */
export const PageUnverified: Story = { parameters: at(page, { ...all, ineligible: "email_unverified" }) };
/** A reader of another server: only people with an account here may subscribe or tip. */
export const PageNotLocal: Story = { parameters: at(page, { ...all, ineligible: "not_local" }) };
/** A reader whose account is restricted: neither while it is. */
export const PageRestricted: Story = { parameters: at(page, { ...all, ineligible: "restricted" }) };
/** A server of tips alone: the tip, no plans, no list. */
export const PageTipsOnly: Story = { parameters: at(page, { program: false, tips: {} }) };
/** A server of tips alone, for a reader whose address is not confirmed: why they may not tip, no Tip. */
export const PageTipsOnlyUnverified: Story = {
  parameters: at(page, { program: false, tips: {}, ineligible: "email_unverified" }),
};
/** A server of tips alone, for a reader of another server. */
export const PageTipsOnlyNotLocal: Story = {
  parameters: at(page, { program: false, tips: {}, ineligible: "not_local" }),
};
/** A server of tips alone, for a reader whose account is restricted. */
export const PageTipsOnlyRestricted: Story = {
  parameters: at(page, { program: false, tips: {}, ineligible: "restricted" }),
};
/** A server that takes no support. */
export const PageOff: Story = { parameters: at(page, undefined) };
export const PageLoading: Story = { parameters: at(page, all, { state: "loading" }) };
export const PageError: Story = { parameters: at(page, all, { state: "error" }) };
/** Nobody chose to be listed. */
export const PageEmpty: Story = { parameters: at(page, all, { state: "empty" }) };
export const PageLong: Story = { parameters: at(page, all, { state: "long" }) };
/** More listed supporters than a page holds: Show more under the first twenty. */
export const PageMany: Story = { parameters: screen({ at: page, server: manySupporters }) };
/** The same after Show more: the rest under the first twenty, and no Show more. */
export const PageManyMore: Story = { parameters: screen({ at: page, server: manySupporters, more: true }) };

// ---- The settings' Supporter -------------------------------------------------------------------------------------------

/** No subscription: the way to the plans, and the tip. */
export const Settings: Story = { parameters: at(settings, all) };
/** A subscription whose first payment is not made: it waits, with what it charges, and the plans may be chosen again. */
export const SettingsPending: Story = { parameters: at(settings, { ...all, subscription: "pending" }) };
/** Renewing, monthly: its date, plan and price; the portal and Cancel; the listing. */
export const SettingsActive: Story = { parameters: at(settings, { ...all, subscription: "active", listed: true }) };
/** Renewing at an earlier price the plan kept for the reader. */
export const SettingsKeptPrice: Story = { parameters: at(settings, { ...all, subscription: "kept" }) };
/** Renewing, yearly, the list shown to signed-in readers. */
export const SettingsYearly: Story = {
  parameters: at(settings, { ...all, subscription: "yearly", list: "signed_in" }),
};
/** Cancelled: it ends at the period's end, the perks until then, with Resume. */
export const SettingsCanceling: Story = { parameters: at(settings, { ...all, subscription: "canceling" }) };
/** A change to a lower plan scheduled for the period's end, with Keep my plan. */
export const SettingsScheduled: Story = { parameters: at(settings, { ...all, subscription: "scheduled" }) };
/** A failed payment in its grace: said once, with Update payment, the primary action. */
export const SettingsPastDue: Story = { parameters: at(settings, { ...all, subscription: "past-due" }) };
/** A failed payment past its grace: the perks ended, with Update payment. */
export const SettingsLapsed: Story = { parameters: at(settings, { ...all, subscription: "lapsed" }) };
/** A reader whose address is not confirmed: why they may not subscribe, in a sentence. */
export const SettingsUnverified: Story = { parameters: at(settings, { ...all, ineligible: "email_unverified" }) };
/** Back from the checkout, paid: thanks, with the plan. */
export const SettingsReturnedPaid: Story = {
  parameters: at(`${settings}?checkout=success&checkout_id=fake_cs_1`, { ...all, subscription: "active" }),
};
/** Back from the checkout, its payment not settled yet: the screen reads it again. */
export const SettingsReturnedSettling: Story = {
  parameters: at(`${settings}?checkout=success&checkout_id=fake_cs_1`, { ...all, subscription: "pending" }),
};
/** Back from the checkout, left unpaid: nothing was charged. */
export const SettingsReturnedCanceled: Story = {
  parameters: at(`${settings}?checkout=canceled`, { ...all, subscription: "pending" }),
};
export const SettingsLoading: Story = { parameters: at(settings, all, { state: "loading" }) };
export const SettingsError: Story = { parameters: at(settings, all, { state: "error" }) };

// ---- The tip ------------------------------------------------------------------------------------------------------------

/** The operator's amounts, the smallest chosen, what a tip is under them, and Tip ¥500. */
export const Tip: Story = { parameters: at(tip, all) };
/** Back from the checkout, paid: thanks, with its amount. */
export const TipPaid: Story = {
  parameters: at(`${tip}?tip={tip:paid}&checkout=success`, { ...all, tips: { paid: true } }),
};
/** Back from the checkout, its payment not settled yet. */
export const TipSettling: Story = {
  parameters: at(`${tip}?tip={tip:open}&checkout=success`, { ...all, tips: { open: true } }),
};
/** Back from the checkout, left unpaid: nothing was charged, and the amounts to tip again. */
export const TipUnpaid: Story = {
  parameters: at(`${tip}?tip={tip:open}&checkout=canceled`, { ...all, tips: { open: true } }),
};
/** A reader whose address is not confirmed: the amounts closed, why in a sentence, no Tip. */
export const TipUnverified: Story = { parameters: at(tip, { ...all, ineligible: "email_unverified" }) };
/** A reader of another server: the amounts closed, why in a sentence, no Tip. */
export const TipNotLocal: Story = { parameters: at(tip, { ...all, ineligible: "not_local" }) };
/** A reader whose account is restricted: the amounts closed, why in a sentence, no Tip. */
export const TipRestricted: Story = { parameters: at(tip, { ...all, ineligible: "restricted" }) };
/** A server of tips alone, for a reader whose address is not confirmed: said before the press, no Tip. */
export const TipOnlyUnverified: Story = {
  parameters: at(tip, { program: false, tips: {}, ineligible: "email_unverified" }),
};
/** A server of tips alone, for a reader of another server. */
export const TipOnlyNotLocal: Story = { parameters: at(tip, { program: false, tips: {}, ineligible: "not_local" }) };
/** A server of tips alone, for a reader whose account is restricted. */
export const TipOnlyRestricted: Story = { parameters: at(tip, { program: false, tips: {}, ineligible: "restricted" }) };
/** A server whose program is on and takes no tips. */
export const TipOff: Story = { parameters: at(tip, { list: "everyone" }) };
export const TipLoading: Story = { parameters: at(tip, all, { state: "loading" }) };
export const TipError: Story = { parameters: at(tip, all, { state: "error" }) };
