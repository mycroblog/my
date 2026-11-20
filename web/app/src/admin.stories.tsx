// SPDX-License-Identifier: AGPL-3.0-only

// The server's administration of its settings, texts, rules, relays, login providers and OAuth apps (FR-001, FR-010,
// FR-705, FR-724, FR-725, FR-922, ADR-0058 §1), on the fake server (app.samples.tsx) as its administrator (hana) reads
// it. The root: the screens of This server and Connections, with what waits counted and with nothing waiting. Server settings: its pages in groups, and each page as a
// form: General with the thumbnail, the icon and logo set and built in, Sign-up and the same on a one-person server,
// Posts and features, Federation, Automated review, Search engines and AI, Voice chat, Deleting old posts, Appeals and
// records; loading, failing, and an address that names no page. Terms and policies, and a document. Translation with a
// provider and with none. Rules: those in effect and retired, a rule, a retired rule, a new rule, and an address that
// names none. Relays: joined, waiting and rejected, none, a joined relay, a rejected one, and a new one. Login
// providers: the fake's three, none, the limit to providers waiting for one, a provider, and a new one. OAuth apps: of
// every way, kept to one, and the denied hosts. What follows a press (a question before saving, a refused field,
// saved) is the route tests'.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta, type Fake } from "./app.samples";

const meta = {
  title: "Administration",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/admin", viewer: "hana" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The screen at `at` as the scenario's administrator reads it. */
const at = (path: string, more: Omit<Fake, "at"> = {}) => screen({ at: path, viewer: "hana", ...more });

/**
 * Administration: This server (settings, terms and policies, rules, translation) and Connections, with the requests of
 * custom emoji waiting for review counted on Custom emoji and the open errors on Errors.
 */
export const Root: Story = { parameters: at("/admin") };
/** Administration with no request of custom emoji waiting and no open error: neither entry counts anything. */
export const RootNothingWaiting: Story = {
  parameters: at("/admin", { server: { staffServer: { emoji: { none: true } }, operations: "none" } }),
};

/** Server settings: its pages in groups, each with a line of what it holds. */
export const Settings: Story = { parameters: at("/admin/settings") };
/** General: the name and descriptions, the default language, the contacts, and the thumbnail with its description. */
export const SettingsGeneral: Story = { parameters: at("/admin/settings/general", { server: { thumbnail: true } }) };
/** The icon and logo, a logo and a logo for dark backgrounds set, the icon built in. */
export const SettingsBranding: Story = { parameters: at("/admin/settings/branding", { server: { logo: true } }) };
/** The icon and logo, all built in. */
export const SettingsBrandingBuiltIn: Story = { parameters: at("/admin/settings/branding") };
/** Sign-up: who can sign up, each mode with what it does, member invites, login providers only, and apps. */
export const SettingsRegistration: Story = {
  parameters: at("/admin/settings/registration", { server: { registrations: "approval" } }),
};
/** Sign-up on a one-person server: the sections of signing up say that nobody else signs up. */
export const SettingsRegistrationSingleUser: Story = {
  parameters: at("/admin/settings/registration", { server: { singleUser: true } }),
};
/** Posts and features: posts, timelines, link cards, drafts, articles, events, and the front page. */
export const SettingsContent: Story = { parameters: at("/admin/settings/content") };
/** Federation: its direction and its servers, each with what it does, signed fetches, media, posts, domain actions. */
export const SettingsFederation: Story = { parameters: at("/admin/settings/federation") };
/** Automated review: each decision hook's mode and what it does when its plugin fails. */
export const SettingsHooks: Story = { parameters: at("/admin/settings/hooks") };
/** Search engines and AI: what each choice states, the default of new accounts, and what search covers. */
export const SettingsDiscovery: Story = { parameters: at("/admin/settings/discovery") };
/** Voice chat: who can open rooms, what a local room shows outside, and how long who took part is kept. */
export const SettingsVoice: Story = { parameters: at("/admin/settings/voice") };
/** Deleting old posts: the default of new accounts and what it keeps, and the maximum for everyone. */
export const SettingsAutoDelete: Story = { parameters: at("/admin/settings/auto-delete") };
/** Appeals and records: how long people can appeal, and how long each record is kept. */
export const SettingsRecords: Story = { parameters: at("/admin/settings/records") };
export const SettingsLoading: Story = { parameters: at("/admin/settings/general", { state: "loading" }) };
export const SettingsError: Story = { parameters: at("/admin/settings/general", { state: "error" }) };
/** An address that names no page of the settings. */
export const SettingsMissing: Story = { parameters: at("/admin/settings/none") };

/** Terms and policies: a door to each document. */
export const Documents: Story = { parameters: at("/admin/documents") };
/** The terms of service, in Markdown. */
export const DocumentTerms: Story = { parameters: at("/admin/documents/terms") };
/** The commerce disclosure, not written yet. */
export const DocumentEmpty: Story = { parameters: at("/admin/documents/commerce-disclosure") };

/** Translation: this month's characters and kept answers, the server's two providers, one used, and the browser's own. */
export const Translation: Story = {
  parameters: at("/admin/translation", { server: { translation: { provider: "answers", browser: true } } }),
};
/** Translation with no provider used. */
export const TranslationNoProvider: Story = { parameters: at("/admin/translation") };

/** Rules: the four in effect in their order, with Add a rule, and the two retired. */
export const Rules: Story = { parameters: at("/admin/rules") };
/** A rule in effect: its text, explanation and place, and Retire last. */
export const Rule: Story = { parameters: at("/admin/rules/{admin:rule:0}") };
/** A retired rule: what it said, and when it was retired. */
export const RuleRetired: Story = { parameters: at("/admin/rules/{admin:rule:4}") };
/** A new rule. */
export const RuleNew: Story = { parameters: at("/admin/rules/new") };
/** An address that names no rule. */
export const RuleMissing: Story = { parameters: at("/admin/rules/none") };
export const RulesLoading: Story = { parameters: at("/admin/rules", { state: "loading" }) };

/** Relays: one joined, one waiting to be accepted that only sends, one that rejected the server. */
export const Relays: Story = { parameters: at("/admin/relays") };
/** No relay yet. */
export const RelaysEmpty: Story = { parameters: at("/admin/relays", { server: { admin: { lists: "none" } } }) };
/** A joined relay: where it stands, what it brought in the last day, and Leave. */
export const Relay: Story = { parameters: at("/admin/relays/{admin:relay:0}") };
/** A relay that rejected the server: Join again, and Delete last. */
export const RelayRejected: Story = { parameters: at("/admin/relays/{admin:relay:2}") };
/** A new relay. */
export const RelayNew: Story = { parameters: at("/admin/relays/new") };

/** Login providers: a Mastodon server disabled, GitHub for an Organization, and an OpenID Connect workspace. */
export const LoginProviders: Story = { parameters: at("/admin/login-providers") };
/** No login provider yet. */
export const LoginProvidersEmpty: Story = {
  parameters: at("/admin/login-providers", { server: { admin: { lists: "none" } } }),
};
/** Sign-in is set to login providers only and none is enabled: the summary says it starts once one is. */
export const LoginProvidersLimitWaiting: Story = {
  parameters: at("/admin/login-providers", { server: { admin: { providersOnlyWaiting: true } } }),
};
/** An OpenID Connect provider: its callback URL to copy, its state, its connection and who can use it, Delete last. */
export const LoginProvider: Story = { parameters: at("/admin/login-providers/{admin:provider:2}") };
/** A new login provider, of OpenID Connect at first. */
export const LoginProviderNew: Story = { parameters: at("/admin/login-providers/new") };

/** OAuth apps of every way of registration, one disabled, the server's own last; and the door to the denied hosts. */
export const OAuthApps: Story = { parameters: at("/admin/oauth-apps") };
/** The OAuth apps' second page, two a page. */
export const OAuthAppsSecondPage: Story = {
  parameters: at("/admin/oauth-apps", { server: { staffPage: 2 }, next: true }),
};
/** The apps registered by a person. */
export const OAuthAppsByPeople: Story = { parameters: at("/admin/oauth-apps?registration=user") };
/** The denied hosts of metadata documents, one denied, and the form that denies another. */
export const OAuthHosts: Story = { parameters: at("/admin/oauth-apps/hosts") };
/** The denied hosts' second page, one a page. */
export const OAuthHostsSecondPage: Story = {
  parameters: at("/admin/oauth-apps/hosts", { server: { staffPage: 1 }, next: true }),
};
