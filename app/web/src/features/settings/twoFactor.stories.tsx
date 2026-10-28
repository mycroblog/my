// SPDX-License-Identifier: AGPL-3.0-only
// Two-factor authentication (/settings/two-factor) in the states of ADR-0112 §1.
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

/** GET /two_factor answering `state`. */
function twoFactor(state: Schemas["TwoFactor"]) {
  return http.get(`${apiBase}/two_factor`, ({ request }) => withViewer(request, () => HttpResponse.json(state)));
}

const meta = {
  title: "Screens/Settings/TwoFactor",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings/two-factor", signedIn: true } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Two-factor authentication off, with the way to turn it on. */
export const Normal: Story = {};

/** Two-factor authentication on. */
export const Enabled: Story = answering(twoFactor({ enabled: true }));

/** The state has not arrived yet. */
export const Loading: Story = loadingOn("get", "/two_factor");

/** The server failed to answer the state. */
export const ServerError: Story = failingOn("get", "/two_factor");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
