// SPDX-License-Identifier: AGPL-3.0-only

// The account's preferences as the app's data: what the server says of them, a change's answer in its place, a refused
// change's error, and what a change of the timelines' languages marks to be read again. What a reader sees while a
// change is on its way needs a client's render, which the features' tests have (app-features, preferences.dom.test.tsx).

import { describe, expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { NetworkError, ProblemError } from "@mycroblog/mykit";
import { viewerQuery } from "./accounts";
import type { AppData } from "./app-data";
import { refusedField, useComposeSettings } from "./compose";
import {
  preferencesQuery,
  updatePreferences,
  usePreferences,
  useUpdatePreferences,
  type PreferencesChange,
} from "./preferences";
import { fakeData } from "./test/data";
import { probe } from "./test/probe";
import { homeTimelineQuery, listTimelineQuery } from "./timeline";

/** What the cache holds of the preferences: what the server last said. */
const cached = (data: AppData) => data.queryClient.getQueryData(preferencesQuery(data).queryKey);

/** What `change` settles with: the preferences the server answered, or the error it was refused with. */
const settled = (data: AppData, change: PreferencesChange) => updatePreferences(data, change).catch((e: unknown) => e);

describe("reading", () => {
  test("a new account's preferences follow the browser, insert new posts and draw large cards", async () => {
    const { data, requests } = fakeData();

    expect(await data.queryClient.query(preferencesQuery(data))).toMatchObject({
      locale_follows_browser: true,
      time_zone_follows_browser: true,
      time_zone_chosen: false,
      timeline_auto_insert: true,
      link_card_size: "large",
      timeline_languages: [],
    });
    expect(requests).toEqual(["GET /preferences"]);
  });

  const chosen = { locale: "ja", locale_follows_browser: false, link_card_size: "small" } as const;

  test("the reader's preferences are what the server said, and none until it is read", async () => {
    const { data } = fakeData({ preferences: chosen });
    expect(probe(data, () => usePreferences())).toBeUndefined();
    expect(probe(data, () => usePreferences(false))).toBeUndefined();

    const read = await data.queryClient.query(preferencesQuery(data));

    expect(read).toMatchObject(chosen);
    expect(probe(data, () => usePreferences())).toEqual(read);
    expect(probe(data, () => usePreferences(true))).toEqual(read);
  });

  // A finding of #3199: without a session the hook is said to give none, and it gives what the cache holds, the
  // preferences of the account that read them.
  test.failing("a reader without a session has none, whatever the cache holds", async () => {
    const { data } = fakeData({ preferences: chosen });
    await data.queryClient.query(preferencesQuery(data));

    expect(probe(data, () => usePreferences(false))).toBeUndefined();
  });
});

describe("a change", () => {
  test("answers with the preferences after it, which take the cache's place without another read", async () => {
    const { fake, data, requests } = fakeData();
    await data.queryClient.query(preferencesQuery(data));
    const sent = fake.script("PATCH /preferences");

    const answered = await updatePreferences(data, { link_card_size: "small", timeline_auto_insert: false });

    expect(sent.seen.map((request) => request.body)).toEqual([
      { link_card_size: "small", timeline_auto_insert: false },
    ]);
    expect(answered).toMatchObject({ link_card_size: "small", timeline_auto_insert: false });
    expect(cached(data)).toEqual(answered);
    expect(probe(data, () => usePreferences())).toEqual(answered);
    expect(requests).toEqual(["GET /preferences", "PATCH /preferences"]);
  });

  test("is the server's to apply: until it answers, the cache holds what it last said", async () => {
    const { fake, data } = fakeData();
    const before = await data.queryClient.query(preferencesQuery(data));
    const held = fake.script("PATCH /preferences", { hold: "response" });

    const change = updatePreferences(data, { link_card_size: "small" });
    await held.served();
    expect(cached(data)).toEqual(before);
    held.release();
    const answered = await change;

    expect(answered.link_card_size).toBe("small");
    expect(cached(data)).toEqual(answered);
  });

  test("follows the server's rules: a language or a zone chosen stops following the browser, a policy changes by kind", async () => {
    const { data } = fakeData();
    await data.queryClient.query(preferencesQuery(data));

    const answered = await updatePreferences(data, {
      locale: "ja",
      time_zone: "America/New_York",
      default_interaction_policy: { quote: { automatic: "followers", manual: "everyone" } },
    });

    expect(answered).toMatchObject({
      locale: "ja",
      locale_follows_browser: false,
      time_zone: "America/New_York",
      time_zone_chosen: true,
      time_zone_follows_browser: false,
      default_interaction_policy: {
        reply: { automatic: "everyone" },
        announce: { automatic: "everyone" },
        quote: { automatic: "followers", manual: "everyone" },
      },
    });
    // Following the browser again keeps what was chosen, for what has no browser.
    expect(
      await updatePreferences(data, { locale_follows_browser: true, time_zone_follows_browser: true }),
    ).toMatchObject({
      locale: "ja",
      locale_follows_browser: true,
      time_zone: "America/New_York",
      time_zone_chosen: true,
      time_zone_follows_browser: true,
    });
  });

  test("made through the hook is the same change, on the data of its provider", async () => {
    const { data } = fakeData();
    await data.queryClient.query(preferencesQuery(data));

    const answered = await probe(data, () => useUpdatePreferences())({ require_media_description: true });

    expect(answered.require_media_description).toBe(true);
    expect(cached(data)).toEqual(answered);
  });

  test("of the new post's defaults reaches the composer's settings", async () => {
    const { data } = fakeData();
    await data.queryClient.query(preferencesQuery(data));
    expect(probe(data, () => useComposeSettings())).toMatchObject({
      visibility: "public",
      language: "und",
      requireDescriptions: false,
    });

    await updatePreferences(data, {
      default_visibility: "private",
      default_post_language: "ja",
      require_media_description: true,
      default_interaction_policy: { quote: { automatic: "nobody" } },
    });

    expect(probe(data, () => useComposeSettings())).toMatchObject({
      visibility: "private",
      language: "ja",
      requireDescriptions: true,
      policy: { reply: { audience: "everyone" }, quote: { audience: "nobody", approval: false } },
    });
  });
});

describe("a change the server refuses", () => {
  test("rejects with the server's problem, and the cache keeps what the server last said", async () => {
    const { fake, data } = fakeData();
    const before = await data.queryClient.query(preferencesQuery(data));
    fake.script("PATCH /preferences", { fail: 422 });

    const error = await settled(data, { link_card_size: "small" });

    expect(error).toBeInstanceOf(ProblemError);
    expect((error as ProblemError).status).toBe(422);
    expect(cached(data)).toEqual(before);
    expect(probe(data, () => usePreferences())).toEqual(before);
  });

  test("rejects with the network's error when no answer came", async () => {
    const { fake, data } = fakeData();
    const before = await data.queryClient.query(preferencesQuery(data));
    fake.script("PATCH /preferences", { fail: "network" });

    expect(await settled(data, { timeline_auto_insert: false })).toBeInstanceOf(NetworkError);
    expect(cached(data)).toEqual(before);
  });

  // The values the server refuses, each with the member its problem names: what a settings form marks.
  const refused: readonly (readonly [string, PreferencesChange, string])[] = [
    ["a language that is no tag", { locale: "not a tag" }, "locale"],
    ["the undetermined language as the interface's", { locale: "und" }, "locale"],
    ["a time zone nobody has", { time_zone: "Mars/Olympus_Mons" }, "time_zone"],
    ["a post language that is no tag", { default_post_language: "not a tag" }, "default_post_language"],
    ["a timeline language that is no tag", { timeline_languages: ["en", "not a tag"] }, "timeline_languages"],
    [
      "more than 64 timeline languages",
      { timeline_languages: Array.from({ length: 65 }, (_, i) => `en-x-n${String(i)}`) },
      "timeline_languages",
    ],
  ];

  for (const [what, change, member] of refused)
    test(`${what} is refused with its member named, and nothing changes`, async () => {
      const { data } = fakeData();
      const before = await data.queryClient.query(preferencesQuery(data));

      const error = await settled(data, change);

      expect(error).toBeInstanceOf(ProblemError);
      expect(refusedField(error)?.member).toBe(member);
      expect(cached(data)).toEqual(before);
      expect((await data.api.GET("/preferences")).data).toEqual(before);
    });
});

describe("the timelines' languages", () => {
  /** A server where the reader follows sam, who posted, and keeps a list of sam; its timelines and the viewer read. */
  async function withTimelines() {
    const made = fakeData({
      seed: {
        people: scenario.people,
        follows: [{ follower: "yuki", followee: "sam" }],
        posts: [{ key: "loaf", author: "sam", text: "The first loaf.", visibility: "public", language: "en" }],
        lists: [{ key: "bread", owner: "yuki", name: "Bread", members: ["sam"] }],
      },
    });
    const { fake, data } = made;
    const list = fake.listPath(0).replace("/lists/", "");
    await data.queryClient.query(preferencesQuery(data));
    await data.queryClient.query(viewerQuery(data));
    await data.queryClient.infiniteQuery(homeTimelineQuery(data));
    await data.queryClient.infiniteQuery(listTimelineQuery(data, list));
    const marked = (key: readonly unknown[]) => data.queryClient.getQueryState(key)?.isInvalidated;
    return {
      ...made,
      /** Whether the home timeline, the list's and the viewer are marked to be read again. */
      marks: () => ({
        home: marked(homeTimelineQuery(data).queryKey),
        list: marked(listTimelineQuery(data, list).queryKey),
        viewer: marked(viewerQuery(data).queryKey),
      }),
    };
  }

  test("changed, every timeline read is marked to be read again, and nothing else", async () => {
    const { data, marks } = await withTimelines();

    const answered = await updatePreferences(data, { timeline_languages: ["ja", "en", "ja"] });

    // Sorted, each once, as the server keeps them.
    expect(answered.timeline_languages).toEqual(["en", "ja"]);
    expect(marks()).toEqual({ home: true, list: true, viewer: false });
  });

  test("a change of anything else leaves the timelines as they were read", async () => {
    const { data, marks } = await withTimelines();

    await updatePreferences(data, { link_card_size: "small", timeline_auto_insert: false });

    expect(marks()).toEqual({ home: false, list: false, viewer: false });
  });

  test("a refused change of them marks nothing", async () => {
    const { data, marks } = await withTimelines();

    expect(await settled(data, { timeline_languages: ["not a tag"] })).toBeInstanceOf(ProblemError);

    expect(marks()).toEqual({ home: false, list: false, viewer: false });
  });
});
