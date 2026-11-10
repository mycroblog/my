// SPDX-License-Identifier: AGPL-3.0-only
// The supporters feature (ADR-0070 §13, ADR-0072 §6, §8, §13, §14): the user's page of the supporter plans
// (`/settings/supporter`) with the final confirmation, the provider's hosted checkout and portal, and the changes of
// the user's subscription; the plans and the link to the supporter list on the information page; the supporter list
// (`/about/supporters`); the user's choice to be on it on the profile settings; and the tips (ADR-0162): the user's
// page of the tips (`/settings/supporter/tip`) with the amounts, the final confirmation, the provider's hosted checkout
// and the thanks, and the entries to it on the information page, the funding meter's panel and the plans' page. The Tauri app shows no way to
// subscribe (lib/platform.ts). Routes and other features use it through this file only (web-client/README.md A3).
export {
  ownSupporterSubscriptionQueryOptions,
  supporterListingQueryOptions,
  supporterListQueryOptions,
  supporterPlansQueryOptions,
  syncOwnSubscription,
} from "./api/supporters";
export { AboutSupportersSection, SupporterListingSettings, SupporterListView } from "./components/SupporterList";
export { SupporterSubscriptionSettings } from "./components/SupporterSubscriptionSettings";
export { type SupporterReturn, useSupporterEntry } from "./hooks/useSupporterSubscription";
export { ownTipQueryOptions, tipAmountsQueryOptions } from "./api/tips";
export { AboutTipsSection, FundingMeterTipLink, TipSettings } from "./components/Tips";
export type { TipReturn } from "./hooks/useTip";
