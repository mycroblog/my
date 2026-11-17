// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { scenario, type FakeOptions, type FakeSeed } from "@my/e2e/fake";
import { marked, markStatus } from "./interactions";
import { cachedStatus, shadowed } from "./shadow";
import type { Status } from "./statuses";
import { fakeData } from "./test/data";
import { statusQuery } from "./thread";
import { homeTimelineQuery } from "./timeline";

/** A server where the reader follows sam, whose post `post` holds `interactions`. */
const posted = (interactions: FakeSeed["interactions"] = []): FakeOptions => ({
  seed: {
    people: scenario.people,
    follows: [{ follower: "yuki", followee: "sam" }],
    posts: [{ key: "post", author: "sam", text: "The crumb is finally open.", visibility: "public", language: "en" }],
    interactions,
  },
});

/** The reader's favourite and hana's of the post, and two 🎉 of others. */
const favourited: FakeSeed["interactions"] = [
  { kind: "favourite", by: "yuki", post: "post" },
  { kind: "favourite", by: "hana", post: "post" },
  { kind: "reaction", by: "sam", post: "post", emoji: "🎉" },
  { kind: "reaction", by: "hana", post: "post", emoji: "🎉" },
];

/** The reader's 👍 on the post, and two 🎉 of others. */
const reacted: FakeSeed["interactions"] = [
  { kind: "reaction", by: "yuki", post: "post", emoji: "👍" },
  { kind: "reaction", by: "sam", post: "post", emoji: "🎉" },
  { kind: "reaction", by: "hana", post: "post", emoji: "🎉" },
];

/**
 * AppData on a server of `options`, with the home timeline and the post of `key` alone, as its thread's screen reads
 * it, in the cache; `read()` is each of the two copies as it shows, `id` the post's ID, and `requests` what was sent
 * since.
 */
async function cached(options: FakeOptions = posted(), key = "post") {
  const { fake, data, requests } = fakeData(options);
  const id = fake.idOf(key);
  await data.queryClient.infiniteQuery(homeTimelineQuery(data));
  await data.queryClient.query(statusQuery(data, id));
  requests.length = 0;
  const shown = (copy: Status | undefined) => copy && shadowed(data, copy);
  const read = () => ({
    timeline: shown(
      data.queryClient
        .getQueryData(homeTimelineQuery(data).queryKey)
        ?.pages.flatMap((p) => p.items)
        .find((s) => s.id === id),
    ),
    thread: shown(data.queryClient.getQueryData(statusQuery(data, id).queryKey)),
  });
  return { fake, data, requests, read, id };
}

test("a favourite shows at once in every cached copy, then as the server answered", async () => {
  const { fake, data, requests, read, id } = await cached();
  // Others favourited the post since it was read: the server's answer counts them.
  const write = fake.script("POST /statuses/{id}/favourite", {
    hold: "response",
    change: (status) => ({ ...status, favourites_count: 5 }),
  });
  const done = markStatus(data, id, { type: "favourite", on: true });
  expect(read().timeline?.favourited).toBe(true);
  expect(read().thread?.favourites_count).toBe(1);
  write.release();
  await done;
  expect(requests).toEqual([`POST /statuses/${id}/favourite`]);
  expect(read().timeline?.favourites_count).toBe(5);
  expect(read().thread?.favourites_count).toBe(5);
});

test("a failed write puts back what every copy showed, and rejects", async () => {
  const { fake, data, read, id } = await cached(
    posted(["yuki", "hana", "marco"].map((by) => ({ kind: "boost", by, post: "post" }))),
  );
  fake.script("DELETE /statuses/{id}/boost", { fail: 422 });
  const rejected = await markStatus(data, id, { type: "boost", on: false }).then(
    () => false,
    () => true,
  );
  expect(rejected).toBe(true);
  expect(read().timeline?.boosted).toBe(true);
  expect(read().thread?.boosts_count).toBe(3);
});

test("each mark sends its own request, set or taken back", async () => {
  const { data, requests, id } = await cached();
  await markStatus(data, id, { type: "boost", on: true });
  await markStatus(data, id, { type: "bookmark", on: false });
  await markStatus(data, id, { type: "reaction", name: "🎉", on: true });
  await markStatus(data, id, { type: "reaction", name: "🎉", on: false });
  expect(requests).toEqual([
    `POST /statuses/${id}/boost`,
    `DELETE /statuses/${id}/bookmark`,
    `PUT /statuses/${id}/reaction`,
    `DELETE /statuses/${id}/reaction`,
  ]);
});

// A reader has a favourite or one reaction on a post, never both (FR-116).
test("a reaction takes back the favourite or the reader's other reaction, as the server does", async () => {
  const liked = (await cached(posted(favourited))).read().timeline;
  if (liked === undefined) throw new Error("test: the timeline holds no post");
  expect(liked).toMatchObject({ favourited: true, favourites_count: 2, reactions: [{ name: "🎉", count: 2 }] });
  const after = marked(liked, { type: "reaction", name: "🎉", on: true });
  expect(after.favourited).toBe(false);
  expect(after.favourites_count).toBe(1);
  expect(after.reactions).toEqual([{ name: "🎉", count: 3, me: true }]);
  const again = marked(after, { type: "favourite", on: true });
  expect(again.reactions).toEqual([{ name: "🎉", count: 2, me: false }]);

  const thumbed = (await cached(posted(reacted))).read().timeline;
  if (thumbed === undefined) throw new Error("test: the timeline holds no post");
  expect(thumbed.reactions).toEqual([
    { name: "🎉", count: 2, me: false },
    { name: "👍", count: 1, me: true },
  ]);
  expect(marked(thumbed, { type: "reaction", name: "🎉", on: true }).reactions).toEqual([
    { name: "🎉", count: 3, me: true },
  ]);
});

for (const [had, interactions] of [
  ["the favourite", favourited],
  ["the other reaction", reacted],
] as const)
  test(`a failed reaction puts back ${had} it took, in every copy`, async () => {
    const { fake, data, read, id } = await cached(posted(interactions));
    const before = read().timeline;
    const write = fake.script("PUT /statuses/{id}/reaction", { hold: "request", fail: 422 });
    const done = markStatus(data, id, { type: "reaction", name: "🎉", on: true }).then(
      () => "resolved",
      () => "rejected",
    );
    expect(read().timeline).toMatchObject({ favourited: false, reactions: [{ name: "🎉", count: 3, me: true }] });
    write.release();
    expect(await done).toBe("rejected");
    expect(read().timeline).toEqual(before);
    expect(read().thread).toMatchObject({
      favourited: before?.favourited ?? false,
      reactions: before?.reactions ?? [],
    });
  });

test("the cached status is found wherever a query holds it, and none when none does", async () => {
  const { data, id } = await cached(posted([{ kind: "bookmark", by: "yuki", post: "post" }]));
  expect(cachedStatus(data, id)?.bookmarked).toBe(true);
  expect(cachedStatus(data, "00000000-0000-7000-8000-00000000ffff")).toBeUndefined();
});

test("a favourite and a boost in flight at once keep each other's mark, whichever the server answers first", async () => {
  for (const first of [0, 1]) {
    const { fake, data, read, id } = await cached();
    // The server does each write when it comes, and its answer comes when the test lets it go.
    const answers = [
      fake.script("POST /statuses/{id}/favourite", { hold: "response" }),
      fake.script("POST /statuses/{id}/boost", { hold: "response" }),
    ] as const;
    const favourite = markStatus(data, id, { type: "favourite", on: true });
    const boost = markStatus(data, id, { type: "boost", on: true });
    await Promise.all(answers.map((answer) => answer.served()));
    answers[first === 0 ? 0 : 1].release();
    await (first === 0 ? favourite : boost);
    expect(read().timeline).toMatchObject({ favourited: true, boosted: true });
    answers[first === 0 ? 1 : 0].release();
    await Promise.all([favourite, boost]);
    expect(read().timeline).toMatchObject({ favourited: true, favourites_count: 1, boosted: true, boosts_count: 1 });
    expect(read().thread).toMatchObject({ favourited: true, boosted: true });
  }
});

test("the answer to an older write of a mark leaves the copies to the latest one", async () => {
  const { fake, data, read, id } = await cached();
  const set = fake.script("POST /statuses/{id}/favourite", { hold: "response" });
  const taken = fake.script("DELETE /statuses/{id}/favourite", { hold: "response" });
  const on = markStatus(data, id, { type: "favourite", on: true });
  const off = markStatus(data, id, { type: "favourite", on: false });
  set.release();
  await on;
  expect(read().timeline?.favourited).toBe(false);
  taken.release();
  await off;
  expect(read().timeline).toMatchObject({ favourited: false, favourites_count: 0 });
});

test("a failed write puts back only its own mark, not a boost set meanwhile", async () => {
  const { fake, data, read, id } = await cached();
  const refused = fake.script("POST /statuses/{id}/favourite", { hold: "request", fail: 422 });
  const boosted = fake.script("POST /statuses/{id}/boost", { hold: "response" });
  const favourite = markStatus(data, id, { type: "favourite", on: true }).catch(() => "rejected");
  const boost = markStatus(data, id, { type: "boost", on: true });
  refused.release();
  expect(await favourite).toBe("rejected");
  expect(read().timeline?.favourited ?? false).toBe(false);
  expect(read().timeline).toMatchObject({ boosted: true, boosts_count: 1 });
  boosted.release();
  await boost;
  expect(read().timeline?.favourited ?? false).toBe(false);
  expect(read().timeline?.boosted).toBe(true);
});

// With the fake's `approvals`, layla approves by hand the boosts of all but those her post mentions, and yuki's post
// admits the boosts of yuki's followers at once; hana follows both.
test("a boost of a post whose author approves the reader's boosts waits at once, and counts nowhere", async () => {
  const { fake, data, read, id } = await cached({ viewer: "hana", approvals: true }, "layla-asks");
  expect(read().timeline?.permissions?.announce).toBe("pending");
  const write = fake.script("POST /statuses/{id}/boost", { hold: "response" });
  const done = markStatus(data, id, { type: "boost", on: true });
  expect(read().timeline?.boost_approval).toBe("pending");
  expect(read().thread?.boost_approval).toBe("pending");
  expect(read().timeline?.boosts_count).toBe(0);
  write.release();
  await done;
  expect(read().timeline).toMatchObject({ boost_approval: "pending", boosts_count: 0 });
  expect(read().timeline?.boosted ?? false).toBe(false);
});

test("a boost of a post that admits the reader at once does not wait", async () => {
  const { data, read, id } = await cached({ viewer: "hana", approvals: true }, "yuki-asks");
  expect(read().timeline?.permissions?.announce).toBe("allowed");
  await markStatus(data, id, { type: "boost", on: true });
  expect(read().timeline?.boost_approval).toBeUndefined();
  expect(read().timeline).toMatchObject({ boosted: true, boosts_count: 1 });
});

test("muting a thread shows at once in every cached copy, and stays once the server answered", async () => {
  const { fake, data, requests, read, id } = await cached();
  const write = fake.script("POST /statuses/{id}/mute", { hold: "response" });
  const done = markStatus(data, id, { type: "mute", on: true });
  expect(read().timeline?.muted).toBe(true);
  expect(read().thread?.muted).toBe(true);
  write.release();
  await done;
  expect(requests).toEqual([`POST /statuses/${id}/mute`]);
  expect(read().timeline?.muted).toBe(true);
  expect(read().thread?.muted).toBe(true);
});

test("unmuting a thread asks the server to end the mute", async () => {
  const { data, requests, read, id } = await cached();
  await markStatus(data, id, { type: "mute", on: true });
  expect(read().timeline?.muted).toBe(true);
  requests.length = 0;
  await markStatus(data, id, { type: "mute", on: false });
  expect(requests).toEqual([`DELETE /statuses/${id}/mute`]);
  expect(read().timeline?.muted).toBe(false);
});

test("a mute the server refused is taken back in every copy, and rejects", async () => {
  const { fake, data, read, id } = await cached();
  fake.script("POST /statuses/{id}/mute", { fail: 422 });
  const outcome = await markStatus(data, id, { type: "mute", on: true }).then(
    () => "resolved",
    () => "rejected",
  );
  expect(outcome).toBe("rejected");
  expect(read().timeline?.muted).toBe(false);
  expect(read().thread?.muted).toBe(false);
});
