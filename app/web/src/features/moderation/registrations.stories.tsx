// SPDX-License-Identifier: AGPL-3.0-only
// The registrations awaiting approval (/moderation/registrations; web-client/screens/moderation.md) in the states of
// ADR-0112 §1.
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
import { registrationFixture } from "./test/handlers";
import { hoursAgo, listing, moderatorViewer, showing, storyId, withoutPermission } from "./test/stories";

/** A registration without a reason. */
function withoutReason(overrides: Parameters<typeof registrationFixture>[0]) {
  const { reason: _, ...registration } = registrationFixture(overrides);
  return registration;
}

/** Sign-ups with a short, a long and no reason, an unverified address, and one by invite. */
const registrations = listing("/admin/registrations", [
  registrationFixture({ since: hoursAgo(2), submitted_at: hoursAgo(2) }),
  registrationFixture({
    user_id: storyId("b1", 2),
    account_id: storyId("c1", 2),
    username: "maximiliana_theodora_wolfharte",
    email: "maximiliana.theodora.wolfharte@a-very-long-mail-provider.example",
    reason:
      "I run a small bakery and a cycling club, and I would like one place to post about both: opening hours, rides on the weekend, and the occasional recipe. A friend of mine is a member here and recommended this server.",
    since: hoursAgo(20),
    submitted_at: hoursAgo(20),
  }),
  withoutReason({
    user_id: storyId("b1", 3),
    account_id: storyId("c1", 3),
    username: "dana",
    email: "dana@example.org",
    email_verified: false,
    since: hoursAgo(50),
    submitted_at: hoursAgo(50),
  }),
  registrationFixture({
    user_id: storyId("b1", 4),
    account_id: storyId("c1", 4),
    username: "kenji",
    email: "kenji@example.jp",
    reason: "写真と自転車について投稿したいです。",
    invite_id: storyId("a6", 1),
    since: hoursAgo(75),
    submitted_at: hoursAgo(75),
  }),
]);

const meta = {
  title: "Screens/Moderation/Registrations",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: { path: "/moderation/registrations", signedIn: true, handlers: [moderatorViewer, registrations] },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Four sign-ups awaiting approval. */
export const Normal: Story = showing("maximiliana.theodora.wolfharte@a-very-long-mail-provider.example");

/** No sign-up awaits approval. */
export const Empty: Story = answering(listing("/admin/registrations", []));

/** The registrations have not arrived yet. */
export const Loading: Story = loadingOn("get", "/admin/registrations");

/** The server failed to answer the registrations. */
export const ServerError: Story = failingOn("get", "/admin/registrations");

/** A member without manage_accounts is told why. */
export const WithoutPermission: Story = withoutPermission;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
