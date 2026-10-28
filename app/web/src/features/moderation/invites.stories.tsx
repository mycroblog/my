// SPDX-License-Identifier: AGPL-3.0-only
// The invites (/moderation/invites; web-client/screens/moderation.md): the form that creates one and the invites
// of the staff and the members, in every state, in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { longNameAccountFixture, otherAccountFixture } from "../../test/fixtures";
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
import { inviteFixture } from "./test/handlers";
import {
  hoursAgo,
  hoursFromNow,
  listing,
  moderatorViewer,
  monaAccount,
  showing,
  storyId,
  withoutPermission,
} from "./test/stories";

/** Invites usable, used up, revoked, expired, and of a member who may no longer invite, newest first. */
const invites = listing("/admin/invites", [
  inviteFixture({
    id: storyId("a6", 1),
    code: "BAKERYCLUB2026AB",
    max_uses: 10,
    uses: 3,
    created_by: monaAccount,
    created_at: hoursAgo(2),
    expires_at: hoursFromNow(24 * 7),
  }),
  inviteFixture({
    id: storyId("a6", 2),
    code: "UNLIMITEDINVITE2",
    uses: 41,
    created_at: hoursAgo(30),
  }),
  inviteFixture({
    id: storyId("a6", 3),
    code: "MEMBERINVITE3456",
    creator_kind: "member",
    created_by: longNameAccountFixture,
    uses: 1,
    state: "exhausted",
    created_at: hoursAgo(60),
    expires_at: hoursFromNow(24),
  }),
  inviteFixture({
    id: storyId("a6", 4),
    code: "REVOKEDINVITE789",
    state: "revoked",
    created_by: monaAccount,
    created_at: hoursAgo(100),
  }),
  inviteFixture({
    id: storyId("a6", 5),
    code: "EXPIREDINVITEABC",
    state: "expired",
    created_at: hoursAgo(24 * 20),
    expires_at: hoursAgo(24 * 13),
  }),
  inviteFixture({
    id: storyId("a6", 6),
    code: "INELIGIBLEDEF234",
    creator_kind: "member",
    created_by: otherAccountFixture,
    state: "creator_ineligible",
    created_at: hoursAgo(24 * 25),
  }),
  inviteFixture({
    id: storyId("a6", 7),
    code: "DELETEDCREATOR56",
    creator_kind: "member",
    created_by_deleted: true,
    created_at: hoursAgo(24 * 30),
    expires_at: hoursFromNow(24 * 2),
  }),
]);

const meta = {
  title: "Screens/Moderation/Invites",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/moderation/invites", signedIn: true, handlers: [moderatorViewer, invites] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Invites of the staff and of members, in every state. */
export const Normal: Story = showing(/BAKERYCLUB2026AB/);

/** No invite has been created. */
export const Empty: Story = answering(listing("/admin/invites", []));

/** The invites have not arrived yet. */
export const Loading: Story = loadingOn("get", "/admin/invites");

/** The server failed to answer the invites. */
export const ServerError: Story = failingOn("get", "/admin/invites");

/** A member without manage_invites is told why. */
export const WithoutPermission: Story = withoutPermission;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
