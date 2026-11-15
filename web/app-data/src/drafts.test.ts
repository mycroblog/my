// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { ProblemError } from "@mycroblog/mykit";
import { defaultComposeSettings, publishDraft, refusal, saveDraft, useComposeSettings } from "./compose";
import {
  defaultDraftLimits,
  deleteDraft,
  DraftConflictError,
  DraftGoneError,
  draftListsQuery,
  readDraft,
  scheduleDraft,
  unscheduleDraft,
  useDrafts,
  type ServerDraft,
} from "./drafts";
import { cachedStatus } from "./shadow";
import { fakeAppData, json, page, status } from "./test/fake";
import { probe } from "./test/probe";
import { homeTimelineQuery } from "./timeline";

/** A draft of the API with the fields a test sets. */
const draft = (id: string, fields: Partial<ServerDraft> = {}): ServerDraft => ({
  id,
  created_at: "2026-11-14T00:00:00Z",
  updated_at: "2026-11-14T01:00:00Z",
  text: `Draft ${id}`,
  spoiler_text: "",
  sensitive: false,
  visibility: "public",
  media: [],
  ...fields,
});

/** A draft's answer, with the ETag of its next save. */
const answer = (body: ServerDraft, etag: string, code = 200) =>
  new Response(JSON.stringify(body), { status: code, headers: { "Content-Type": "application/json", ETag: etag } });

const problem = (code: number, detail?: string) =>
  json(code, { type: "about:blank", title: "Problem", status: code, ...(detail === undefined ? {} : { detail }) });

/** `METHOD path?query If-Match` of a request. */
const line = (request: Request) => {
  const url = new URL(request.url);
  const ifMatch = request.headers.get("If-Match");
  return `${request.method} ${url.pathname.replace(/^\/api\/v1/, "")}${url.search}${ifMatch === null ? "" : ` ${ifMatch}`}`;
};

/** Whether the lists of drafts are marked to be read again. */
const stale = (data: ReturnType<typeof fakeAppData>["data"]) =>
  data.queryClient.getQueryState(draftListsQuery(data).queryKey)?.isInvalidated;

/** Data whose lists of drafts are read already, to see a write mark them stale. */
async function withLists(handler: (request: Request) => Response | Promise<Response>) {
  const seen: string[] = [];
  const fake = fakeAppData((request) => {
    seen.push(line(request));
    if (request.method === "GET" && new URL(request.url).pathname.endsWith("/drafts")) return json(200, { items: [] });
    return handler(request);
  });
  await fake.data.queryClient.query(draftListsQuery(fake.data));
  seen.length = 0;
  return { ...fake, seen };
}

test("a draft saved before is saved over with the ETag it was read with, and the next save sends the new one", async () => {
  const { data, seen } = await withLists(() => answer(draft("d1", { scheduled_at: "2026-11-20T03:00:00Z" }), '"2"'));
  const saved = await saveDraft(data, { text: "hi", visibility: "public" }, { id: "d1", etag: '"1"' });
  expect(seen).toEqual(['PUT /drafts/d1 "1"']);
  expect(saved).toEqual({ id: "d1", etag: '"2"', scheduledAt: "2026-11-20T03:00:00Z" });
  expect(stale(data)).toBe(true);
});

test("a draft never saved is saved as a new one, with its ETag", async () => {
  const { data, seen } = await withLists(() => answer(draft("d1"), '"1"', 201));
  expect(await saveDraft(data, { text: "hi", visibility: "public" })).toEqual({ id: "d1", etag: '"1"' });
  expect(seen).toEqual(["POST /drafts"]);
});

test("a save the server refuses for a newer one saved elsewhere (412) is refused, when the reader is to choose", async () => {
  const { data, seen } = await withLists(() => problem(412));
  const error = await saveDraft(
    data,
    { text: "hi", visibility: "public" },
    { id: "d1", etag: '"1"' },
    {
      onConflict: "refuse",
    },
  ).catch((e: unknown) => e);
  expect(error).toBeInstanceOf(DraftConflictError);
  // Nothing is saved: neither over the newer one nor as another draft.
  expect(seen).toEqual(['PUT /drafts/d1 "1"']);
  expect(stale(data)).toBe(true);
});

test("a draft deleted elsewhere since it was read is saved as a new one", async () => {
  const { data, seen } = await withLists((request) =>
    request.method === "PUT" ? problem(404) : answer(draft("d2"), '"1"', 201),
  );
  expect(await saveDraft(data, { text: "hi", visibility: "public" }, { id: "d1", etag: '"3"' })).toEqual({
    id: "d2",
    etag: '"1"',
  });
  expect(seen).toEqual(['PUT /drafts/d1 "3"', "POST /drafts"]);
});

test("a draft gone since it was read is refused when the reader is to choose: a new one could post it twice", async () => {
  // Its publication's answer was lost, or its schedule fired: Post again must not save it as a new draft to publish.
  const { data, seen } = await withLists((request) =>
    request.method === "PUT" ? problem(404) : answer(draft("d2"), '"1"', 201),
  );
  const error = await saveDraft(
    data,
    { text: "hi", visibility: "public" },
    { id: "d1", etag: '"3"' },
    { onConflict: "refuse" },
  ).catch((e: unknown) => e);
  expect(error).toBeInstanceOf(DraftGoneError);
  expect(seen).toEqual(['PUT /drafts/d1 "3"']);
});

test("a save refused for anything else rejects with the server's words, and saves no copy", async () => {
  const { data, seen } = await withLists(() => problem(422, "You have 100 drafts, the most you may keep."));
  const error = await saveDraft(
    data,
    { text: "hi", visibility: "public" },
    { id: "d1", etag: '"1"' },
    {
      onConflict: "refuse",
    },
  ).catch((e: unknown) => e);
  expect(error).not.toBeInstanceOf(DraftConflictError);
  expect(refusal(error)).toBe("You have 100 drafts, the most you may keep.");
  expect(seen).toEqual(['PUT /drafts/d1 "1"']);
});

test("the lists read every page of the drafts and of the scheduled posts apart, the scheduled soonest first", async () => {
  const seen: string[] = [];
  const { data } = fakeAppData((request) => {
    seen.push(line(request));
    const url = new URL(request.url);
    if (url.searchParams.get("scheduled") === "true")
      return json(200, {
        items: [
          draft("s-late", { scheduled_at: "2026-11-30T03:00:00Z" }),
          draft("s-soon", { scheduled_at: "2026-11-16T03:00:00Z" }),
          draft("s-mid", { scheduled_at: "2026-11-20T03:00:00Z" }),
        ],
      });
    if (url.searchParams.get("cursor") === "c2") return json(200, { items: [draft("d3")] });
    return json(200, { items: [draft("d1"), draft("d2")], next_cursor: "c2" });
  });
  const lists = await data.queryClient.query(draftListsQuery(data));
  expect(lists.drafts.map((d) => d.id)).toEqual(["d1", "d2", "d3"]);
  expect(lists.scheduled.map((d) => d.id)).toEqual(["s-soon", "s-mid", "s-late"]);
  // The page size is the client's to choose: the lists differ by `scheduled`, and the next page by its cursor.
  expect(seen.map((s) => s.replace(/&?limit=\d+/, "")).toSorted()).toEqual(
    ["GET /drafts?scheduled=false", "GET /drafts?scheduled=false&cursor=c2", "GET /drafts?scheduled=true"].toSorted(),
  );
});

test("a draft is read with the ETag its next save sends, and its schedule", async () => {
  const { data } = fakeAppData(() => answer(draft("d1", { scheduled_at: "2026-11-20T03:00:00Z" }), '"7"'));
  const { draft: read, saved } = await readDraft(data, "d1");
  expect(read.text).toBe("Draft d1");
  expect(saved).toEqual({ id: "d1", etag: '"7"', scheduledAt: "2026-11-20T03:00:00Z" });
});

test("scheduling sends the time, answers the draft with its schedule, and marks the lists stale", async () => {
  const bodies: unknown[] = [];
  const { data, seen } = await withLists(async (request) => {
    bodies.push(await request.json());
    return answer(draft("d1", { scheduled_at: "2026-11-20T03:00:00Z" }), '"4"');
  });
  expect(await scheduleDraft(data, "d1", "2026-11-20T03:00:00.000Z")).toEqual({
    id: "d1",
    etag: '"4"',
    scheduledAt: "2026-11-20T03:00:00Z",
  });
  expect(seen).toEqual(["PUT /drafts/d1/schedule"]);
  expect(bodies).toEqual([{ publish_at: "2026-11-20T03:00:00.000Z" }]);
  expect(stale(data)).toBe(true);
});

test("a time past the server's limits (422) rejects with its words, and the lists are read again", async () => {
  const { data } = await withLists(() => problem(422, "A post can be scheduled no sooner than 5 minutes from now."));
  const error = await scheduleDraft(data, "d1", "2026-11-14T00:01:00.000Z").catch((e: unknown) => e);
  expect(error).toBeInstanceOf(ProblemError);
  expect(refusal(error)).toBe("A post can be scheduled no sooner than 5 minutes from now.");
  expect(stale(data)).toBe(true);
});

test("taking a schedule back answers the draft without one; one not scheduled (409) is read as it is", async () => {
  const { data, seen } = await withLists(() => answer(draft("d1"), '"5"'));
  expect(await unscheduleDraft(data, "d1")).toEqual({ id: "d1", etag: '"5"' });
  expect(seen).toEqual(["DELETE /drafts/d1/schedule"]);
  expect(stale(data)).toBe(true);

  const twice = await withLists((request) => (request.method === "DELETE" ? problem(409) : answer(draft("d1"), '"6"')));
  expect(await unscheduleDraft(twice.data, "d1")).toEqual({ id: "d1", etag: '"6"' });
  expect(twice.seen).toEqual(["DELETE /drafts/d1/schedule", "GET /drafts/d1"]);
});

test("deleting a draft, or one already gone (404), marks the lists stale; another refusal rejects", async () => {
  const { data, seen } = await withLists(() => new Response(null, { status: 204 }));
  await deleteDraft(data, "d1");
  expect(seen).toEqual(["DELETE /drafts/d1"]);
  expect(stale(data)).toBe(true);

  const gone = await withLists(() => problem(404));
  await deleteDraft(gone.data, "d1");
  expect(stale(gone.data)).toBe(true);

  const refused = await withLists(() => problem(500));
  expect(await deleteDraft(refused.data, "d1").catch((e: unknown) => e)).toBeInstanceOf(ProblemError);
});

test("publishing a draft posts it from the server, tops the home timeline with it, and marks the lists stale", async () => {
  const seen: string[] = [];
  const { data } = fakeAppData((request) => {
    seen.push(line(request));
    if (request.method === "POST") return json(201, status("new"));
    if (new URL(request.url).pathname.endsWith("/drafts")) return json(200, { items: [] });
    return json(200, page(["1"]));
  });
  await data.queryClient.infiniteQuery(homeTimelineQuery(data));
  await data.queryClient.query(draftListsQuery(data));
  seen.length = 0;
  const posted = await publishDraft(data, "d1");
  expect(posted.id).toBe("new");
  expect(seen).toEqual(["POST /drafts/d1/publish"]);
  expect(data.queryClient.getQueryData(homeTimelineQuery(data).queryKey)?.pages[0]?.items.map((s) => s.id)).toEqual([
    "new",
    "1",
  ]);
  expect(cachedStatus(data, "new")?.id).toBe("new");
  expect(stale(data)).toBe(true);
});

test("a draft gone when it is published (404) rejects as gone: it may have been posted", async () => {
  const { data, seen } = await withLists(() => problem(404));
  expect(await publishDraft(data, "d1").catch((e: unknown) => e)).toBeInstanceOf(DraftGoneError);
  expect(seen).toEqual(["POST /drafts/d1/publish"]);
  expect(stale(data)).toBe(true);
});

test("useDrafts gives the lists once read, and deletes and unschedules", async () => {
  const seen: string[] = [];
  const { data } = fakeAppData((request) => {
    seen.push(line(request));
    if (request.method === "GET")
      return json(200, {
        items: new URL(request.url).searchParams.get("scheduled") === "true" ? [draft("s1")] : [draft("d1")],
      });
    if (request.method === "DELETE" && new URL(request.url).pathname.endsWith("/schedule"))
      return answer(draft("s1"), '"2"');
    return new Response(null, { status: 204 });
  });
  expect(probe(data, useDrafts)).toMatchObject({ lists: undefined, failed: false });
  await data.queryClient.query(draftListsQuery(data));
  const drafts = probe(data, useDrafts);
  expect(drafts.lists?.drafts.map((d) => d.id)).toEqual(["d1"]);
  expect(drafts.lists?.scheduled.map((d) => d.id)).toEqual(["s1"]);
  seen.length = 0;
  await drafts.unschedule("s1");
  await drafts.remove("d1");
  expect(seen.filter((s) => !s.startsWith("GET"))).toEqual(["DELETE /drafts/s1/schedule", "DELETE /drafts/d1"]);
});

test("the composer schedules within the instance's limits, the server's defaults until they are read", async () => {
  const { data } = fakeAppData(() =>
    json(200, {
      domain: "my.example",
      limits: {
        max_post_characters: 500,
        drafts: { ...defaultDraftLimits, scheduled_post_min_lead_minutes: 1, scheduled_post_max_lead_days: 30 },
      },
    }),
  );
  expect(probe(data, useComposeSettings).schedule).toEqual({ minLeadMinutes: 5, maxLeadDays: 366 });
  expect(defaultComposeSettings.schedule).toEqual({ minLeadMinutes: 5, maxLeadDays: 366 });
  await data.queryClient.query(data.queries.queryOptions("get", "/instance"));
  expect(probe(data, useComposeSettings).schedule).toEqual({ minLeadMinutes: 1, maxLeadDays: 30 });
});
