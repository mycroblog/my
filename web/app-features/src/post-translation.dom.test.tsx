// SPDX-License-Identifier: AGPL-3.0-only

// The translation of a drawn post: to whom Translate is offered, where it goes, what it asks the server for, and what
// the post shows once the server answered.

import { afterEach, expect, test } from "bun:test";
import type { PostAction, PostStatus } from "@my/app-components";
import { homeTimelineQuery, instanceQuery } from "@my/app-data";
import { scenario, type FakeOptions } from "@my/e2e/fake";
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
 * The post `of` (or sam's boost of the welcome; or the post of `seed` whose key is `of.key`) drawn for a reader in
 * `locale` (a visitor when `visitor`), on a server whose instance offers `offer`: the post, the ID of the post
 * translated, what the post asked of its screen, and the server's requests of a translation.
 */
async function drawn(
  of: keyof typeof posts | "boost" | { key: string },
  offer: FakeOptions["translation"],
  { locale = "en", visitor = false, seed }: { locale?: string; visitor?: boolean; seed?: FakeOptions["seed"] } = {},
) {
  const { fake, data, requests } = fakeData({
    ...(offer === undefined ? {} : { translation: offer }),
    ...(seed === undefined ? {} : { seed }),
  });
  const sent = fake.script("POST /statuses/{id}/translation");
  await data.queryClient.query(instanceQuery(data));
  const id = fake.idOf(typeof of === "object" ? of.key : posts[of === "boost" ? "ja" : of]);
  const shown: PostStatus | undefined =
    of === "boost"
      ? (await data.queryClient.infiniteQuery(homeTimelineQuery(data))).pages[0]?.items.find((s) => s.boost?.id === id)
      : (await data.api.GET("/statuses/{id}", { params: { path: { id } } })).data;
  if (shown === undefined) throw new Error(`test: the server has no ${typeof of === "object" ? of.key : of} post`);
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

test("a provider that does not answer says so, and Try again asks it again", async () => {
  const { user, post, translations, translate } = await drawn("ja", { provider: "failed" });
  const refusal = "This server's translation service is not answering. Try again";
  const said = () => post.queryAllByText(/^This server's translation service/).map((line) => line.textContent);
  const [button] = translate();
  if (button === undefined) throw new Error("test: the post offers no Translate");

  await user.click(button);
  await waitFor(() => {
    if (said().length === 0) throw new Error("the post says nothing of the refusal");
  });
  expect(said().map((line) => line.replaceAll("\u2060", "").replaceAll("\u00a0", " ").trim())).toEqual([refusal]);
  await user.click(post.getByRole("button", { name: "Try again" }));

  await waitFor(() => {
    if (translations().length !== 2) throw new Error("the translation was not asked for again");
  });
  await waitFor(() => {
    if (post.queryAllByRole("button", { name: "Try again" }).length !== 1) throw new Error("the refusal is not back");
  });
  // The post stays as it was: nothing of it is translated.
  expect(post.queryAllByText(/^\[en\]/)).toHaveLength(0);
  expect(translate()).toHaveLength(0);
});

test("over the monthly limit, the line says so and when translation comes back, with nothing to press", async () => {
  const { user, post, translate } = await drawn("ja", { provider: "limit" });
  const [button] = translate();
  if (button === undefined) throw new Error("test: the post offers no Translate");

  await user.click(button);

  await waitFor(() => {
    post.getByText(/^This server's monthly translation limit is reached\./);
  });
  expect(translate()).toHaveLength(0);
  expect(post.queryAllByRole("button", { name: "Try again" })).toHaveLength(0);
});

test("behind a content warning, the warning, the poll's options and the alt text are translated with the text", async () => {
  const soup = {
    name: "soup.png",
    description: "湯気の立つスープ",
    drawing: { width: 64, height: 64, top: [200, 120, 80], bottom: [120, 60, 40], shapes: [] },
  } as const;
  const seed = {
    people: scenario.people,
    posts: [
      {
        key: "folded",
        author: "yuki",
        text: "土曜のパンはどれにしますか？",
        language: "ja",
        visibility: "public",
        spoiler: "パンの話",
        poll: { options: ["ライ麦", "サワー種"], days: 1 },
        images: [soup],
      },
    ],
  } satisfies FakeOptions["seed"];
  const { user, post, translate } = await drawn({ key: "folded" }, withProvider, { seed });
  const [button] = translate();
  if (button === undefined) throw new Error("test: the post offers no Translate");

  // The translate line sits in the warning's band: it translates with the text still folded.
  await user.click(button);
  expect(await post.findByText("[en] パンの話")).not.toBeNull();
  await user.click(post.getByRole("button", { name: "Show" }));
  expect(await post.findByText("[en] 土曜のパンはどれにしますか？")).not.toBeNull();
  const options = within(post.getByRole("group", { name: "Poll" }));
  expect(options.getByRole("radio", { name: "[en] ライ麦" })).not.toBeNull();
  expect(options.getByRole("radio", { name: "[en] サワー種" })).not.toBeNull();
  // The fake's server marks the media of a post with a warning sensitive: a second Show uncovers them.
  await user.click(post.getByRole("button", { name: "Show" }));
  expect(await post.findByRole("img", { name: `[en] ${soup.description}` })).not.toBeNull();
  await user.click(post.getByRole("button", { name: "Show original" }));
  expect(await post.findByText("パンの話")).not.toBeNull();
  expect(post.getByRole("img", { name: soup.description })).not.toBeNull();
});
