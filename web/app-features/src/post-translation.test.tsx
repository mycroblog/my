// SPDX-License-Identifier: AGPL-3.0-only
// The translation of a drawn post as its first render gives it, without a DOM: what it offers, and where Translate
// goes. What follows a press (the translation shown, a refusal, the browser's translator taking over) needs the post
// drawn again, which the app's story tests of the post's page press and check in a browser (web/app/src/post.stories.tsx).

import { afterEach, expect, test } from "bun:test";
import type { PostStatus } from "@my/app-components";
import { instanceQuery, type InstanceTranslation } from "@my/app-data";
import { inAnotherLanguage, usePostTranslation, type PostTranslationState } from "./post-translation";
import { render, server, status } from "./test/server";

afterEach(() => {
  Reflect.deleteProperty(globalThis, "Translator");
});

/** A server whose one provider receives the default visibilities, as the demo server's fake does. */
const withProvider: InstanceTranslation = {
  browser: false,
  providers: [{ id: "fake", name: "Demo Translate", visibilities: ["public", "local", "unlisted"] }],
};

const translated = (language: string) => ({
  language,
  source_language: "ja",
  content: `<p>[${language}] 小さなサーバー</p>`,
  spoiler_text: "",
  media_attachments: [],
  poll_options: [],
  provider: { id: "fake", name: "Demo Translate" },
});

/** A post of lena's in `language`, with the fields a test sets. */
const post = (language: string, fields: Record<string, unknown> = {}) =>
  status("1", "lena", { language, content: "<p>小さなサーバー</p>", ...fields }) as unknown as PostStatus;

/**
 * The translation of `shown` as a reader in `locale` (a visitor when `visitor`) first sees it, on a server whose
 * instance offers `offer` and that answers `answers` besides; and the server's requests.
 */
async function translation(
  shown: PostStatus,
  offer: InstanceTranslation,
  {
    locale = "en",
    visitor = false,
    answers = {},
  }: { locale?: string; visitor?: boolean; answers?: Record<string, unknown> } = {},
) {
  const { data, requests, received } = server({ "/instance": { translation: offer }, ...answers });
  await data.queryClient.query(instanceQuery(data));
  const seen: { state?: PostTranslationState } = {};
  function Probe() {
    seen.state = usePostTranslation(shown, visitor);
    return null;
  }
  render(data, <Probe />, locale);
  const state = seen.state;
  if (state === undefined) throw new Error("test: the probe did not render");
  return { ...state, received, translations: () => requests.filter((r) => r.includes("/translation")) };
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
  expect((await translation(post("ja"), withProvider)).props.translatable).toBe(true);
  expect((await translation(post("ja", { visibility: "unlisted" }), withProvider)).props.translatable).toBe(true);
});

test("a post in the reader's own language is not offered", async () => {
  expect((await translation(post("en"), withProvider)).props.translatable).not.toBe(true);
  expect((await translation(post("ja"), withProvider, { locale: "ja" })).props.translatable).not.toBe(true);
});

test("without a provider and without the browser's translator, nothing is offered and nothing is sent", async () => {
  const none = await translation(post("ja"), { browser: false, providers: [] });
  expect(none.props.translatable).not.toBe(true);
  none.act("translate");
  expect(none.translations()).toEqual([]);
});

test("a followers-only or direct post that no provider may receive is not offered without the browser's translator", async () => {
  for (const visibility of ["private", "direct"]) {
    const closed = await translation(post("ja", { visibility }), withProvider);
    expect(closed.props.translatable).not.toBe(true);
    closed.act("translate");
    expect(closed.translations()).toEqual([]);
  }
});

test("a visitor is never offered the server's provider: only the browser translates for them", async () => {
  const visiting = await translation(post("ja"), withProvider, { visitor: true });
  expect(visiting.props.translatable).not.toBe(true);
  visiting.act("translate");
  expect(visiting.translations()).toEqual([]);
});

test("first drawn, a post shows no translation, nothing on its way and no refusal", async () => {
  const { props } = await translation(post("ja"), withProvider);
  expect(props.translation).toBeUndefined();
  expect(props.translating).toBeFalsy();
  expect(props.translationFailure).toBeUndefined();
});

test("Translate asks the server for the post in the reader's language; the hook says it handled it", async () => {
  const shown = await translation(post("ja"), withProvider, {
    answers: { "/statuses/1/translation": translated("en") },
  });
  expect(shown.act("translate")).toBe(true);
  expect(await shown.received("POST /statuses/1/translation")).toEqual({ target_language: "en" });
});

test("a reader of Japanese asks for Japanese", async () => {
  const shown = await translation(post("en"), withProvider, {
    locale: "ja",
    answers: { "/statuses/1/translation": translated("ja") },
  });
  shown.act("translate");
  expect(await shown.received("POST /statuses/1/translation")).toEqual({ target_language: "ja" });
});

test("where the browser may translate too, the server's provider is asked first", async () => {
  Object.defineProperty(globalThis, "Translator", {
    configurable: true,
    value: {
      availability: () => Promise.resolve("available"),
      create: () => Promise.reject(new Error("test: the browser was asked before the server")),
    },
  });
  const shown = await translation(
    post("ja"),
    { ...withProvider, browser: true },
    {
      answers: { "/statuses/1/translation": translated("en") },
    },
  );
  shown.act("translate");
  expect(await shown.received("POST /statuses/1/translation")).toEqual({ target_language: "en" });
});

test("the post's other actions are not the hook's", async () => {
  const shown = await translation(post("ja"), withProvider);
  expect(shown.act("favourite")).toBe(false);
  expect(shown.act("reply")).toBe(false);
  expect(shown.translations()).toEqual([]);
});

test("a boost translates the post it boosts", async () => {
  const boost = status("2", "kenji", { language: "en", boost: post("ja") }) as unknown as PostStatus;
  const shown = await translation(boost, withProvider, { answers: { "/statuses/1/translation": translated("en") } });
  expect(shown.props.translatable).toBe(true);
  shown.act("translate");
  expect(await shown.received("POST /statuses/1/translation")).toEqual({ target_language: "en" });
});
