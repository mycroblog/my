// SPDX-License-Identifier: AGPL-3.0-only
// The voice chat history of the settings (/settings/voice) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse, type RequestHandler } from "msw";
import { expect } from "storybook/test";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { answering, inLocale, narrow, renderScreen, screenLoader, settled } from "../../test/stories";
import { failing, neverAnswering, showsSkeleton, voiceOn } from "./test/stories";

/** The server with voice chat on, answering the history with `items`. */
function hosted(items: Schemas["VoiceRoomHistoryPage"]["items"]): RequestHandler[] {
  return [
    voiceOn,
    http.get(`${apiBase}/voice/history`, ({ request }) => withViewer(request, () => HttpResponse.json({ items }))),
  ];
}

/** A room still open and one that ended when everyone left. */
const twoRooms = hosted([
  { id: "0192f0c4-7a8e-7000-8000-0000000000f2", name: "Late show", opened_at: "2026-10-26T20:00:00Z" },
  {
    id: "0192f0c4-7a8e-7000-8000-0000000000f1",
    name: "Morning call",
    opened_at: "2026-10-25T08:00:00Z",
    ended_at: "2026-10-25T09:00:00Z",
    end_reason: "empty",
  },
]);

const meta = {
  title: "Screens/Settings/VoiceHistory",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings/voice", signedIn: true, handlers: twoRooms } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Two rooms the user hosted, newest first. */
export const Normal: Story = {
  play: async (context) => {
    await settled(context);
    await expect(context.canvas.getByRole("heading", { level: 2, name: "Late show" })).toBeInTheDocument();
  },
};

/** The user has not hosted a room. */
export const Empty: Story = {
  ...answering(...hosted([])),
  play: async (context) => {
    await settled(context);
    await expect(context.canvas.getByRole("heading", { name: "You haven't hosted a room yet" })).toBeInTheDocument();
  },
};

/** The history has not arrived yet: the shape of its cards, without a spinner. */
export const Loading: Story = {
  ...answering(voiceOn, neverAnswering("/voice/history")),
  play: async (context) => {
    await showsSkeleton(context, "Loading the rooms");
  },
};

/** The server failed to answer the history: the error in its place, with a way to try again. */
export const ServerError: Story = {
  ...answering(voiceOn, failing("/voice/history")),
  play: async (context) => {
    await settled(context);
    await expect(await context.canvas.findByText("Couldn't load the rooms")).toBeInTheDocument();
  },
};

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** At 320 CSS px. */
export const Narrow: Story = narrow;
