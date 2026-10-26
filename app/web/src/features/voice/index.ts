// SPDX-License-Identifier: AGPL-3.0-only
// The voice chat feature (ADR-0100 §8, §9, §13): the list of the open rooms (`/voice`) with starting a room, the panel
// of the third column, the rooms of a hashtag, the page of a room before joining it (`/voice/{id}`) with its
// settings, the entry on a profile, and the history of the rooms the user hosted (`/settings/voice`). Nothing of it
// shows while the server has voice chat off. The call of a room goes on across pages: VoiceCallProvider (in the app's
// providers) holds it, and VoiceCallBar (at the end of every page) shows it. Routes and other features use it
// through this file only (web-client/README.md A3).
export { voiceHistoryQueryOptions, voiceRoomQueryOptions, voiceRoomSummaryQueryOptions, voiceRoomsQueryOptions } from "./api/voice";
export { ProfileVoiceRoom } from "./components/ProfileVoiceRoom";
export { VoiceCallBar } from "./components/VoiceCallBar";
export { VoiceCallProvider } from "./components/VoiceCallProvider";
export { VoiceHistoryView } from "./components/VoiceHistoryView";
export { speakerLabels, visibilityLabels } from "./components/VoiceRoomFormDialog";
export { TagVoiceRooms, VoiceRail, VoiceRoomList } from "./components/VoiceRooms";
export { VoiceRoomView } from "./components/VoiceRoomView";
export { voiceEndReasonLabel } from "./hooks/useVoiceHistory";
export { useVoiceAvailable } from "./hooks/useVoiceRooms";
