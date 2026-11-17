// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { scenario, type FakeApi, type FakeSeed, type Route } from "@my/e2e/fake";
import { accountStatusesQuery } from "./accounts";
import type { AppData } from "./app-data";
import { prependHomeStatuses } from "./arrivals";
import { useStatus } from "./compose";
import { markStatus } from "./interactions";
import { notificationsQuery } from "./notifications";
import { changeStatus, receiveGone, removeStatus, shadowed, shadowStatus, useShadow } from "./shadow";
import type { Status } from "./statuses";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { cacheReply, statusContextQuery, statusQuery, useThread, type StatusContext } from "./thread";
import { homeTimelineQuery, useHomeTimeline } from "./timeline";

type Post = FakeSeed["posts"][number];

const by = (author: string, key: string, more: Partial<Post> = {}): Post => ({
  key,
  author,
  text: `Post ${key}.`,
  visibility: "public",
  language: "en",
  ...more,
});

const favourite = (s: Status): Status => ({ ...s, favourited: true, favourites_count: 1 });

/**
 * AppData on a server where the reader follows sam and hana: sam posted `1` and then `2`, hana quoted `1` in `q` and
 * boosted `1`, and marco, whom the reader does not follow, quoted `1` in `q2`, which hana boosted too. The home timeline
 * is read: `read()` is its statuses as they show, and `raw()` as the cache holds them.
 */
async function cached() {
  const { fake, data } = fakeData({
    seed: {
      people: scenario.people,
      follows: ["sam", "hana"].map((followee) => ({ follower: "yuki", followee })),
      posts: [by("sam", "1"), by("sam", "2"), by("hana", "q", { quote: "1" }), by("marco", "q2", { quote: "1" })],
      interactions: [
        { kind: "boost", by: "hana", post: "1" },
        { kind: "boost", by: "hana", post: "q2" },
      ],
    },
  });
  const key = homeTimelineQuery(data).queryKey;
  await data.queryClient.infiniteQuery(homeTimelineQuery(data));
  const raw = () => data.queryClient.getQueryData(key)?.pages[0]?.items ?? [];
  const read = () =>
    raw().flatMap((s) => {
      const shown = shadowed(data, s);
      return shown === undefined ? [] : [shown];
    });
  const [one, two, q, q2] = ["1", "2", "q", "q2"].map(fake.idOf) as [string, string, string, string];
  /** The copies of `1` among `items`: the post itself, hana's boost of it, and the post that quotes it. */
  const copies = (items: readonly Status[]) => ({
    alone: items.find((s) => s.id === one),
    boost: items.find((s) => s.boost?.id === one),
    quote: items.find((s) => s.id === q),
  });
  return { fake, data, read, raw, copies, one, two, q, q2 };
}

describe("a change", () => {
  test("shows on the status on a timeline, and on a boost and a quote of it, leaving the cache as it was", async () => {
    const { data, read, raw, copies, one } = await cached();
    shadowStatus(data, one, favourite);
    const { alone, boost, quote } = copies(read());
    expect(alone?.favourited).toBe(true);
    expect(boost?.boost?.favourited).toBe(true);
    expect(boost?.favourited).toBe(false);
    expect(quote?.quote?.quoted_status?.favourited).toBe(true);
    expect(copies(raw()).alone?.favourited).toBe(false);
  });

  test("keeps a status without a change as it was", async () => {
    const { data, read, copies, two } = await cached();
    const before = copies(read()).alone;
    shadowStatus(data, two, favourite);
    expect(before).toBeDefined();
    expect(copies(read()).alone).toBe(before as Status);
  });

  test("gives the hooks a new useShadow() once it is laid or settled, and the same one while nothing changes", async () => {
    // React Compiler memoizes what a hook works out of useShadow() on that function's identity alone.
    const { data, one } = await cached();
    const before = probe(data, useShadow);
    expect(probe(data, useShadow)).toBe(before);
    const shadow = shadowStatus(data, one, favourite);
    const laid = probe(data, useShadow);
    expect(laid).not.toBe(before);
    shadow.confirm();
    expect(probe(data, useShadow)).not.toBe(laid);
  });

  test("taken off, shows every copy again as it was", async () => {
    const { data, read, copies, one } = await cached();
    shadowStatus(data, one, favourite).discard();
    expect(copies(read()).alone?.favourited).toBe(false);
  });

  test("the server took stays on through a fetch begun before, and gives way to one begun after", async () => {
    const { data, read, copies, one } = await cached();
    const query = homeTimelineQuery(data);
    const write = shadowStatus(data, one, favourite);
    // A fetch begun while the write is in flight answers without it.
    const before = data.queryClient.refetchQueries({ queryKey: query.queryKey });
    write.confirm();
    await before;
    expect(copies(read()).alone?.favourited).toBe(true);
    // A fetch begun after shows what the server says, even when its answer did not change.
    await data.queryClient.refetchQueries({ queryKey: query.queryKey });
    expect(copies(read()).alone?.favourited).toBe(false);
  });

  test("the server took stays on when a post goes in above it on the home timeline", async () => {
    const { fake, data, read, one } = await cached();
    const before = read().map((s) => s.id);
    shadowStatus(data, one, favourite).confirm();
    const arrived = fake.newPosts(1);
    prependHomeStatuses(data, arrived);
    expect(read().map((s) => s.id)).toEqual([...arrived.map((s) => s.id), ...before]);
    expect(
      read()
        .filter((s) => s.favourited === true)
        .map((s) => s.id),
    ).toEqual([one]);
  });

  test("the server took stays on when a reply goes in above it in a thread", async () => {
    // Under the reader's post, sam's reply and then hana's: a reply to sam's goes in between them.
    const { fake, data } = fakeData({
      seed: {
        people: scenario.people,
        posts: [by("yuki", "0"), by("sam", "a", { replyTo: "0" }), by("hana", "b", { replyTo: "0" })],
      },
    });
    const [root, a, b] = ["0", "a", "b"].map(fake.idOf) as [string, string, string];
    const context = statusContextQuery(data, root);
    await data.queryClient.query(context);
    shadowStatus(data, b, favourite).confirm();
    const { data: reply } = await data.api.POST("/statuses", {
      body: { text: "@sam Yes.", visibility: "public", in_reply_to_id: a },
    });
    if (reply === undefined) throw new Error("test: the server posted nothing");
    cacheReply(data.queryClient, reply);
    const descendants = data.queryClient.getQueryData<StatusContext>(context.queryKey)?.descendants ?? [];
    expect(descendants.map((s) => [s.id, shadowed(data, s)?.favourited])).toEqual([
      [a, false],
      [reply.id, false],
      [b, true],
    ]);
  });

  test("the server took gives way on a post a write by hand brings as the server has it now", async () => {
    const { fake, data, read, raw, copies, one } = await cached();
    shadowStatus(data, one, favourite).confirm();
    // A boost arrives that carries the post as the server has it by now: others favourited it too.
    const [arrived] = fake.newPosts(1);
    const now = copies(raw()).alone;
    if (arrived === undefined || now === undefined) throw new Error("test: no post");
    prependHomeStatuses(data, [{ ...arrived, boost: { ...now, favourites_count: 5 } }]);
    expect(read()[0]?.boost?.favourites_count).toBe(5);
  });
});

describe("a status gone", () => {
  test("leaves a timeline with the boosts of it, and a quote of it shows it deleted", async () => {
    const { data, read, one, two, q, q2 } = await cached();
    // Newest first: hana's boosts of marco's quote and of the post, then hana's quote, and sam's two posts.
    expect(read().map((s) => s.boost?.id ?? s.id)).toEqual([q2, one, q, two, one]);
    receiveGone(data, one);
    const items = read();
    expect(items.map((s) => s.boost?.id ?? s.id)).toEqual([q2, q, two]);
    expect(items[1]?.quote).toEqual({ state: "deleted" });
    expect(items[0]?.boost?.quote).toEqual({ state: "deleted" });
  });

  test("leaves a thread, an account's posts and the notifications about it, and the status alone is read again", async () => {
    // sam mentions the reader in a post, which notifies them, and posts another.
    const { fake, data } = fakeData({
      seed: { people: scenario.people, posts: [by("sam", "1", { text: "@yuki Bread at nine." }), by("sam", "3")] },
    });
    const { queryClient } = data;
    const [one, three] = ["1", "3"].map(fake.idOf) as [string, string];
    const single = statusQuery(data, one).queryKey;
    const posts = accountStatusesQuery(data, fake.accountId("sam"), "posts");
    const notifications = notificationsQuery(data, "all");
    await queryClient.query(statusQuery(data, one));
    await queryClient.infiniteQuery(posts);
    await queryClient.infiniteQuery(notifications);
    const shown = (s: Status | undefined) => s && shadowed(data, s);
    const notified = () => queryClient.getQueryData(notifications.queryKey)?.pages[0]?.items[0]?.status;
    expect(notified()?.id).toBe(one);
    receiveGone(data, one);
    expect(queryClient.getQueryState(single)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryData(posts.queryKey)?.pages[0]?.items.map((s) => shown(s)?.id)).toEqual([
      three,
      undefined,
    ]);
    expect(shown(notified())).toBeUndefined();
  });

  test("stays gone in what a fetch begun before brings", async () => {
    const { fake, data } = fakeData({
      seed: {
        people: scenario.people,
        follows: [{ follower: "yuki", followee: "sam" }],
        posts: [by("sam", "1"), by("sam", "2")],
      },
    });
    const query = homeTimelineQuery(data);
    const read = fake.script("GET /timelines/home", { hold: "response" });
    const fetched = data.queryClient.infiniteQuery(query);
    await read.served();
    receiveGone(data, fake.idOf("1"));
    read.release();
    await fetched;
    const items = data.queryClient.getQueryData(query.queryKey)?.pages[0]?.items ?? [];
    expect(items.map((s) => s.id)).toEqual([fake.idOf("2"), fake.idOf("1")]);
    expect(items.filter((s) => shadowed(data, s) !== undefined).map((s) => s.id)).toEqual([fake.idOf("2")]);
  });
});

// ---- Through the hooks the screens read ----------------------------------------------------------------------------

/** A server that holds three posts of the reader's: `0`, their reply `1` to it, and `2`. */
function server() {
  const { fake, data } = fakeData({
    seed: { people: scenario.people, posts: [by("yuki", "0"), by("yuki", "1", { replyTo: "0" }), by("yuki", "2")] },
  });
  const [zero, one, two] = ["0", "1", "2"].map(fake.idOf) as [string, string, string];
  return { fake, data, zero, one, two };
}

type Server = ReturnType<typeof server>;

/** Fills the cache as the home timeline, the thread of post 0 and the screen of post 1 load it. */
async function load({ data, zero, one }: Server) {
  await Promise.all([
    data.queryClient.infiniteQuery(homeTimelineQuery(data)),
    data.queryClient.query(statusQuery(data, zero)),
    data.queryClient.query(statusContextQuery(data, zero)),
    data.queryClient.query(statusQuery(data, one)),
  ]);
}

/** Post 1 as each screen shows it: on the home timeline, in the thread of post 0, and alone; and the IDs of the lists. */
function shown({ data, zero, one }: Server) {
  const home = probe(data, () => useHomeTimeline().statuses);
  const thread = probe(data, () => useThread(zero).context?.descendants ?? []);
  const alone = probe(data, () => useStatus(one).status);
  return {
    home: home.find((s) => s.id === one),
    thread: thread.find((s) => s.id === one),
    alone,
    ids: { home: home.map((s) => s.id), thread: thread.map((s) => s.id) },
  };
}

/** The operations that read what `load` fills the cache with: four requests of three operations. */
const reads: readonly Route[] = ["GET /timelines/home", "GET /statuses/{id}", "GET /statuses/{id}/context"];

/**
 * Refetches every query: the server answers each read as it is now, before the write that waits, and the answers
 * wait; the function returned lets them come, with the post as it was.
 */
async function raceRefetch({ fake, data }: Server) {
  const held = reads.map((route) => fake.script(route, { hold: "response" }));
  const refetched = data.queryClient.refetchQueries();
  await Promise.all([...held.map((read) => read.served()), held[1]?.served(2)]);
  return async () => {
    for (const read of held) read.end();
    await refetched;
  };
}

/** A write of `route` that waits at the server, which has not done it yet; `fail` makes its answer a failure. */
const waiting = (fake: FakeApi, route: Route, fail = false) =>
  fake.script(route, { hold: "request", ...(fail ? { fail: 500 } : {}) });

const rejects = (p: Promise<unknown>) =>
  p.then(
    () => false,
    () => true,
  );

type Write = {
  readonly name: string;
  readonly route: Route;
  readonly start: (s: Server) => Promise<unknown>;
  readonly shows: (s: Status | undefined) => boolean;
};

/** Edits post 1 by hand, as the screen of its edit does. */
const edit = ({ data, one }: { data: AppData; one: string }) =>
  data.api.PUT("/statuses/{id}", { params: { path: { id: one } }, body: { text: "edited" } });

const changes: Write[] = [
  {
    name: "a favourite",
    route: "POST /statuses/{id}/favourite",
    start: (s) => markStatus(s.data, s.one, { type: "favourite", on: true }),
    shows: (s) => s?.favourited === true && s.favourites_count === 1,
  },
  {
    name: "a boost",
    route: "POST /statuses/{id}/boost",
    start: (s) => markStatus(s.data, s.one, { type: "boost", on: true }),
    shows: (s) => s?.boosted === true && s.boosts_count === 1,
  },
  {
    name: "an edit",
    route: "PUT /statuses/{id}",
    start: (s) =>
      changeStatus(s.data, s.one, { change: (p) => ({ ...p, content: "<p>edited</p>" }), send: () => edit(s) }),
    shows: (s) => s?.content === "<p>edited</p>",
  },
];

const everywhere = (shows: Write["shows"], seen: ReturnType<typeof shown>) => ({
  home: shows(seen.home),
  thread: shows(seen.thread),
  alone: shows(seen.alone),
});

describe.each(changes)("$name", ({ route, start, shows }) => {
  test("shows at once on the home timeline, in a thread and on the post alone, and stays once answered", async () => {
    const s = server();
    await load(s);
    expect(everywhere(shows, shown(s))).toEqual({ home: false, thread: false, alone: false });
    const write = waiting(s.fake, route);
    const done = start(s);
    expect(everywhere(shows, shown(s))).toEqual({ home: true, thread: true, alone: true });
    write.release();
    await done;
    expect(everywhere(shows, shown(s))).toEqual({ home: true, thread: true, alone: true });
  });

  test("survives a refetch that raced it and answered the post as it was", async () => {
    const s = server();
    await load(s);
    const write = waiting(s.fake, route);
    const done = start(s);
    const answerReads = await raceRefetch(s);
    write.release();
    await done;
    await answerReads();
    expect(everywhere(shows, shown(s))).toEqual({ home: true, thread: true, alone: true });
  });

  test("failed, shows every copy as it was again, and rejects", async () => {
    const s = server();
    await load(s);
    const was = shown(s);
    const write = waiting(s.fake, route, true);
    const done = start(s);
    write.release();
    expect(await rejects(done)).toBe(true);
    const seen = shown(s);
    expect(everywhere(shows, seen)).toEqual({ home: false, thread: false, alone: false });
    expect([seen.home, seen.thread, seen.alone]).toEqual([was.home, was.thread, was.alone]);
  });
});

describe("a delete", () => {
  const route = "DELETE /statuses/{id}";
  const remove = ({ data, one }: Server) =>
    removeStatus(data, one, () => data.api.DELETE("/statuses/{id}", { params: { path: { id: one } } }));

  test("takes the post off the home timeline, the thread and its own screen at once", async () => {
    const s = server();
    await load(s);
    const write = waiting(s.fake, route);
    const done = remove(s);
    const now = shown(s);
    expect([now.ids, now.alone]).toEqual([{ home: [s.two, s.zero], thread: [] }, undefined]);
    write.release();
    await done;
    expect(shown(s).ids).toEqual({ home: [s.two, s.zero], thread: [] });
  });

  test("keeps the post off through a refetch that raced it and still held the post", async () => {
    const s = server();
    await load(s);
    const write = waiting(s.fake, route);
    const done = remove(s);
    const answerReads = await raceRefetch(s);
    write.release();
    await done;
    await answerReads();
    expect(shown(s).ids).toEqual({ home: [s.two, s.zero], thread: [] });
  });

  test("keeps the post off through a refetch begun after it", async () => {
    const s = server();
    await load(s);
    await remove(s);
    await s.data.queryClient.refetchQueries();
    expect(shown(s).ids).toEqual({ home: [s.two, s.zero], thread: [] });
  });

  test("failed, puts the post back in every list and view, and rejects", async () => {
    const s = server();
    await load(s);
    const write = waiting(s.fake, route, true);
    const done = remove(s);
    write.release();
    expect(await rejects(done)).toBe(true);
    const seen = shown(s);
    expect([seen.ids, seen.alone?.id]).toEqual([{ home: [s.two, s.one, s.zero], thread: [s.one] }, s.one]);
  });
});
