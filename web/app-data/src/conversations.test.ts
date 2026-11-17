// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { fakeOrigin, scenario, type FakeApi } from "@my/e2e/fake";
import { InfiniteQueryObserver } from "@tanstack/react-query";
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
import { startStream } from "./stream";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { fakeBrowser, fakeStreams } from "./test/tabs";
import { statusContextQuery } from "./thread";

/** The direct posts of sam to the reader, oldest first, each a conversation of its own: one more than a page. */
const keys = Array.from({ length: 21 }, (_, i) => `direct-${String(i + 1)}`);
/** The newest (unread), the one under it (read), and the oldest (unread), which the second page holds. */
const [top, read, last] = [keys.at(-1) ?? "", keys.at(-2) ?? "", keys[0] ?? ""];

/**
 * AppData of a reader with a session, whose server holds the conversations of `keys`, of which the reader read all
 * but `top` and `last`; the pages of the list are read, both unless `pages` is 1. `ids` are the conversations' IDs as
 * the server lists them, newest first, and `id` the ID of the conversation of a post.
 */
async function reader(pages: 1 | 2 = 2) {
  const { fake, data: plain, requests } = fakeData({
    seed: {
      people: scenario.people,
      posts: keys.map((key) => ({
        key,
        author: "sam",
        text: `@yuki Message ${key}.`,
        visibility: "direct",
        language: "en",
      })),
      conversationReads: keys.filter((key) => key !== top && key !== last).map((post) => ({ by: "yuki", post })),
    },
  });
  const data: AppData = { ...plain, connection: { server: fakeOrigin } };
  const first = await data.queryClient.infiniteQuery({ ...conversationsQuery(data), staleTime: Infinity });
  const second =
    pages === 1 ? undefined : (await new InfiniteQueryObserver(data.queryClient, conversationsQuery(data)).fetchNextPage()).data;
  const listed = (second ?? first).pages.flatMap((p) => p.items);
  const id = (key: string) => listed.find((c) => c.last_status.id === fake.idOf(key))?.id ?? `none of ${key}`;
  return { fake, data, requests, ids: listed.map((c) => c.id), id };
}

/** The conversation `id` as the server holds it now. */
async function now(data: AppData, id: string): Promise<Conversation> {
  const { data: conversation } = await data.api.GET("/conversations/{id}", { params: { path: { id } } });
  if (conversation === undefined) throw new Error(`test: the server has no conversation ${id}`);
  return conversation;
}

/** A new post by sam in the conversation at `index` of the server's, newest first: the conversation as it is then. */
async function message(fake: FakeApi, data: AppData, index: number, id: string) {
  const post = fake.newMessage(index);
  return { post, conversation: await now(data, id) };
}

const without = (ids: readonly string[], ...gone: string[]) => ids.filter((id) => !gone.includes(id));
const listed = (data: AppData) => probe(data, () => useConversations()).items.map((c) => c.id);
const unread = (data: AppData) =>
  probe(data, () => useConversations())
    .items.filter((c) => c.unread)
    .map((c) => c.id);

test("the list is the server's conversations, page after page, the one with the newest post first", async () => {
  const { data, requests, ids, id } = await reader();
  expect(ids).toHaveLength(21);
  expect([ids[0], ids[1], ids.at(-1)]).toEqual([id(top), id(read), id(last)]);
  expect(listed(data)).toEqual(ids);
  expect(requests).toEqual(["GET /conversations", `GET /conversations?cursor=${ids[19] ?? ""}`]);
});

test("the list says which conversations are unread, and the navigation counts them", async () => {
  const { data, id } = await reader();
  expect(unread(data)).toEqual([id(top), id(last)]);
  expect(probe(data, () => useUnreadConversations())).toBe(2);
});

test("the navigation has no count before the list is read, and without a session it reads nothing", () => {
  const { data, requests } = fakeData();
  expect(probe(data, () => useUnreadConversations())).toBeUndefined();
  expect(probe(data, () => useUnreadConversations(false))).toBeUndefined();
  expect(requests).toEqual([]);
});

test("reading a conversation marks it read on the server, at once in the list, and the count drops", async () => {
  const { fake, data, requests, id } = await reader();
  const write = fake.script("POST /conversations/{id}/read", { hold: "response" });
  requests.length = 0;
  const done = markConversationRead(data, id(top));
  expect(unread(data)).toEqual([id(last)]);
  expect(probe(data, () => useUnreadConversations())).toBe(1);
  write.release();
  await done;
  expect(requests).toEqual([`POST /conversations/${id(top)}/read`]);
  expect(unread(data)).toEqual([id(last)]);
  expect(probe(data, () => useUnreadConversations())).toBe(1);
});

test("a read that the server refused shows the conversation unread again", async () => {
  const { fake, data, id } = await reader();
  fake.script("POST /conversations/{id}/read", { fail: 500 });
  await markConversationRead(data, id(top)).catch(() => undefined);
  expect(unread(data)).toEqual([id(top), id(last)]);
});

test("hiding a conversation takes it out of the list at once, and it stays out once the server answered", async () => {
  const { fake, data, requests, ids, id } = await reader();
  const write = fake.script("POST /conversations/{id}/hide", { hold: "response" });
  requests.length = 0;
  const done = hideConversation(data, id(read));
  expect(listed(data)).toEqual(without(ids, id(read)));
  write.release();
  await done;
  expect(requests).toEqual([`POST /conversations/${id(read)}/hide`]);
  expect(listed(data)).toEqual(without(ids, id(read)));
});

test("a conversation the server refused to hide comes back to the list", async () => {
  const { fake, data, ids, id } = await reader();
  fake.script("POST /conversations/{id}/hide", { fail: 500 });
  await hideConversation(data, id(read)).catch(() => undefined);
  expect(listed(data)).toEqual(ids);
});

test("a hidden conversation comes back at the top when a new post arrives in it", async () => {
  const { fake, data, ids, id } = await reader();
  await hideConversation(data, id(read));
  receiveConversation(data, (await message(fake, data, 1, id(read))).conversation);
  expect(listed(data)).toEqual([id(read), ...without(ids, id(read))]);
  expect(unread(data)).toEqual([id(read), id(top), id(last)]);
});

test("a new conversation arriving goes to the top of the list, unread", async () => {
  // The reader read the first page alone, which does not hold the oldest conversation.
  const { fake, data, ids } = await reader(1);
  expect(probe(data, () => useUnreadConversations())).toBe(1);
  const arrived = await latest(data, fake.newMessage(20).id);
  if (arrived === undefined) throw new Error("test: the server lists no conversation of the new post");
  expect(ids).not.toContain(arrived.id);
  receiveConversation(data, arrived);
  expect(listed(data)).toEqual([arrived.id, ...ids]);
  expect(probe(data, () => useUnreadConversations())).toBe(2);
});

/** The conversation whose latest post is `status`, from the server's list read to its end. */
async function latest(data: AppData, status: string): Promise<Conversation | undefined> {
  for (let cursor: string | undefined; ; ) {
    const { data: page } = await data.api.GET("/conversations", {
      params: { query: cursor === undefined ? {} : { cursor } },
    });
    const found = page?.items.find((c) => c.last_status.id === status);
    if (found !== undefined || page?.next_cursor === undefined) return found;
    cursor = page.next_cursor;
  }
}

test("a new post in a conversation lower in the list moves it to the top with that post", async () => {
  const { fake, data, ids, id } = await reader();
  const { post, conversation } = await message(fake, data, 20, id(last));
  receiveConversation(data, conversation);
  expect(listed(data)).toEqual([id(last), ...without(ids, id(last))]);
  expect(probe(data, () => useConversations()).items[0]?.last_status.id).toBe(post.id);
});

test("a conversation read on another device arrives read and keeps its place", async () => {
  const { data, ids, id } = await reader();
  // The other device's read, which the stream then brings.
  const { data: read } = await data.api.POST("/conversations/{id}/read", { params: { path: { id: id(top) } } });
  if (read === undefined) throw new Error("test: the server did not mark the conversation read");
  receiveConversation(data, read);
  expect(listed(data)).toEqual(ids);
  expect(unread(data)).toEqual([id(last)]);
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
  const { fake, data, ids, id } = await reader();
  const stream = await streaming(data);
  const { conversation } = await message(fake, data, 20, id(last));
  stream.send({ type: "conversation", conversation });
  expect(listed(data)).toEqual([id(last), ...without(ids, id(last))]);
});

test("a conversation whose latest post was deleted leaves the list", async () => {
  const { fake, data, ids, id } = await reader();
  const stream = await streaming(data);
  stream.send({ type: "delete", id: fake.idOf(read) });
  expect(listed(data)).toEqual(without(ids, id(read)));
});

test("after the stream reconnects, the list and every conversation are read again", async () => {
  const { data, id } = await reader();
  await data.queryClient.query(conversationQuery(data, id(top)));
  const stream = await streaming(data);
  const stale = () => [
    data.queryClient.getQueryState(conversationsQuery(data).queryKey)?.isInvalidated,
    data.queryClient.getQueryState(conversationQuery(data, id(top)).queryKey)?.isInvalidated,
  ];
  expect(stale()).toEqual([false, false]);
  stream.open(true);
  expect(stale()).toEqual([true, true]);
});

test("a conversation opened from the list shows at once, before the server answers", async () => {
  const { fake, data, requests, id } = await reader();
  requests.length = 0;
  const one = probe(data, () => useConversation(id(read)));
  expect(one.conversation?.id).toBe(id(read));
  expect(one.conversation?.last_status.id).toBe(fake.idOf(read));
  expect(requests).toEqual([]);
});

test("a conversation's posts are its direct posts of the thread, oldest first", async () => {
  // sam answers a public post of their own with a direct post to the reader, who answers it.
  const { fake, data } = fakeData({
    seed: {
      people: scenario.people,
      posts: [
        { key: "public", author: "sam", text: "Bread on Saturday.", visibility: "public", language: "en" },
        { key: "asks", author: "sam", replyTo: "public", text: "@yuki Coming?", visibility: "direct", language: "en" },
        { key: "answers", author: "yuki", replyTo: "asks", text: "@sam Yes!", visibility: "direct", language: "en" },
      ],
    },
  });
  const [conversation] = (await data.queryClient.infiniteQuery(conversationsQuery(data))).pages[0]?.items ?? [];
  expect(conversation?.last_status.id).toBe(fake.idOf("answers"));
  await data.queryClient.query(statusContextQuery(data, fake.idOf("answers")));
  const posts = probe(data, () => useConversationPosts(conversation));
  expect(posts.status).toBe("success");
  expect(posts.posts.map((s) => s.id)).toEqual([fake.idOf("asks"), fake.idOf("answers")]);
});

test("a conversation's posts wait for the conversation", () => {
  const { data } = fakeData();
  expect(probe(data, () => useConversationPosts(undefined))).toMatchObject({ status: "pending", posts: [] });
});

// The server's `unread` is a post by another account newer than the read point, so the reader's own reply leaves it as
// it was (api/spec/components/schemas/Conversation.yaml).
test("a direct reply the reader posts becomes the conversation's latest post at the top of the list, its unread state unchanged", async () => {
  const { fake, data, ids, id } = await reader();
  const reply = await postStatus(
    data,
    { text: "@sam see you", visibility: "direct", in_reply_to_id: fake.idOf(last) },
    "key-1",
  );
  expect(listed(data)).toEqual([id(last), ...without(ids, id(last))]);
  const [first] = probe(data, () => useConversations()).items;
  expect(first?.last_status.id).toBe(reply.id);
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
  const { fake, data, ids, id } = await reader();
  const write = fake.script("POST /conversations/{id}/read", { hold: "response" });
  const done = markConversationRead(data, id(last));
  // The server marked it read up to its latest post; then a newer one arrives, before the answer does.
  await write.served();
  const { post, conversation } = await message(fake, data, 20, id(last));
  receiveConversation(data, conversation);
  write.release();
  await done;
  expect(listed(data)).toEqual([id(last), ...without(ids, id(last))]);
  expect(seen(data, id(last))).toEqual({ list: { last: post.id, unread: true }, own: { last: post.id, unread: true } });
});

test("a read answered for the same latest post updates the conversation in its place", async () => {
  const { fake, data, ids, id } = await reader();
  // The server's answer names one more account than the list did.
  const { data: kai } = await data.api.GET("/accounts/{id}", { params: { path: { id: fake.accountId("kai") } } });
  if (kai === undefined) throw new Error("test: the server has no kai");
  fake.script("POST /conversations/{id}/read", { change: (c) => ({ ...c, accounts: [...c.accounts, kai] }) });
  await markConversationRead(data, id(last));
  expect(listed(data)).toEqual(ids);
  const one = probe(data, () => useConversations()).items.find((c) => c.id === id(last));
  expect(one?.unread).toBe(false);
  expect(one?.accounts.map((a) => a.id)).toEqual([fake.accountId("sam"), fake.accountId("kai")]);
});

test("a newer post arriving while the hide is in flight brings the conversation back, and it stays after the answer", async () => {
  const { fake, data, ids, id } = await reader();
  const write = fake.script("POST /conversations/{id}/hide", { hold: "response" });
  const done = hideConversation(data, id(read));
  expect(listed(data)).toEqual(without(ids, id(read)));
  await write.served();
  const { post, conversation } = await message(fake, data, 1, id(read));
  receiveConversation(data, conversation);
  const back = [id(read), ...without(ids, id(read))];
  expect(listed(data)).toEqual(back);
  write.release();
  await done;
  expect(listed(data)).toEqual(back);
  expect(seen(data, id(read)).list).toEqual({ last: post.id, unread: true });
});

test("a hidden conversation stays out on an event with the same latest post, and comes back with a newer one", async () => {
  const { fake, data, ids, id } = await reader();
  await hideConversation(data, id(read));
  // The echo of a read on another device: the same latest post.
  receiveConversation(data, await now(data, id(read)));
  expect(listed(data)).toEqual(without(ids, id(read)));
  receiveConversation(data, (await message(fake, data, 1, id(read))).conversation);
  expect(listed(data)).toEqual([id(read), ...without(ids, id(read))]);
});

test("hiding a conversation the cache does not hold sends nothing and changes nothing", async () => {
  const { data, requests, ids } = await reader();
  requests.length = 0;
  await hideConversation(data, "00000000-0000-7000-8000-00000000ffff");
  expect(requests).toEqual([]);
  expect(listed(data)).toEqual(ids);
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
