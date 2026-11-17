// SPDX-License-Identifier: AGPL-3.0-only

// A reader of English translates a post written in Japanese through the server's provider (the fake's "Demo
// Translate", which puts "[en] " before each text it translates) and goes back to the original, in the home timeline
// and in the post's thread. To whom Translate is offered, what a refusal says and what is translated with the text are
// the feature's (post-translation.dom.test.tsx). The reader is Sam, who follows Yuki, on a server that holds the test's
// posts alone.

import { expect, test } from "bun:test";
import { scenario, type FakeSeed } from "@my/e2e/fake";
import { within } from "@testing-library/react";
import { renderApp, words } from "./test/app";
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

/** Sam's home on a server that holds `posts` and whose provider translates. */
const home = (posts: readonly Post[]) =>
  renderApp("/", {
    viewer: "sam",
    translation: { provider: "answers" },
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
