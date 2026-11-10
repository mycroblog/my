// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the direct messages (web-client/README.md S1, ADR-0154 §6): the user's conversations,
// the one with the newest direct post first, a page at a time (GET /conversations), and marking one read. Each
// conversation of a page carries its participants, its newest post and whether it is unread, so the list and the
// navigation's unread count read nothing per conversation. Keys follow openapi-react-query's `[method, path, init]`.
import type { components } from "@mycroblog/mykit";
import { type InfiniteData, infiniteQueryOptions, type QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";

export type Conversation = components["schemas"]["Conversation"];
type ConversationPage = components["schemas"]["ConversationPage"];
export type ConversationAccount = components["schemas"]["Account"];
export type ConversationStatus = components["schemas"]["Status"];

/** The pages of the list as the cache holds them. */
type ConversationPages = InfiniteData<ConversationPage, string | undefined>;

/**
 * The user's conversations, a page at a time (GET /conversations): one request per page, keyset paged by the page's
 * `next_cursor`; a page without one is the last. The navigation's unread count reads the same query.
 */
export function conversationsQueryOptions(api: Api) {
  return infiniteQueryOptions({
    queryKey: ["get", "/conversations"] as const,
    queryFn: async ({ pageParam, signal }) => {
      const query = pageParam === undefined ? {} : { cursor: pageParam };
      const { data, error } = await api.fetch.GET("/conversations", { params: { query }, signal });
      // The middleware throws for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.next_cursor,
  });
}

/** Marks the conversation read up to its newest direct post (POST /conversations/{id}/read); returns it after that. */
export async function markConversationRead(api: Api, id: string): Promise<Conversation> {
  const { data, error } = await api.fetch.POST("/conversations/{id}/read", { params: { path: { id } } });
  if (data === undefined) throw new ApiError(error);
  return data;
}

/**
 * The pages with `conversation` as the server now shows it: in its place while its newest post is the same (it was
 * read), and otherwise first, since a newer post puts it at the top of the list (from wherever it was before).
 */
function withConversation(pages: ConversationPages, conversation: Conversation): ConversationPages {
  const inPlace = pages.pages.some((page) =>
    page.items.some((item) => item.id === conversation.id && item.last_status.id === conversation.last_status.id),
  );
  if (inPlace) {
    return {
      ...pages,
      pages: pages.pages.map((page) => ({
        ...page,
        items: page.items.map((item) => (item.id === conversation.id ? conversation : item)),
      })),
    };
  }
  return {
    ...pages,
    pages: pages.pages.map((page, index) => {
      const others = page.items.filter((item) => item.id !== conversation.id);
      return { ...page, items: index === 0 ? [conversation, ...others] : others };
    }),
  };
}

/**
 * Puts a conversation that the server answered or streamed into the list's cache (S1): the list and the navigation's
 * unread count show it at once, without reading the list again. Nothing is cached before the list's first page.
 */
export function cacheConversation(queryClient: QueryClient, api: Api, conversation: Conversation): void {
  queryClient.setQueryData<ConversationPages>(conversationsQueryOptions(api).queryKey, (pages) =>
    pages === undefined ? pages : withConversation(pages, conversation),
  );
}
