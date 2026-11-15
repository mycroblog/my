// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { deleteStatus, editStatus, readPostSource, readStatusHistory, RedraftError, redraftStatus } from "./own-posts";
import { shadowed } from "./shadow";
import type { Status } from "./statuses";
import { fakeAppData, json, status } from "./test/fake";
import { homeTimelineQuery } from "./timeline";

const post = (fields: Partial<Status> = {}) =>
  status("1", {
    content: "<p>Bread at nine</p>",
    created_at: "2026-11-14T03:00:00Z",
    visibility: "unlisted",
    sensitive: false,
    spoiler_text: "",
    media_attachments: [],
    ...fields,
  });

const source = { id: "1", text: "Bread at nine", spoiler_text: "", language: "en" };

const refused = () => json(422, { type: "about:blank", title: "Unprocessable", status: 422 });

/** Each request as `METHOD /path` after /api/v1. */
const sent = (request: Request) => `${request.method} ${new URL(request.url).pathname.replace("/api/v1", "")}`;

/**
 * AppData whose server answers `answers` by `METHOD /path` (a function for an answer the test holds, any other
 * request a 404), with the home timeline and the thread of post 0 holding `items`; `read()` is the first post of each
 * as it shows, or undefined when it is gone from it.
 */
function cached(items: Status[], answers: Record<string, () => Response | Promise<Response>>) {
  const requests: string[] = [];
  const { data } = fakeAppData((request) => {
    const key = sent(request);
    requests.push(key);
    return answers[key]?.() ?? json(404, { type: "about:blank", title: "Not found", status: 404 });
  });
  const timeline = homeTimelineQuery(data).queryKey;
  const context = data.queries.queryOptions("get", "/statuses/{id}/context", {
    params: { path: { id: "0" } },
  }).queryKey;
  data.queryClient.setQueryData(timeline, { pages: [{ items }], pageParams: [undefined] });
  data.queryClient.setQueryData(context, { ancestors: [], descendants: items.map((s) => ({ ...s })) });
  const shown = (copy: Status | undefined) => copy && shadowed(data, copy);
  const read = () => ({
    timeline: shown(data.queryClient.getQueryData(timeline)?.pages[0]?.items[0]),
    thread: shown(data.queryClient.getQueryData<{ descendants: Status[] }>(context)?.descendants[0]),
  });
  return { data, requests, read };
}

const rejects = (p: Promise<unknown>) =>
  p.then(
    () => false,
    () => true,
  );

test("a delete sends DELETE, and the post leaves every list at once, before the server answers", async () => {
  const { promise: answered, resolve: answer } = Promise.withResolvers<Response>();
  const { data, requests, read } = cached([post()], { "DELETE /statuses/1": () => answered });
  const done = deleteStatus(data, "1");
  expect(read()).toEqual({ timeline: undefined, thread: undefined });
  answer(json(200, post()));
  await done;
  expect(requests).toEqual(["DELETE /statuses/1"]);
  expect(read()).toEqual({ timeline: undefined, thread: undefined });
});

test("a refused delete puts the post back in every list, and rejects", async () => {
  const { data, read } = cached([post()], { "DELETE /statuses/1": refused });
  expect(await rejects(deleteStatus(data, "1"))).toBe(true);
  expect(read().timeline?.id).toBe("1");
  expect(read().thread?.id).toBe("1");
});

test("a delete the network lost puts the post back, and rejects", async () => {
  const { data, read } = cached([post()], {
    "DELETE /statuses/1": () => Promise.reject(new TypeError("Failed to fetch")),
  });
  expect(await rejects(deleteStatus(data, "1"))).toBe(true);
  expect(read().timeline?.id).toBe("1");
});

test("the source holds what the author typed and, from the post, its audience, images, reply and quote", async () => {
  const image = { id: "m1", type: "image", url: "https://m/a.webp", description: "A loaf" };
  const shown = post({
    sensitive: true,
    spoiler_text: "Food",
    media_attachments: [image] as Status["media_attachments"],
    in_reply_to_id: "7",
    quote: { state: "accepted", quoted_status: status("9") },
  });
  const { data } = cached([shown], {
    "GET /statuses/1/source": () => json(200, { ...source, spoiler_text: "Food" }),
    "GET /statuses/1": () => json(200, shown),
  });
  const read = await readPostSource(data, "1");
  expect(read).toMatchObject({
    id: "1",
    text: "Bread at nine",
    spoiler_text: "Food",
    language: "en",
    visibility: "unlisted",
    sensitive: true,
    in_reply_to_id: "7",
    quoted_status_id: "9",
  });
  expect(read.media.map((m) => m.id)).toEqual(["m1"]);
  expect(read.poll).toBeUndefined();
});

test("the source of a post with a poll holds its options, how it counts, and how long it ran", async () => {
  const shown = post({
    poll: {
      id: "p",
      expires_at: "2026-11-15T03:00:00Z",
      expired: false,
      multiple: true,
      votes_count: 0,
      voters_count: 0,
      options: [
        { title: "Rye", votes_count: 0 },
        { title: "Spelt", votes_count: 0 },
      ],
      emojis: [],
    } as NonNullable<Status["poll"]>,
  });
  const { data } = cached([shown], {
    "GET /statuses/1/source": () => json(200, source),
    "GET /statuses/1": () => json(200, shown),
  });
  expect((await readPostSource(data, "1")).poll).toEqual({
    options: ["Rye", "Spelt"],
    multiple: true,
    duration: 86_400,
  });
});

test("a source the server refuses rejects", async () => {
  const { data } = cached([post()], { "GET /statuses/1/source": refused, "GET /statuses/1": () => json(200, post()) });
  expect(await rejects(readPostSource(data, "1"))).toBe(true);
});

test("a redraft reads the source before it deletes, resolves with it, and the post leaves every list", async () => {
  const { data, requests, read } = cached([post()], {
    "GET /statuses/1/source": () => json(200, source),
    "GET /statuses/1": () => json(200, post()),
    "DELETE /statuses/1": () => json(200, post()),
  });
  const redrafted = await redraftStatus(data, "1");
  expect(redrafted).toMatchObject({ text: "Bread at nine", visibility: "unlisted" });
  expect(requests.indexOf("GET /statuses/1/source")).toBeLessThan(requests.indexOf("DELETE /statuses/1"));
  expect(requests).toContain("DELETE /statuses/1");
  expect(read()).toEqual({ timeline: undefined, thread: undefined });
});

test("a redraft whose source cannot be read deletes nothing, and rejects", async () => {
  const { data, requests, read } = cached([post()], {
    "GET /statuses/1/source": refused,
    "GET /statuses/1": () => json(200, post()),
    "DELETE /statuses/1": () => json(200, post()),
  });
  expect(await rejects(redraftStatus(data, "1"))).toBe(true);
  expect(requests).not.toContain("DELETE /statuses/1");
  expect(read().timeline?.id).toBe("1");
});

test("a redraft whose delete is refused keeps the post, and rejects", async () => {
  const { data, read } = cached([post()], {
    "GET /statuses/1/source": () => json(200, source),
    "GET /statuses/1": () => json(200, post()),
    "DELETE /statuses/1": refused,
  });
  expect(await rejects(redraftStatus(data, "1"))).toBe(true);
  expect(read().timeline?.id).toBe("1");
  expect(read().thread?.id).toBe("1");
});

test("a redraft whose delete was lost rejects with the source, and again deletes without reading it", async () => {
  let deletes = 0;
  const { data, requests, read } = cached([post()], {
    "GET /statuses/1/source": () => json(200, source),
    "GET /statuses/1": () => json(200, post()),
    "DELETE /statuses/1": () =>
      (deletes += 1) === 1 ? Promise.reject(new TypeError("Failed to fetch")) : json(200, post()),
  });
  const failed = await redraftStatus(data, "1").catch((e: unknown) => e);
  expect(failed).toBeInstanceOf(RedraftError);
  const kept = (failed as RedraftError).source;
  expect(kept).toMatchObject({ text: "Bread at nine" });
  expect(read().timeline?.id).toBe("1");
  requests.length = 0;
  expect(await redraftStatus(data, "1", kept)).toBe(kept);
  expect(requests).toEqual(["DELETE /statuses/1"]);
  expect(read()).toEqual({ timeline: undefined, thread: undefined });
});

test("a redraft again whose post the server no longer finds resolves with the source kept", async () => {
  const { data, read } = cached([post()], {
    "DELETE /statuses/1": () => json(404, { type: "about:blank", title: "Not found", status: 404 }),
  });
  const kept = { ...source, visibility: "unlisted", sensitive: false, media: [] } as const;
  expect(await redraftStatus(data, "1", kept)).toBe(kept);
  expect(read()).toEqual({ timeline: undefined, thread: undefined });
});

test("an edit sends PUT with what the composer wrote, and every copy shows the server's answer", async () => {
  const { promise: answered, resolve: answer } = Promise.withResolvers<Response>();
  const edited = post({ content: "<p>Bread at <em>ten</em></p>", edited_at: "2026-11-14T04:00:00Z" });
  const { data, read } = cached([post()], {
    "PUT /statuses/1": () => answered,
  });
  const done = editStatus(data, "1", { text: "Bread at *ten*", spoiler_text: "Food", language: "en" });
  answer(json(200, edited));
  await done;
  // The server renders the text: the copies show its answer, not what was sent.
  expect(read().timeline).toMatchObject({ content: "<p>Bread at <em>ten</em></p>", edited_at: "2026-11-14T04:00:00Z" });
  expect(read().thread).toMatchObject({ content: "<p>Bread at <em>ten</em></p>", edited_at: "2026-11-14T04:00:00Z" });
});

test("an edit that takes a link out shows the post without its card, as the server answered", async () => {
  const card = { url: "https://bakery.example/", title: "Bakery", description: "", site_name: "bakery.example" };
  const { data, read } = cached([post({ card })], {
    "PUT /statuses/1": () => json(200, post({ content: "<p>Bread</p>" })),
  });
  await editStatus(data, "1", { text: "Bread" });
  expect(read().timeline?.content).toBe("<p>Bread</p>");
  expect(read().timeline?.card).toBeUndefined();
  expect(read().thread?.card).toBeUndefined();
});

test("an edit's request carries what the composer wrote", async () => {
  let body: unknown;
  const { data } = fakeAppData(async (request) => {
    if (sent(request) === "PUT /statuses/1") body = await request.json();
    return json(200, post({ content: "<p>Bread at ten</p>" }));
  });
  await editStatus(data, "1", { text: "Bread at ten", spoiler_text: "Food", language: "en" });
  expect(body).toEqual({ text: "Bread at ten", spoiler_text: "Food", language: "en" });
});

test("a refused edit leaves every copy as it was, and rejects", async () => {
  const { data, read } = cached([post()], { "PUT /statuses/1": refused });
  expect(await rejects(editStatus(data, "1", { text: "Bread at ten" }))).toBe(true);
  expect(read().timeline?.content).toBe("<p>Bread at nine</p>");
  expect(read().thread?.content).toBe("<p>Bread at nine</p>");
});

const version = (content: string, created_at: string) => ({
  content,
  spoiler_text: "",
  sensitive: false,
  created_at,
  media_attachments: [],
  emojis: [],
});

test("the history is the server's versions, oldest first, each with its time", async () => {
  const items = [
    version("<p>Bread at nine</p>", "2026-11-14T03:00:00Z"),
    version("<p>Bread at ten</p>", "2026-11-14T04:00:00Z"),
  ];
  const { data, requests } = cached([post()], { "GET /statuses/1/history": () => json(200, { items }) });
  const history = await readStatusHistory(data, "1");
  expect(requests).toEqual(["GET /statuses/1/history"]);
  expect(history.map((v) => [v.content, v.created_at])).toEqual([
    ["<p>Bread at nine</p>", "2026-11-14T03:00:00Z"],
    ["<p>Bread at ten</p>", "2026-11-14T04:00:00Z"],
  ]);
});

test("after an edit, the history is read again and holds the new version", async () => {
  const first = [version("<p>Bread at nine</p>", "2026-11-14T03:00:00Z")];
  const second = [...first, version("<p>Bread at ten</p>", "2026-11-14T04:00:00Z")];
  let items = first;
  const { data } = cached([post()], {
    "GET /statuses/1/history": () => json(200, { items }),
    "PUT /statuses/1": () => {
      items = second;
      return json(200, post({ content: "<p>Bread at ten</p>", edited_at: "2026-11-14T04:00:00Z" }));
    },
  });
  expect(await readStatusHistory(data, "1")).toHaveLength(1);
  await editStatus(data, "1", { text: "Bread at ten" });
  expect((await readStatusHistory(data, "1")).map((v) => v.content)).toEqual([
    "<p>Bread at nine</p>",
    "<p>Bread at ten</p>",
  ]);
});

test("a history the server refuses rejects", async () => {
  const { data } = cached([post()], { "GET /statuses/1/history": refused });
  expect(await rejects(readStatusHistory(data, "1"))).toBe(true);
});
