// SPDX-License-Identifier: AGPL-3.0-only

import { describe, expect, test } from "bun:test";
import {
  autoDeleteQuery,
  previewAutoDelete,
  saveAutoDelete,
  useAutoDelete,
  type AutoDeletePolicy,
} from "./auto-delete";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { refusedWith } from "./test/refused";

/** A policy that keeps nothing in particular, deleting posts older than `days`. */
const policy = (days: number): AutoDeletePolicy => ({
  enabled: true,
  min_age_days: days,
  keep_pinned: false,
  keep_bookmarked: false,
  keep_favourited: false,
  keep_media: false,
  keep_polls: false,
  keep_visibilities: [],
  keep_min_boosts: 0,
  keep_min_favourites: 0,
});

describe("useAutoDelete", () => {
  test("is undefined until it is read, then the saved policy with the server's limit over it", async () => {
    const { data } = fakeData();
    expect(probe(data, useAutoDelete)).toBeUndefined();
    await data.queryClient.query(autoDeleteQuery(data));
    const settings = probe(data, useAutoDelete);
    expect(settings).toMatchObject({ enabled: true, min_age_days: 180, keep_media: true });
    expect(settings?.server_limit).toMatchObject({ max_age_days: 365, allow_media: false, bound: true });
    // What the server does under its limit: the saved keep of media does not hold.
    expect(settings?.server_limit?.effective.keep_media).toBe(false);
  });

  test("has no limit of the server when the server sets none", async () => {
    const { data } = fakeData({ settings: { autoDeleteLimit: undefined } });
    await data.queryClient.query(autoDeleteQuery(data));
    expect(probe(data, useAutoDelete)?.server_limit).toBeUndefined();
  });
});

describe("saveAutoDelete", () => {
  test("puts the policy, and the cache holds the settings the server answered", async () => {
    const { fake, data, requests } = fakeData();
    const sent = fake.script("PUT /preferences/auto_delete");
    await data.queryClient.query(autoDeleteQuery(data));
    const saved = await saveAutoDelete(data, { ...policy(90), keep_pinned: true });
    expect(requests.filter((r) => !r.startsWith("GET "))).toEqual(["PUT /preferences/auto_delete"]);
    expect(sent.seen.map((s) => s.body)).toEqual([{ ...policy(90), keep_pinned: true }]);
    expect(saved).toMatchObject({ min_age_days: 90, keep_pinned: true, keep_media: false });
    expect(probe(data, useAutoDelete)).toEqual(saved);
  });

  test("a policy under the server's limit is not bound by it", async () => {
    const { data } = fakeData();
    const saved = await saveAutoDelete(data, policy(90));
    expect(saved.server_limit?.bound).toBe(false);
  });

  test("a policy over the server's limit comes back bound, with what the server does instead", async () => {
    const { data } = fakeData();
    const saved = await saveAutoDelete(data, policy(1000));
    expect(saved.min_age_days).toBe(1000);
    expect(saved.server_limit).toMatchObject({ bound: true });
    expect(saved.server_limit?.effective.min_age_days).toBe(365);
  });

  test("an age out of the range is refused with a 422, and the cache keeps the settings", async () => {
    const { data } = fakeData();
    const before = await data.queryClient.query(autoDeleteQuery(data));
    for (const days of [0, 3651, 1.5]) expect((await refusedWith(saveAutoDelete(data, policy(days)))).status).toBe(422);
    expect(probe(data, useAutoDelete)).toEqual(before);
  });
});

describe("previewAutoDelete", () => {
  test("estimates how many posts a policy would delete, with examples, without saving it", async () => {
    const { fake, data, requests } = fakeData();
    const saved = fake.script("PUT /preferences/auto_delete");
    const preview = await previewAutoDelete(data, policy(30));
    expect(preview.count).toBeGreaterThan(0);
    expect(typeof preview.more).toBe("boolean");
    expect(Array.isArray(preview.examples)).toBe(true);
    expect(requests).toEqual(["POST /preferences/auto_delete/preview"]);
    expect(saved.seen).toHaveLength(0);
  });

  test("a shorter keep estimates more posts than a longer one", async () => {
    const { data } = fakeData();
    const short = await previewAutoDelete(data, policy(7));
    const long = await previewAutoDelete(data, policy(700));
    expect(short.count).toBeGreaterThan(long.count);
  });

  test("an age out of the range is refused with a 422", async () => {
    const { data } = fakeData();
    expect((await refusedWith(previewAutoDelete(data, policy(0)))).status).toBe(422);
  });
});
