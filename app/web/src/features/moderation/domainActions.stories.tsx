// SPDX-License-Identifier: AGPL-3.0-only
// The domain actions (/moderation/domain-actions; web-client/screens/moderation.md): the actions on remote servers,
// the form that takes one, and the import and export of domain blocks, in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { longDomainAccountFixture } from "../../test/fixtures";
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
import { domainActionFixture } from "./test/handlers";
import { hoursAgo, hoursFromNow, listing, moderatorViewer, showing, storyId, withoutPermission } from "./test/stories";

/** Actions of every kind, a few with comments, one hidden by obfuscation, one expiring, and two that ended. */
const actions = listing("/admin/domain_actions", [
  domainActionFixture({
    id: storyId("a0", 1),
    domain: "spam.example",
    kind: "suspend",
    public_comment: "Spam and advertising from every account.",
    private_note: "Reported by three moderators of other servers.",
    issued_at: hoursAgo(5),
  }),
  domainActionFixture({ id: storyId("a0", 2), domain: "spam.example", kind: "reject_reports", issued_at: hoursAgo(5) }),
  domainActionFixture({
    id: storyId("a0", 3),
    domain: longDomainAccountFixture.domain,
    kind: "limit",
    public_comment: "Many unsolicited replies; their posts reach only their followers here.",
    issued_at: hoursAgo(30),
    expires_at: hoursFromNow(24 * 14),
  }),
  domainActionFixture({
    id: storyId("a0", 4),
    domain: "media-heavy.example",
    kind: "reject_media",
    issued_at: hoursAgo(80),
  }),
  domainActionFixture({
    id: storyId("a0", 5),
    domain: "nsfw.example",
    kind: "force_sensitive",
    obfuscate: true,
    issued_at: hoursAgo(200),
  }),
  domainActionFixture({
    id: storyId("a0", 6),
    domain: "fixed.example",
    kind: "limit",
    state: "lifted",
    issued_at: hoursAgo(500),
    lifted_at: hoursAgo(300),
  }),
  domainActionFixture({
    id: storyId("a0", 7),
    domain: "temporary.example",
    kind: "suspend",
    state: "expired",
    issued_at: hoursAgo(900),
    expires_at: hoursAgo(700),
  }),
]);

const meta = {
  title: "Screens/Moderation/DomainActions",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/moderation/domain-actions", signedIn: true, handlers: [moderatorViewer, actions] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The actions in effect: a suspension, a limit on a long domain, rejections and forced sensitivity. */
export const Normal: Story = showing("Spam and advertising from every account.");

/** Every action, those lifted and expired too. */
export const All: Story = { parameters: { screen: { path: "/moderation/domain-actions?filter=all" } } };

/** No action on any server. */
export const Empty: Story = answering(listing("/admin/domain_actions", []));

/** The actions have not arrived yet. */
export const Loading: Story = loadingOn("get", "/admin/domain_actions");

/** The server failed to answer the actions. */
export const ServerError: Story = failingOn("get", "/admin/domain_actions");

/** A member without manage_domains is told why. */
export const WithoutPermission: Story = withoutPermission;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
