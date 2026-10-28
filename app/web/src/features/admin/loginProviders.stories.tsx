// SPDX-License-Identifier: AGPL-3.0-only
// The login providers (/admin/login-providers; web-client/screens/admin-login-providers.md) in the states of
// ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { loginProviderFixture } from "./test/handlers";
import { adminAnswering, adminFailingOn, adminLoadingOn, adminScreen, withoutPermission } from "./test/stories";

/** A provider of each kind: an OpenID Connect one that lets new users sign up, GitHub, Discord and Mastodon. */
const providers: Schemas["AdminLoginProvider"][] = [
  loginProviderFixture("0199f0e0-0000-7000-8000-0000000000c1", {
    display_name: "Example Corporation single sign-on for employees and contractors",
    issuer: "https://login.a-long-identity-provider-domain.example/realms/employees",
    scopes: ["profile", "email", "groups"],
    allow_signup: true,
    linked_users: 1284,
  }),
  loginProviderFixture("0199f0e0-0000-7000-8000-0000000000c2", {
    kind: "github",
    display_name: "GitHub",
    organizations: ["mycroblog", "my-org"],
    linked_users: 42,
  }),
  loginProviderFixture("0199f0e0-0000-7000-8000-0000000000c3", {
    kind: "discord",
    display_name: "Discord",
    guilds: ["123456789012345678"],
    state: "disabled",
    linked_users: 0,
  }),
  loginProviderFixture("0199f0e0-0000-7000-8000-0000000000c4", {
    kind: "mastodon",
    display_name: "Social",
    issuer: "https://social.example",
    allow_link: false,
    linked_users: 7,
  }),
];

const list = http.get(`${apiBase}/admin/login_providers`, ({ request }) =>
  withViewer(request, () => HttpResponse.json({ items: providers })),
);

const meta = {
  title: "Screens/Admin/Login providers",
  ...adminScreen("/admin/login-providers", [list]),
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Four providers of each kind, one disabled, with their linked users. */
export const Normal: Story = {};

/** No provider set up yet (the default handlers). */
export const Empty: Story = adminAnswering();

/** A moderator without manage_login_providers. */
export const MissingPermission: Story = withoutPermission;

/** The providers have not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/login_providers");

/** The server failed to answer the providers. */
export const ServerError: Story = adminFailingOn("get", "/admin/login_providers");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
