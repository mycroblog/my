// SPDX-License-Identifier: AGPL-3.0-only
// The password page (/settings/password) in the states of ADR-0112 §1.
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

/** GET /accounts/password answering whether the user has a password. */
function password(has_password: boolean) {
  return http.get(`${apiBase}/accounts/password`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ has_password } satisfies Schemas["PasswordStatus"])),
  );
}

const meta = {
  title: "Screens/Settings/Password",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings/password", signedIn: true } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A user with a password: the form to change it. */
export const Normal: Story = {};

/** A user without a password, who may set one. */
export const WithoutPassword: Story = answering(password(false));

/** The state of the password has not arrived yet. */
export const Loading: Story = loadingOn("get", "/accounts/password");

/** The server failed to answer the state of the password. */
export const ServerError: Story = failingOn("get", "/accounts/password");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
