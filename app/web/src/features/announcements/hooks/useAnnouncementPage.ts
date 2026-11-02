// SPDX-License-Identifier: AGPL-3.0-only
// The page of one announcement (/announcements/$id, FR-726, web-client/screens/announcement.md): the announcement,
// opening it marks it read when it is unread and shows now (ADR-0125 §2), and "Mark as read" stays for when that failed.
// A 404, on opening it or on reading it again while it shows, says it is not there for the users any more.
import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { ApiError, errorMessage } from "../../../lib/errors";
import { type Announcement, announcementQueryOptions } from "../api/announcements";
import { useMarkAnnouncementRead } from "./useMarkAnnouncementRead";

export type AnnouncementPage =
  | { status: "loading" }
  | { status: "not-found" }
  | { status: "error"; reason: string; retry: () => void; isRetrying: boolean }
  | {
      status: "ready";
      announcement: Announcement;
      /** Marks the announcement read; `onRead` runs once the cache has it read. */
      markRead: (onRead: () => void) => void;
      isMarking: boolean;
      /** Why marking it read failed, localized. */
      markError: string | undefined;
    };

export function useAnnouncementPage(id: string): AnnouncementPage {
  const api = useApi();
  // The loader has asked already: a refusal it got (a withdrawn or unknown announcement) is not asked again on mount.
  const query = useQuery({ ...announcementQueryOptions(api, id), retry: false, retryOnMount: false });
  const mark = useMarkAnnouncementRead();
  // A refetch that finds it gone keeps its last data in the cache; the 404 wins.
  const notFound = query.error instanceof ApiError && query.error.status === 404;
  const announcement = notFound ? undefined : query.data;
  // Opening an announcement marks it read, once. The route gives each announcement a component of its own.
  const openedRef = useRef(false);
  const { mutate } = mark;
  useEffect(() => {
    if (announcement === undefined || openedRef.current) return;
    openedRef.current = true;
    if (!announcement.read && !announcement.ended) mutate(announcement.id);
  }, [announcement, mutate]);
  if (notFound) return { status: "not-found" };
  if (announcement !== undefined) {
    return {
      status: "ready",
      announcement,
      markRead: (onRead) => {
        mark.mutate(announcement.id, { onSuccess: onRead });
      },
      isMarking: mark.markingIds.includes(announcement.id),
      markError: mark.error,
    };
  }
  if (query.error !== null) {
    return {
      status: "error",
      reason: errorMessage(query.error),
      retry: () => void query.refetch(),
      isRetrying: query.isFetching,
    };
  }
  return { status: "loading" };
}
