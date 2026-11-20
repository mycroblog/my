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

describe("the voice rooms", () => {
  type Room = components["schemas"]["VoiceRoom"];
  type Join = components["schemas"]["VoiceRoomJoin"];

  /** Joins the fake's room at `index`, and opens the stream of the session. */
  const inRoom = async (fake: FakeApi, index = 0) => {
    const room = fake.voice.roomId(index);
    const join = (await ask(fake, "POST", `/voice/rooms/${room}/participation`)) as Answered<Join>;
    const response = await fake.serve(request("GET", `/voice/rooms/${room}/stream?session=${join.body.session}`));
    if (response.body === null) throw new Error("test: the stream has no body");
    return { room, join, response, reader: response.body.getReader() };
  };
  /** The next frame of a stream, as text, or null once it ended. */
  const frame = async (reader: ReadableStreamDefaultReader<Uint8Array>) => {
    const { value, done } = await reader.read();
    return done ? null : new TextDecoder().decode(value);
  };
  /** The name and the data of an event frame. */
  const event = (text: string | null) => {
    const match = /^event: (\w+)\ndata: (.*)\n\n$/s.exec(text ?? "");
    return match === null ? text : { event: match[1], data: JSON.parse(match[2] ?? "null") as unknown };
  };
  const beat = (fake: FakeApi, room: string, session: string) =>
    ask(fake, "POST", `/voice/rooms/${room}/heartbeat`, { session }).then((answer) => answer.status);

  test("the stories' server lists open rooms, reads one and a history, as a reader who is in none", async () => {
    const fake = createFakeApi();
    const list = (await ask(fake, "GET", "/voice/rooms")) as Answered<{ items: Room[] }>;
    expect(list.status).toBe(200);
    expect(list.body.items.length).toBeGreaterThan(1);
    expect(list.body.items.map((r) => r.id).slice(0, 2)).toEqual([fake.voice.roomId(0), fake.voice.roomId(1)]);
    const one = (await ask(fake, "GET", `/voice/rooms/${fake.voice.roomId(0)}`)) as Answered<Room>;
    expect([one.status, one.body.id]).toEqual([200, fake.voice.roomId(0)]);
    // No session of anyone is shown outside the stream.
    expect(list.body.items.flatMap((r) => r.participants).filter((p) => p.session !== undefined)).toEqual([]);
    const history = await ask(fake, "GET", "/voice/history");
    expect([history.status, (history.body as { items: unknown[] }).items.length]).toEqual([200, 2]);
    expect(fake.voice.joined()).toBeNull();
    const nowhere = await ask(fake, "GET", "/voice/rooms/nowhere");
    expect(nowhere.status).toBe(404);
  });

  test("a join gives a new session and a grant, whose stream says it connected and the room, with the reader in it", async () => {
    const fake = createFakeApi();
    const { room, join, response, reader } = await inRoom(fake);
    expect(join.status).toBe(200);
    expect(join.body.grant).toMatchObject({ kind: "p2p", ice_servers: [], relay_only: false });
    expect(Date.parse(join.body.grant.expires_at)).toBeGreaterThan(Date.now());
    expect(fake.voice.joined()).toEqual({ room, session: join.body.session });
    expect(response.headers.get("Content-Type")).toBe("text/event-stream");
    const [connected, first] = [await frame(reader), await frame(reader)];
    expect(connected).toBe(": connected\n\n");
    const state = event(first) as { event: string; data: Room };
    expect(state.event).toBe("state");
    expect(state.data.participants.find((p) => p.account.id === fake.accountId("yuki"))).toMatchObject({
      role: "listener",
      session: join.body.session,
    });
    expect(fake.voice.opened()).toBe(1);
  });

  test("the session's heartbeat and its new grant are answered, another session's are not", async () => {
    const fake = createFakeApi();
    const { room, join } = await inRoom(fake);
    const beats = [await beat(fake, room, join.body.session), await beat(fake, room, "another")];
    expect(beats).toEqual([204, 404]);
    const grant = await ask(fake, "POST", `/voice/rooms/${room}/grant`, { session: join.body.session });
    expect(grant.status).toBe(200);
    expect(grant.body).toMatchObject({ kind: "p2p" });
    const another = await ask(fake, "POST", `/voice/rooms/${room}/grant`, { session: "another" });
    expect(another.status).toBe(404);
  });

  test("the microphone, a reaction and speaking are said on the stream; a listener's microphone is refused", async () => {
    const fake = createFakeApi();
    const { room, join, reader } = await inRoom(fake);
    await frame(reader);
    await frame(reader);
    const session = join.body.session;
    const refused = await ask(fake, "POST", `/voice/rooms/${room}/mic`, { session, on: true });
    expect([refused.status, (refused.body as { type: string }).type]).toEqual([
      403,
      "/api/v1/problems/voice-cannot-speak",
    ]);
    const speaking = (await ask(fake, "POST", `/voice/rooms/${room}/speaker`)) as Answered<Room>;
    expect(speaking.status).toBe(200);
    const spoke = event(await frame(reader)) as { event: string; data: Room };
    expect(spoke.event).toBe("state");
    expect(spoke.data.participants.find((p) => p.account.id === fake.accountId("yuki"))?.role).toBe("speaker");
    const mic = await ask(fake, "POST", `/voice/rooms/${room}/mic`, { session, on: true });
    const micSaid = event(await frame(reader));
    const reaction = await ask(fake, "POST", `/voice/rooms/${room}/reactions`, { emoji: "👏" });
    const reactionSaid = event(await frame(reader));
    expect([mic.status, micSaid, reaction.status, reactionSaid]).toEqual([
      204,
      { event: "mic", data: { participant: fake.accountId("yuki"), on: true } },
      204,
      { event: "reaction", data: { participant: fake.accountId("yuki"), emoji: "👏" } },
    ]);
  });

  test("a dropped stream ends without a word and opens again on the same session", async () => {
    const fake = createFakeApi();
    const { room, join, reader } = await inRoom(fake);
    await frame(reader);
    await frame(reader);
    fake.voice.drop();
    const [ended, beaten] = [await frame(reader), await beat(fake, room, join.body.session)];
    expect([ended, beaten]).toEqual([null, 204]);
    const again = await fake.serve(request("GET", `/voice/rooms/${room}/stream?session=${join.body.session}`));
    expect(again.status).toBe(200);
    expect(fake.voice.opened()).toBe(2);
  });

  test("removed, replaced and ended are said, close the stream, and the session is gone; ended takes the room away", async () => {
    for (const [name, reason] of [
      ["removed", "removed"],
      ["replaced", "replaced"],
      ["ended", "host"],
    ] as const) {
      const fake = createFakeApi();
      const { room, join, reader } = await inRoom(fake);
      await frame(reader);
      await frame(reader);
      fake.voice.send({ event: name, data: { reason } });
      const said = [event(await frame(reader)), await frame(reader)];
      expect(said).toEqual([{ event: name, data: { reason } }, null]);
      const beaten = await beat(fake, room, join.body.session);
      expect([beaten, fake.voice.joined()]).toEqual([404, null]);
      const stream = await fake.serve(request("GET", `/voice/rooms/${room}/stream?session=${join.body.session}`));
      expect(stream.status).toBe(404);
      const list = (await ask(fake, "GET", "/voice/rooms")) as Answered<{ items: Room[] }>;
      expect([name, list.body.items.some((r) => r.id === room)]).toEqual([name, name !== "ended"]);
    }
  });

  test("leaving ends the session with its stream, and the reader is no longer counted", async () => {
    const fake = createFakeApi();
    const before = ((await ask(fake, "GET", `/voice/rooms/${fake.voice.roomId(0)}`)) as Answered<Room>).body;
    const { room, join, reader } = await inRoom(fake);
    await frame(reader);
    await frame(reader);
    const left = await ask(fake, "DELETE", `/voice/rooms/${room}/participation`);
    const said = event(await frame(reader));
    const beaten = await beat(fake, room, join.body.session);
    expect([left.status, said, beaten, fake.voice.joined()]).toEqual([
      204,
      { event: "removed", data: { reason: "left" } },
      404,
      null,
    ]);
    const after = ((await ask(fake, "GET", `/voice/rooms/${room}`)) as Answered<Room>).body;
    expect([after.participants_count, after.participants.length]).toEqual([
      before.participants_count,
      before.participants.length,
    ]);
    const again = await ask(fake, "DELETE", `/voice/rooms/${room}/participation`);
    expect(again.status).toBe(404);
  });

  test("joining another room moves the reader there, and joining the same room again replaces the session", async () => {
    const fake = createFakeApi();
    const first = await inRoom(fake);
    await frame(first.reader);
    await frame(first.reader);
    const other = fake.voice.roomId(1);
    const moved = (await ask(fake, "POST", `/voice/rooms/${other}/participation`)) as Answered<Join>;
    const movedOut = event(await frame(first.reader));
    expect(movedOut).toEqual({ event: "removed", data: { reason: "moved" } });
    expect(fake.voice.joined()).toEqual({ room: other, session: moved.body.session });
    expect(moved.body.session).not.toBe(first.join.body.session);
    const stream = await fake.serve(request("GET", `/voice/rooms/${other}/stream?session=${moved.body.session}`));
    if (stream.body === null) throw new Error("test: the stream has no body");
    const reader = stream.body.getReader();
    await frame(reader);
    await frame(reader);
    const again = (await ask(fake, "POST", `/voice/rooms/${other}/participation`)) as Answered<Join>;
    const replaced = event(await frame(reader));
    expect(replaced).toEqual({ event: "replaced", data: { reason: "replaced" } });
    expect(fake.voice.joined()).toEqual({ room: other, session: again.body.session });
  });
});

describe("the events", () => {
  type Event = components["schemas"]["Event"];
  type Candidate = components["schemas"]["EventCandidate"];
  type Problem = components["schemas"]["Problem"];

  const read = async (fake: FakeApi, key: string) =>
    (await ask(fake, "GET", `/events/${fake.events.id(key)}`)) as Answered<Event>;
  const problemOf = (answer: Answered<unknown>) => [answer.status, (answer.body as Problem).type];
  /** An instant `days` days from now, as the API writes it. */
  const inDays = (days: number, hours = 0) => new Date(Date.now() + (days * 24 + hours) * 3_600_000).toISOString();

  test("the list holds the upcoming events the reader sees, the nearest first, a cancelled one too", async () => {
    const fake = createFakeApi();
    const list = (await ask(fake, "GET", "/events?limit=40")) as Answered<{ items: Event[] }>;
    const ids = list.body.items.map((e) => e.id);
    for (const key of ["meetup", "photo-walk", "bread-class", "garden-day", "coffee-voice", "flea-market"])
      expect(ids).toContain(fake.events.id(key));
    // Not for the reader (the organizer's followers, an account they block), and one that ended.
    for (const key of ["followers-brunch", "flash-sale", "picnic"]) expect(ids).not.toContain(fake.events.id(key));
    const starts = list.body.items.map((e) => Date.parse(e.starts_at));
    expect(starts).toEqual(starts.toSorted((a, b) => a - b));
    // An ended one is read alone, and one the reader does not see is not there.
    expect((await read(fake, "picnic")).status).toBe(200);
    expect((await read(fake, "followers-brunch")).status).toBe(404);
    const sams = (await ask(fake, "GET", `/events?organizer_id=${fake.accountId("sam")}`)) as Answered<{
      items: Event[];
    }>;
    expect(new Set(sams.body.items.map((e) => e.organizer.username))).toEqual(new Set(["sam"]));
    const none = (await ask(createFakeApi({ around: "none" }), "GET", "/events")) as Answered<{ items: Event[] }>;
    expect(none.body.items).toEqual([]);
  });

  test("the reader's part in each event, and the places they see: the others only by their kind", async () => {
    const fake = createFakeApi();
    const [poll, co, invited, approved, unapproved, open] = await Promise.all(
      ["photo-walk", "bread-class", "garden-day", "book-swap", "cooking-night", "tiny-concert"].map((k) =>
        read(fake, k),
      ),
    ).then((answers) => answers.map((a) => a.body));
    expect([poll?.viewer.organizer, poll?.state, poll?.places.map((p) => p.kind)]).toEqual([
      true,
      "unscheduled",
      ["physical"],
    ]);
    expect([co?.viewer.coorganizer, co?.places.map((p) => p.kind), co?.hidden_places]).toEqual([
      true,
      ["physical", "url"],
      [],
    ]);
    expect([invited?.viewer.invited, invited?.visibility, invited?.places.length]).toEqual([true, "invited", 1]);
    expect([approved?.viewer.answer, approved?.viewer.approved, approved?.places.length]).toEqual(["going", true, 2]);
    expect([unapproved?.viewer.answer, unapproved?.places, unapproved?.hidden_places]).toEqual([
      "maybe",
      [],
      ["physical"],
    ]);
    expect([open?.capacity, open?.going_count, open?.places.map((p) => p.kind)]).toEqual([3, 3, ["url"]]);
  });

  test("a voice place before the start, with its room open, and with one that could not open", async () => {
    const fake = createFakeApi();
    const voice = async (key: string) => (await read(fake, key)).body.places.find((p) => p.kind === "voice");
    const before = await voice("radio-hour");
    expect(before).toEqual({ kind: "voice", speakers: "manual" });
    const open = await voice("coffee-voice");
    const room = (await ask(fake, "GET", `/voice/rooms/${open?.room_id ?? "none"}`)) as Answered<{
      host: { username: string };
    }>;
    expect([room.status, room.body.host.username]).toEqual([200, "sam"]);
    expect((await voice("trail-talk"))?.room_unavailable).toBe("too_many_rooms");
  });

  test("an answer counts and lists the reader until the start; at capacity, cancelled, started and polled it is refused", async () => {
    const fake = createFakeApi();
    const answer = (key: string, a: string) => ask(fake, "PUT", `/events/${fake.events.id(key)}/rsvp`, { answer: a });
    const going = (await answer("meetup", "going")) as Answered<Event>;
    expect([going.status, going.body.viewer.answer]).toEqual([200, "going"]);
    const attendees = (await ask(fake, "GET", `/events/${fake.events.id("meetup")}/attendees`)) as Answered<{
      items: { account: { username: string }; answer: string }[];
    }>;
    expect(attendees.body.items.map((a) => [a.account.username, a.answer])).toEqual([["yuki", "going"]]);
    expect(problemOf(await answer("tiny-concert", "going"))).toEqual([409, "/api/v1/problems/event-full"]);
    expect((await answer("tiny-concert", "maybe")).status).toBe(200);
    expect(problemOf(await answer("flea-market", "going"))).toEqual([409, "/api/v1/problems/event-closed"]);
    expect(problemOf(await answer("coffee-voice", "declined"))).toEqual([409, "/api/v1/problems/event-closed"]);
    expect(problemOf(await answer("photo-walk", "going"))).toEqual([409, "/api/v1/problems/event-unscheduled"]);
    // Not going ends an approval, and the places shown to the approved go.
    const declined = (await answer("book-swap", "declined")) as Answered<Event>;
    expect([declined.body.viewer.approved, declined.body.hidden_places]).toEqual([false, ["physical", "url"]]);
  });

  test("a poll is answered, lists who answered, and once its organizer confirms a candidate takes no more answers", async () => {
    const fake = createFakeApi();
    const id = fake.events.id("photo-walk");
    const candidates = (await ask(fake, "GET", `/events/${id}/candidates`)) as Answered<{ items: Candidate[] }>;
    const [first, second] = candidates.body.items;
    if (first === undefined || second === undefined) throw new Error("test: the poll has no two candidates");
    expect([first.yes_count, first.vote]).toEqual([2, undefined]);
    const voted = (await ask(fake, "PUT", `/events/${id}/candidates/${first.id}/vote`, {
      vote: "maybe",
    })) as Answered<Candidate>;
    expect([voted.body.vote, voted.body.maybe_count]).toEqual(["maybe", 1]);
    const ballots = (await ask(fake, "GET", `/events/${id}/candidates/${first.id}/votes?vote=yes`)) as Answered<{
      items: { account: { username: string } }[];
    }>;
    expect(ballots.body.items.map((b) => b.account.username).toSorted()).toEqual(["hana", "sam"]);
    const added = (await ask(fake, "POST", `/events/${id}/candidates`, {
      starts_at: inDays(20),
      ends_at: inDays(20, 2),
    })) as Answered<Candidate>;
    expect(added.status).toBe(201);
    expect((await ask(fake, "DELETE", `/events/${id}/candidates/${added.body.id}`)).status).toBe(204);
    const confirmed = (await ask(fake, "POST", `/events/${id}/candidates/${second.id}/confirm`)) as Answered<Event>;
    expect([confirmed.body.state, confirmed.body.starts_at]).toEqual(["scheduled", second.starts_at]);
    expect(problemOf(await ask(fake, "PUT", `/events/${id}/candidates/${first.id}/vote`, { vote: "yes" }))).toEqual([
      409,
      "/api/v1/problems/event-scheduled",
    ]);
    // A poll keeps its last candidate.
    const made = (await ask(fake, "POST", "/events", {
      title: "One day only",
      candidates: [{ starts_at: inDays(3), ends_at: inDays(3, 1) }],
    })) as Answered<Event>;
    const only = (await ask(fake, "GET", `/events/${made.body.id}/candidates`)) as Answered<{ items: Candidate[] }>;
    const last = await ask(fake, "DELETE", `/events/${made.body.id}/candidates/${only.body.items[0]?.id ?? ""}`);
    expect(problemOf(last)).toEqual([409, "/api/v1/problems/event-last-candidate"]);
  });

  test("an organizer creates within the limits; a voice place, a date out of them and both dates are refused", async () => {
    const create = (fake: FakeApi, body: object) => ask(fake, "POST", "/events", { title: "Night walk", ...body });
    const fake = createFakeApi();
    const made = (await create(fake, {
      starts_at: inDays(2),
      ends_at: inDays(2, 2),
      time_zone: "Asia/Tokyo",
      places: [{ kind: "voice" }, { kind: "url", name: "Map", url: "https://maps.example/walk" }],
    })) as Answered<Event>;
    expect([made.status, made.body.viewer.organizer, made.body.places[0]]).toEqual([
      201,
      true,
      { kind: "voice", speakers: "manual" },
    ]);
    const field = async (body: object) => ((await create(fake, body)).body as Problem).field;
    const faults = [
      await field({ starts_at: inDays(-1), ends_at: inDays(1) }),
      await field({ starts_at: inDays(1), ends_at: inDays(9) }),
      await field({ starts_at: inDays(400), ends_at: inDays(400, 1) }),
      await field({}),
      await field({
        starts_at: inDays(1),
        ends_at: inDays(1, 1),
        places: [{ kind: "url", name: "x", url: "ftp://x.example" }],
      }),
    ];
    expect(faults).toEqual(["starts_at", "ends_at", "starts_at", "candidates", "places"]);
    const noVoice = createFakeApi({ voiceStarters: "permitted" });
    expect(
      problemOf(await create(noVoice, { starts_at: inDays(1), ends_at: inDays(1, 1), places: [{ kind: "voice" }] })),
    ).toEqual([422, "/api/v1/problems/event-voice-unavailable"]);
    const notEligible = createFakeApi({ eventCreators: "permitted" });
    expect(problemOf(await create(notEligible, { starts_at: inDays(1), ends_at: inDays(1, 1) }))).toEqual([
      403,
      "/api/v1/problems/event-not-eligible",
    ]);
    for (let i = 0; i < 9; i++) await create(fake, { starts_at: inDays(1), ends_at: inDays(1, 1) });
    const limited = await fake.serve(
      request("POST", "/events", { title: "One more", starts_at: inDays(1), ends_at: inDays(1, 1) }),
    );
    expect([limited.status, Number(limited.headers.get("Retry-After")) > 0]).toEqual([429, true]);
  });

  test("a co-organizer changes, invites once, approves; the organizer alone cancels and names co-organizers", async () => {
    const fake = createFakeApi();
    const id = fake.events.id("bread-class");
    const changed = (await ask(fake, "PATCH", `/events/${id}`, { capacity: 20 })) as Answered<Event>;
    expect([changed.body.capacity, changed.body.revision_count]).toEqual([20, 2]);
    expect(problemOf(await ask(fake, "PATCH", `/events/${id}`, { visibility: "public" }))).toEqual([
      403,
      "/api/v1/problems/event-organizer-only",
    ]);
    const invite = (who: string) => ask(fake, "POST", `/events/${id}/invitations`, { account_id: fake.accountId(who) });
    expect((await invite("ren")).status).toBe(201);
    expect(problemOf(await invite("ren"))).toEqual([409, "/api/v1/problems/event-already-invited"]);
    expect(problemOf(await invite("quickdeals"))).toEqual([422, "/api/v1/problems/event-invalid-invitee"]);
    const invitations = (await ask(fake, "GET", `/events/${id}/invitations`)) as Answered<{
      items: { account: { username: string } }[];
    }>;
    expect(invitations.body.items.map((i) => i.account.username)).toEqual(["mio", "ren"]);
    expect((await ask(fake, "DELETE", `/events/${id}/invitations/${fake.accountId("ren")}`)).status).toBe(204);
    expect((await ask(fake, "DELETE", `/events/${id}/invitations/${fake.accountId("ren")}`)).status).toBe(404);
    expect(problemOf(await invite("ren"))).toEqual([409, "/api/v1/problems/event-already-invited"]);
    const approval = `/events/${id}/attendees/${fake.accountId("layla")}/approval`;
    const approved = (await ask(fake, "PUT", approval)) as Answered<{ approved: boolean }>;
    expect(approved.body.approved).toBe(true);
    expect(((await ask(fake, "DELETE", approval)) as Answered<{ approved: boolean }>).body.approved).toBe(false);
    expect(problemOf(await ask(fake, "PUT", `/events/${id}/attendees/${fake.accountId("ren")}/approval`))).toEqual([
      422,
      "/api/v1/problems/event-not-attending",
    ]);
    expect(problemOf(await ask(fake, "PUT", `/events/${id}/coorganizers/${fake.accountId("mio")}`))).toEqual([
      403,
      "/api/v1/problems/event-organizer-only",
    ]);
    expect(problemOf(await ask(fake, "POST", `/events/${id}/cancel`, {}))).toEqual([
      403,
      "/api/v1/problems/event-organizer-only",
    ]);
    // The reader's own event: co-organizers up to 3, then cancelled with a note, and closed.
    const own = fake.events.id("photo-walk");
    for (const who of ["sam", "layla"])
      expect((await ask(fake, "PUT", `/events/${own}/coorganizers/${fake.accountId(who)}`)).status).toBe(200);
    expect(problemOf(await ask(fake, "PUT", `/events/${own}/coorganizers/${fake.accountId("mio")}`))).toEqual([
      409,
      "/api/v1/problems/event-too-many-coorganizers",
    ]);
    const removed = (await ask(
      fake,
      "DELETE",
      `/events/${own}/coorganizers/${fake.accountId("sam")}`,
    )) as Answered<Event>;
    expect(removed.body.coorganizers.map((a) => a.username)).toEqual(["hana", "layla"]);
    const cancelled = (await ask(fake, "POST", `/events/${own}/cancel`, { note: "Too much snow" })) as Answered<Event>;
    expect([cancelled.body.state, cancelled.body.cancellation?.note]).toEqual(["cancelled", "Too much snow"]);
    expect(problemOf(await ask(fake, "PATCH", `/events/${own}`, { title: "Again" }))).toEqual([
      409,
      "/api/v1/problems/event-closed",
    ]);
  });

  test("the .ics names only the places the reader sees, and exists once the date is fixed", async () => {
    const fake = createFakeApi();
    const ics = async (key: string) => {
      const response = await fake.serve(request("GET", `/events/${fake.events.id(key)}/calendar.ics`));
      return { response, text: await response.text() };
    };
    const shown = await ics("book-swap");
    expect(shown.response.headers.get("Content-Type")).toBe("text/calendar");
    expect(shown.response.headers.get("Content-Disposition")).toBe(
      `attachment; filename="${fake.events.id("book-swap")}.ics"`,
    );
    expect(shown.text).toContain("METHOD:PUBLISH");
    expect(shown.text).toContain("LOCATION:Café Lumière");
    expect((await ics("cooking-night")).text).not.toContain("LOCATION");
    expect((await ics("flea-market")).text).toContain("STATUS:CANCELLED");
    expect((await ics("photo-walk")).response.status).toBe(409);
  });

  test("the reader's reminders are the defaults until they choose, the earliest first; more than 3 are refused", async () => {
    const fake = createFakeApi();
    const path = "/preferences/event_reminders";
    expect((await ask(fake, "GET", path)).body).toEqual({ minutes_before: [1440, 60], defaults: true });
    expect((await ask(fake, "PUT", path, { minutes_before: [30, 2880] })).body).toEqual({
      minutes_before: [2880, 30],
      defaults: false,
    });
    const refused = (await ask(fake, "PUT", path, { minutes_before: [10, 20, 30, 40] })) as Answered<Problem>;
    expect([refused.status, refused.body.field]).toEqual([422, "minutes_before"]);
    expect(((await ask(fake, "PUT", path, { minutes_before: [1] })) as Answered<Problem>).status).toBe(422);
    expect((await ask(fake, "PUT", path, { defaults: true })).body).toEqual({
      minutes_before: [1440, 60],
      defaults: true,
    });
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
    const failure: unknown = await fake.serve(request("GET", "/admin/webhooks")).catch((e: unknown) => e);
    expect(failure).toBeInstanceOf(UnimplementedError);
    expect((failure as UnimplementedError).operation).toBe("GET /api/v1/admin/webhooks");
    expect((failure as Error).message).toBe("fake: GET /api/v1/admin/webhooks is no operation the fake API implements");
    expect(fake.unimplemented).toEqual(["GET /api/v1/admin/webhooks"]);
  });

  test("fails for a method the operation's path does not have", async () => {
    const fake = createFakeApi();
    const failure: unknown = await fake.serve(request("DELETE", "/instance")).catch((e: unknown) => e);
    expect(failure).toBeInstanceOf(UnimplementedError);
  });

  test("can still be scripted: the script counts its requests, and may answer for the fake", async () => {
    const fake = createFakeApi();
    const relays = fake.script("GET /admin/jobs", { fail: 403 });
    const refused = await ask(fake, "GET", "/admin/jobs");
    expect([refused.status, relays.seen.length, fake.unimplemented]).toEqual([403, 1, []]);
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
