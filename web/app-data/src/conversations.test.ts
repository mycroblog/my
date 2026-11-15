// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { InfiniteQueryObserver } from "@tanstack/react-query";
import type { StreamEvent } from "@mycroblog/mykit";
import { createElement, useState } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import type { AppData } from "./app-data";
import { postStatus } from "./compose";
import {
  conversationQuery,
  conversationsQuery,
  hideConversation,
  markConversationRead,
  receiveConversation,
  useConversation,
  useConversationPosts,
  useConversations,
  useConversationWrites,
  useUnreadConversations,
  type Conversation,
  type ConversationWrites,
} from "./conversations";
import { AppDataProvider } from "./provider";
import type { Status } from "./statuses";
import { startStream } from "./stream";
import { fakeAppData, json, status } from "./test/fake";
import { probe } from "./test/probe";
import { fakeBrowser, fakeStreams } from "./test/tabs";
import { statusContextQuery } from "./thread";

/** A direct post of `id`, made at the minute `minute` of the day. */
const direct = (id: string, minute: number, fields: Partial<Status> = {}): Status =>
  status(id, {
    visibility: "direct",
    created_at: `2026-11-15T10:${String(minute).padStart(2, "0")}:00Z`,
    ...fields,
  });

/** The conversation `id` whose latest post is `last`, with Sam, unread or read. */
const conversation = (id: string, last: Status, unread: boolean): Conversation =>
  ({ id, accounts: [{ id: "sam", acct: "sam" }], last_status: last, unread }) as Conversation;

const problem = (code: number) => json(code, { type: "about:blank", title: "Refused", status: code });

/**
 * AppData whose server answers the conversations with c3 (unread) and c2 (read) on the first page and c1 (unread) on
 * the next, and every write with `write()`; both pages are read.
 */
async function reader(write: (request: Request) => Response | Promise<Response> = () => json(200, {})) {
  const pages: Record<string, unknown> = {
    first: {
      items: [conversation("c3", direct("s3", 3), true), conversation("c2", direct("s2", 2), false)],
      next_cursor: "k2",
    },
    k2: { items: [conversation("c1", direct("s1", 1), true)] },
  };
  const fake = fakeAppData((request) => {
    if (request.method !== "GET") return write(request);
    const url = new URL(request.url);
    if (url.pathname === "/api/v1/conversations") return json(200, pages[url.searchParams.get("cursor") ?? "first"]);
    return problem(404);
  });
  const data: AppData = { ...fake.data, connection: { server: "https://example.com" } };
  await data.queryClient.infiniteQuery({ ...conversationsQuery(data), staleTime: Infinity });
  await new InfiniteQueryObserver(data.queryClient, conversationsQuery(data)).fetchNextPage();
  return { data, requests: fake.requests };
}

const listed = (data: AppData) => probe(data, () => useConversations()).items.map((c) => c.id);
const unread = (data: AppData) =>
  probe(data, () => useConversations())
    .items.filter((c) => c.unread)
    .map((c) => c.id);

test("the list is the server's conversations, page after page, the one with the newest post first", async () => {
  const { data, requests } = await reader();
  expect(listed(data)).toEqual(["c3", "c2", "c1"]);
  expect(requests.map((u) => u.pathname + u.search)).toEqual([
    "/api/v1/conversations",
    "/api/v1/conversations?cursor=k2",
  ]);
});

test("the list says which conversations are unread, and the navigation counts them", async () => {
  const { data } = await reader();
  expect(unread(data)).toEqual(["c3", "c1"]);
  expect(probe(data, () => useUnreadConversations())).toBe(2);
});

test("the navigation has no count before the list is read, and without a session it reads nothing", () => {
  const { data, requests } = fakeAppData(() => json(200, { items: [] }));
  expect(probe(data, () => useUnreadConversations())).toBeUndefined();
  expect(probe(data, () => useUnreadConversations(false))).toBeUndefined();
  expect(requests.map((u) => u.pathname)).not.toContain("/api/v1/conversations");
});

test("reading a conversation marks it read on the server, at once in the list, and the count drops", async () => {
  const { promise: answered, resolve: answer } = Promise.withResolvers<Response>();
  const sent: string[] = [];
  const { data } = await reader((request) => {
    sent.push(`${request.method} ${new URL(request.url).pathname}`);
    return answered;
  });
  const done = markConversationRead(data, "c3");
  expect(unread(data)).toEqual(["c1"]);
  expect(probe(data, () => useUnreadConversations())).toBe(1);
  answer(json(200, conversation("c3", direct("s3", 3), false)));
  await done;
  expect(sent).toEqual(["POST /api/v1/conversations/c3/read"]);
  expect(unread(data)).toEqual(["c1"]);
  expect(probe(data, () => useUnreadConversations())).toBe(1);
});

test("a read that the server refused shows the conversation unread again", async () => {
  const { data } = await reader(() => problem(500));
  await markConversationRead(data, "c3").catch(() => undefined);
  expect(unread(data)).toEqual(["c3", "c1"]);
});

test("hiding a conversation takes it out of the list at once, and it stays out once the server answered", async () => {
  const { promise: answered, resolve: answer } = Promise.withResolvers<Response>();
  const sent: string[] = [];
  const { data } = await reader((request) => {
    sent.push(`${request.method} ${new URL(request.url).pathname}`);
    return answered;
  });
  const done = hideConversation(data, "c2");
  expect(listed(data)).toEqual(["c3", "c1"]);
  answer(new Response(null, { status: 204 }));
  await done;
  expect(sent).toEqual(["POST /api/v1/conversations/c2/hide"]);
  expect(listed(data)).toEqual(["c3", "c1"]);
});

test("a conversation the server refused to hide comes back to the list", async () => {
  const { data } = await reader(() => problem(500));
  await hideConversation(data, "c2").catch(() => undefined);
  expect(listed(data)).toEqual(["c3", "c2", "c1"]);
});

test("a hidden conversation comes back at the top when a new post arrives in it", async () => {
  const { data } = await reader(() => new Response(null, { status: 204 }));
  await hideConversation(data, "c2");
  receiveConversation(data, conversation("c2", direct("s4", 4), true));
  expect(listed(data)).toEqual(["c2", "c3", "c1"]);
  expect(unread(data)).toEqual(["c2", "c3", "c1"]);
});

test("a new conversation arriving goes to the top of the list, unread", async () => {
  const { data } = await reader();
  receiveConversation(data, conversation("c9", direct("s9", 9), true));
  expect(listed(data)).toEqual(["c9", "c3", "c2", "c1"]);
  expect(probe(data, () => useUnreadConversations())).toBe(3);
});

test("a new post in a conversation lower in the list moves it to the top with that post", async () => {
  const { data } = await reader();
  receiveConversation(data, conversation("c1", direct("s5", 5), true));
  expect(listed(data)).toEqual(["c1", "c3", "c2"]);
  expect(probe(data, () => useConversations()).items[0]?.last_status.id).toBe("s5");
});

test("a conversation read on another device arrives read and keeps its place", async () => {
  const { data } = await reader();
  receiveConversation(data, conversation("c3", direct("s3", 3), false));
  expect(listed(data)).toEqual(["c3", "c2", "c1"]);
  expect(unread(data)).toEqual(["c1"]);
});

/** Starts the stream of a tab of `data`, opened, and returns what sends its events and opens it again. */
async function streaming(data: AppData) {
  const { run, streams } = fakeStreams();
  startStream(data, { tabs: fakeBrowser().join(), run });
  await Bun.sleep(0);
  const [stream] = streams;
  if (stream === undefined) throw new Error("no stream");
  stream.open(false);
  return stream;
}

test("a conversation arrives on the stream into the list", async () => {
  const { data } = await reader();
  const stream = await streaming(data);
  const event: StreamEvent = { type: "conversation", conversation: conversation("c9", direct("s9", 9), true) };
  stream.send(event);
  expect(listed(data)).toEqual(["c9", "c3", "c2", "c1"]);
});

test("a conversation whose latest post was deleted leaves the list", async () => {
  const { data } = await reader();
  const stream = await streaming(data);
  stream.send({ type: "delete", id: "s2" });
  expect(listed(data)).toEqual(["c3", "c1"]);
});

test("after the stream reconnects, the list and every conversation are read again", async () => {
  const { data } = await reader();
  await data.queryClient.query({
    ...conversationQuery(data, "c3"),
    queryFn: () => conversation("c3", direct("s3", 3), true),
  });
  const stream = await streaming(data);
  const stale = () => [
    data.queryClient.getQueryState(conversationsQuery(data).queryKey)?.isInvalidated,
    data.queryClient.getQueryState(conversationQuery(data, "c3").queryKey)?.isInvalidated,
  ];
  expect(stale()).toEqual([false, false]);
  stream.open(true);
  expect(stale()).toEqual([true, true]);
});

test("a conversation opened from the list shows at once, before the server answers", async () => {
  const { data } = await reader();
  const one = probe(data, () => useConversation("c2"));
  expect(one.conversation?.id).toBe("c2");
  expect(one.conversation?.last_status.id).toBe("s2");
});

test("a conversation's posts are its direct posts of the thread, oldest first", async () => {
  const { data } = fakeAppData(() => json(500, {}));
  const last = direct("s5", 5);
  await data.queryClient.query({
    ...statusContextQuery(data, "s5"),
    queryFn: () => ({
      ancestors: [direct("s3", 3), status("p4", { visibility: "public", created_at: "2026-11-15T10:04:00Z" })],
      descendants: [direct("s1", 1)],
    }),
  });
  const posts = probe(data, () => useConversationPosts(conversation("c5", last, false)));
  expect(posts.status).toBe("success");
  expect(posts.posts.map((s) => s.id)).toEqual(["s1", "s3", "s5"]);
});

test("a conversation's posts wait for the conversation", () => {
  const { data } = fakeAppData(() => json(500, {}));
  expect(probe(data, () => useConversationPosts(undefined))).toMatchObject({ status: "pending", posts: [] });
});

// The server's `unread` is a post by another account newer than the read point, so the reader's own reply leaves it as
// it was (api/spec/components/schemas/Conversation.yaml).
test("a direct reply the reader posts becomes the conversation's latest post at the top of the list, its unread state unchanged", async () => {
  const reply = direct("s8", 8, { in_reply_to_id: "s1" });
  const { data } = await reader(() => json(200, reply));
  await postStatus(data, { text: "@sam see you", visibility: "direct", in_reply_to_id: "s1" }, "key-1");
  expect(listed(data)).toEqual(["c1", "c3", "c2"]);
  const [first] = probe(data, () => useConversations()).items;
  expect(first?.last_status.id).toBe("s8");
  expect(first?.unread).toBe(true);
});

// ---- Races of a read and a hide with the stream ---------------------------------------------------------------------

/** The latest post's ID and the unread state of the conversation `id`, in the list and on its own screen. */
const seen = (data: AppData, id: string) => {
  const listedOne = probe(data, () => useConversations()).items.find((c) => c.id === id);
  const own = probe(data, () => useConversation(id)).conversation;
  return {
    list: listedOne && { last: listedOne.last_status.id, unread: listedOne.unread },
    own: own && { last: own.last_status.id, unread: own.unread },
  };
};

test("a read answered for an older post leaves the newer post that arrived meanwhile, unread, at the top", async () => {
  const { promise: answered, resolve: answer } = Promise.withResolvers<Response>();
  const { data } = await reader(() => answered);
  const done = markConversationRead(data, "c1");
  receiveConversation(data, conversation("c1", direct("s9", 9), true));
  answer(json(200, conversation("c1", direct("s1", 1), false)));
  await done;
  expect(listed(data)).toEqual(["c1", "c3", "c2"]);
  expect(seen(data, "c1")).toEqual({ list: { last: "s9", unread: true }, own: { last: "s9", unread: true } });
});

test("a read answered for the same latest post updates the conversation in its place", async () => {
  const { data } = await reader(() =>
    json(200, {
      ...conversation("c1", direct("s1", 1), false),
      accounts: [
        { id: "sam", acct: "sam" },
        { id: "kai", acct: "kai" },
      ],
    }),
  );
  await markConversationRead(data, "c1");
  expect(listed(data)).toEqual(["c3", "c2", "c1"]);
  const c1 = probe(data, () => useConversations()).items.find((c) => c.id === "c1");
  expect(c1?.unread).toBe(false);
  expect(c1?.accounts.map((a) => a.id)).toEqual(["sam", "kai"]);
});

test("a newer post arriving while the hide is in flight brings the conversation back, and it stays after the answer", async () => {
  const { promise: answered, resolve: answer } = Promise.withResolvers<Response>();
  const { data } = await reader(() => answered);
  const done = hideConversation(data, "c2");
  expect(listed(data)).toEqual(["c3", "c1"]);
  receiveConversation(data, conversation("c2", direct("s9", 9), true));
  expect(listed(data)).toEqual(["c2", "c3", "c1"]);
  answer(new Response(null, { status: 204 }));
  await done;
  expect(listed(data)).toEqual(["c2", "c3", "c1"]);
  expect(seen(data, "c2").list).toEqual({ last: "s9", unread: true });
});

test("a hidden conversation stays out on an event with the same latest post, and comes back with a newer one", async () => {
  const { data } = await reader(() => new Response(null, { status: 204 }));
  await hideConversation(data, "c2");
  // The echo of a read on another device: the same latest post.
  receiveConversation(data, conversation("c2", direct("s2", 2), false));
  expect(listed(data)).toEqual(["c3", "c1"]);
  receiveConversation(data, conversation("c2", direct("s9", 9), true));
  expect(listed(data)).toEqual(["c2", "c3", "c1"]);
});

test("hiding a conversation the cache does not hold sends nothing and changes nothing", async () => {
  const sent: string[] = [];
  const { data } = await reader((request) => {
    sent.push(`${request.method} ${new URL(request.url).pathname}`);
    return new Response(null, { status: 204 });
  });
  await hideConversation(data, "c9");
  expect(sent).toEqual([]);
  expect(listed(data)).toEqual(["c3", "c2", "c1"]);
});

test("the writes are the same object across renders, so that an effect may depend on them", async () => {
  const { data } = await reader();
  const rendered: ConversationWrites[] = [];
  function Twice() {
    const writes = useConversationWrites();
    const [round, setRound] = useState(0);
    rendered.push(writes);
    // An update during the render renders the same component again, its hooks kept.
    if (round === 0) setRound(1);
    return null;
  }
  renderToStaticMarkup(createElement(AppDataProvider, { data, children: createElement(Twice) }));
  expect(rendered).toHaveLength(2);
  expect(rendered[1]).toBe(rendered[0] as ConversationWrites);
});
