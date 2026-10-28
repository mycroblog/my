// SPDX-License-Identifier: AGPL-3.0-only
// The user's own OAuth apps (/settings/apps; web-client/screens/oauth-apps.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { answering, failingOn, inLocale, loadingOn, narrow, renderScreen, screenLoader, settled } from "../../test/stories";
import { ownApps } from "./test/storyFixtures";

/** GET /oauth_apps answering `items`. */
function registered(items: Schemas["OAuthApp"][]) {
  return http.get(`${apiBase}/oauth_apps`, ({ request }) => withViewer(request, () => HttpResponse.json({ items })));
}

const meta = {
  title: "Screens/Settings/OAuthApps",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings/apps", signedIn: true, handlers: [registered(ownApps)] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A confidential web app, a public native one with a long name, and a disabled one. */
export const Normal: Story = {};

/** No apps registered yet. */
export const Empty: Story = answering(registered([]));

/** The apps have not arrived yet. */
export const Loading: Story = loadingOn("get", "/oauth_apps");

/** The server failed to answer the apps. */
export const ServerError: Story = failingOn("get", "/oauth_apps");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
