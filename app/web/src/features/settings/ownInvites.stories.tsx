// SPDX-License-Identifier: AGPL-3.0-only
// The user's own invites (/settings/invites) in the states of ADR-0112 §1.
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
import { ownInvitePageFixture } from "./test/handlers";
import { ownInvites } from "./test/storyFixtures";

/** GET /invites answering `items`, with `overrides` of the page. */
function inviting(items: Schemas["OwnInvite"][], overrides: Partial<Omit<Schemas["OwnInvitePage"], "items">> = {}) {
  return http.get(`${apiBase}/invites`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(ownInvitePageFixture(items, overrides))),
  );
}

const meta = {
  title: "Screens/Settings/Invites",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings/invites", signedIn: true, handlers: [inviting(ownInvites)] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Two usable invites, one used up, one expired and one revoked. */
export const Normal: Story = {};

/** No invites yet, with the form to create one. */
export const Empty: Story = answering(inviting([]));

/** A user who may not create invites any more. */
export const CannotCreate: Story = answering(inviting(ownInvites, { can_create: false }));

/** The invites have not arrived yet. */
export const Loading: Story = loadingOn("get", "/invites");

/** The server failed to answer the invites. */
export const ServerError: Story = failingOn("get", "/invites");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
