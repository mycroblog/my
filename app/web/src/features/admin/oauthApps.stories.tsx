// SPDX-License-Identifier: AGPL-3.0-only
// The OAuth apps (/admin/oauth-apps; web-client/screens/admin-oauth-apps.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { hostDenialFixture, oauthClientFixture } from "./test/handlers";
import { adminAnswering, adminFailingOn, adminLoadingOn, adminScreen, withoutPermission } from "./test/stories";

/** Apps of every registration route: the web client itself, apps of users, a metadata document, and dynamic ones. */
const clients: Schemas["AdminOAuthClient"][] = [
  oauthClientFixture("0199f0e0-0000-7000-8000-0000000000a0", {
    name: "my web",
    client_id: "web",
    registration: "first_party",
    scope: "read write follow push",
  }),
  oauthClientFixture("0199f0e0-0000-7000-8000-0000000000a1", {
    owner_id: "0192f0c4-7a8e-7000-8000-000000000002",
    website: "https://tootle.example",
  }),
  oauthClientFixture("0199f0e0-0000-7000-8000-0000000000a2", {
    name: "A third-party client with a name long enough to wrap on a phone screen",
    owner_id: "0192f0c4-7a8e-7000-8000-000000000002",
    confidential: true,
    scope: "read",
  }),
  oauthClientFixture("0199f0e0-0000-7000-8000-0000000000a3", {
    registration: "metadata_document",
    client_id: "https://app.example/client.json",
    name: "Doc",
    website: "https://app.example",
  }),
  oauthClientFixture("0199f0e0-0000-7000-8000-0000000000a4", {
    registration: "dynamic",
    name: "Spammy bot",
    disabled: true,
  }),
];

const hosts: Schemas["AdminOAuthClientHostDenial"][] = [
  hostDenialFixture("0199f0e0-0000-7000-8000-0000000000f1", "spam.example"),
  hostDenialFixture("0199f0e0-0000-7000-8000-0000000000f2", "a-very-long-subdomain.of-an-abusive-host.example", {
    denied_by: "0192f0c4-7a8e-7000-8000-000000000001",
  }),
];

const handlers = [
  http.get(`${apiBase}/admin/oauth_clients`, ({ request }) => {
    const registration = new URL(request.url).searchParams.get("registration");
    const items = clients.filter((client) => registration === null || client.registration === registration);
    return withViewer(request, () => HttpResponse.json({ items }));
  }),
  http.get(`${apiBase}/admin/oauth_client_host_denials`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: hosts })),
  ),
];

const meta = { title: "Screens/Admin/OAuth apps", ...adminScreen("/admin/oauth-apps", handlers) } satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Apps of every registration route, one disabled. */
export const Normal: Story = {};

/** The hosts whose metadata documents the server refuses. */
export const DeniedHosts: Story = { parameters: { screen: { path: "/admin/oauth-apps?tab=hosts" } } };

/** No app (the default handlers). */
export const Empty: Story = adminAnswering();

/** A moderator without manage_oauth_apps. */
export const MissingPermission: Story = withoutPermission;

/** The apps have not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/oauth_clients");

/** The server failed to answer the apps. */
export const ServerError: Story = adminFailingOn("get", "/admin/oauth_clients");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
