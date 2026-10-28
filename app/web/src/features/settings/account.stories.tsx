// SPDX-License-Identifier: AGPL-3.0-only
// The account page (/settings/account; web-client/screens/account.md): the archives of the user's data and deleting the
// account, in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { answering, failingOn, inLocale, loadingOn, narrow, renderScreen, screenLoader, settled } from "../../test/stories";
import { dataExports } from "./test/storyFixtures";

/** GET /exports answering `items`. */
function exporting(items: Schemas["DataExport"][]) {
  return http.get(`${apiBase}/exports`, ({ request }) => withViewer(request, () => HttpResponse.json({ items })));
}

const meta = {
  title: "Screens/Settings/Account",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings/account", signedIn: true, handlers: [exporting(dataExports)] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** An archive being made, one ready to download, and one that failed. */
export const Normal: Story = {};

/** No archives requested yet. */
export const Empty: Story = answering(exporting([]));

/** A user who signs in only with another service, without a password. */
export const WithoutPassword: Story = answering(
  exporting([]),
  http.get(`${apiBase}/accounts/password`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ has_password: false } satisfies Schemas["PasswordStatus"])),
  ),
);

/** The archives have not arrived yet. */
export const Loading: Story = loadingOn("get", "/exports");

/** The server failed to answer the archives. */
export const ServerError: Story = failingOn("get", "/exports");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
