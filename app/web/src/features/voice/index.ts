// SPDX-License-Identifier: AGPL-3.0-only
// The voice chat feature (ADR-0100 §8, §9, §13): the list of the open rooms (`/voice`) with starting a room, the panel
// of the third column, the rooms of a hashtag, the page of a room before joining it (`/voice/{id}`) with its
// settings, the entry on a profile, and the history of the rooms the user hosted (`/settings/voice`). Nothing of it
// shows while the server has voice chat off. The audio of a call is not here. Routes and other features use it
// through this file only (web-client/README.md A3).
export { voiceHistoryQueryOptions, voiceRoomQueryOptions, voiceRoomSummaryQueryOptions, voiceRoomsQueryOptions } from "./api/voice";
export { ProfileVoiceRoom } from "./components/ProfileVoiceRoom";
export { VoiceHistoryView } from "./components/VoiceHistoryView";
export { TagVoiceRooms, VoiceRail, VoiceRoomList } from "./components/VoiceRooms";
export { VoiceRoomView } from "./components/VoiceRoomView";
export { useVoiceAvailable } from "./hooks/useVoiceRooms";
