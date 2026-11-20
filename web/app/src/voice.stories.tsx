// SPDX-License-Identifier: AGPL-3.0-only

// The voice chat rooms for the listener (ADR-0100 §5, §8, §9, §10; FR-924, FR-928, FR-930), on the fake server
// (app.samples.tsx): the list of the open rooms, empty, loading and failing; a room before the reader joins it (who is
// in it, a co-host, a raised hand, a participant the reader blocks, marked); in it as a listener, with a hand raised,
// as a speaker, muted by a host, with reactions rising and people talking, its sound held back, the microphone
// refused, the connection lost; why the reader is no longer in it (a host removed them, another device took the call,
// the host ended the room), on its page and in the toast of another screen; a room that ended and one the reader does
// not see; the page for a visitor, with and without the room's summary, and of a room that ended; the voice chat off;
// and the ways in: a hashtag's page with its rooms, the profile of a room's host, and the notifications of a room.
// And for the host: the form that opens a room (its address for a reader who may not, and the server's refusals),
// the room just opened, a room hosted with others in it, a hand let speak, a co-host named, the room handed over, its
// settings and invitations (and their addresses for a reader who does not host it), and the rooms one hosted in the
// settings.
// More of the open rooms, the invitations of the reader's room and the rooms they hosted than a page holds, before and
// after Show more (pages.samples.ts, `more`).

import { PreviewStill } from "@my/design-system/preview";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";
import { manyVoice } from "./pages.samples";

const meta = {
  title: "Voice chat rooms",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/voice" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The open rooms in the server's order, each with who hosts it, how many are in it and who speaks. */
export const Rooms: Story = {};
export const RoomsEmpty: Story = { parameters: screen({ at: "/voice", state: "empty" }) };
export const RoomsLoading: Story = { parameters: screen({ at: "/voice", state: "loading" }) };
export const RoomsError: Story = { parameters: screen({ at: "/voice", state: "error" }) };
/** More open rooms than a page holds: Show more under the first twenty. */
export const RoomsMany: Story = { parameters: screen({ at: "/voice", server: manyVoice }) };
/** The same after Show more: the rest under the first twenty, and no Show more. */
export const RoomsManyMore: Story = { parameters: screen({ at: "/voice", server: manyVoice, more: true }) };
/** A visitor: the list asks them to sign in. */
export const RoomsVisitor: Story = { parameters: screen({ at: "/voice", signedOut: true }) };

/**
 * A room before the reader joins it: the host, a co-host and a speaker on its stage; its listeners, a hand raised
 * and one the reader blocks marked; and Join under the sentence that joining shows the reader's name in the list.
 */
export const BeforeJoin: Story = { parameters: screen({ at: "/voice/{room:0}" }) };
export const RoomLoading: Story = { parameters: screen({ at: "/voice/{room:0}", state: "loading" }) };
export const RoomError: Story = { parameters: screen({ at: "/voice/{room:0}", state: "error" }) };
/** A room before joining, while the reader is in another one: joining leaves it. */
export const BeforeJoinInAnother: Story = {
  parameters: screen({ at: "/voice/{room:1}", joined: { room: 0 } }),
};

/** In the room as a listener who may not speak without asking: Raise hand, React, Leave; the call's bar. */
export const Listening: Story = { parameters: screen({ at: "/voice/{room:0}", joined: { room: 0 } }) };
/** A hand raised: Lower hand, knocked out, and the reader's tile marked. */
export const HandRaised: Story = {
  parameters: screen({ at: "/voice/{room:0}", joined: { room: 0, steps: ["hand"] } }),
};
/** In a room where everyone may speak: Speak in place of the hand. */
export const MaySpeak: Story = { parameters: screen({ at: "/voice/{room:1}", joined: { room: 1 } }) };
/** A speaker with the microphone on: Mute and Stop speaking among the room's controls. */
export const Speaking: Story = {
  parameters: screen({ at: "/voice/{room:1}", joined: { room: 1, steps: ["speaker"] } }),
};
/** A speaker a host muted: the microphone off, named Unmute, and the line that says who muted them. */
export const MutedByHost: Story = {
  parameters: screen({ at: "/voice/{room:1}", joined: { room: 1, steps: ["speaker", "muted"] } }),
};
/**
 * Reactions over the stage (one from a participant the reader blocks is hidden), held at a moment of their rise so that
 * a shot shows them, and the host talking.
 */
export const Reactions: Story = {
  parameters: screen({ at: "/voice/{room:0}", joined: { room: 0, steps: ["talking", "reactions"] } }),
  decorators: [
    (Shown) => (
      <PreviewStill>
        <Shown />
      </PreviewStill>
    ),
  ],
};
/** The browser holds the room's sound back: Hear the room, in the room, whose page shows no call bar. */
export const SoundHeld: Story = {
  parameters: screen({ at: "/voice/{room:0}", joined: { room: 0, steps: ["held"] } }),
};
/** The browser refused the microphone. */
export const MicrophoneRefused: Story = {
  parameters: screen({ at: "/voice/{room:1}", joined: { room: 1, steps: ["speaker", "mic-refused"] } }),
};
/** The connection lost, coming back by itself. */
export const Reconnecting: Story = {
  parameters: screen({ at: "/voice/{room:0}", joined: { room: 0, steps: ["reconnecting"] } }),
};

/** A host removed the reader: the room's page says so over the room. */
export const Removed: Story = {
  parameters: screen({ at: "/voice/{room:0}", joined: { room: 0, steps: ["removed"] } }),
};
/** The reader joined on another device: the call ended here. */
export const Replaced: Story = {
  parameters: screen({ at: "/voice/{room:0}", joined: { room: 0, steps: ["replaced"] } }),
};
/** The host ended the room: the page says so, and that the room ended. */
export const RoomEnded: Story = {
  parameters: screen({ at: "/voice/{room:0}", joined: { room: 0, steps: ["ended"] } }),
};
/** A host removed the reader while they were on another screen: a toast says so, and leads to the room. */
export const RemovedElsewhere: Story = {
  parameters: screen({ at: "/notifications", joined: { room: 0, steps: ["removed"] } }),
};

/** A room that ended, opened from an old link. */
export const Ended: Story = { parameters: screen({ at: "/voice/{room:ended}" }) };
/** A room the reader does not see. */
export const NotSeen: Story = { parameters: screen({ at: "/voice/{room:hidden}" }) };

/** A visitor: only this server's people join, and Sign in; nothing of the room. */
export const Visitor: Story = { parameters: screen({ at: "/voice/{room:0}", signedOut: true }) };
/** A visitor of a server that shows its rooms' summaries: the name, the host and the description. */
export const VisitorSummary: Story = {
  parameters: screen({ at: "/voice/{room:0}", signedOut: true, server: { voiceSummaries: true } }),
};
/** A visitor of a room that ended, on a server that shows its rooms' summaries: its name, and that it ended. */
export const VisitorEnded: Story = {
  parameters: screen({ at: "/voice/{room:ended}", signedOut: true, server: { voiceSummaries: true } }),
};
/** The voice chat off on the server. */
export const VoiceOff: Story = { parameters: screen({ at: "/voice/{room:0}", server: { voiceOff: true } }) };

/** A hashtag's page with its open rooms over its posts. */
export const HashtagRooms: Story = { parameters: screen({ at: "/tags/coffee" }) };
/** The profile of a room's host: the room where they host now, which leads to it. */
export const ProfileEntry: Story = { parameters: screen({ at: "/@sam" }) };
/** The notifications: a room's start and an invitation lead to the room. */
export const Notifications: Story = { parameters: screen({ at: "/notifications" }) };

// ---- The host's side (ADR-0100 §3, §5, §6, §7, §9, §13; FR-922, FR-923, FR-925, FR-926, FR-927, FR-929) ----------

/** The rooms of a server where only some may open rooms: no way to open one for this reader. */
export const RoomsCannotOpen: Story = {
  parameters: screen({ at: "/voice", server: { voiceStarters: "permitted" } }),
};
/**
 * The form that opens a room: its name, icon, description, hashtags, who sees it and who speaks, and the start post,
 * checked, whose draft holds the room's name as it is written.
 */
export const OpenRoom: Story = { parameters: screen({ at: "/voice/new" }) };
/** The form's address for a reader who may not open rooms. */
export const OpenRoomNotAllowed: Story = {
  parameters: screen({ at: "/voice/new", server: { voiceStarters: "permitted" } }),
};
/** The room the reader just opened, alone on its stage, its menu in its head. */
export const Opened: Story = {
  parameters: screen({ at: "/voice/{room:opened}", joined: { room: "opened" } }),
};
/**
 * The reader hosts a room with others in it: a co-host and a speaker on the stage, a raised hand among the
 * listeners; each tile but the reader's opens what the host may do with them.
 */
export const Hosting: Story = { parameters: screen({ at: "/voice/{room:2}", joined: { room: 2 } }) };
/** The host let the raised hand speak: the hand is down. */
export const HostGranted: Story = {
  parameters: screen({ at: "/voice/{room:2}", joined: { room: 2, steps: ["granted"] } }),
};
/** The host named a listener co-host. */
export const HostCohost: Story = {
  parameters: screen({ at: "/voice/{room:2}", joined: { room: 2, steps: ["cohost"] } }),
};
/** The host handed the room over: the reader stays in it, another hosts it, and the room's menu is gone. */
export const HandedOver: Story = {
  parameters: screen({ at: "/voice/{room:2}", joined: { room: 2, steps: ["handed-over"] } }),
};
/** The settings of the room the reader hosts, as the room has them. */
export const RoomSettings: Story = { parameters: screen({ at: "/voice/{room:2}/settings" }) };
/** The settings' address of a room the reader does not host. */
export const RoomSettingsNotHost: Story = { parameters: screen({ at: "/voice/{room:0}/settings" }) };
/** The invitations of the room the reader hosts, none yet, with the search of who to invite. */
export const Invitations: Story = {
  parameters: screen({ at: "/voice/{room:2}/invitations", joined: { room: 2 } }),
};
/** Two invitations, a speaker's and a listener's, each with Revoke. */
export const Invited: Story = {
  parameters: screen({ at: "/voice/{room:2}/invitations", joined: { room: 2, steps: ["invited"] } }),
};
/** More invitations than a page holds: Show more under the oldest twenty. */
export const InvitedMany: Story = {
  parameters: screen({ at: "/voice/{room:2}/invitations", joined: { room: 2 }, server: manyVoice }),
};
/** The same after Show more: the rest under the oldest twenty, and no Show more. */
export const InvitedManyMore: Story = {
  parameters: screen({ at: "/voice/{room:2}/invitations", joined: { room: 2 }, server: manyVoice, more: true }),
};
/** The invitations' address of a room the reader neither hosts nor co-hosts. */
export const InvitationsNotHost: Story = {
  parameters: screen({ at: "/voice/{room:0}/invitations", joined: { room: 0 } }),
};
/** The rooms the reader hosted, in the settings: one open now, two that ended and why. */
export const History: Story = { parameters: screen({ at: "/settings/voice", joined: { room: "opened" } }) };
/** None hosted yet. */
export const HistoryEmpty: Story = { parameters: screen({ at: "/settings/voice", state: "empty" }) };
export const HistoryLoading: Story = { parameters: screen({ at: "/settings/voice", state: "loading" }) };
export const HistoryError: Story = { parameters: screen({ at: "/settings/voice", state: "error" }) };
/** More hosted rooms than a page holds: Show more under the newest twenty. */
export const HistoryMany: Story = { parameters: screen({ at: "/settings/voice", server: manyVoice }) };
/** The same after Show more: the older rooms under the newest twenty, and no Show more. */
export const HistoryManyMore: Story = {
  parameters: screen({ at: "/settings/voice", server: manyVoice, more: true }),
};
