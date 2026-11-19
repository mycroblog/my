// SPDX-License-Identifier: AGPL-3.0-only

// The voice chat rooms for the listener (ADR-0100 §5, §8, §9, §10; FR-924, FR-928, FR-930), on the fake server
// (app.samples.tsx): the list of the open rooms, empty, loading and failing; a room before the reader joins it (who is
// in it, a co-host, a raised hand, a participant the reader blocks, marked); in it as a listener, with a hand raised,
// as a speaker, muted by a host, with reactions rising and people talking, its sound held back, the microphone
// refused, the connection lost; why the reader is no longer in it (a host removed them, another device took the call,
// the host ended the room), on its page and in the toast of another screen; a room that ended and one the reader does
// not see; the page for a visitor, with and without the room's summary, and of a room that ended; the voice chat off;
// and the ways in: a hashtag's page with its rooms, the profile of a room's host, and the notifications of a room.

import { PreviewStill } from "@my/design-system/preview";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

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
