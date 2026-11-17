// SPDX-License-Identifier: AGPL-3.0-only

// The translation of a drawn post: to whom Translate is offered, where it goes, what it asks the server for, and what
// the post shows once the server answered.

import { afterEach, expect, test } from "bun:test";
import type { PostAction, PostStatus } from "@my/app-components";
import { homeTimelineQuery, instanceQuery } from "@my/app-data";
import type { FakeOptions } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { TranslatablePost } from "./post-translation";
import { fakeData, renderFeature } from "./test/feature";

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
 * The post `of` (or sam's boost of the welcome) drawn for a reader in `locale` (a visitor when `visitor`), on a server
 * whose instance offers `offer`: the post, the ID of the post translated, what the post asked of its screen, and the
 * server's requests of a translation.
 */
async function drawn(
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
  const asked: PostAction[] = [];
  const user = renderFeature(
    data,
    <TranslatablePost
      status={shown}
      onAction={(action) => asked.push(action)}
      signIn={visitor ? () => undefined : undefined}
    />,
    locale,
  );
  const post = within(screen.getByRole("article"));
  // The line is drawn once the hook knows whether the browser translates.
  await waitFor(() => post.getByRole("button", { name: locale === "ja" ? "その他" : "More" }));
  return {
    user,
    post,
    id,
    asked,
    translate: () => post.queryAllByRole("button", { name: locale === "ja" ? "翻訳する" : "Translate" }),
    sent: async () => (await sent.arrived()).body,
    translations: () => requests.filter((r) => r.includes("/translation")),
  };
}

test("a signed-in reader is offered Translate on a post in another language that the server's provider may receive", async () => {
  expect((await drawn("ja", withProvider)).translate()).toHaveLength(1);
});

test("an unlisted post is offered too", async () => {
  expect((await drawn("unlisted", withProvider)).translate()).toHaveLength(1);
});

test("a post in the reader's own language is not offered", async () => {
  expect((await drawn("en", withProvider)).translate()).toHaveLength(0);
});

test("a Japanese post is not offered to a reader of Japanese", async () => {
  expect((await drawn("ja", withProvider, { locale: "ja" })).translate()).toHaveLength(0);
});

test("without a provider and without the browser's translator, nothing is offered", async () => {
  expect((await drawn("ja", undefined)).translate()).toHaveLength(0);
});

for (const visibility of ["private", "direct"] as const)
  test(`a ${visibility} post, which no provider may receive, is not offered without the browser's translator`, async () => {
    expect((await drawn(visibility, withProvider)).translate()).toHaveLength(0);
  });

test("a visitor is never offered the server's provider: only the browser translates for them", async () => {
  expect((await drawn("ja", withProvider, { visitor: true })).translate()).toHaveLength(0);
});

test("first drawn, a post shows no translation, nothing on its way and no refusal", async () => {
  const { post, translations } = await drawn("ja", withProvider);

  expect(post.queryAllByRole("button", { name: /Show original|Try again|Translate in this browser/ })).toHaveLength(0);
  expect(post.queryAllByText(/Translating…|translated|translation/i)).toHaveLength(0);
  expect(translations()).toEqual([]);
});

test("Translate asks the server for the post in the reader's language, shows the translation, and Show original takes it back", async () => {
  const { user, post, id, sent, translations, translate, asked } = await drawn("ja", withProvider);
  const [button] = translate();
  if (button === undefined) throw new Error("test: the post offers no Translate");

  await user.click(button);

  expect(await sent()).toEqual({ target_language: "en" });
  expect(translations()).toEqual([`POST /statuses/${id}/translation`]);
  await user.click(await post.findByRole("button", { name: "Show original" }));
  expect(translate()).toHaveLength(1);
  // The translation is the post's own: its screen hears nothing of it.
  expect(asked).toEqual([]);
});

test("a reader of Japanese asks for Japanese", async () => {
  const { user, sent, translate } = await drawn("en", withProvider, { locale: "ja" });
  const [button] = translate();
  if (button === undefined) throw new Error("test: the post offers no Translate");

  await user.click(button);

  expect(await sent()).toEqual({ target_language: "ja" });
});

test("where the browser may translate too, the server's provider is asked first", async () => {
  Object.defineProperty(globalThis, "Translator", {
    configurable: true,
    value: {
      availability: () => Promise.resolve("available"),
      create: () => Promise.reject(new Error("test: the browser was asked before the server")),
    },
  });
  const { user, post, sent } = await drawn("ja", { ...withProvider, browser: true });

  await user.click(await post.findByRole("button", { name: "Translate" }));

  expect(await sent()).toEqual({ target_language: "en" });
  expect(await post.findByRole("button", { name: "Show original" })).not.toBeNull();
});

test("the post's other actions are its screen's, and ask for no translation", async () => {
  const { user, post, id, asked, translations } = await drawn("ja", withProvider);

  await user.click(post.getByRole("button", { name: /^Favourite/ }));
  await user.click(post.getByRole("button", { name: /^Reply/ }));

  expect(asked).toEqual([
    { type: "favourite", id },
    { type: "reply", id },
  ]);
  expect(translations()).toEqual([]);
});

test("a boost translates the post it boosts", async () => {
  const { user, post, id, sent, translations, translate } = await drawn("boost", withProvider);
  const [button] = translate();
  if (button === undefined) throw new Error("test: the boost offers no Translate");

  await user.click(button);

  expect(await sent()).toEqual({ target_language: "en" });
  expect(translations()).toEqual([`POST /statuses/${id}/translation`]);
  expect(await post.findByRole("button", { name: "Show original" })).not.toBeNull();
});
