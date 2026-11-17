// SPDX-License-Identifier: AGPL-3.0-only

// A reader of English translates a post written in Japanese through the server's provider (the fake's "Demo
// Translate", which puts "[en] " before each text it translates) and goes back to the original, in the home timeline
// and in the post's thread; the content warning, the poll's options and the alt text are translated with the text. The
// provider down, a followers-only post the provider may not receive, and the monthly limit each say why. The reader is
// Sam, who follows Yuki, on a server that holds the test's posts alone.

import { expect, test } from "bun:test";
import { scenario, type FakeSeed } from "@my/e2e/fake";
import { waitFor, within } from "@testing-library/react";
import { renderApp, words, type AppOptions } from "./test/app";
import { findPost } from "./test/posts";

type Post = FakeSeed["posts"][number];

const yuki = "Yuki 雪";
const note = "Post translated from Japanese by Demo Translate · Show original";

/** A public post of Yuki's in Japanese. */
const japanese = (key: string, text: string, more: Partial<Post> = {}): Post => ({
  key,
  author: "yuki",
  text,
  language: "ja",
  visibility: "public",
  ...more,
});

/** Sam's home on a server that holds `posts` and translates as `provider` says. */
const home = (posts: readonly Post[], provider: NonNullable<AppOptions["translation"]>["provider"] = "answers") =>
  renderApp("/", {
    viewer: "sam",
    translation: { provider },
    seed: { people: scenario.people, follows: [{ follower: "sam", followee: "yuki" }], posts },
  });

/** The paragraph of `post` whose words begin with `start`. */
const lineOf = (post: HTMLElement, start: string) =>
  within(post)
    .getAllByRole("paragraph")
    .map(words)
    .filter((line) => line.startsWith(start));

const button = (post: HTMLElement, name: string) => within(post).queryByRole("button", { name });

test("a reader translates a post in the home timeline, the request reaching the server, and goes back", async () => {
  const text = "今朝は尾根まで歩きました。";
  const app = await home([japanese("ridge", text)]);
  const asked = app.fake.script("POST /statuses/{id}/translation");
  const post = await findPost(yuki, text);

  await app.user.click(within(post).getByRole("button", { name: "Translate" }));

  expect(await within(post).findByText(`[en] ${text}`)).toBeDefined();
  expect(asked.seen.map((request) => request.body.target_language)).toEqual(["en"]);
  expect(lineOf(post, "Post translated")).toEqual([note]);
  expect(button(post, "Translate")).toBeNull();

  await app.user.click(within(post).getByRole("button", { name: "Show original" }));

  expect(await within(post).findByText(text)).toBeDefined();
  expect(button(post, "Translate")).not.toBeNull();
  expect(button(post, "Show original")).toBeNull();
});

test("a reader translates a post in its thread, and goes back", async () => {
  const text = "駐車場から二時間くらいです。";
  const app = await home([japanese("walk", text)]);
  await app.navigate(app.fake.postPath("walk"));
  const post = await findPost(yuki, text);

  await app.user.click(within(post).getByRole("button", { name: "Translate" }));

  expect(await within(post).findByText(`[en] ${text}`)).toBeDefined();
  expect(lineOf(post, "Post translated")).toEqual([note]);

  await app.user.click(within(post).getByRole("button", { name: "Show original" }));

  expect(await within(post).findByText(text)).toBeDefined();
  expect(button(post, "Translate")).not.toBeNull();
});

test("a post in the reader's own language offers no translation", async () => {
  const text = "Up on the ridge before dawn.";
  await home([japanese("dawn", text, { language: "en" })]);

  const post = await findPost(yuki, text);
  expect(button(post, "Translate")).toBeNull();
});

test("behind a content warning, the warning, the poll's options and the alt text are translated with the text", async () => {
  const soup = {
    name: "soup.png",
    description: "湯気の立つスープ",
    drawing: { width: 64, height: 64, top: [200, 120, 80], bottom: [120, 60, 40], shapes: [] },
  } as const;
  const app = await home([
    japanese("bread", "土曜のパンはどれにしますか？", {
      spoiler: "パンの話",
      poll: { options: ["ライ麦", "サワー種"], days: 1 },
    }),
    japanese("lunch", "今日の昼ごはんです。", { spoiler: "食べ物の写真", images: [soup] }),
  ]);

  // The translate line sits in the warning's band: it translates with the text still folded.
  const poll = await findPost(yuki, "パンの話");
  await app.user.click(within(poll).getByRole("button", { name: "Translate" }));
  expect(await within(poll).findByText("[en] パンの話")).toBeDefined();
  expect(lineOf(poll, "Post translated")).toEqual([note]);
  await app.user.click(within(poll).getByRole("button", { name: "Show" }));
  expect(await within(poll).findByText("[en] 土曜のパンはどれにしますか？")).toBeDefined();
  const options = within(within(poll).getByRole("group", { name: "Poll" }));
  expect(options.getByRole("radio", { name: "[en] ライ麦" })).toBeDefined();
  expect(options.getByRole("radio", { name: "[en] サワー種" })).toBeDefined();

  const image = await findPost(yuki, "食べ物の写真");
  await app.user.click(within(image).getByRole("button", { name: "Translate" }));
  expect(await within(image).findByText("[en] 食べ物の写真")).toBeDefined();
  await app.user.click(within(image).getByRole("button", { name: "Show" }));
  expect(await within(image).findByText("[en] 今日の昼ごはんです。")).toBeDefined();
  // The fake's server marks the media of a post with a warning sensitive: a second Show uncovers them.
  await app.user.click(within(image).getByRole("button", { name: "Show" }));
  expect(await within(image).findByRole("img", { name: `[en] ${soup.description}` })).toBeDefined();
  await app.user.click(within(image).getByRole("button", { name: "Show original" }));
  expect(await within(image).findByText("食べ物の写真")).toBeDefined();
  expect(within(image).getByRole("img", { name: soup.description })).toBeDefined();
});

test("the provider down says so, and Try again asks it again", async () => {
  const text = "今日は雨です。";
  const refusal = "This server's translation service is not answering. Try again";
  const app = await home([japanese("rain", text)], "failed");
  const asked = app.fake.script("POST /statuses/{id}/translation");
  const post = await findPost(yuki, text);

  await app.user.click(within(post).getByRole("button", { name: "Translate" }));
  await waitFor(() => {
    expect(lineOf(post, "This server's translation service")).toEqual([refusal]);
  });

  await app.user.click(within(post).getByRole("button", { name: "Try again" }));
  await waitFor(() => {
    expect(asked.seen).toHaveLength(2);
  });
  await waitFor(() => {
    expect(lineOf(post, "This server's translation service")).toEqual([refusal]);
  });
  expect(within(post).getByText(text)).toBeDefined();
});

test("a followers-only post, which the provider may not receive, offers no translation", async () => {
  const text = "フォロワーだけに: 集合は二階です。";
  await home([japanese("upstairs", text, { visibility: "private" })]);

  const post = await findPost(yuki, text);
  expect(button(post, "Translate")).toBeNull();
});

test("over the monthly limit, the line says so and when translation comes back", async () => {
  const text = "山の朝は静かです。";
  const app = await home([japanese("morning", text)], "limit");
  const post = await findPost(yuki, text);

  await app.user.click(within(post).getByRole("button", { name: "Translate" }));

  await waitFor(() => {
    expect(lineOf(post, "This server's monthly")).toEqual([
      "This server's monthly translation limit is reached. Translation is available again next month.",
    ]);
  });
  expect(button(post, "Translate")).toBeNull();
  expect(button(post, "Try again")).toBeNull();
});
