// SPDX-License-Identifier: AGPL-3.0-only

// The mark of the stream in the app's shell: a signed-in reader sees the state of their session's stream, named by the
// catalog's words for it; a visitor, who has no stream, sees no mark.

import { expect, test } from "bun:test";
import { screen } from "@testing-library/react";
import { renderApp } from "../test/app";
import { fakeApp, type FakeApp } from "../test/server";

type Key = "stream_connecting" | "stream_open" | "stream_reconnecting" | "stream_stopped";
const keys: readonly Key[] = ["stream_connecting", "stream_open", "stream_reconnecting", "stream_stopped"];

/** The catalog of the app in `locale`, as its translators keep it. */
const catalogs = new Map<string, unknown>();
for (const locale of ["en", "ja"])
  catalogs.set(locale, await Bun.file(new URL(`../../locales/${locale}/app.json`, import.meta.url)).json());

/** The words of the app's message `app.<key>` in `locale`, as the catalog has them. */
function message(key: Key, locale = "en"): string {
  const catalog = catalogs.get(locale);
  if (typeof catalog !== "object" || catalog === null) throw new Error(`test: no catalog of ${locale}`);
  const entry: unknown = Reflect.get(catalog, `app.${key}`) ?? Reflect.get(catalog, key);
  const words: unknown = typeof entry === "object" && entry !== null ? Reflect.get(entry, "defaultMessage") : entry;
  if (typeof words !== "string") throw new Error(`test: the catalog of ${locale} has no app.${key}`);
  return words;
}

/** The marks of the stream on the screen, in any of its states. */
const marks = (locale = "en") => keys.flatMap((key) => screen.queryAllByRole("img", { name: message(key, locale) }));

test("a signed-in reader sees the mark of their stream, named as open once it connected", async () => {
  await renderApp("/");
  expect(await screen.findByRole("img", { name: message("stream_open") })).toBeDefined();
  expect(marks()).toHaveLength(1);
});

test("the mark of the stream is named in the reader's language", async () => {
  await renderApp("/", { locale: "ja" });
  expect(await screen.findByRole("img", { name: message("stream_open", "ja") })).toBeDefined();
});

test("before the server answers the stream, its mark says it connects", async () => {
  const on = fakeApp();
  on.fake.script("GET /streaming", { hold: "response" });
  await renderApp("/", { on });
  expect(await screen.findByRole("img", { name: message("stream_connecting") })).toBeDefined();
  expect(marks()).toHaveLength(1);
});

test("while the stream waits to connect again after a failure, its mark says it reconnects", async () => {
  const on = fakeApp();
  on.fake.script("GET /streaming", { fail: 502 });
  await renderApp("/", { on });
  expect(await screen.findByRole("img", { name: message("stream_reconnecting") })).toBeDefined();
});

test("a stream the server refused for good leaves a mark that says it stopped", async () => {
  const on = fakeApp();
  on.fake.script("GET /streaming", { fail: 403 });
  await renderApp("/", { on });
  expect(await screen.findByRole("img", { name: message("stream_stopped") })).toBeDefined();
  expect(marks()).toHaveLength(1);
});

test("a visitor, who has no stream, sees no mark of one", async () => {
  await renderApp("/@sam", { visitor: true });
  expect(await screen.findByRole("main")).toBeDefined();
  expect(marks()).toEqual([]);
});

/** `app` as a reader whose data has no connection to the server: their app starts no stream. */
function withoutConnection(app: FakeApp): FakeApp {
  const { connection, ...data } = app.data;
  expect(connection).toBeDefined();
  return { ...app, data };
}

test("a signed-in reader whose app has no stream sees no mark of one", async () => {
  await renderApp("/", { on: withoutConnection(fakeApp()) });
  expect(await screen.findByRole("main")).toBeDefined();
  expect(marks()).toEqual([]);
});
