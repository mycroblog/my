// SPDX-License-Identifier: AGPL-3.0-only
// The direct messages by conversation (FR-212, ADR-0154 §6): the reader's conversations, each by its latest direct post
// that they may see, newest first, with the other accounts and whether it is unread; one conversation and its posts;
// and the writes of the reader, reading one and hiding one, which the server shares across their devices.
//
// The stream (FR-207) sends a conversation again whenever a direct post arrives in it or the reader reads it on a
// device: it takes the place of the cached one, at the top of the list when its latest post is newer, and a hidden one
// comes back with its new post. A conversation the reader hides leaves the list at once, and comes back if the server
// refuses. The posts of a conversation are its thread around its latest post, its `direct` posts alone, oldest first.
import { ProblemError, type components } from "@mycroblog/mykit";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useMemo, useSyncExternalStore } from "react";
import type { AppData } from "./app-data";
import { pagedQuery, usePagedList, type PagedList, type Pages } from "./paged";
import { useAppData } from "./provider";
import { showList, useShadow, type ShowStatus } from "./shadow";
import type { Status } from "./statuses";
import { statusContextQuery, type StatusContext } from "./thread";

/** A conversation of the reader's direct messages: its other accounts, its latest post, and whether it is unread. */
export type Conversation = components["schemas"]["Conversation"];

/** The reader's conversations, the one with the newest direct post first. */
export function conversationsQuery({ api, queries }: AppData) {
  return pagedQuery<Conversation>(queries.queryOptions("get", "/conversations").queryKey, async (cursor, signal) => {
    const { data } = await api.GET("/conversations", {
      params: { query: cursor === undefined ? {} : { cursor } },
      signal,
    });
    return data;
  });
}

/** The reader's conversation of `id`. */
export function conversationQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/conversations/{id}", { params: { path: { id } } });
}

// ---- The conversations being hidden --------------------------------------------------------------------------------

/** The conversations being hidden, each by the ID of the latest post it was hidden at. */
type Hiding = ReadonlyMap<string, string>;

type Store = {
  hiding: Hiding;
  /** The conversations the server hid, each by the ID of the latest post it was hidden at, until a newer one arrives. */
  readonly hidden: Map<string, string>;
  readonly listeners: Set<() => void>;
};

const stores = new WeakMap<AppData, Store>();

function storeOf(data: AppData): Store {
  let store = stores.get(data);
  if (!store) {
    store = { hiding: new Map(), hidden: new Map(), listeners: new Set() };
    stores.set(data, store);
  }
  return store;
}

function setHiding(store: Store, hiding: Hiding): void {
  store.hiding = hiding;
  for (const listener of store.listeners) listener();
}

/** Whether `c` is being hidden as it is: a newer post arriving meanwhile brings it back. */
const isHiding = (hiding: Hiding, c: Conversation) => hiding.get(c.id) === c.last_status.id;

function useHiding(data: AppData): Hiding {
  const store = storeOf(data);
  return useSyncExternalStore(
    (listener) => {
      store.listeners.add(listener);
      return () => {
        store.listeners.delete(listener);
      };
    },
    () => store.hiding,
    () => store.hiding,
  );
}

// ---- The cache -----------------------------------------------------------------------------------------------------

/** IDs are UUIDv7: in their canonical form, a later one sorts after an earlier one. */
const newer = (a: string, b: string) => a > b;

/** The cached conversation of `id`: alone, or in the list. */
function cachedConversation(data: AppData, id: string): Conversation | undefined {
  const alone = data.queryClient.getQueryData<Conversation>(conversationQuery(data, id).queryKey);
  if (alone !== undefined) return alone;
  const list = data.queryClient.getQueryData<Pages<Conversation>>(conversationsQuery(data).queryKey);
  return list?.pages.flatMap((page) => page.items).find((c) => c.id === id);
}

/**
 * Puts a conversation the server answered into the cache, in place of the cached one. An answer whose latest post is
 * older than the cached one's is out of date (a read answered after a newer post arrived) and changes nothing. One with
 * a newer latest post goes to the top of the list, as does one the cache does not know (a new one); one with the same
 * latest post (a read) changes it where it is, and leaves out of the list one that is not in it (hidden).
 */
function putConversation(data: AppData, conversation: Conversation): void {
  const { queryClient } = data;
  const before = cachedConversation(data, conversation.id);
  if (before !== undefined && newer(before.last_status.id, conversation.last_status.id)) return;
  // One the reader hid comes back with a newer post alone.
  const { hidden } = storeOf(data);
  const hiddenAt = hidden.get(conversation.id);
  if (hiddenAt !== undefined && newer(conversation.last_status.id, hiddenAt)) hidden.delete(conversation.id);
  const stillHidden = hidden.has(conversation.id);
  const arrived = !stillHidden && (before === undefined || newer(conversation.last_status.id, before.last_status.id));
  if (before !== undefined) followThread(data, before.last_status, conversation.last_status);
  queryClient.setQueryData(conversationQuery(data, conversation.id).queryKey, conversation);
  listConversation(data, conversation, arrived);
}

/**
 * Puts a conversation into the cached list, at its top when it `arrived`. While the list is being read, the answer may
 * or may not hold it as it is now (a read answered after the list was asked): a list that holds an answer is read again
 * instead. Its first read is not asked again before it has answered, so the conversation waits for that answer and
 * goes into it (`waited`); a first read that failed leaves no list to put it in.
 */
function listConversation(data: AppData, conversation: Conversation, arrived: boolean, waited = false): void {
  const { queryClient } = data;
  const { queryKey } = conversationsQuery(data);
  const list = queryClient.getQueryCache().find({ queryKey, exact: true });
  if (list?.state.fetchStatus === "fetching") {
    if (waited || list.state.data !== undefined || list.promise === undefined) {
      void queryClient.invalidateQueries({ queryKey, exact: true });
      return;
    }
    const after = () => {
      listConversation(data, conversation, arrived, true);
    };
    list.promise.then(after, after);
    return;
  }
  queryClient.setQueryData<Pages<Conversation>>(queryKey, (cached) => {
    const [first, ...rest] = cached?.pages ?? [];
    if (!cached || !first) return cached;
    const held = cached.pages.flatMap((page) => page.items).find((c) => c.id === conversation.id);
    if (!arrived && held === undefined) return cached;
    if (held !== undefined && newer(held.last_status.id, conversation.last_status.id)) return cached;
    if (held !== undefined && !newer(conversation.last_status.id, held.last_status.id)) {
      const pages = cached.pages.map((page) => ({
        ...page,
        items: page.items.map((c) => (c.id === conversation.id ? conversation : c)),
      }));
      return { ...cached, pages };
    }
    const without = (items: readonly Conversation[]) => items.filter((c) => c.id !== conversation.id);
    return {
      ...cached,
      pages: [
        { ...first, items: [conversation, ...without(first.items)] },
        ...rest.map((p) => ({ ...p, items: without(p.items) })),
      ],
    };
  });
}

/**
 * Gives a conversation's new latest post that replies to its latest one the thread that one had, and itself under it,
 * so that its posts show at once, without a read of its thread.
 */
function followThread(data: AppData, last: Status, next: Status): void {
  if (next.id === last.id || next.in_reply_to_id !== last.id) return;
  const { queryClient } = data;
  const known = queryClient.getQueryData<StatusContext>(statusContextQuery(data, last.id).queryKey);
  const key = statusContextQuery(data, next.id).queryKey;
  if (known === undefined || queryClient.getQueryData(key) !== undefined) return;
  queryClient.setQueryData(key, { ancestors: [...known.ancestors, last], descendants: [] });
}

/**
 * Takes the conversation of `id` out of the cached list while its latest post is still `last`: one a newer post reached
 * since stays.
 */
function dropConversation(data: AppData, id: string, last: string): void {
  const gone = (c: Conversation) => c.id === id && c.last_status.id === last;
  data.queryClient.setQueryData<Pages<Conversation>>(conversationsQuery(data).queryKey, (cached) =>
    cached === undefined
      ? cached
      : { ...cached, pages: cached.pages.map((page) => ({ ...page, items: page.items.filter((c) => !gone(c)) })) },
  );
}

// ---- The stream ----------------------------------------------------------------------------------------------------

/**
 * Puts a conversation that the stream received into the cache: a direct post arrived in it (it goes to the top of the
 * list, and one hidden comes back), or the reader read it on a device.
 */
export function receiveConversation(data: AppData, conversation: Conversation): void {
  putConversation(data, conversation);
}

/** Reads the conversations again: direct posts may have arrived while the stream was closed. */
export function receiveConversationsGap(data: AppData): void {
  void data.queryClient.invalidateQueries({ queryKey: conversationsQuery(data).queryKey, exact: true });
  void data.queryClient.invalidateQueries({ queryKey: ["get", "/conversations/{id}"] });
}

/**
 * Writes what a direct post of the reader's makes certain: the conversation of the post it replies to, while that is
 * the conversation's latest, has it as its latest post now, at the top of the list. Whether it is unread stays as the
 * server said: the reader's own post moves no read point.
 */
export function cacheConversationPost(data: AppData, status: Status): void {
  const parent = status.in_reply_to_id;
  if (status.visibility !== "direct" || parent === undefined) return;
  const ids = new Set<string>();
  const list = data.queryClient.getQueryData<Pages<Conversation>>(conversationsQuery(data).queryKey);
  for (const c of list?.pages.flatMap((page) => page.items) ?? []) if (c.last_status.id === parent) ids.add(c.id);
  for (const [, c] of data.queryClient.getQueriesData<Conversation>({ queryKey: ["get", "/conversations/{id}"] }))
    if (c?.last_status.id === parent) ids.add(c.id);
  for (const id of ids) {
    const cached = cachedConversation(data, id);
    if (cached !== undefined) putConversation(data, { ...cached, last_status: status });
  }
}

// ---- The writes ----------------------------------------------------------------------------------------------------

/**
 * Marks the conversation of `id` read on every device of the reader: it shows read at once, and the cache holds what
 * the server answered. It rejects with the server's problem, the conversation showing again as it was.
 */
export async function markConversationRead(data: AppData, id: string): Promise<void> {
  const before = cachedConversation(data, id);
  if (before?.unread === true) putConversation(data, { ...before, unread: false });
  try {
    const { data: answer } = await data.api.POST("/conversations/{id}/read", { params: { path: { id } } });
    if (answer !== undefined) putConversation(data, answer);
  } catch (e) {
    const now = cachedConversation(data, id);
    if (before?.unread === true && now !== undefined && now.last_status.id === before.last_status.id)
      putConversation(data, { ...now, unread: true });
    throw e;
  }
}

/**
 * Hides the conversation of `id` on every device of the reader, until a new post arrives in it: it leaves the list at
 * once, and comes back if the server refuses, with the server's problem, or when a newer post arrives, even while the
 * hide is in flight.
 */
export async function hideConversation(data: AppData, id: string): Promise<void> {
  const store = storeOf(data);
  const last = cachedConversation(data, id)?.last_status.id;
  if (last === undefined) return;
  setHiding(store, new Map([...store.hiding, [id, last]]));
  try {
    await data.api.POST("/conversations/{id}/hide", { params: { path: { id } } });
    // Hidden as it was: an event with no newer post (a read on a device) leaves it out of the list.
    if (cachedConversation(data, id)?.last_status.id === last) store.hidden.set(id, last);
    dropConversation(data, id, last);
  } finally {
    const hiding = new Map(store.hiding);
    hiding.delete(id);
    setHiding(store, hiding);
  }
}

// ---- The hooks -----------------------------------------------------------------------------------------------------

/** Each conversation with its latest post as it shows, one kept as it is while that does not change. */
const shownConversations = new WeakMap<Conversation, { status: Status; shown: Conversation }>();

/** The conversation with its latest post as it shows (shadow.ts); undefined once that post is gone. */
function showConversation(show: ShowStatus, c: Conversation): Conversation | undefined {
  const status = show(c.last_status);
  if (status === undefined) return undefined;
  if (status === c.last_status) return c;
  const memo = shownConversations.get(c);
  if (memo?.status === status) return memo.shown;
  const shown = { ...c, last_status: status };
  shownConversations.set(c, { status, shown });
  return shown;
}

/**
 * The reader's conversations, for the list, each with its latest post as it shows: one being hidden leaves it at once,
 * and one whose latest post is gone leaves it until the list is read again.
 */
export function useConversations(enabled = true): PagedList<Conversation> {
  const data = useAppData();
  const list = usePagedList(conversationsQuery(data), enabled);
  const hiding = useHiding(data);
  const show = useShadow();
  const items = list.items.flatMap((c) => {
    if (isHiding(hiding, c)) return [];
    const shown = showConversation(show, c);
    return shown === undefined ? [] : [shown];
  });
  return { ...list, items };
}

/**
 * How many of the reader's conversations are unread, among those the list has read (its first page, unless the reader
 * read further), for the navigation; undefined until the server answered, and without a session (`signedIn` false).
 */
export function useUnreadConversations(signedIn = true): number | undefined {
  const { items, status } = useConversations(signedIn);
  if (status !== "success") return undefined;
  return items.filter((c) => c.unread).length;
}

/** What a conversation's screen shows of it: the conversation once read, why it could not be, and reading it again. */
export type OneConversation = {
  /** The conversation, with its latest post as it shows; undefined until the server answered with it. */
  readonly conversation: Conversation | undefined;
  /** `pending` until the server answered, `gone` when the reader has no such conversation, `error` when it failed. */
  readonly status: "pending" | "gone" | "error" | "success";
  /** Reads the conversation again: the reader's try again after it failed. */
  readonly refetch: () => void;
};

/** The reader's conversation of `id`, at once from the list when it holds it. */
export function useConversation(id: string): OneConversation {
  const data = useAppData();
  const show = useShadow();
  const query = useQuery({
    ...conversationQuery(data, id),
    initialData: () => cachedConversation(data, id),
  });
  const { error } = query;
  return {
    conversation: query.data && (showConversation(show, query.data) ?? query.data),
    status:
      query.status === "error"
        ? error instanceof ProblemError && error.status === 404
          ? "gone"
          : "error"
        : query.status,
    refetch: () => {
      void query.refetch();
    },
  };
}

/** What a conversation's screen shows of its posts. */
export type ConversationPosts = {
  /** Its direct posts as they show, oldest first; empty until the server answered. */
  readonly posts: readonly Status[];
  /** `pending` until the server answered, `error` when it failed, then `success`. */
  readonly status: "pending" | "error" | "success";
  /** Reads the posts again, after they failed. */
  readonly refetch: () => void;
};

const byTime = (a: Status, b: Status) =>
  Date.parse(a.created_at) - Date.parse(b.created_at) || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0);

/**
 * The posts of a conversation: the thread around its latest post, its direct posts alone, oldest first, as they show
 * (shadow.ts). A newer latest post keeps the posts shown until its thread arrives.
 */
export function useConversationPosts(conversation: Conversation | undefined): ConversationPosts {
  const data = useAppData();
  const show = useShadow();
  const last = conversation?.last_status;
  const query = useQuery({
    ...statusContextQuery(data, last?.id ?? ""),
    enabled: last !== undefined,
    placeholderData: keepPreviousData,
  });
  const context = query.data;
  const all = context === undefined || last === undefined ? [] : [...context.ancestors, last, ...context.descendants];
  const seen = new Set<string>();
  const direct = all.filter((s) => {
    if (s.visibility !== "direct" || seen.has(s.id)) return false;
    seen.add(s.id);
    return true;
  });
  return {
    posts: showList(show, direct.toSorted(byTime)),
    status: last === undefined ? "pending" : query.status,
    refetch: () => {
      void query.refetch();
    },
  };
}

/** The reader's writes on their conversations, from a screen. */
export type ConversationWrites = {
  /** Marks the conversation of `id` read; it rejects with the server's problem. */
  readonly read: (id: string) => Promise<void>;
  /** Hides the conversation of `id` until a new post arrives in it; it rejects with the server's problem. */
  readonly hide: (id: string) => Promise<void>;
};

/**
 * The reader's writes on their conversations, for the session of the nearest AppDataProvider: the same functions while
 * the session is, so that an effect may depend on them.
 */
export function useConversationWrites(): ConversationWrites {
  const data = useAppData();
  return useMemo(
    () => ({
      read: (id: string) => markConversationRead(data, id),
      hide: (id: string) => hideConversation(data, id),
    }),
    [data],
  );
}
