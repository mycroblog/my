// SPDX-License-Identifier: AGPL-3.0-only

// The fake API itself (ADR-0171 §5): what a test scripts on it, its stream, the seed in place of the scenario, what a
// visitor is answered, and the failure of an operation it does not implement. Unit tests, without a DOM.

import { describe, expect, test } from "bun:test";
import type { components } from "@mycroblog/mykit";
import { createFakeApi, fakeOrigin, scenario, UnimplementedError, type FakeApi, type FakeSeed } from "./index";

type Status = components["schemas"]["Status"];
type StatusPage = { items: Status[]; next_cursor?: string };

/** A request of the client API, as the app's client sends it. */
const request = (method: string, path: string, body?: unknown, headers: Record<string, string> = {}) =>
  new Request(`${fakeOrigin}/api/v1${path}`, {
    method,
    headers: { ...(body === undefined ? {} : { "Content-Type": "application/json" }), ...headers },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });

/** The fake's answer to a request: its status, its media type, and its JSON body. */
type Answered<T> = { status: number; type: string | null; body: T };

/** The fake's answer to a request. */
async function ask(fake: FakeApi, method: string, path: string, body?: unknown): Promise<Answered<unknown>> {
  const response = await fake.serve(request(method, path, body));
  const text = await response.text();
  return {
    status: response.status,
    type: response.headers.get("Content-Type"),
    body: text === "" ? undefined : (JSON.parse(text) as unknown),
  };
}

/** A server where the reader follows sam, who posted `post`. */
const small: Partial<FakeSeed> = {
  people: scenario.people,
  follows: [{ follower: "yuki", followee: "sam" }],
  posts: [{ key: "post", author: "sam", text: "The crumb is finally open.", visibility: "public", language: "en" }],
};

/** The post of `small` as the reader is answered it. */
const post = async (fake: FakeApi) =>
  ((await ask(fake, "GET", `/statuses/${fake.idOf("post")}`)) as Answered<Status>).body;

/** Whether `promise` settled by the next turn of the event loop. */
const settled = (promise: Promise<unknown>) =>
  Promise.race([
    promise.then(() => true),
    new Promise<boolean>((resolve) => {
      setTimeout(() => {
        resolve(false);
      }, 0);
    }),
  ]);

describe("a script", () => {
  test("that names nothing counts and keeps the requests of its operation, and of no other", async () => {
    const fake = createFakeApi({ seed: small });
    const id = fake.idOf("post");
    const favourites = fake.script("POST /statuses/{id}/favourite");
    await ask(fake, "POST", `/statuses/${id}/favourite`);
    await ask(fake, "POST", `/statuses/${id}/bookmark`);
    await fake.serve(request("PUT", `/statuses/${id}/reaction`, { emoji: "🎉" }, { "X-Test": "1" }));
    expect(favourites.seen.map((s) => s.url.pathname)).toEqual([`/api/v1/statuses/${id}/favourite`]);
    const reactions = fake.script("PUT /statuses/{id}/reaction");
    await fake.serve(request("PUT", `/statuses/${id}/reaction`, { emoji: "👍" }, { "X-Test": "2" }));
    expect(reactions.seen.map((s) => [s.body, s.headers.get("X-Test")])).toEqual([[{ emoji: "👍" }, "2"]]);
  });

  test("resolves arrived(n) with the nth request, already here or still to come", async () => {
    const fake = createFakeApi({ seed: small });
    const reads = fake.script("GET /timelines/home");
    const second = reads.arrived(2);
    await ask(fake, "GET", "/timelines/home");
    const waits = await settled(second);
    expect(waits).toBe(false);
    await ask(fake, "GET", "/timelines/home?limit=5");
    const [first, later] = [await reads.arrived(), await second];
    expect([first.url.search, later.url.search]).toEqual(["", "?limit=5"]);
  });

  test("held at the request, the fake does nothing until it is released", async () => {
    const fake = createFakeApi({ seed: small });
    const write = fake.script("POST /statuses/{id}/favourite", { hold: "request" });
    const answer = ask(fake, "POST", `/statuses/${fake.idOf("post")}/favourite`) as Promise<Answered<Status>>;
    await write.arrived();
    const [answered, before] = [await settled(answer), await post(fake)];
    expect([answered, before.favourited]).toEqual([false, false]);
    write.release();
    const [after, now] = [await answer, await post(fake)];
    expect([after.body.favourited, now.favourited]).toEqual([true, true]);
  });

  test("held at the response, the fake did what was asked, and served(n) says when", async () => {
    const fake = createFakeApi({ seed: small });
    const write = fake.script("POST /statuses/{id}/favourite", { hold: "response" });
    const answer = ask(fake, "POST", `/statuses/${fake.idOf("post")}/favourite`) as Promise<Answered<Status>>;
    await write.served();
    const [answered, meanwhile] = [await settled(answer), await post(fake)];
    expect([answered, meanwhile.favourited]).toEqual([false, true]);
    write.release();
    const after = await answer;
    expect(after.body.favourited).toBe(true);
  });

  test("released, it holds no later request", async () => {
    const fake = createFakeApi({ seed: small });
    const read = fake.script("GET /timelines/home", { hold: "request" });
    read.release();
    const answer = (await ask(fake, "GET", "/timelines/home")) as Answered<StatusPage>;
    expect(answer.body.items).toHaveLength(1);
  });

  test("fails a request with a problem of a status, and the fake does not do it", async () => {
    const fake = createFakeApi({ seed: small });
    fake.script("POST /statuses/{id}/favourite", { fail: 422 });
    const refused = await ask(fake, "POST", `/statuses/${fake.idOf("post")}/favourite`);
    expect(refused).toEqual({
      status: 422,
      type: "application/problem+json",
      body: { type: "about:blank", title: "The request failed", status: 422 },
    });
    const now = await post(fake);
    expect(now.favourited).toBe(false);
  });

  test("fails a request with the problem given", async () => {
    const fake = createFakeApi({ seed: small });
    const problem = { type: "/api/v1/problems/status-rejected", title: "Refused", status: 422, detail: "Too long." };
    fake.script("POST /statuses", { fail: problem });
    const refused = await ask(fake, "POST", "/statuses", { text: "hi", visibility: "public" });
    expect([refused.status, refused.body]).toEqual([422, problem]);
  });

  test("fails a request as one that got no answer", async () => {
    const fake = createFakeApi({ seed: small });
    fake.script("GET /timelines/home", { fail: "network" });
    const failure: unknown = await fake.serve(request("GET", "/timelines/home")).catch((e: unknown) => e);
    expect(failure).toBeInstanceOf(TypeError);
  });

  test("failed and held at the response, the fake did what was asked before the failure comes", async () => {
    const fake = createFakeApi({ seed: small });
    const write = fake.script("POST /statuses/{id}/favourite", { hold: "response", fail: 500 });
    const answer = ask(fake, "POST", `/statuses/${fake.idOf("post")}/favourite`);
    await write.served();
    write.release();
    const [failed, now] = [await answer, await post(fake)];
    expect([failed.status, now.favourited]).toEqual([500, true]);
  });

  test("changes the body of an answer, and nothing the fake holds", async () => {
    const fake = createFakeApi({ seed: small });
    fake.script("GET /statuses/{id}", { times: 1, change: (status) => ({ ...status, favourites_count: 5 }) });
    const [changed, again] = [await post(fake), await post(fake)];
    expect([changed.favourites_count, again.favourites_count]).toEqual([5, 0]);
  });

  test("acts on its first `times` requests, and still keeps the later ones", async () => {
    const fake = createFakeApi({ seed: small });
    const reads = fake.script("GET /timelines/home", { times: 2, fail: 503 });
    const statuses = [];
    for (let i = 0; i < 3; i += 1) statuses.push((await ask(fake, "GET", "/timelines/home")).status);
    expect(statuses).toEqual([503, 503, 200]);
    expect(reads.seen).toHaveLength(3);
  });

  test("of several of one operation, the first that still acts does; one that only counts leaves it to the next", async () => {
    const fake = createFakeApi({ seed: small });
    const counted = fake.script("GET /timelines/home");
    fake.script("GET /timelines/home", { times: 1, fail: 500 });
    fake.script("GET /timelines/home", { fail: 503 });
    const statuses = [];
    for (let i = 0; i < 2; i += 1) statuses.push((await ask(fake, "GET", "/timelines/home")).status);
    expect(statuses).toEqual([500, 503]);
    expect(counted.seen).toHaveLength(2);
  });

  test("ended, it lets go of what it holds, and the later requests are answered as without it", async () => {
    const fake = createFakeApi({ seed: small });
    const read = fake.script("GET /timelines/home", { hold: "request", fail: 500 });
    const held = ask(fake, "GET", "/timelines/home");
    await read.arrived();
    read.end();
    const [first, later] = [await held, (await ask(fake, "GET", "/timelines/home")) as Answered<StatusPage>];
    expect([first.status, later.status, read.seen.length]).toEqual([500, 200, 1]);
  });
});

describe("the stream", () => {
  /** The next frame of the open stream `response`, as text. */
  const frame = async (reader: ReadableStreamDefaultReader<Uint8Array>) => {
    const { value } = await reader.read();
    return new TextDecoder().decode(value);
  };

  test("stays open and silent until an event is sent, which every open stream gets as the server writes it", async () => {
    const fake = createFakeApi({ seed: small });
    const open = async () => {
      const response = await fake.serve(request("GET", "/streaming"));
      expect(response.headers.get("Content-Type")).toBe("text/event-stream");
      if (response.body === null) throw new Error("test: the stream has no body");
      return response.body.getReader();
    };
    const [one, other] = [await open(), await open()];
    const next = frame(one);
    const silent = await settled(next);
    expect(silent).toBe(false);
    fake.send({ type: "delete", id: "gone" });
    const frames = [await next, await frame(other)];
    expect(frames).toEqual(Array.from({ length: 2 }, () => 'event: delete\ndata: {"id":"gone"}\n\n'));
  });

  test("a new post of the fake's goes to the home timeline and on the stream", async () => {
    const fake = createFakeApi({ seed: small });
    const response = await fake.serve(request("GET", "/streaming"));
    if (response.body === null) throw new Error("test: the stream has no body");
    const reader = response.body.getReader();
    const [made] = fake.newPosts(1);
    const sent = await frame(reader);
    expect(sent.startsWith("event: update\ndata: ")).toBe(true);
    expect((JSON.parse(sent.slice(sent.indexOf("data: ") + 6)) as Status).id).toBe(made?.id ?? "");
    const home = (await ask(fake, "GET", "/timelines/home")) as Answered<StatusPage>;
    expect(home.body.items.map((s) => s.id)).toEqual([made?.id ?? "", fake.idOf("post")]);
  });

  test("a stream whose request is given up ends", async () => {
    const fake = createFakeApi({ seed: small });
    const gone = new AbortController();
    const response = await fake.serve(new Request(`${fakeOrigin}/api/v1/streaming`, { signal: gone.signal }));
    if (response.body === null) throw new Error("test: the stream has no body");
    const reader = response.body.getReader();
    gone.abort();
    const { done } = await reader.read();
    expect(done).toBe(true);
  });
});

describe("the seed", () => {
  test("without one, the fake holds the scenario", async () => {
    const fake = createFakeApi();
    const [home, lists] = [
      (await ask(fake, "GET", "/timelines/home")) as Answered<StatusPage>,
      (await ask(fake, "GET", "/lists")) as Answered<{ items: { name: string }[] }>,
    ];
    expect(home.body.items).toHaveLength(20);
    expect(lists.body.items.map((l) => l.name)).toContain("Bread and coffee");
    expect(fake.idOf("sam-bread")).not.toBe("");
  });

  test("a seed replaces the scenario: the lists it names, and nothing else", async () => {
    const fake = createFakeApi({ seed: small });
    const [home, lists, drafts, announcements] = [
      (await ask(fake, "GET", "/timelines/home")) as Answered<StatusPage>,
      (await ask(fake, "GET", "/lists")) as Answered<{ items: unknown[] }>,
      (await ask(fake, "GET", "/drafts")) as Answered<{ items: unknown[] }>,
      (await ask(fake, "GET", "/announcements")) as Answered<{ items: unknown[] }>,
    ];
    expect(home.body.items.map((s) => s.id)).toEqual([fake.idOf("post")]);
    expect([lists.body.items, drafts.body.items, announcements.body.items]).toEqual([[], [], []]);
    expect(() => fake.idOf("sam-bread")).toThrow("no post or draft sam-bread");
  });

  test("each fake holds its own state", async () => {
    const [one, other] = [createFakeApi({ seed: small }), createFakeApi({ seed: small })];
    await ask(one, "POST", `/statuses/${one.idOf("post")}/favourite`);
    const [changed, untouched] = [await post(one), await post(other)];
    expect([changed.favourited, untouched.favourited]).toEqual([true, false]);
  });

  test("the reader is the person the options name, with the rights of their roles", async () => {
    const credentials = async (viewer: string) =>
      (
        (await ask(createFakeApi({ viewer }), "GET", "/accounts/verify_credentials")) as Answered<{
          acct: string;
          permissions: string[];
        }>
      ).body;
    const [yuki, marco] = [await credentials("yuki"), await credentials("marco")];
    expect([yuki.acct, yuki.permissions]).toEqual(["yuki", []]);
    expect(marco.permissions).toContain("manage_reports");
    expect(() => createFakeApi({ viewer: "nobody" })).toThrow("no person nobody");
  });
});

describe("a visitor", () => {
  /** A server where yuki favourited, boosted, bookmarked and reacted, and voted in a poll. */
  const marked: Partial<FakeSeed> = {
    people: scenario.people,
    posts: [
      { key: "post", author: "sam", text: "The crumb is finally open.", visibility: "public", language: "en" },
      {
        key: "asks",
        author: "sam",
        text: "Which loaf?",
        visibility: "public",
        language: "en",
        poll: { options: ["Sourdough", "Rye"], days: 1 },
      },
      { key: "own", author: "yuki", text: "A walk.", visibility: "public", language: "en", pinned: true },
      { key: "kept", author: "sam", text: "For my followers.", visibility: "private", language: "en" },
    ],
    interactions: [
      { kind: "favourite", by: "yuki", post: "post" },
      { kind: "boost", by: "yuki", post: "post" },
      { kind: "bookmark", by: "yuki", post: "post" },
      { kind: "reaction", by: "yuki", post: "own", emoji: "🎉" },
      { kind: "vote", by: "yuki", post: "asks", choices: [1] },
    ],
  };
  /** The members of a status that are a reader's own. */
  const own = ["favourited", "boosted", "bookmarked", "muted", "pinned"] as const;

  test("is answered what anyone may read, with nothing of a reader's own", async () => {
    const [reader, visitor] = [createFakeApi({ seed: marked }), createFakeApi({ seed: marked, visitor: true })];
    const read = async (fake: FakeApi, key: string) =>
      ((await ask(fake, "GET", `/statuses/${fake.idOf(key)}`)) as Answered<Status>).body;
    const [mine, theirs] = [await read(reader, "post"), await read(visitor, "post")];
    expect(own.slice(0, 4).map((member) => mine[member])).toEqual([true, true, true, false]);
    expect(own.filter((member) => member in theirs)).toEqual([]);
    expect([theirs.favourites_count, theirs.boosts_count]).toEqual([1, 1]);
    const [voted, poll] = [(await read(reader, "asks")).poll, (await read(visitor, "asks")).poll];
    expect([voted?.voted, voted?.own_votes]).toEqual([true, [1]]);
    expect(poll === undefined || "voted" in poll || "own_votes" in poll).toBe(false);
    expect(poll?.votes_count).toBe(1);
    const [pinned, reacted] = [await read(reader, "own"), await read(visitor, "own")];
    expect([pinned.pinned, pinned.reactions]).toEqual([true, [{ name: "🎉", count: 1, me: true }]]);
    expect(reacted.reactions).toEqual([{ name: "🎉", count: 1 }]);
  });

  test("reads the public timelines and no post kept to followers", async () => {
    const visitor = createFakeApi({ seed: marked, visitor: true });
    const local = (await ask(visitor, "GET", "/timelines/local")) as Answered<StatusPage>;
    expect(local.body.items.map((s) => s.id)).toEqual(["own", "asks", "post"].map(visitor.idOf));
    expect(local.body.items.flatMap((s) => own.filter((member) => member in s))).toEqual([]);
    const kept = await ask(visitor, "GET", `/statuses/${visitor.idOf("kept")}`);
    expect(kept.status).toBe(404);
  });

  test("is refused what needs a session, with a 401", async () => {
    const visitor = createFakeApi({ seed: marked, visitor: true });
    const refused = [];
    for (const [method, path] of [
      ["GET", "/timelines/home"],
      ["GET", "/notifications"],
      ["POST", `/statuses/${visitor.idOf("post")}/favourite`],
      // The paths of these read like an account's, a status's or a hashtag's, which a visitor may read.
      ["GET", "/accounts/verify_credentials"],
      ["GET", `/accounts/relationships?id=${visitor.accountId("sam")}`],
      ["GET", "/accounts/search?q=sam"],
      ["GET", "/accounts/suggestions"],
      ["GET", "/statuses/search?q=crumb"],
      ["GET", "/tags/search?q=ba"],
    ] as const)
      refused.push((await ask(visitor, method, path)).status);
    expect(refused).toEqual(refused.map(() => 401));
    const open = [];
    for (const path of [
      `/accounts/${visitor.accountId("sam")}`,
      "/accounts/lookup?acct=sam",
      "/tags/baking",
      "/instance",
    ])
      open.push((await ask(visitor, "GET", path)).status);
    expect(open).toEqual([200, 200, 200, 200]);
    const now = (await ask(visitor, "GET", `/statuses/${visitor.idOf("post")}`)) as Answered<Status>;
    expect(now.body.favourites_count).toBe(1);
  });
});

describe("an operation the fake does not implement", () => {
  test("fails by name, and is kept among those it refused", async () => {
    const fake = createFakeApi();
    const failure: unknown = await fake.serve(request("GET", "/admin/reports")).catch((e: unknown) => e);
    expect(failure).toBeInstanceOf(UnimplementedError);
    expect((failure as UnimplementedError).operation).toBe("GET /api/v1/admin/reports");
    expect((failure as Error).message).toBe("fake: GET /api/v1/admin/reports is no operation the fake API implements");
    expect(fake.unimplemented).toEqual(["GET /api/v1/admin/reports"]);
  });

  test("fails for a method the operation's path does not have", async () => {
    const fake = createFakeApi();
    const failure: unknown = await fake.serve(request("DELETE", "/instance")).catch((e: unknown) => e);
    expect(failure).toBeInstanceOf(UnimplementedError);
  });

  test("can still be scripted: the script counts its requests, and may answer for the fake", async () => {
    const fake = createFakeApi();
    const reports = fake.script("GET /admin/reports", { fail: 403 });
    const refused = await ask(fake, "GET", "/admin/reports");
    expect([refused.status, reports.seen.length, fake.unimplemented]).toEqual([403, 1, []]);
  });
});

describe("what the reader writes", () => {
  test("a post made twice with one Idempotency-Key is made once", async () => {
    const fake = createFakeApi({ seed: small });
    const make = async (key: string) => {
      const response = await fake.serve(
        request("POST", "/statuses", { text: "hi", visibility: "public" }, { "Idempotency-Key": key }),
      );
      return ((await response.json()) as Status).id;
    };
    const [first, retried, other] = [await make("k1"), await make("k1"), await make("k2")];
    expect([retried === first, other === first]).toEqual([true, false]);
    const home = (await ask(fake, "GET", "/timelines/home")) as Answered<StatusPage>;
    expect(home.body.items.map((s) => s.id)).toEqual([other, first, fake.idOf("post")]);
  });

  test("a reply counts on the post it answers until it is deleted, and its source is what was written", async () => {
    const fake = createFakeApi({ seed: small });
    const reply = (await ask(fake, "POST", "/statuses", {
      text: "@sam Which flour?",
      spoiler_text: "Bread",
      visibility: "public",
      in_reply_to_id: fake.idOf("post"),
    })) as Answered<Status>;
    const counted = await post(fake);
    const source = await ask(fake, "GET", `/statuses/${reply.body.id}/source`);
    expect([reply.status, counted.replies_count]).toEqual([201, 1]);
    expect(source.body).toEqual({
      id: reply.body.id,
      text: "@sam Which flour?",
      spoiler_text: "Bread",
      language: "en",
    });
    const deleted = await ask(fake, "DELETE", `/statuses/${reply.body.id}`);
    const after = await post(fake);
    expect([deleted.status, after.replies_count]).toEqual([204, 0]);
  });

  test("a reply to a post whose author approves the reader's replies waits, and counts nowhere", async () => {
    const fake = createFakeApi({ approvals: true });
    const asks = fake.idOf("layla-asks");
    const reply = (await ask(fake, "POST", "/statuses", {
      text: "hi",
      visibility: "public",
      in_reply_to_id: asks,
    })) as Answered<Status>;
    const parent = (await ask(fake, "GET", `/statuses/${asks}`)) as Answered<Status>;
    expect([reply.body.approval, parent.body.replies_count]).toEqual(["pending", 0]);
  });

  test("an edit that takes the link out of a post takes its card out", async () => {
    const fake = createFakeApi({ viewer: "layla" });
    const id = fake.idOf("layla-jokes");
    const before = (await ask(fake, "GET", `/statuses/${id}`)) as Answered<Status>;
    const after = (await ask(fake, "PUT", `/statuses/${id}`, {
      text: "The hardest part is always the jokes.",
    })) as Answered<Status>;
    expect([before.body.card?.site_name, "card" in after.body]).toEqual(["Layla's notes", false]);
  });

  test("the reader's account and a post have the IDs the fake names", async () => {
    const fake = createFakeApi({ seed: small });
    const sam = (await ask(fake, "GET", "/accounts/lookup?acct=sam")) as Answered<{ id: string }>;
    const read = await post(fake);
    expect([sam.body.id, read.id, read.account.id]).toEqual([
      fake.accountId("sam"),
      fake.idOf("post"),
      fake.accountId("sam"),
    ]);
  });

  test("the reader has not read as many of their newest notifications as the options say", async () => {
    const marker = async (unreadNotifications?: number) => {
      const fake = createFakeApi(unreadNotifications === undefined ? {} : { unreadNotifications });
      return ((await ask(fake, "GET", "/notifications/marker")) as Answered<{ unread_count: number }>).body
        .unread_count;
    };
    const counts = [await marker(), await marker(0), await marker(5)];
    expect(counts).toEqual([3, 0, 5]);
  });
});
