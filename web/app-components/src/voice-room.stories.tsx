// SPDX-License-Identifier: AGPL-3.0-only

// A voice chat room as a line of a list (ADR-0100 §8), from the room the API sends: with its icon emoji and the faces
// of who speaks, the host first; a custom emoji as its icon; none, the room's glyph in its place, with no speaker but
// the host; and the entry of a profile, where the account hosts the room or speaks in it. The names are samples.

import { RowItem, RowList } from "@my/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { PostAccount } from "./status";
import { VoiceRoomEntryLine, VoiceRoomLine, type RoomShown } from "./voice-room";

const account = (username: string, display_name: string): PostAccount => ({
  id: username,
  username,
  acct: username,
  domain: "example.social",
  display_name,
  emojis: [],
});

const mika = account("mika", "Mika");
const ren = account("ren", "Ren 蓮");
const sora = account("sora", "Sora");

const rooms: readonly RoomShown[] = [
  {
    id: "1",
    name: "Morning coffee chat",
    icon: { name: "☕" },
    host: mika,
    participants_count: 20,
    participants: [
      { account: ren, role: "speaker", hosting: false },
      { account: mika, role: "speaker", hosting: true },
      { account: sora, role: "listener", hosting: false },
    ],
  },
  {
    id: "2",
    name: "A room with a name long enough to wrap onto a second line, and then to be cut at the end of it",
    host: sora,
    participants_count: 1,
    participants: [{ account: sora, role: "speaker", hosting: true }],
  },
];

const meta = { title: "Voice room line", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** Rooms of a list: the faces of who speaks lead by the host's, and a room without an icon shows the room's glyph. */
export const Lines: Story = {
  render: () => (
    <RowList label="Voice chat rooms">
      {rooms.map((room) => (
        <RowItem key={room.id}>
          <VoiceRoomLine room={room} />
        </RowItem>
      ))}
    </RowList>
  ),
};

/** The entry of a profile: the account hosts the room, or speaks in it. */
export const ProfileEntries: Story = {
  render: () => (
    <RowList label="Voice chat rooms">
      <RowItem>
        <VoiceRoomEntryLine
          entry={{ id: "1", name: "Morning coffee chat", icon: { name: "☕" }, role: "host", participants_count: 20 }}
        />
      </RowItem>
      <RowItem>
        <VoiceRoomEntryLine entry={{ id: "3", name: "Release party", role: "speaker", participants_count: 48 }} />
      </RowItem>
    </RowList>
  ),
};
