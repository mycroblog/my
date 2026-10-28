// SPDX-License-Identifier: AGPL-3.0-only
// The page of a voice chat room (/voice/{id}; web-client/screens/voice-room.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse, type RequestHandler } from "msw";
import { expect } from "storybook/test";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem, withViewer } from "../../test/msw/http";
import { answering, narrow, renderScreen, screenLoader, settled } from "../../test/stories";
import { roomId, voiceRoomFixture } from "./test/handlers";
import { failing, neverAnswering, showsSkeleton, voiceOn } from "./test/stories";

/** The server with voice chat on, answering the room with `room`. */
function openRoom(room: Schemas["VoiceRoom"]): RequestHandler[] {
  return [
    voiceOn,
    http.get(`${apiBase}/voice/rooms/:id`, ({ request }) => withViewer(request, () => HttpResponse.json(room))),
  ];
}

/** The room is gone for the user (a 404), and its summary is `summary`. */
function goneRoom(summary: Schemas["VoiceRoomSummary"]): RequestHandler[] {
  return [
    voiceOn,
    http.get(`${apiBase}/voice/rooms/:id`, () => problem(404, "about:blank", "Not Found")),
    http.get(`${apiBase}/voice/rooms/:id/summary`, () => HttpResponse.json(summary)),
  ];
}

const meta = {
  title: "Screens/Voice/Room",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: `/voice/${roomId}`, signedIn: true, handlers: openRoom(voiceRoomFixture()) } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Bob's open room, before the user joins: the stage with its speakers. */
export const Normal: Story = {
  play: async (context) => {
    await settled(context);
    await expect(context.canvas.getByRole("heading", { level: 1, name: "Friday night radio" })).toBeInTheDocument();
  },
};

/** The room has not arrived yet: the shape of its stage, without a spinner. */
export const Loading: Story = {
  ...answering(voiceOn, neverAnswering("/voice/rooms/:id")),
  play: async (context) => {
    await showsSkeleton(context, "Loading the room");
  },
};

/** The server failed to answer the room: the error under the page's heading, with a way to try again. */
export const ServerError: Story = {
  ...answering(voiceOn, failing("/voice/rooms/:id")),
  play: async (context) => {
    await settled(context);
    await expect(await context.canvas.findByText("Couldn't load the room")).toBeInTheDocument();
  },
};

/** The room ended: it says so, with the way to the rooms open now. */
export const Ended: Story = answering(...goneRoom({ id: roomId, state: "ended", name: "Friday night radio" }));

/** No such room. */
export const NotFound: Story = answering(
  voiceOn,
  http.get(`${apiBase}/voice/rooms/:id`, () => problem(404, "about:blank", "Not Found")),
);

/** At 320 CSS px. */
export const Narrow: Story = narrow;
