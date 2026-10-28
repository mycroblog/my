// SPDX-License-Identifier: AGPL-3.0-only
// The open voice chat rooms (/voice; web-client/screens/voice-rooms.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse, type RequestHandler } from "msw";
import { expect } from "storybook/test";
import { accountFixture, instanceFixture, otherAccountFixture, type Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { answering, inLocale, narrow, renderScreen, screenLoader, settled } from "../../test/stories";
import { participantFixture, voiceRoomFixture } from "./test/handlers";
import { failing, neverAnswering, showsSkeleton, voiceOn } from "./test/stories";

/** The server with voice chat on, answering the list with `rooms`. */
function openRooms(rooms: Schemas["VoiceRoom"][]): RequestHandler[] {
  return [
    voiceOn,
    http.get(`${apiBase}/voice/rooms`, ({ request }) => withViewer(request, () => HttpResponse.json({ items: rooms }))),
  ];
}

/** Bob's room, and a second one with three speakers and a listener. */
const twoRooms = openRooms([
  voiceRoomFixture(),
  voiceRoomFixture({
    id: "0192f0c4-7a8e-7000-8000-0000000000f2",
    name: "Morning coffee",
    description: "",
    hashtags: [],
    participants: [
      participantFixture(otherAccountFixture, { role: "speaker", host: true }),
      participantFixture(accountFixture(21, "Dave", instanceFixture.domain), { role: "speaker" }),
      participantFixture(accountFixture(22, "Erin", instanceFixture.domain)),
    ],
  }),
]);

const meta = {
  title: "Screens/Voice/Rooms",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/voice", signedIn: true, handlers: twoRooms } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Two open rooms, each a card that links to its page. */
export const Normal: Story = {
  play: async (context) => {
    await settled(context);
    await expect(context.canvas.getByRole("link", { name: "Friday night radio" })).toBeInTheDocument();
  },
};

/** No room is open: the empty state says where rooms show up. */
export const Empty: Story = {
  ...answering(...openRooms([])),
  play: async (context) => {
    await settled(context);
    await expect(context.canvas.getByRole("heading", { name: "No rooms are open right now" })).toBeInTheDocument();
  },
};

/** The rooms have not arrived yet: the shape of their cards, without a spinner. */
export const Loading: Story = {
  ...answering(voiceOn, neverAnswering("/voice/rooms")),
  play: async (context) => {
    await showsSkeleton(context, "Loading the rooms");
  },
};

/** The server failed to answer the rooms: the error in the list's place, with a way to try again. */
export const ServerError: Story = {
  ...answering(voiceOn, failing("/voice/rooms")),
  play: async (context) => {
    await settled(context);
    await expect(await context.canvas.findByText("Couldn't load the rooms")).toBeInTheDocument();
  },
};

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
