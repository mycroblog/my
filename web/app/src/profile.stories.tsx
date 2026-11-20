// SPDX-License-Identifier: AGPL-3.0-only

// A profile, a story per relationship the reader (yuki) meets it in: their own, following each other (the bell of new
// posts off, then on), locked and followed, following them, blocked and muted, an account of another server that
// follows them, and the same with its server muted; one that moved, and one that is not there (nao's, deactivated);
// the reader's private note on it; the home of a reader who mutes a server; and a story per state every screen is
// shown in (app.samples.tsx), on a profile of another; a visitor's, of a local account and of a remote one, and the
// first page of a visitor of a one-person server, its operator's profile. What a press opens or leaves is the part's own: the note's
// editor (the app components' Profile note), the profile's and a post's menu (the design system's Menu everywhere), a
// confirmation (its List management), and the line in a post's place once its author was muted or blocked (its
// Caption line).

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const other = "/@sam";
const remote = "/@sora@photos.example";
const note = "Met at the bakery on Saturday. Ask about the rye starter. 土曜のパン屋で会った。";

const meta = {
  title: "Profile",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/profile" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Own: Story = {};
export const FollowingEachOther: Story = { parameters: screen({ at: other }) };
export const NotifyOn: Story = { parameters: screen({ at: other, server: { subscriptions: ["sam"] } }) };
export const LockedFollowing: Story = { parameters: screen({ at: "/@ren" }) };
export const FollowsYou: Story = { parameters: screen({ at: "/@mio" }) };
export const Blocked: Story = { parameters: screen({ at: "/@quickdeals" }) };
export const Muted: Story = { parameters: screen({ at: "/@kai" }) };
export const Remote: Story = { parameters: screen({ at: remote }) };
export const ServerMuted: Story = { parameters: screen({ at: remote, server: { domainMutes: ["photos.example"] } }) };
export const Moved: Story = { parameters: screen({ at: "/@sora@social.example" }) };
export const NotFound: Story = { parameters: screen({ at: "/@nao" }) };
export const NoteWritten: Story = { parameters: screen({ at: other, server: { notes: { sam: note } } }) };
/** The home timeline of a reader who mutes photos.example: sam's boost of its post is not there, sam's own posts are. */
export const HomeAfterServerMute: Story = {
  parameters: screen({ at: "/", server: { domainMutes: ["photos.example"] } }),
};
export const Empty: Story = { parameters: screen({ at: other, state: "empty" }) };
export const Loading: Story = { parameters: screen({ at: other, state: "loading" }) };
export const Failed: Story = { parameters: screen({ at: other, state: "error" }) };
export const LongText: Story = { parameters: screen({ at: other, state: "long" }) };
/** Read without a session: Follow and the menu's Report lead to the sign-in, as their posts' actions do. */
export const Visitor: Story = { parameters: screen({ at: other, signedOut: true }) };
export const VisitorRemote: Story = { parameters: screen({ at: remote, signedOut: true }) };
/** The first page of a one-person server, signed out: its operator's profile (ADR-0152 §7). */
export const VisitorSingleUser: Story = {
  parameters: screen({ at: "/", signedOut: true, server: { singleUser: true } }),
};
