// SPDX-License-Identifier: AGPL-3.0-only

// The reader's preferences while a change of them is on its way (#3199): what the app shows is what the server last
// said with the change applied, from the moment it is made, and without it again once the server refused it. The
// cache keeps what the server said, so a test reads what a part of the app reads, `usePreferences`, on a client's
// render; what the server answers and what it refuses is the data's own test (app-data, preferences.test.ts).

import { describe, expect, test } from "bun:test";
import {
  preferencesQuery,
  updatePreferences,
  usePreferences,
  type AppData,
  type Preferences,
  type PreferencesChange,
} from "@my/app-data";
import { NetworkError, ProblemError } from "@mycroblog/mykit";
import { act } from "@testing-library/react";
import { fakeData, renderFeature } from "./test/feature";

/**
 * A reader whose preferences are read, and a part of the app that shows them: `shown()` is what it shows now, and
 * `change(c)` makes a change as a press does, returning what it settles with (the server's answer, or the error).
 */
async function reading() {
  const { fake, data } = fakeData();
  const read = await data.queryClient.query(preferencesQuery(data));
  const seen: { now?: Preferences | undefined } = {};
  function Shown() {
    seen.now = usePreferences();
    return null;
  }
  renderFeature(data, <Shown />);
  return {
    fake,
    data,
    read,
    shown: () => seen.now,
    change: (c: PreferencesChange) => made(data, c),
  };
}

/** Makes `change` inside React's act, as a press does, and gives what it settles with. */
function made(data: AppData, change: PreferencesChange): Promise<Preferences | Error> {
  let settles: Promise<Preferences | Error> | undefined;
  act(() => {
    settles = updatePreferences(data, change).catch((e: unknown) => (e instanceof Error ? e : new Error(String(e))));
  });
  if (settles === undefined) throw new Error("test: the change was not made");
  return settles;
}

/** Waits for `settles` inside React's act, so that what the answer changes is drawn. */
const settled = <T,>(settles: Promise<T>): Promise<T> => act(() => settles);

/** What the cache holds of the preferences: what the server last said. */
const cached = (data: AppData) => data.queryClient.getQueryData(preferencesQuery(data).queryKey);

test("a change shows at once, before the server is asked, while the cache keeps what the server said", async () => {
  const { fake, data, read, shown, change } = await reading();
  expect(shown()).toEqual(read);
  const sent = fake.script("PATCH /preferences", { hold: "request" });

  const settles = change({ link_card_size: "small", timeline_auto_insert: false });

  // No turn passed: the request has not reached the server.
  expect(shown()).toEqual({ ...read, link_card_size: "small", timeline_auto_insert: false });
  expect(cached(data)).toEqual(read);
  await sent.arrived();
  expect(shown()).toMatchObject({ link_card_size: "small", timeline_auto_insert: false });

  sent.release();
  const answered = await settled(settles);
  expect(answered).toMatchObject({ link_card_size: "small", timeline_auto_insert: false });
  expect(shown()).toEqual(answered as Preferences);
  expect(cached(data)).toEqual(answered as Preferences);
});

describe("a change the server refuses", () => {
  for (const [what, fail, kind] of [
    ["with a problem", 422, ProblemError],
    ["by not answering", "network", NetworkError],
  ] as const)
    test(`${what} shows until then, and no more after: the reader sees what the server last said`, async () => {
      const { fake, data, read, shown, change } = await reading();
      const sent = fake.script("PATCH /preferences", { hold: "request", fail });

      const settles = change({ link_card_size: "small", locale: "en" });
      expect(shown()).toMatchObject({ link_card_size: "small", locale: "en", locale_follows_browser: false });
      await sent.arrived();
      sent.release();

      expect(await settled(settles)).toBeInstanceOf(kind);
      expect(shown()).toEqual(read);
      expect(cached(data)).toEqual(read);
    });
});

describe("two changes on their way", () => {
  /** A reader who held new posts back, then chose small cards, each change held at the server. */
  async function both(first: "refused" | "kept", second: "refused" | "kept") {
    const made = await reading();
    const script = (outcome: "refused" | "kept") =>
      made.fake.script("PATCH /preferences", {
        times: 1,
        hold: "request",
        ...(outcome === "refused" ? { fail: 500 } : {}),
      });
    // A script acts on one request: the first on the first change, the second on the second.
    const scripts = [script(first), script(second)] as const;
    const settles = [made.change({ timeline_auto_insert: false }), made.change({ link_card_size: "small" })] as const;
    expect(made.shown()).toEqual({ ...made.read, timeline_auto_insert: false, link_card_size: "small" });
    await scripts[1].arrived(2);
    return { ...made, scripts, settles };
  }

  test("the one refused first leaves the other showing", async () => {
    const { read, shown, scripts, settles } = await both("refused", "kept");

    scripts[0].release();
    expect(await settled(settles[0])).toBeInstanceOf(ProblemError);
    expect(shown()).toEqual({ ...read, link_card_size: "small" });

    scripts[1].release();
    const answered = await settled(settles[1]);
    expect(answered).toEqual({ ...read, link_card_size: "small" });
    expect(shown()).toEqual({ ...read, link_card_size: "small" });
  });

  test("the one refused last leaves the other as the server answered it", async () => {
    const { read, shown, scripts, settles } = await both("kept", "refused");

    scripts[0].release();
    expect(await settled(settles[0])).toEqual({ ...read, timeline_auto_insert: false });
    // The answer of the first took the cache's place, and the second still shows over it.
    expect(shown()).toEqual({ ...read, timeline_auto_insert: false, link_card_size: "small" });

    scripts[1].release();
    expect(await settled(settles[1])).toBeInstanceOf(ProblemError);
    expect(shown()).toEqual({ ...read, timeline_auto_insert: false });
  });

  test("answered out of their order, each shows until its own answer came", async () => {
    const { read, shown, scripts, settles } = await both("kept", "kept");

    scripts[1].release();
    expect(await settled(settles[1])).toMatchObject({ link_card_size: "small" });
    expect(shown()).toEqual({ ...read, timeline_auto_insert: false, link_card_size: "small" });

    scripts[0].release();
    await settled(settles[0]);
    expect(shown()).toEqual({ ...read, timeline_auto_insert: false, link_card_size: "small" });
  });
});

describe("what shows while a change is on its way is what the server then answers", () => {
  // Each change, and the server's rule it meets.
  const changes: readonly (readonly [string, PreferencesChange])[] = [
    ["a language chosen stops following the browser", { locale: "en" }],
    ["a language given with the choice to follow keeps following", { locale: "en", locale_follows_browser: true }],
    ["a time zone chosen is chosen, and stops following the browser", { time_zone: "America/New_York" }],
    [
      "a time zone given with the choice to follow keeps following",
      { time_zone: "America/New_York", time_zone_follows_browser: true },
    ],
    [
      "a policy changes the kinds it names, and the others stay",
      { default_interaction_policy: { quote: { automatic: "followers", manual: "everyone" } } },
    ],
    ["the timelines' languages are sorted, each once", { timeline_languages: ["ja", "en", "ja", "de"] }],
    [
      "the defaults of a new post",
      { default_visibility: "private", default_post_language: "ja", require_media_description: true },
    ],
  ];

  for (const [rule, c] of changes)
    test(rule, async () => {
      const { fake, read, shown, change } = await reading();
      const sent = fake.script("PATCH /preferences", { hold: "request" });

      const settles = change(c);
      const meanwhile = shown();
      expect(meanwhile).not.toEqual(read);
      await sent.arrived();
      sent.release();

      expect(await settled(settles)).toEqual(meanwhile as Preferences);
    });
});
