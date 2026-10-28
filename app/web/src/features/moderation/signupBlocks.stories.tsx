// SPDX-License-Identifier: AGPL-3.0-only
// The sign-up blocks (/moderation/signup-blocks; web-client/screens/moderation.md): the blocks on email domains,
// email addresses and IP ranges, the form that adds one, and the import, in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
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
import { signupBlockFixture } from "./test/handlers";
import { hoursAgo, hoursFromNow, listing, moderatorViewer, showing, storyId, withoutPermission } from "./test/stories";

/** Blocks of every kind and severity, a few with comments or an expiry, two that ended. */
const blocks = listing("/admin/signup_blocks", [
  signupBlockFixture({
    id: storyId("a1", 1),
    value: "disposable-mail.example",
    comment: "Throwaway addresses used by spam sign-ups.",
    created_at: hoursAgo(3),
  }),
  signupBlockFixture({
    id: storyId("a1", 2),
    kind: "ip_range",
    value: "203.0.113.0/24",
    severity: "require_approval",
    comment: "A hosting provider many bots sign up from; real people may use it too.",
    created_at: hoursAgo(40),
    expires_at: hoursFromNow(24 * 30),
  }),
  signupBlockFixture({
    id: storyId("a1", 3),
    kind: "ip_range",
    value: "2001:db8:abcd:0012::/64",
    created_at: hoursAgo(90),
  }),
  signupBlockFixture({
    id: storyId("a1", 4),
    kind: "email_hash",
    value: "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
    comment: "Banned person signing up again.",
    created_at: hoursAgo(200),
  }),
  signupBlockFixture({
    id: storyId("a1", 5),
    value: "a-very-long-subdomain.of-a-mail-provider-that-hands-out-addresses.example",
    severity: "require_approval",
    created_at: hoursAgo(300),
  }),
  signupBlockFixture({ id: storyId("a1", 6), value: "old.example", state: "lifted", created_at: hoursAgo(900) }),
  signupBlockFixture({
    id: storyId("a1", 7),
    kind: "ip_range",
    value: "198.51.100.0/24",
    state: "expired",
    created_at: hoursAgo(1200),
    expires_at: hoursAgo(500),
  }),
]);

const meta = {
  title: "Screens/Moderation/SignupBlocks",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/moderation/signup-blocks", signedIn: true, handlers: [moderatorViewer, blocks] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The blocks in effect: email domains, a hashed address, IPv4 and IPv6 ranges, rejecting or asking for approval. */
export const Normal: Story = showing("disposable-mail.example");

/** Every block, those lifted and expired too. */
export const All: Story = { parameters: { screen: { path: "/moderation/signup-blocks?filter=all" } } };

/** No sign-up is blocked. */
export const Empty: Story = answering(listing("/admin/signup_blocks", []));

/** The blocks have not arrived yet. */
export const Loading: Story = loadingOn("get", "/admin/signup_blocks");

/** The server failed to answer the blocks. */
export const ServerError: Story = failingOn("get", "/admin/signup_blocks");

/** A member without the permission is told why. */
export const WithoutPermission: Story = withoutPermission;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
