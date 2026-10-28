// SPDX-License-Identifier: AGPL-3.0-only
// What the page of a room (/voice/{id}, ADR-0100 §9) shows. A signed-in user who sees the open room gets the room:
// its settings and its participants, with the call (useVoiceCall). Anyone else gets the room's summary: a visitor who is not signed in learns that only the
// users of this server join, with the way to sign in, and the summary's name, description, and host only when the
// server shows them (回答 30); a room that ended says so with its name when the summary has it (回答 32). Nothing
// says which of an unknown room, a room of a server with voice chat off, or a room hidden from the user it is,
// beyond what the summary tells.
import { useQuery } from "@tanstack/react-query";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { useViewer } from "../../session";
import {
  type VoiceRoom,
  type VoiceRoomSummary,
  voiceRoomQueryOptions,
  voiceRoomSummaryQueryOptions,
} from "../api/voice";

/** The page of a room as it shows now. */
export type VoiceRoomPage =
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  /** No such room, or voice chat is off. */
  | { status: "not-found" }
  /** The room ended; its name only when the summary shows it. */
  | { status: "ended"; name: string | undefined }
  /** An open room shown by its summary: to a visitor who is not signed in (`signedIn` false), or to a user who does not see it. */
  | { status: "summary"; summary: VoiceRoomSummary; signedIn: boolean }
  | {
      status: "room";
      room: VoiceRoom;
      /** Whether the user hosts the room, who changes its settings. */
      isHost: boolean;
    };

function isNotFound(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404;
}

export function useVoiceRoomPage(id: string): VoiceRoomPage {
  const api = useApi();
  const session = useSessionStatus();
  const viewer = useViewer();
  const signedIn = session === "signed-in";
  // The loader has asked already: a refusal it got (an ended or hidden room) is not asked again on mount.
  const room = useQuery({ ...voiceRoomQueryOptions(api, id), enabled: signedIn, retry: false, retryOnMount: false });
  // The summary is read when the room is not there for the user: signed out, or a 404 of the room.
  const needsSummary = session === "signed-out" || (signedIn && isNotFound(room.error));
  const summary = useQuery({
    ...voiceRoomSummaryQueryOptions(api, id),
    enabled: needsSummary,
    retry: false,
    retryOnMount: false,
  });
  if (session === "unknown") return { status: "loading" };
  // A refetch that finds the room gone (it ended while the page showed it) keeps the room's last data in the cache; the
  // 404 wins, so that the page says the room ended instead of offering to join it.
  if (signedIn && room.data !== undefined && !isNotFound(room.error)) {
    const viewerId = viewer.status === "signed-in" ? viewer.account.id : undefined;
    return {
      status: "room",
      room: room.data,
      isHost: room.data.host.id === viewerId,
    };
  }
  const failed =
    signedIn && room.error !== null && !isNotFound(room.error)
      ? room
      : summary.error !== null && !isNotFound(summary.error)
        ? summary
        : undefined;
  if (failed !== undefined) {
    return {
      status: "error",
      reason: errorMessage(failed.error),
      retry: () => void failed.refetch(),
      isRetrying: failed.isFetching,
    };
  }
  if (isNotFound(summary.error)) return { status: "not-found" };
  if (summary.data !== undefined) {
    if (summary.data.state === "ended") return { status: "ended", name: summary.data.name };
    return { status: "summary", summary: summary.data, signedIn };
  }
  return { status: "loading" };
}
