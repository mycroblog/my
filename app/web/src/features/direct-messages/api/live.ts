// SPDX-License-Identifier: AGPL-3.0-only
// What the streaming API's `conversation` events change in the cache (web-client/README.md S10): the conversation as the
// server now shows it, written into the list that the navigation's unread count and the direct messages page read. A
// burst of them reads nothing. A list with a read on its way, which may have missed the event, is read again instead,
// and so is everything after the stream reconnected, since the server replays nothing (client-api.md §5).
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { type Conversation, cacheConversation, conversationsQueryOptions } from "./conversations";

/** A conversation from the stream: a direct post arrived in it, or the user read it on a device. */
export function receiveConversation(queryClient: QueryClient, api: Api, conversation: Conversation): void {
  const { queryKey } = conversationsQueryOptions(api);
  const state = queryClient.getQueryState(queryKey);
  // Nothing reads the list yet (signed in a moment ago): its first read has it.
  if (state === undefined) return;
  if (state.data === undefined || state.fetchStatus === "fetching") {
    void queryClient.invalidateQueries({ queryKey, exact: true });
    return;
  }
  cacheConversation(queryClient, api, conversation);
}

/** After the stream reconnected: reads the list again, which the navigation keeps on screen for its count. */
export function catchUpConversations(queryClient: QueryClient, api: Api): void {
  void queryClient.invalidateQueries({ queryKey: conversationsQueryOptions(api).queryKey, exact: true });
}
