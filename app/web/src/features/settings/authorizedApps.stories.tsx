// SPDX-License-Identifier: AGPL-3.0-only
// The apps the user authorized (/settings/authorized-apps) in the states of
// ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import {
  answering,
  failingOn,
  inLocale,
  loadingOn,
  narrow,
  renderScreen,
  screenLoader,
  settled,
} from "../../test/stories";
import { authorizedApps } from "./test/storyFixtures";

/** GET /authorized_apps answering `items`. */
function authorized(items: Schemas["AuthorizedApp"][]) {
  return http.get(`${apiBase}/authorized_apps`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items })),
  );
}

const meta = {
  title: "Screens/Settings/AuthorizedApps",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings/authorized-apps", signedIn: true, handlers: [authorized(authorizedApps)] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** This server's app, an app a user registered, and two that registered themselves, one never used since. */
export const Normal: Story = {};

/** No apps authorized. */
export const Empty: Story = answering(authorized([]));

/** The apps have not arrived yet. */
export const Loading: Story = loadingOn("get", "/authorized_apps");

/** The server failed to answer the apps. */
export const ServerError: Story = failingOn("get", "/authorized_apps");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
