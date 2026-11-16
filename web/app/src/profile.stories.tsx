// SPDX-License-Identifier: AGPL-3.0-only

// A profile, a story per relationship the reader (yuki) meets it in: their own, following each other (the bell of new
// posts off, then on), locked and followed, following them, blocked and muted, an account of another server that
// follows them, and the same with its server muted; the reader's private note on it, written and in its editor; the
// confirmations of its menu (block, remove a follower); a post's menu on another's post, and that post once the reader
// muted or blocked its author or muted its server from it (profile.plays.ts); and a story per state every screen is
// shown in (app.samples.tsx), on a profile of another.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { minimumWidth, rightToLeft, screen, screenMeta } from "./app.samples";
import * as plays from "./profile.plays";

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
export const FollowingEachOther: Story = { parameters: screen({ at: other }), play: plays.notifyOff };
export const NotifyOn: Story = {
  parameters: screen({ at: other, server: { subscriptions: ["sam"] } }),
  play: plays.notifyOn,
};
export const LockedFollowing: Story = { parameters: screen({ at: "/@ren" }) };
export const FollowsYou: Story = { parameters: screen({ at: "/@mio" }) };
export const Blocked: Story = { parameters: screen({ at: "/@quickdeals" }) };
export const Muted: Story = { parameters: screen({ at: "/@kai" }) };
export const Remote: Story = { parameters: screen({ at: remote }), play: plays.noBell };
export const ServerMuted: Story = {
  parameters: screen({ at: remote, server: { domainMutes: ["photos.example"] } }),
  play: plays.serverMuted,
};
export const NoteWritten: Story = {
  parameters: screen({ at: other, server: { notes: { sam: note } } }),
  play: plays.noteWritten,
};
export const NoteEditing: Story = {
  parameters: screen({ at: other, server: { notes: { sam: note } } }),
  play: plays.noteEditing,
};
export const BlockConfirm: Story = { parameters: screen({ at: other }), play: plays.blockConfirm };
export const RemoveFollowerConfirm: Story = { parameters: screen({ at: remote }), play: plays.removeFollowerConfirm };
export const PostMenuOfAnother: Story = { parameters: screen({ at: "/" }), play: plays.postMenuOpen };
export const PostAfterMute: Story = { parameters: screen({ at: "/" }), play: plays.mutedFromPost };
export const PostAfterBlock: Story = { parameters: screen({ at: "/" }), play: plays.blockedFromPost };
export const PostAfterServerMute: Story = { parameters: screen({ at: "/" }), play: plays.serverMutedFromPost };
export const HomeAfterServerMute: Story = { parameters: screen({ at: "/" }), play: plays.homeAfterServerMute };
export const Empty: Story = { parameters: screen({ at: other, state: "empty" }) };
export const Loading: Story = { parameters: screen({ at: other, state: "loading" }) };
export const Failed: Story = { parameters: screen({ at: other, state: "error" }) };
export const LongText: Story = { parameters: screen({ at: other, state: "long" }) };
export const RightToLeft: Story = { parameters: screen({ at: other }), globals: rightToLeft };
export const MinimumWidth: Story = { parameters: screen({ at: other }), globals: minimumWidth };
