// SPDX-License-Identifier: AGPL-3.0-only
import { useNotice } from "@my/ui";
import { useQueryClient, useSuspenseInfiniteQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useT } from "../../../lib/i18n";
import type { AccountTimelineFilter } from "../api/timelines";
import { remoteFetchStarted, rereadRemoteAccountPosts, timelineQueryOptions } from "../api/timelines";

/** How long the server gets to store a remote account's recent posts before they are read again, in ms (ADR-0044). */
export const remoteFetchRereadDelay = 4000;

/**
 * Whether the server is fetching the recent posts of the remote account `accountId` from its server, as the
 * first page of its posts (`filter`) told (ADR-0044); the route loader has fetched that page. While it is,
 * the polite live region says so once (web-client/README.md Y7) and, `remoteFetchRereadDelay` ms later, the posts
 * are read again once (rereadRemoteAccountPosts), which ends it. The timer is the external system (R8):
 * leaving the profile before it fires cancels the read, and the next visit reads again.
 */
export function useRemoteFetch(accountId: string, filter: AccountTimelineFilter): boolean {
  const api = useApi();
  const queryClient = useQueryClient();
  const t = useT();
  const notice = useNotice();
  const { data } = useSuspenseInfiniteQuery(timelineQueryOptions(api, { kind: "account", id: accountId, filter }));
  const fetching = remoteFetchStarted(data);
  const message = t("client:profile.remote.fetching");

  useEffect(() => {
    if (!fetching) return;
    notice.inPlace(message, "polite");
    const timer = setTimeout(() => {
      void rereadRemoteAccountPosts(queryClient, api, { kind: "account", id: accountId, filter });
    }, remoteFetchRereadDelay);
    return () => {
      clearTimeout(timer);
    };
  }, [fetching, message, notice, queryClient, api, accountId, filter]);

  return fetching;
}
