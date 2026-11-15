// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import { accountStatusesQuery } from "./accounts";
import { prependHomeStatuses } from "./arrivals";
import { useStatus } from "./compose";
import { markStatus } from "./interactions";
import { notificationsQuery } from "./notifications";
import { changeStatus, receiveGone, removeStatus, shadowed, shadowStatus, useShadow } from "./shadow";
import type { Status } from "./statuses";
import { fakeAppData, json, page, status } from "./test/fake";
import { probe } from "./test/probe";
import { cacheReply, statusContextQuery, statusQuery, useThread, type StatusContext } from "./thread";
import { homeTimelineQuery, useHomeTimeline } from "./timeline";

const favourite = (s: Status): Status => ({ ...s, favourited: true, favourites_count: 1 });

function cached(items: Status[]) {
  const { data } = fakeAppData(() => new Response(null, { status: 500 }));
  const key = homeTimelineQuery(data).queryKey;
  data.queryClient.setQueryData(key, { pages: [{ items }], pageParams: [undefined] });
  const read = () =>
    (data.queryClient.getQueryData(key)?.pages[0]?.items ?? []).flatMap((s) => {
      const shown = shadowed(data, s);
      return shown === undefined ? [] : [shown];
    });
  return { data, read };
}

describe("a change", () => {
  test("shows on the status on a timeline, and on a boost and a quote of it, leaving the cache as it was", () => {
    const one = status("1");
    const { data, read } = cached([
      one,
      status("b", { boost: status("1") }),
      status("q", { quote: { state: "accepted", quoted_status: status("1") } }),
    ]);
    shadowStatus(data, "1", favourite);
    const [alone, boost, quote] = read();
    expect(alone?.favourited).toBe(true);
    expect(boost?.boost?.favourited).toBe(true);
    expect(boost?.favourited).toBeUndefined();
    expect(quote?.quote?.quoted_status?.favourited).toBe(true);
    expect(one.favourited).toBeUndefined();
  });

  test("keeps a status without a change as it was", () => {
    const { data, read } = cached([status("1")]);
    const before = read()[0];
    shadowStatus(data, "9", favourite);
    expect(read()[0]).toBe(before);
  });

  test("gives the hooks a new useShadow() once it is laid or settled, and the same one while nothing changes", () => {
    // React Compiler memoizes what a hook works out of useShadow() on that function's identity alone.
    const { data } = cached([status("1")]);
    const before = probe(data, useShadow);
    expect(probe(data, useShadow)).toBe(before);
    const shadow = shadowStatus(data, "1", favourite);
    const laid = probe(data, useShadow);
    expect(laid).not.toBe(before);
    shadow.confirm();
    expect(probe(data, useShadow)).not.toBe(laid);
  });

  test("taken off, shows every copy again as it was", () => {
    const { data, read } = cached([status("1")]);
    shadowStatus(data, "1", favourite).discard();
    expect(read()[0]?.favourited).toBeUndefined();
  });

  test("the server took stays on through a fetch begun before, and gives way to one begun after", async () => {
    const answers: Response[] = [];
    const { data } = fakeAppData(() => answers.shift() ?? json(500, {}));
    const query = homeTimelineQuery(data);
    const read = () => {
      const s = data.queryClient.getQueryData(query.queryKey)?.pages[0]?.items[0];
      return s && shadowed(data, s);
    };
    answers.push(json(200, page(["1"])));
    await data.queryClient.infiniteQuery(query);
    const write = shadowStatus(data, "1", favourite);
    // A fetch begun while the write is in flight answers without it.
    answers.push(json(200, page(["1"])));
    const before = data.queryClient.refetchQueries({ queryKey: query.queryKey });
    write.confirm();
    await before;
    expect(read()?.favourited).toBe(true);
    // A fetch begun after shows what the server says, even when its answer did not change.
    answers.push(json(200, page(["1"])));
    await data.queryClient.refetchQueries({ queryKey: query.queryKey });
    expect(read()?.favourited).toBeUndefined();
  });

  test("the server took stays on when a post goes in above it on the home timeline", () => {
    const { data, read } = cached([status("2"), status("1"), status("4")]);
    shadowStatus(data, "1", favourite).confirm();
    prependHomeStatuses(data, [status("3")]);
    expect(read().map((s) => [s.id, s.favourited])).toEqual([
      ["3", undefined],
      ["2", undefined],
      ["1", true],
      ["4", undefined],
    ]);
  });

  test("the server took stays on when a reply goes in above it in a thread", () => {
    const { data } = cached([]);
    const key = statusContextQuery(data, "0").queryKey;
    data.queryClient.setQueryData(key, { ancestors: [], descendants: [status("1"), status("4")] });
    shadowStatus(data, "1", favourite).confirm();
    cacheReply(data.queryClient, status("r", { in_reply_to_id: "0" }));
    const descendants = data.queryClient.getQueryData<StatusContext>(key)?.descendants ?? [];
    expect(descendants.map((s) => [s.id, shadowed(data, s)?.favourited])).toEqual([
      ["r", undefined],
      ["1", true],
      ["4", undefined],
    ]);
  });

  test("the server took gives way on a post a write by hand brings as the server has it now", () => {
    const { data, read } = cached([status("2"), status("1"), status("4")]);
    shadowStatus(data, "1", favourite).confirm();
    prependHomeStatuses(data, [status("b", { boost: status("1", { favourites_count: 5 }) })]);
    expect(read()[0]?.boost?.favourites_count).toBe(5);
  });
});

describe("a status gone", () => {
  test("leaves a timeline with the boosts of it, and a quote of it shows it deleted", () => {
    const { data, read } = cached([
      status("2"),
      status("1"),
      status("b", { boost: status("1") }),
      status("q", { quote: { state: "accepted", quoted_status: status("1") } }),
      status("bq", { boost: status("q2", { quote: { state: "accepted", quoted_status: status("1") } }) }),
    ]);
    receiveGone(data, "1");
    const items = read();
    expect(items.map((s) => s.id)).toEqual(["2", "q", "bq"]);
    expect(items[1]?.quote).toEqual({ state: "deleted" });
    expect(items[2]?.boost?.quote).toEqual({ state: "deleted" });
  });

  test("leaves a thread, an account's posts and the notifications about it, and the status alone is read again", () => {
    const { data } = cached([]);
    const { queryClient, queries } = data;
    const single = queries.queryOptions("get", "/statuses/{id}", { params: { path: { id: "1" } } }).queryKey;
    const posts = accountStatusesQuery(data, "a", "posts").queryKey;
    const notifications = notificationsQuery(data, "all").queryKey;
    queryClient.setQueryData(single, status("1"));
    queryClient.setQueryData(posts, { pages: [{ items: [status("1"), status("3")] }], pageParams: [undefined] });
    queryClient.setQueryData(notifications, {
      pages: [{ items: [{ id: "n1", type: "favourite", created_at: "", status: status("1") }] }],
      pageParams: [undefined],
    });
    receiveGone(data, "1");
    const shown = (s: Status | undefined) => s && shadowed(data, s);
    expect(queryClient.getQueryState(single)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryData(posts)?.pages[0]?.items.map((s) => shown(s)?.id)).toEqual([undefined, "3"]);
    expect(shown(queryClient.getQueryData(notifications)?.pages[0]?.items[0]?.status)).toBeUndefined();
  });

  test("stays gone in what a fetch begun before brings", async () => {
    let answer: (response: Response) => void = () => {};
    const { data } = fakeAppData(
      () =>
        new Promise<Response>((resolve) => {
          answer = resolve;
        }),
    );
    const query = homeTimelineQuery(data);
    const fetched = data.queryClient.infiniteQuery(query);
    await Bun.sleep(0);
    receiveGone(data, "1");
    answer(json(200, page(["2", "1"])));
    await fetched;
    const items = data.queryClient.getQueryData(query.queryKey)?.pages[0]?.items ?? [];
    expect(items.filter((s) => shadowed(data, s) !== undefined).map((s) => s.id)).toEqual(["2"]);
  });
});

// ---- Through the hooks the screens read ----------------------------------------------------------------------------

const post = () =>
  status("1", { favourited: false, favourites_count: 0, boosted: false, boosts_count: 0, reactions: [] });

/** What the server holds before any write; its reads answer it, after a write too. */
const before: Record<string, unknown> = {
  "/api/v1/timelines/home": { items: [status("2"), post()] },
  "/api/v1/statuses/0": status("0"),
  "/api/v1/statuses/0/context": { ancestors: [], descendants: [post()] },
  "/api/v1/statuses/1": post(),
};

/**
 * A server whose reads answer `before` and whose writes wait for the test's answer. Reads it is told to hold wait
 * until released: a refetch that races a write.
 */
function server() {
  // The answer of each write that reached the server, resolved by answerWrite; and the next one, once it arrives.
  const writes: ((response: Response) => void)[] = [];
  let nextWrite = Promise.withResolvers<undefined>();
  let held: { count: number; arrived: () => void; release: Promise<undefined> } | undefined;
  const { data } = fakeAppData(async (request) => {
    if (request.method !== "GET") return send();
    const hold = held;
    if (hold !== undefined) {
      hold.count -= 1;
      if (hold.count === 0) hold.arrived();
      await hold.release;
    }
    const body = before[new URL(request.url).pathname];
    return body === undefined ? json(404, { type: "about:blank", status: 404 }) : json(200, body);
  });
  /** A write reaching the server: it waits for the answer the test gives answerWrite. */
  function send(): Promise<Response> {
    const { promise, resolve } = Promise.withResolvers<Response>();
    writes.push(resolve);
    nextWrite.resolve(undefined);
    return promise;
  }
  /** Holds the next `count` reads; `arrived` resolves once all of them reached the server. */
  const holdReads = (count: number) => {
    const arrived = Promise.withResolvers<undefined>();
    const release = Promise.withResolvers<undefined>();
    held = {
      count,
      arrived: () => {
        arrived.resolve(undefined);
      },
      release: release.promise,
    };
    return {
      arrived: arrived.promise,
      release: () => {
        held = undefined;
        release.resolve(undefined);
      },
    };
  };
  /** Answers the oldest write waiting, once one reached the server. */
  const answerWrite = async (response: Response) => {
    while (writes.length === 0) {
      await nextWrite.promise;
      nextWrite = Promise.withResolvers<undefined>();
    }
    writes.shift()?.(response);
  };
  /** A write the test sends by hand (an edit, a delete): it rejects when the server answers a failure. */
  const sendByHand = async () => {
    const response = await send();
    if (!response.ok) throw new Error(`the server answered ${response.status}`);
    return response;
  };
  return { data, holdReads, answerWrite, sendByHand };
}

type Server = ReturnType<typeof server>;

/** Fills the cache as the home timeline, the thread of post 0 and the screen of post 1 load it. */
async function load({ data }: Server) {
  await Promise.all([
    data.queryClient.infiniteQuery(homeTimelineQuery(data)),
    data.queryClient.query(statusQuery(data, "0")),
    data.queryClient.query(statusContextQuery(data, "0")),
    data.queryClient.query(statusQuery(data, "1")),
  ]);
}

/** Post 1 as each screen shows it: on the home timeline, in the thread of post 0, and alone; and the IDs of the lists. */
function shown({ data }: Server) {
  const home = probe(data, () => useHomeTimeline().statuses);
  const thread = probe(data, () => useThread("0").context?.descendants ?? []);
  const alone = probe(data, () => useStatus("1").status);
  return {
    home: home.find((s) => s.id === "1"),
    thread: thread.find((s) => s.id === "1"),
    alone,
    ids: { home: home.map((s) => s.id), thread: thread.map((s) => s.id) },
  };
}

/** Refetches every query and holds its reads at the server; the function returned lets them answer the old data. */
async function raceRefetch(s: Server) {
  const reads = s.holdReads(4);
  const refetched = s.data.queryClient.refetchQueries();
  await reads.arrived;
  return async () => {
    reads.release();
    await refetched;
  };
}

const rejects = (p: Promise<unknown>) =>
  p.then(
    () => false,
    () => true,
  );

const failure = () => json(500, { type: "about:blank", title: "Internal", status: 500 });

type Write = {
  readonly name: string;
  readonly start: (s: Server) => Promise<unknown>;
  readonly answer: () => Response;
  readonly shows: (s: Status | undefined) => boolean;
};

const changes: Write[] = [
  {
    name: "a favourite",
    start: (s) => markStatus(s.data, "1", { type: "favourite", on: true }),
    answer: () => json(200, { ...post(), favourited: true, favourites_count: 1 }),
    shows: (s) => s?.favourited === true && s.favourites_count === 1,
  },
  {
    name: "a boost",
    start: (s) => markStatus(s.data, "1", { type: "boost", on: true }),
    answer: () => json(200, { ...post(), boosted: true, boosts_count: 1 }),
    shows: (s) => s?.boosted === true && s.boosts_count === 1,
  },
  {
    name: "an edit",
    start: (s) =>
      changeStatus(s.data, "1", { change: (p) => ({ ...p, content: "<p>edited</p>" }), send: s.sendByHand }),
    answer: () => json(200, { ...post(), content: "<p>edited</p>" }),
    shows: (s) => s?.content === "<p>edited</p>",
  },
];

const everywhere = (shows: Write["shows"], seen: ReturnType<typeof shown>) => ({
  home: shows(seen.home),
  thread: shows(seen.thread),
  alone: shows(seen.alone),
});

describe.each(changes)("$name", ({ start, answer, shows }) => {
  test("shows at once on the home timeline, in a thread and on the post alone, and stays once answered", async () => {
    const s = server();
    await load(s);
    const done = start(s);
    expect(everywhere(shows, shown(s))).toEqual({ home: true, thread: true, alone: true });
    await s.answerWrite(answer());
    await done;
    expect(everywhere(shows, shown(s))).toEqual({ home: true, thread: true, alone: true });
  });

  test("survives a refetch that raced it and answered the post as it was", async () => {
    const s = server();
    await load(s);
    const done = start(s);
    const answerReads = await raceRefetch(s);
    await s.answerWrite(answer());
    await done;
    await answerReads();
    expect(everywhere(shows, shown(s))).toEqual({ home: true, thread: true, alone: true });
  });

  test("failed, shows every copy as it was again, and rejects", async () => {
    const s = server();
    await load(s);
    const done = start(s);
    await s.answerWrite(failure());
    expect(await rejects(done)).toBe(true);
    const seen = shown(s);
    expect(everywhere(shows, seen)).toEqual({ home: false, thread: false, alone: false });
    expect([seen.home, seen.thread, seen.alone]).toEqual([post(), post(), post()]);
  });
});

describe("a delete", () => {
  const remove = (s: Server) => removeStatus(s.data, "1", s.sendByHand);
  const deleted = () => new Response(null, { status: 204 });

  test("takes the post off the home timeline, the thread and its own screen at once", async () => {
    const s = server();
    await load(s);
    const done = remove(s);
    const now = shown(s);
    expect([now.ids, now.alone]).toEqual([{ home: ["2"], thread: [] }, undefined]);
    await s.answerWrite(deleted());
    await done;
    expect(shown(s).ids).toEqual({ home: ["2"], thread: [] });
  });

  test("keeps the post off through a refetch that raced it and still held the post", async () => {
    const s = server();
    await load(s);
    const done = remove(s);
    const answerReads = await raceRefetch(s);
    await s.answerWrite(deleted());
    await done;
    await answerReads();
    expect(shown(s).ids).toEqual({ home: ["2"], thread: [] });
  });

  test("keeps the post off through a refetch begun after it", async () => {
    const s = server();
    await load(s);
    const done = remove(s);
    await s.answerWrite(deleted());
    await done;
    await s.data.queryClient.refetchQueries();
    expect(shown(s).ids).toEqual({ home: ["2"], thread: [] });
  });

  test("failed, puts the post back in every list and view, and rejects", async () => {
    const s = server();
    await load(s);
    const done = remove(s);
    await s.answerWrite(failure());
    expect(await rejects(done)).toBe(true);
    const seen = shown(s);
    expect([seen.ids, seen.alone?.id]).toEqual([{ home: ["2", "1"], thread: ["1"] }, "1"]);
  });
});
