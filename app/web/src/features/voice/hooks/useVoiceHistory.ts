// SPDX-License-Identifier: AGPL-3.0-only
// The rooms the user hosted (ADR-0100 §13, 回答 32), for the settings page; and the open room of an account's profile
// where it hosts or speaks, which only a user who sees the room gets (§8, 回答 33: being a listener is never shown).
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useIntl } from "react-intl";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { type ClientMessageKey } from "../../../lib/i18n";
import { shellQueryRetry } from "../../../lib/query";
import {
  accountVoiceRoomQueryOptions,
  type VoiceRoomEntry,
  type VoiceRoomHistoryEntry,
  voiceHistoryQueryOptions,
} from "../api/voice";
import { useVoiceAvailable } from "./useVoiceRooms";

/** The sentence of each reason a room ended (api/spec VoiceRoomHistoryEntry); another reason reads as `other`. */
const endReasons: Readonly<Record<string, ClientMessageKey>> = {
  host: "client:voice.end_reason.host",
  empty: "client:voice.end_reason.empty",
  host_disconnected: "client:voice.end_reason.host_disconnected",
  host_ineligible: "client:voice.end_reason.host_ineligible",
  moderator: "client:voice.end_reason.moderator",
  time_limit: "client:voice.end_reason.time_limit",
  disabled: "client:voice.end_reason.disabled",
};

/** The sentence of why a room ended, `other` for a reason the app does not know. */
export function voiceEndReasonLabel(reason: string | undefined): ClientMessageKey {
  return endReasons[reason ?? ""] ?? "client:voice.end_reason.other";
}

/** A date and time as the page shows it: the machine-readable value and the text in the user's locale. */
export type DateView = { dateTime: string; text: string };

/** A room of the history as its row shows it. */
export type VoiceHistoryRow = {
  id: string;
  name: string;
  opened: DateView;
  /** When it ended, or undefined while it is open. */
  ended: DateView | undefined;
  /** Why it ended, or undefined while it is open. */
  reason: ClientMessageKey | undefined;
};

export type VoiceHistory =
  | { status: "unavailable" }
  | { status: "loading" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | {
      status: "ready";
      rows: readonly VoiceHistoryRow[];
      hasMore: boolean;
      loadMore: () => void;
      isLoadingMore: boolean;
      moreError: string | undefined;
    };

export function useVoiceHistory(): VoiceHistory {
  const api = useApi();
  const intl = useIntl();
  const query = useInfiniteQuery({ ...voiceHistoryQueryOptions(api), retry: shellQueryRetry, retryOnMount: false });
  const dateOf = (value: string): DateView => ({
    dateTime: value,
    text: intl.formatDate(value, { dateStyle: "medium", timeStyle: "short" }),
  });
  const rowOf = (entry: VoiceRoomHistoryEntry): VoiceHistoryRow => ({
    id: entry.id,
    name: entry.name,
    opened: dateOf(entry.opened_at),
    ended: entry.ended_at === undefined ? undefined : dateOf(entry.ended_at),
    reason: entry.ended_at === undefined ? undefined : voiceEndReasonLabel(entry.end_reason),
  });
  if (query.data !== undefined) {
    return {
      status: "ready",
      rows: query.data.pages.flatMap((page) => page.items).map(rowOf),
      hasMore: query.hasNextPage,
      loadMore: () => {
        if (!query.isFetching) void query.fetchNextPage();
      },
      isLoadingMore: query.isFetchingNextPage,
      moreError: query.isFetchNextPageError ? errorMessage(query.error) : undefined,
    };
  }
  if (query.error !== null) {
    if (query.error instanceof ApiError && query.error.status === 404) return { status: "unavailable" };
    return {
      status: "error",
      reason: errorMessage(query.error),
      retry: () => void query.refetch(),
      isRetrying: query.isFetching,
    };
  }
  return { status: "loading" };
}

/**
 * The open room where the account `accountId` hosts or speaks, when the user sees it (GET /accounts/{id}); undefined
 * while voice chat is off, while it loads, and when there is none. A failure shows nothing: the profile goes on.
 */
export function useProfileVoiceRoom(accountId: string): VoiceRoomEntry | undefined {
  const api = useApi();
  const available = useVoiceAvailable();
  const query = useQuery({
    ...accountVoiceRoomQueryOptions(api, accountId),
    enabled: available,
    retry: shellQueryRetry,
    retryOnMount: false,
  });
  return available ? query.data?.voice_room : undefined;
}
