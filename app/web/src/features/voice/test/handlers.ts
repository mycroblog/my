// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the voice feature (web-client/README.md §7): the server has voice chat off, so every
// voice operation that the app calls answers 404 and nothing of voice chat shows until a test turns it on. The
// fixtures build the rooms that a test serves.
import { http } from "msw";
import { otherAccountFixture, type Schemas, viewerFixture } from "../../../test/fixtures";
import { apiBase, problem } from "../../../test/msw/http";

/** The ID of the room of the fixtures. */
export const roomId = "0192f0c4-7a8e-7000-8000-0000000000f1";

/** A participant of a room: `account` as a listener, with `overrides`. */
export function participantFixture(account: Schemas["Account"], overrides: Partial<Schemas["VoiceRoomParticipant"]> = {}): Schemas["VoiceRoomParticipant"] {
  return { account, role: "listener", host: false, cohost: false, hand_raised: false, blocked: false, ...overrides };
}

/** Bob's open room for every local user, with Bob as its host and only speaker, with `overrides`. */
export function voiceRoomFixture(overrides: Partial<Schemas["VoiceRoom"]> = {}): Schemas["VoiceRoom"] {
  const participants = overrides.participants ?? [participantFixture(otherAccountFixture, { role: "speaker", host: true })];
  return {
    id: roomId,
    name: "Friday night radio",
    description: "Talking about the week's music.",
    hashtags: ["music"],
    visibility: "local",
    speakers: "manual",
    host: otherAccountFixture,
    opened_at: "2026-10-26T12:00:00Z",
    participants_count: participants.length,
    participants,
    ...overrides,
  };
}

/** Alice's (the viewer's) open room, with her as its only participant. */
export function ownRoomFixture(overrides: Partial<Schemas["VoiceRoom"]> = {}): Schemas["VoiceRoom"] {
  return voiceRoomFixture({ host: viewerFixture, participants: [participantFixture(viewerFixture, { role: "speaker", host: true })], ...overrides });
}

const voiceOff = () => problem(404, "about:blank", "Not Found");

export const handlers = [
  http.get(`${apiBase}/voice/rooms`, voiceOff),
  http.post(`${apiBase}/voice/rooms`, voiceOff),
  http.get(`${apiBase}/voice/rooms/:id`, voiceOff),
  http.patch(`${apiBase}/voice/rooms/:id`, voiceOff),
  http.get(`${apiBase}/voice/rooms/:id/summary`, voiceOff),
  http.post(`${apiBase}/voice/rooms/:id/participation`, voiceOff),
  http.delete(`${apiBase}/voice/rooms/:id/participation`, voiceOff),
  http.post(`${apiBase}/voice/rooms/:id/heartbeat`, voiceOff),
  http.get(`${apiBase}/voice/rooms/:id/stream`, voiceOff),
  http.post(`${apiBase}/voice/rooms/:id/signals`, voiceOff),
  http.post(`${apiBase}/voice/rooms/:id/mic`, voiceOff),
  http.post(`${apiBase}/voice/rooms/:id/grant`, voiceOff),
  http.post(`${apiBase}/voice/rooms/:id/reactions`, voiceOff),
  http.post(`${apiBase}/voice/rooms/:id/speaker`, voiceOff),
  http.delete(`${apiBase}/voice/rooms/:id/speaker`, voiceOff),
  http.post(`${apiBase}/voice/rooms/:id/hand`, voiceOff),
  http.delete(`${apiBase}/voice/rooms/:id/hand`, voiceOff),
  http.post(`${apiBase}/voice/rooms/:id/host`, voiceOff),
  http.post(`${apiBase}/voice/rooms/:id/invitations`, voiceOff),
  http.delete(`${apiBase}/voice/rooms/:id/participants/:accountId`, voiceOff),
  http.post(`${apiBase}/voice/rooms/:id/participants/:accountId/grant`, voiceOff),
  http.delete(`${apiBase}/voice/rooms/:id/participants/:accountId/grant`, voiceOff),
  http.delete(`${apiBase}/voice/rooms/:id/participants/:accountId/hand`, voiceOff),
  http.post(`${apiBase}/voice/rooms/:id/participants/:accountId/mute`, voiceOff),
  http.post(`${apiBase}/voice/rooms/:id/participants/:accountId/cohost`, voiceOff),
  http.delete(`${apiBase}/voice/rooms/:id/participants/:accountId/cohost`, voiceOff),
  http.get(`${apiBase}/voice/history`, voiceOff),
];
