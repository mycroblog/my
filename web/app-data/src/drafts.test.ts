// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { scenario, type FakeApi, type FakeOptions } from "@my/e2e/fake";
import { ProblemError } from "@mycroblog/mykit";
import type { AppData } from "./app-data";
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
  type SavedDraft,
} from "./drafts";
import { cachedStatus } from "./shadow";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { homeTimelineQuery } from "./timeline";

// In the scenario the reader keeps drafts (`river` among them) and scheduled posts (`weekly` among them).
const river = "yuki-draft-river";
const weekly = "yuki-scheduled-weekly";
/** The ID of no draft: of one deleted on another device since this one read it. */
const gone = "00000000-0000-7000-8000-00000000ffff";

/** What a save answered, its ETag as "an ETag" when it reads as the server's do, and its ID as `id` says. */
const shape = (saved: SavedDraft, id: string = saved.id) => ({
  ...saved,
  id: saved.id === id ? "the draft" : "another",
  etag: /^"\d+"$/.test(saved.etag) ? "an ETag" : saved.etag,
});

/** Whether the lists of drafts are marked to be read again. */
const stale = (data: AppData) => data.queryClient.getQueryState(draftListsQuery(data).queryKey)?.isInvalidated;

/** The `If-Match` of each save over a draft the server was sent, once `fake` is told to keep them. */
const saves = (fake: FakeApi) => {
  const sent = fake.script("PUT /drafts/{id}");
  return () => sent.seen.map((s) => s.headers.get("If-Match"));
};

/** Data whose lists of drafts are read already, to see a write mark them stale, and the requests sent since. */
async function withLists(options: FakeOptions = {}) {
  const made = fakeData(options);
  await made.data.queryClient.query(draftListsQuery(made.data));
  made.requests.length = 0;
  return made;
}

test("a draft saved before is saved over with the ETag it was read with, and the next save sends the new one", async () => {
  const { fake, data, requests } = await withLists();
  const id = fake.idOf(weekly);
  const sent = saves(fake);
  const { saved: read } = await readDraft(data, id);
  requests.length = 0;
  const saved = await saveDraft(data, { text: "hi", visibility: "public" }, read);
  expect(requests).toEqual([`PUT /drafts/${id}`]);
  expect(sent()).toEqual([read.etag]);
  expect(shape(saved, id)).toEqual({ id: "the draft", etag: "an ETag", scheduledAt: read.scheduledAt });
  expect(saved.etag).not.toBe(read.etag);
  expect(read.scheduledAt).toBeDefined();
  expect(stale(data)).toBe(true);
});

test("a draft never saved is saved as a new one, with its ETag", async () => {
  const { data, requests } = await withLists();
  const saved = await saveDraft(data, { text: "hi", visibility: "public" });
  expect(shape(saved)).toEqual({ id: "the draft", etag: "an ETag" });
  expect((await readDraft(data, saved.id)).draft.text).toBe("hi");
  expect(requests.slice(0, 1)).toEqual(["POST /drafts"]);
});

test("a save the server refuses for a newer one saved elsewhere (412) is refused, when the reader is to choose", async () => {
  const { fake, data, requests } = await withLists();
  const id = fake.idOf(river);
  const sent = saves(fake);
  const error = await saveDraft(
    data,
    { text: "hi", visibility: "public" },
    { id, etag: '"1"' },
    {
      onConflict: "refuse",
    },
  ).catch((e: unknown) => e);
  expect(error).toBeInstanceOf(DraftConflictError);
  // Nothing is saved: neither over the newer one nor as another draft.
  expect(requests).toEqual([`PUT /drafts/${id}`]);
  expect(sent()).toEqual(['"1"']);
  expect(stale(data)).toBe(true);
});

test("a draft deleted elsewhere since it was read is saved as a new one", async () => {
  const { fake, data, requests } = await withLists();
  const sent = saves(fake);
  const saved = await saveDraft(data, { text: "hi", visibility: "public" }, { id: gone, etag: '"3"' });
  expect(shape(saved, gone)).toEqual({ id: "another", etag: "an ETag" });
  expect(requests).toEqual([`PUT /drafts/${gone}`, "POST /drafts"]);
  expect(sent()).toEqual(['"3"']);
});

test("a draft gone since it was read is refused when the reader is to choose: a new one could post it twice", async () => {
  // Its publication's answer was lost, or its schedule fired: Post again must not save it as a new draft to publish.
  const { data, requests } = await withLists();
  const error = await saveDraft(
    data,
    { text: "hi", visibility: "public" },
    { id: gone, etag: '"3"' },
    { onConflict: "refuse" },
  ).catch((e: unknown) => e);
  expect(error).toBeInstanceOf(DraftGoneError);
  expect(requests).toEqual([`PUT /drafts/${gone}`]);
});

test("a save refused for anything else rejects with the server's words, and saves no copy", async () => {
  const { fake, data, requests } = await withLists();
  const id = fake.idOf(river);
  fake.script("PUT /drafts/{id}", {
    fail: { type: "about:blank", title: "Problem", status: 422, detail: "You have 100 drafts, the most you may keep." },
  });
  const error = await saveDraft(
    data,
    { text: "hi", visibility: "public" },
    { id, etag: '"1"' },
    {
      onConflict: "refuse",
    },
  ).catch((e: unknown) => e);
  expect(error).not.toBeInstanceOf(DraftConflictError);
  expect(refusal(error)).toBe("You have 100 drafts, the most you may keep.");
  expect(requests).toEqual([`PUT /drafts/${id}`]);
});

test("the lists read every page of the drafts and of the scheduled posts apart, the scheduled soonest first", async () => {
  // More drafts than the largest page the server gives (40), and three scheduled posts, saved in another order than
  // their times.
  const plain = Array.from({ length: 45 }, (_, i) => `draft-${String(i + 1)}`);
  const times = { late: 20, soon: 2, mid: 9 };
  const { fake, data, requests } = fakeData({
    seed: {
      people: scenario.people,
      drafts: [
        ...plain.map((key) => ({ key, author: "yuki", text: `Draft ${key}`, language: "en" })),
        ...Object.entries(times).map(([key, inDays]) => ({
          key,
          author: "yuki",
          text: `Scheduled ${key}`,
          language: "en",
          schedule: { inDays, hour: 3 },
        })),
      ],
    },
  });
  const lists = await data.queryClient.query(draftListsQuery(data));
  // The server lists the newest save first.
  expect(lists.drafts.map((d) => d.id)).toEqual(plain.toReversed().map(fake.idOf));
  expect(lists.scheduled.map((d) => d.id)).toEqual(["soon", "mid", "late"].map(fake.idOf));
  // The page size is the client's to choose: the lists differ by `scheduled`, and the next page by its cursor.
  const asked = requests.map((r) => new URL(r.slice("GET ".length), "https://demo.test").searchParams);
  const pages = (scheduled: string) =>
    asked.filter((q) => q.get("scheduled") === scheduled).map((q) => q.has("cursor"));
  expect(pages("true")).toEqual([false]);
  expect(pages("false").length).toBeGreaterThan(1);
  expect(pages("false")).toEqual([
    false,
    ...pages("false")
      .slice(1)
      .map(() => true),
  ]);
});

test("a draft is read with the ETag its next save sends, and its schedule", async () => {
  const { fake, data } = fakeData();
  const id = fake.idOf(weekly);
  const { draft: read, saved } = await readDraft(data, id);
  expect(read.text).toBe("Weekly photo thread is open. Post one picture from your week, no captions needed.");
  expect(read.scheduled_at).toBeDefined();
  expect(shape(saved, id)).toEqual({ id: "the draft", etag: "an ETag", scheduledAt: read.scheduled_at });
});

test("scheduling sends the time, answers the draft with its schedule, and marks the lists stale", async () => {
  const { fake, data, requests } = await withLists();
  const id = fake.idOf(river);
  const sent = fake.script("PUT /drafts/{id}/schedule");
  const at = new Date(Date.now() + 3 * 86_400_000).toISOString();
  expect(shape(await scheduleDraft(data, id, at), id)).toEqual({ id: "the draft", etag: "an ETag", scheduledAt: at });
  expect(requests).toEqual([`PUT /drafts/${id}/schedule`]);
  expect(sent.seen.map((s) => s.body)).toEqual([{ publish_at: at }]);
  expect(stale(data)).toBe(true);
});

test("a time past the server's limits (422) rejects with its words, and the lists are read again", async () => {
  const { fake, data } = await withLists();
  const soon = new Date(Date.now() + 60_000).toISOString();
  const error = await scheduleDraft(data, fake.idOf(river), soon).catch((e: unknown) => e);
  expect(error).toBeInstanceOf(ProblemError);
  expect(refusal(error)).toBe("A post can be scheduled no sooner than 5 minutes from now.");
  expect(stale(data)).toBe(true);
});

test("taking a schedule back answers the draft without one; one not scheduled (409) is read as it is", async () => {
  const { fake, data, requests } = await withLists();
  const id = fake.idOf(weekly);
  expect(shape(await unscheduleDraft(data, id), id)).toEqual({ id: "the draft", etag: "an ETag" });
  expect(requests).toEqual([`DELETE /drafts/${id}/schedule`]);
  expect(stale(data)).toBe(true);

  const twice = await withLists();
  const plain = twice.fake.idOf(river);
  expect(shape(await unscheduleDraft(twice.data, plain), plain)).toEqual({ id: "the draft", etag: "an ETag" });
  expect(twice.requests).toEqual([`DELETE /drafts/${plain}/schedule`, `GET /drafts/${plain}`]);
});

test("deleting a draft, or one already gone (404), marks the lists stale; another refusal rejects", async () => {
  const { fake, data, requests } = await withLists();
  const id = fake.idOf(river);
  await deleteDraft(data, id);
  expect(requests).toEqual([`DELETE /drafts/${id}`]);
  expect(stale(data)).toBe(true);

  const missing = await withLists();
  await deleteDraft(missing.data, gone);
  expect(stale(missing.data)).toBe(true);

  const refused = await withLists();
  refused.fake.script("DELETE /drafts/{id}", { fail: 500 });
  expect(await deleteDraft(refused.data, refused.fake.idOf(river)).catch((e: unknown) => e)).toBeInstanceOf(
    ProblemError,
  );
});

test("publishing a draft posts it from the server, tops the home timeline with it, and marks the lists stale", async () => {
  const { fake, data, requests } = fakeData();
  const id = fake.idOf(river);
  const before = (await data.queryClient.infiniteQuery(homeTimelineQuery(data))).pages[0]?.items.map((s) => s.id);
  await data.queryClient.query(draftListsQuery(data));
  requests.length = 0;
  const posted = await publishDraft(data, id);
  expect(posted.content).toContain("Walked along the river this morning.");
  expect(requests).toEqual([`POST /drafts/${id}/publish`]);
  expect(data.queryClient.getQueryData(homeTimelineQuery(data).queryKey)?.pages[0]?.items.map((s) => s.id)).toEqual([
    posted.id,
    ...(before ?? []),
  ]);
  expect(cachedStatus(data, posted.id)?.id).toBe(posted.id);
  expect(stale(data)).toBe(true);
});

test("a draft gone when it is published (404) rejects as gone: it may have been posted", async () => {
  const { data, requests } = await withLists();
  expect(await publishDraft(data, gone).catch((e: unknown) => e)).toBeInstanceOf(DraftGoneError);
  expect(requests).toEqual([`POST /drafts/${gone}/publish`]);
  expect(stale(data)).toBe(true);
});

test("useDrafts gives the lists once read, and deletes and unschedules", async () => {
  const { fake, data, requests } = fakeData();
  expect(probe(data, useDrafts)).toMatchObject({ lists: undefined, failed: false });
  await data.queryClient.query(draftListsQuery(data));
  const drafts = probe(data, useDrafts);
  expect(drafts.lists?.drafts.map((d) => d.id)).toContain(fake.idOf(river));
  expect(drafts.lists?.scheduled.map((d) => d.id)).toContain(fake.idOf(weekly));
  requests.length = 0;
  await drafts.unschedule(fake.idOf(weekly));
  await drafts.remove(fake.idOf(river));
  expect(requests.filter((s) => !s.startsWith("GET"))).toEqual([
    `DELETE /drafts/${fake.idOf(weekly)}/schedule`,
    `DELETE /drafts/${fake.idOf(river)}`,
  ]);
});

test("the composer schedules within the instance's limits, the server's defaults until they are read", async () => {
  const { fake, data } = fakeData();
  fake.script("GET /instance", {
    change: (instance) => ({
      ...instance,
      limits: {
        ...instance.limits,
        drafts: { ...defaultDraftLimits, scheduled_post_min_lead_minutes: 1, scheduled_post_max_lead_days: 30 },
      },
    }),
  });
  expect(probe(data, useComposeSettings).schedule).toEqual({ minLeadMinutes: 5, maxLeadDays: 366 });
  expect(defaultComposeSettings.schedule).toEqual({ minLeadMinutes: 5, maxLeadDays: 366 });
  await data.queryClient.query(data.queries.queryOptions("get", "/instance"));
  expect(probe(data, useComposeSettings).schedule).toEqual({ minLeadMinutes: 1, maxLeadDays: 30 });
});
