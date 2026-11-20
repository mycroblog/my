// SPDX-License-Identifier: AGPL-3.0-only

// A change of language while the app is open (#3094): the app loads the new language's strings before it draws in
// it, so the screen stays in the language it is in until they are loaded, with no flash of message ids nor of en, and
// a load that fails leaves it there. The tests read every locale with the package's preload, so the strings of a
// language here are held for a test by the loading the Reader asks of strings.ts, and let through for the others.

import { afterEach, describe, expect, mock, test } from "bun:test";
import { preferencesQuery, updatePreferences } from "@my/app-data";
import { cleanup, waitFor } from "@testing-library/react";
import { main, renderApp, type AppOptions } from "./test/app";
import { ownDevice } from "./test/device";
import * as strings from "./strings";

const loadStrings = strings.loadStrings;
const hasStrings = strings.hasStrings;

/** A load a test holds: its promise, and what ends it. */
type Held = { promise: Promise<undefined>; resolve: () => void; reject: (error: Error) => void };

/** The locales whose loading a test holds, each with what lets it end. */
const held = new Map<string, Held>();
/** The loads the app asked for, by locale. */
const asked: string[] = [];

const language = (locale: string) => locale.split("-")[0] ?? locale;

await mock.module("./strings", () => ({
  ...strings,
  loadStrings: (locale: string) => {
    asked.push(locale);
    return held.get(language(locale))?.promise ?? loadStrings(locale);
  },
  hasStrings: (locale: string) => !held.has(language(locale)) && hasStrings(locale),
}));

/** Holds the loading of `locale`'s strings until the test lets it end, loaded or failed. */
function hold(locale: string) {
  let resolve: Held["resolve"] = () => undefined;
  let reject: Held["reject"] = () => undefined;
  const promise = new Promise<undefined>((ok, fail) => {
    resolve = () => {
      ok(undefined);
    };
    reject = fail;
  });
  const load: Held = { promise, resolve, reject };
  held.set(locale, load);
  return {
    loaded: () => {
      held.delete(locale);
      load.resolve();
    },
    failed: () => {
      load.reject(new Error("test: the chunk of the strings did not load"));
    },
  };
}

afterEach(() => {
  held.clear();
  asked.length = 0;
});

const page = () => document.documentElement.lang;

/** The title of the screen in place, as its heading says it. */
const heading = () =>
  main()
    .queryAllByRole("heading", { level: 1 })
    .map((title) => title.textContent)
    .join();

/** The title of the lists' screen, a screen that shows little, in each language. */
const lists = { en: "Lists", ja: "リスト" };

/** Waits until the page says it is in `lang` and the screen's title reads `title`. */
const reads = (lang: string, title: string) =>
  waitFor(() => {
    if (page() !== lang || heading() !== title)
      throw new Error(`test: the page is in "${page()}" under "${heading()}", not in "${lang}" under "${title}"`);
  });

/** Every title the screen showed from now on, each once, in the order it showed them. */
function shown() {
  const seen = new Set<string>();
  const note = () => {
    seen.add(heading());
  };
  note();
  const observer = new MutationObserver(note);
  observer.observe(document.documentElement, { subtree: true, childList: true, characterData: true, attributes: true });
  return {
    all: () => {
      note();
      return [...seen];
    },
    stop: () => {
      observer.disconnect();
    },
  };
}

/** Lets the app run as long as it would take to draw a change: tasks and frames, not one microtask. */
const aWhile = () => new Promise((resolve) => setTimeout(resolve, 150));

/** The app on the lists' screen in `locale`, the account's choice, once the server's preferences are read. */
async function listsIn(locale: "en" | "ja", options: AppOptions = {}) {
  ownDevice();
  cleanup();
  const app = await renderApp("/lists", { preferences: { locale, locale_follows_browser: false }, ...options });
  await waitFor(() => {
    if (app.data.queryClient.getQueryData(preferencesQuery(app.data).queryKey) === undefined)
      throw new Error("test: the preferences are not read yet");
  });
  await reads(locale, lists[locale]);
  return app;
}

describe("a change of language", () => {
  test("keeps the screen in the language it is in until the new one's strings are loaded, then draws in it", async () => {
    const app = await listsIn("en");
    const ja = hold("ja");
    const seen = shown();

    await updatePreferences(app.data, { locale: "ja" });
    await aWhile();
    // The strings of ja are asked for, and until they are loaded the screen reads en, as it did.
    expect(asked.map(language)).toContain("ja");
    expect(seen.all()).toEqual([lists.en]);
    expect(page()).toBe("en");

    ja.loaded();
    await reads("ja", lists.ja);
    seen.stop();
    // Nothing else was drawn on the way: no message ids, no other words.
    expect(seen.all()).toEqual([lists.en, lists.ja]);
  });

  test("from another language than en, shows no en on the way", async () => {
    const app = await listsIn("ja");
    const ar = hold("ar");
    const seen = shown();

    await updatePreferences(app.data, { locale: "ar-XB" });
    await aWhile();
    expect(seen.all()).toEqual([lists.ja]);

    ar.loaded();
    await waitFor(() => {
      if (page() !== "ar-XB") throw new Error(`test: the page is in "${page()}"`);
    });
    seen.stop();
    const titles = seen.all();
    expect(titles).not.toContain(lists.en);
    expect(titles).toHaveLength(2);
  });

  test("whose strings fail to load leaves the screen in the language it is in", async () => {
    const app = await listsIn("en");
    const ja = hold("ja");
    const seen = shown();

    await updatePreferences(app.data, { locale: "ja" });
    await aWhile();
    ja.failed();
    await aWhile();
    seen.stop();

    expect(seen.all()).toEqual([lists.en]);
    expect(page()).toBe("en");
    expect(app.path()).toBe("/lists");
  });
});
