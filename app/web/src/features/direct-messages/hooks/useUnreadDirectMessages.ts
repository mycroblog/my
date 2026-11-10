// SPDX-License-Identifier: AGPL-3.0-only
import { useInfiniteQuery } from "@tanstack/react-query";
import { useApi, useSessionStatus } from "../../../lib/auth/SessionProvider";
import { shellQueryRetry } from "../../../lib/query";
import { conversationsQueryOptions } from "../api/conversations";

/**
 * The number of unread conversations among those the list has loaded: its first page from any screen, since the
 * navigation reads it, and the next ones as the direct messages page loads them. The API has no count of its own, and
 * a conversation turns unread only when a newer post arrives in it, which puts it at the top. 0 while signed out, while
 * the list loads, and when it cannot be loaded (the badge then stays away rather than showing a guess); a count loaded
 * before stays while a refetch fails. The stream keeps it up to date (../api/live.ts).
 */
export function useUnreadDirectMessageCount(): number {
  const api = useApi();
  const status = useSessionStatus();
  const list = useInfiniteQuery({
    ...conversationsQueryOptions(api),
    enabled: status === "signed-in",
    retry: shellQueryRetry,
  });
  if (status !== "signed-in" || list.data === undefined) return 0;
  return list.data.pages.reduce((sum, page) => sum + page.items.filter((item) => item.unread).length, 0);
}
