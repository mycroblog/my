// SPDX-License-Identifier: AGPL-3.0-only
// The translation of a drawn post as its first render gives it, without a DOM: what it offers, and where Translate
// goes. What follows a press (the translation shown, a refusal, the browser's translator taking over) needs the post
// drawn again, which the app's story tests of the post's page press and check in a browser (web/app/src/post.stories.tsx).

import { afterEach, expect, test } from "bun:test";
import type { PostStatus } from "@my/app-components";
import { homeTimelineQuery, instanceQuery } from "@my/app-data";
import type { FakeOptions } from "@my/e2e/fake";
import { inAnotherLanguage, usePostTranslation, type PostTranslationState } from "./post-translation";
import { fakeData, markupOf } from "./test/feature";

afterEach(() => {
  Reflect.deleteProperty(globalThis, "Translator");
});

/** A server whose one provider receives the default visibilities, as the demo server's fake does. */
const withProvider: FakeOptions["translation"] = { provider: "answers" };

/**
 * The posts of the scenario the tests translate: hana's welcome in Japanese (public), her notice (unlisted), marco's
 * hello in English, ren's post for followers, hana's direct message to the reader, and sam's boost of the welcome.
 */
const posts = {
  ja: "hana-welcome",
  unlisted: "hana-unlisted",
  en: "marco-hello",
  private: "ren-followers",
  direct: "hana-group",
} as const;

/**
 * The translation of the post `of` (or of sam's boost of the welcome) as a reader in `locale` (a visitor when
 * `visitor`) first sees it, on a server whose instance offers `offer`; the ID of the post translated, and the server's
 * requests of a translation.
 */
async function translation(
  of: keyof typeof posts | "boost",
  offer: FakeOptions["translation"],
  { locale = "en", visitor = false }: { locale?: string; visitor?: boolean } = {},
) {
  const { fake, data, requests } = fakeData(offer === undefined ? {} : { translation: offer });
  const sent = fake.script("POST /statuses/{id}/translation");
  await data.queryClient.query(instanceQuery(data));
  const id = fake.idOf(posts[of === "boost" ? "ja" : of]);
  const shown: PostStatus | undefined =
    of === "boost"
      ? (await data.queryClient.infiniteQuery(homeTimelineQuery(data))).pages[0]?.items.find((s) => s.boost?.id === id)
      : (await data.api.GET("/statuses/{id}", { params: { path: { id } } })).data;
  if (shown === undefined) throw new Error(`test: the server has no ${of} post`);
  const seen: { state?: PostTranslationState } = {};
  function Probe() {
    seen.state = usePostTranslation(shown as PostStatus, visitor);
    return null;
  }
  markupOf(data, <Probe />, locale);
  const state = seen.state;
  if (state === undefined) throw new Error("test: the probe did not render");
  return {
    ...state,
    id,
    asked: async () => (await sent.arrived()).body,
    translations: () => requests.filter((r) => r.includes("/translation")),
  };
}

test("a post is in another language than the reader's: another language, or the same in another script", () => {
  expect(inAnotherLanguage("ja", "en")).toBe(true);
  expect(inAnotherLanguage("en", "ja")).toBe(true);
  expect(inAnotherLanguage("en", "en")).toBe(false);
  expect(inAnotherLanguage("en-GB", "en")).toBe(false);
  expect(inAnotherLanguage("en", "en-US")).toBe(false);
  expect(inAnotherLanguage("zh-Hant", "zh-Hans")).toBe(true);
  expect(inAnotherLanguage("zh-Hant-TW", "zh-Hant")).toBe(false);
});

test("a post whose language is not known is not offered: it may be the reader's", () => {
  expect(inAnotherLanguage("und", "en")).toBe(false);
  expect(inAnotherLanguage("", "en")).toBe(false);
});

test("a signed-in reader is offered Translate on a post in another language that the server's provider may receive", async () => {
  expect((await translation("ja", withProvider)).props.translatable).toBe(true);
  expect((await translation("unlisted", withProvider)).props.translatable).toBe(true);
});

test("a post in the reader's own language is not offered", async () => {
  expect((await translation("en", withProvider)).props.translatable).not.toBe(true);
  expect((await translation("ja", withProvider, { locale: "ja" })).props.translatable).not.toBe(true);
});

test("without a provider and without the browser's translator, nothing is offered and nothing is sent", async () => {
  const none = await translation("ja", undefined);
  expect(none.props.translatable).not.toBe(true);
  none.act("translate");
  expect(none.translations()).toEqual([]);
});

test("a followers-only or direct post that no provider may receive is not offered without the browser's translator", async () => {
  for (const visibility of ["private", "direct"] as const) {
    const closed = await translation(visibility, withProvider);
    expect(closed.props.translatable).not.toBe(true);
    closed.act("translate");
    expect(closed.translations()).toEqual([]);
  }
});

test("a visitor is never offered the server's provider: only the browser translates for them", async () => {
  const visiting = await translation("ja", withProvider, { visitor: true });
  expect(visiting.props.translatable).not.toBe(true);
  visiting.act("translate");
  expect(visiting.translations()).toEqual([]);
});

test("first drawn, a post shows no translation, nothing on its way and no refusal", async () => {
  const { props } = await translation("ja", withProvider);
  expect(props.translation).toBeUndefined();
  expect(props.translating).toBeFalsy();
  expect(props.translationFailure).toBeUndefined();
});

test("Translate asks the server for the post in the reader's language; the hook says it handled it", async () => {
  const shown = await translation("ja", withProvider);
  expect(shown.act("translate")).toBe(true);
  expect(await shown.asked()).toEqual({ target_language: "en" });
  expect(shown.translations()).toEqual([`POST /statuses/${shown.id}/translation`]);
});

test("a reader of Japanese asks for Japanese", async () => {
  const shown = await translation("en", withProvider, { locale: "ja" });
  shown.act("translate");
  expect(await shown.asked()).toEqual({ target_language: "ja" });
});

test("where the browser may translate too, the server's provider is asked first", async () => {
  Object.defineProperty(globalThis, "Translator", {
    configurable: true,
    value: {
      availability: () => Promise.resolve("available"),
      create: () => Promise.reject(new Error("test: the browser was asked before the server")),
    },
  });
  const shown = await translation("ja", { ...withProvider, browser: true });
  shown.act("translate");
  expect(await shown.asked()).toEqual({ target_language: "en" });
});

test("the post's other actions are not the hook's", async () => {
  const shown = await translation("ja", withProvider);
  expect(shown.act("favourite")).toBe(false);
  expect(shown.act("reply")).toBe(false);
  expect(shown.translations()).toEqual([]);
});

test("a boost translates the post it boosts", async () => {
  const shown = await translation("boost", withProvider);
  expect(shown.props.translatable).toBe(true);
  shown.act("translate");
  expect(await shown.asked()).toEqual({ target_language: "en" });
  expect(shown.translations()).toEqual([`POST /statuses/${shown.id}/translation`]);
});
