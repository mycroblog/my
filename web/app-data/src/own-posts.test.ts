// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { scenario, type FakeOptions, type FakeSeed } from "@my/e2e/fake";
import { deleteStatus, editStatus, readPostSource, readStatusHistory, RedraftError, redraftStatus } from "./own-posts";
import { shadowed } from "./shadow";
import type { Status } from "./statuses";
import { fakeData } from "./test/data";
import { statusQuery } from "./thread";
import { homeTimelineQuery } from "./timeline";

type Post = FakeSeed["posts"][number];

/** The reader's post: what the author typed, unlisted, with `more`. */
const own = (more: Partial<Post> = {}): Post => ({
  key: "post",
  author: "yuki",
  text: "Bread at nine",
  visibility: "unlisted",
  language: "en",
  ...more,
});

/**
 * AppData on a server that holds `posts` (the reader's post alone by default), with the home timeline and the post
 * `post` alone, as its thread's screen reads it, in the cache; `read()` is each of the two copies as it shows, or
 * undefined when the post is gone from it, `id` the post's ID, and `requests` what was sent since.
 */
async function cached(posts: Post[] = [own()], options: FakeOptions = {}) {
  const { fake, data, requests } = fakeData({ seed: { people: scenario.people, posts }, ...options });
  const id = fake.idOf("post");
  await data.queryClient.infiniteQuery(homeTimelineQuery(data));
  await data.queryClient.query(statusQuery(data, id));
  requests.length = 0;
  const shown = (copy: Status | undefined) => copy && shadowed(data, copy);
  const read = () => ({
    timeline: shown(data.queryClient.getQueryData(homeTimelineQuery(data).queryKey)?.pages[0]?.items[0]),
    thread: shown(data.queryClient.getQueryData(statusQuery(data, id).queryKey)),
  });
  return { fake, data, requests, read, id };
}

const rejects = (p: Promise<unknown>) =>
  p.then(
    () => false,
    () => true,
  );

test("a delete sends DELETE, and the post leaves every list at once, before the server answers", async () => {
  const { fake, data, requests, read, id } = await cached();
  const write = fake.script("DELETE /statuses/{id}", { hold: "response" });
  const done = deleteStatus(data, id);
  expect(read()).toEqual({ timeline: undefined, thread: undefined });
  write.release();
  await done;
  expect(requests).toEqual([`DELETE /statuses/${id}`]);
  expect(read()).toEqual({ timeline: undefined, thread: undefined });
});

test("a refused delete puts the post back in every list, and rejects", async () => {
  const { fake, data, read, id } = await cached();
  fake.script("DELETE /statuses/{id}", { fail: 422 });
  expect(await rejects(deleteStatus(data, id))).toBe(true);
  expect(read().timeline?.id).toBe(id);
  expect(read().thread?.id).toBe(id);
});

test("a delete the network lost puts the post back, and rejects", async () => {
  const { fake, data, read, id } = await cached();
  fake.script("DELETE /statuses/{id}", { fail: "network" });
  expect(await rejects(deleteStatus(data, id))).toBe(true);
  expect(read().timeline?.id).toBe(id);
});

test("the source holds what the author typed and, from the post, its audience, images, reply and quote", async () => {
  const [image] = scenario.posts.flatMap((p) => p.images ?? []);
  if (image === undefined) throw new Error("test: the scenario has no image");
  const { fake, data, id } = await cached([
    { key: "asked", author: "sam", text: "When is the bread?", visibility: "public", language: "en" },
    { key: "quoted", author: "sam", text: "The oven is on.", visibility: "public", language: "en" },
    own({ spoiler: "Food", images: [image], replyTo: "asked", quote: "quoted" }),
  ]);
  const read = await readPostSource(data, id);
  expect(read).toMatchObject({
    id,
    text: "Bread at nine",
    spoiler_text: "Food",
    language: "en",
    visibility: "unlisted",
    sensitive: true,
    in_reply_to_id: fake.idOf("asked"),
    quoted_status_id: fake.idOf("quoted"),
  });
  expect(read.media.map((m) => m.description)).toEqual([image.description]);
  expect(read.poll).toBeUndefined();
});

test("the source of a post with a poll holds its options, how it counts, and how long it ran", async () => {
  const { data } = fakeData();
  const { data: posted } = await data.api.POST("/statuses", {
    body: {
      text: "Which flour?",
      visibility: "public",
      poll: { options: ["Rye", "Spelt"], expires_in: 86_400, multiple: true },
    },
  });
  if (posted === undefined) throw new Error("test: the server posted nothing");
  expect((await readPostSource(data, posted.id)).poll).toEqual({
    options: ["Rye", "Spelt"],
    multiple: true,
    duration: 86_400,
  });
});

test("a source the server refuses rejects", async () => {
  const { fake, data, id } = await cached();
  fake.script("GET /statuses/{id}/source", { fail: 422 });
  expect(await rejects(readPostSource(data, id))).toBe(true);
});

test("a redraft reads the source before it deletes, resolves with it, and the post leaves every list", async () => {
  const { data, requests, read, id } = await cached();
  const redrafted = await redraftStatus(data, id);
  expect(redrafted).toMatchObject({ text: "Bread at nine", visibility: "unlisted" });
  expect(requests).toContain(`GET /statuses/${id}/source`);
  expect(requests.indexOf(`GET /statuses/${id}/source`)).toBeLessThan(requests.indexOf(`DELETE /statuses/${id}`));
  expect(read()).toEqual({ timeline: undefined, thread: undefined });
});

test("a redraft whose source cannot be read deletes nothing, and rejects", async () => {
  const { fake, data, requests, read, id } = await cached();
  fake.script("GET /statuses/{id}/source", { fail: 422 });
  expect(await rejects(redraftStatus(data, id))).toBe(true);
  expect(requests).not.toContain(`DELETE /statuses/${id}`);
  expect(read().timeline?.id).toBe(id);
});

test("a redraft whose delete is refused keeps the post, and rejects", async () => {
  const { fake, data, read, id } = await cached();
  fake.script("DELETE /statuses/{id}", { fail: 422 });
  expect(await rejects(redraftStatus(data, id))).toBe(true);
  expect(read().timeline?.id).toBe(id);
  expect(read().thread?.id).toBe(id);
});

test("a redraft whose delete was lost rejects with the source, and again deletes without reading it", async () => {
  const { fake, data, requests, read, id } = await cached();
  fake.script("DELETE /statuses/{id}", { times: 1, fail: "network" });
  const failed = await redraftStatus(data, id).catch((e: unknown) => e);
  expect(failed).toBeInstanceOf(RedraftError);
  const kept = (failed as RedraftError).source;
  expect(kept).toMatchObject({ text: "Bread at nine" });
  expect(read().timeline?.id).toBe(id);
  requests.length = 0;
  expect(await redraftStatus(data, id, kept)).toBe(kept);
  expect(requests).toEqual([`DELETE /statuses/${id}`]);
  expect(read()).toEqual({ timeline: undefined, thread: undefined });
});

test("a redraft again whose post the server no longer finds resolves with the source kept", async () => {
  const { data, read, id } = await cached();
  const kept = await readPostSource(data, id);
  // The delete whose answer was lost had reached the server.
  await data.api.DELETE("/statuses/{id}", { params: { path: { id } } });
  expect(await redraftStatus(data, id, kept)).toBe(kept);
  expect(read()).toEqual({ timeline: undefined, thread: undefined });
});

test("an edit sends PUT with what the composer wrote, and every copy shows the server's answer", async () => {
  const { fake, data, requests, read, id } = await cached();
  const write = fake.script("PUT /statuses/{id}", { hold: "response" });
  const done = editStatus(data, id, { text: "Bread at ten, see https://bakery.test/", spoiler_text: "Food", language: "en" });
  write.release();
  await done;
  expect(requests).toEqual([`PUT /statuses/${id}`]);
  expect(write.seen.map((s) => s.body)).toEqual([
    { text: "Bread at ten, see https://bakery.test/", spoiler_text: "Food", language: "en" },
  ]);
  // The server renders the text: the copies show its answer, not what was sent.
  const { data: answered } = await data.api.GET("/statuses/{id}", { params: { path: { id } } });
  expect(answered?.content).toContain('<a href="https://bakery.test/"');
  expect(answered?.edited_at).toBeDefined();
  const edited = { content: answered?.content ?? "", edited_at: answered?.edited_at ?? "", spoiler_text: "Food" };
  expect(read().timeline).toMatchObject(edited);
  expect(read().thread).toMatchObject(edited);
});

test("an edit that takes a link out shows the post without its card, as the server answered", async () => {
  const { data, read, id } = await cached([
    own({
      text: "Bread at nine, at https://bakery.test/",
      card: { title: "Bakery", description: "A small bakery by the station.", site: "bakery.test" },
    }),
  ]);
  expect(read().timeline?.card?.title).toBe("Bakery");
  await editStatus(data, id, { text: "Bread" });
  expect(read().timeline?.content).toBe("<p>Bread</p>");
  expect(read().timeline?.card).toBeUndefined();
  expect(read().thread?.card).toBeUndefined();
});

test("a refused edit leaves every copy as it was, and rejects", async () => {
  const { fake, data, read, id } = await cached();
  fake.script("PUT /statuses/{id}", { fail: 422 });
  expect(await rejects(editStatus(data, id, { text: "Bread at ten" }))).toBe(true);
  expect(read().timeline?.content).toBe("<p>Bread at nine</p>");
  expect(read().thread?.content).toBe("<p>Bread at nine</p>");
});

/** The reader's post, which said "Bread at eight" before its author edited it. */
const edited = [own({ earlier: ["Bread at eight"] })];

test("the history is the server's versions, oldest first, each with its time", async () => {
  const { data, requests, id } = await cached(edited);
  const history = await readStatusHistory(data, id);
  expect(requests).toEqual([`GET /statuses/${id}/history`]);
  expect(history.map((v) => v.content)).toEqual(["<p>Bread at eight</p>", "<p>Bread at nine</p>"]);
  const times = history.map((v) => Date.parse(v.created_at));
  expect(times.every((time) => !Number.isNaN(time))).toBe(true);
  expect(times[0]).toBeLessThan(times[1] ?? 0);
});

test("after an edit, the history is read again and holds the new version", async () => {
  const { data, id } = await cached();
  expect(await readStatusHistory(data, id)).toHaveLength(1);
  await editStatus(data, id, { text: "Bread at ten" });
  expect((await readStatusHistory(data, id)).map((v) => v.content)).toEqual([
    "<p>Bread at nine</p>",
    "<p>Bread at ten</p>",
  ]);
});

test("a history the server refuses rejects", async () => {
  const { fake, data, id } = await cached();
  fake.script("GET /statuses/{id}/history", { fail: 422 });
  expect(await rejects(readStatusHistory(data, id))).toBe(true);
});
