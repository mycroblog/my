// SPDX-License-Identifier: AGPL-3.0-only
// The relays (/admin/relays; web-client/screens/admin-relays.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { relayFixture } from "./test/handlers";
import { adminAnswering, adminFailingOn, adminLoadingOn, adminScreen, withoutPermission } from "./test/stories";

/** GET /admin/relays answering `relays`. */
function relaysOf(relays: Schemas["AdminRelay"][]) {
  return http.get(`${apiBase}/admin/relays`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items: relays } satisfies Schemas["AdminRelayList"])),
  );
}

/** A rejected relay has no actor: the relay never answered with one. */
const { actor_uri: _, ...rejected } = relayFixture("0199f0e0-0000-7000-8000-0000000000d4", {
  url: "https://relay.closed.example/inbox",
  inbox_url: "https://relay.closed.example/inbox",
  state: "rejected",
  state_changed_at: "2026-10-21T00:00:00Z",
});

/** Relays in every state: a busy joined one, one that only sends, one waiting, one refused, one left, one suspended. */
const everyState = relaysOf([
  relayFixture("0199f0e0-0000-7000-8000-0000000000d1", {
    url: "https://relay.a-very-long-relay-domain-name-for-the-fediverse.example/inbox",
    inbox_url: "https://relay.a-very-long-relay-domain-name-for-the-fediverse.example/inbox",
    taken_last_day: 12840,
    dropped_last_day: 37,
  }),
  relayFixture("0199f0e0-0000-7000-8000-0000000000d2", {
    url: "https://pub.litepub.example/actor",
    inbox_url: "https://pub.litepub.example/inbox",
    protocol: "litepub",
    receive: false,
  }),
  relayFixture("0199f0e0-0000-7000-8000-0000000000d3", {
    url: "https://relay.waiting.example/inbox",
    inbox_url: "https://relay.waiting.example/inbox",
    state: "pending",
    state_changed_at: "2026-10-22T00:00:00Z",
  }),
  rejected,
  relayFixture("0199f0e0-0000-7000-8000-0000000000d5", {
    url: "https://relay.old.example/inbox",
    inbox_url: "https://relay.old.example/inbox",
    state: "left",
  }),
  relayFixture("0199f0e0-0000-7000-8000-0000000000d6", {
    url: "https://relay.spammy.example/inbox",
    inbox_url: "https://relay.spammy.example/inbox",
    taken_last_day: 120,
    dropped_last_day: 118,
    domain_suspended: true,
  }),
]);

const meta = { title: "Screens/Admin/Relays", ...adminScreen("/admin/relays", [everyState]) } satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Relays in every state, a busy one with a long domain, and one whose domain is suspended. */
export const Normal: Story = {};

/** A server that has joined no relay (the default handlers). */
export const Empty: Story = adminAnswering();

/** A moderator without manage_settings. */
export const MissingPermission: Story = withoutPermission;

/** The relays have not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/relays");

/** The server failed to answer the relays. */
export const ServerError: Story = adminFailingOn("get", "/admin/relays");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px, where the long relay address wraps. */
export const Narrow: Story = narrow;
