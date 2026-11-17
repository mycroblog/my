// SPDX-License-Identifier: AGPL-3.0-only

// A post's poll, as one screen shows it to a visitor: a vote is taken to the sign-in. The poll itself, its line in the
// reader's language and a vote that gets no answer are the post's and the features' (post-poll.dom.test.tsx,
// post-actions.dom.test.tsx); that a vote stays is the E2E tests' (web/e2e/specs/poll.spec.ts).

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { waitFor, within } from "@testing-library/react";
import { renderApp, type AppOptions } from "./test/app";
import { findPost } from "./test/posts";

const yuki = "Yuki 雪";

/** A server that holds one public post of Yuki's, a poll of `options`, which a visitor reads. */
const polled = (text: string, options: readonly string[]): AppOptions => ({
  seed: {
    people: scenario.people,
    posts: [{ key: "poll", author: "yuki", text, language: "en", visibility: "public", poll: { options, days: 1 } }],
  },
});

test("a visitor who votes is taken to sign in", async () => {
  const text = "Which bread for Saturday?";
  const app = await renderApp("/local", { ...polled(text, ["Rye", "Sourdough"]), visitor: true });
  const poll = within(await findPost(yuki, text)).getByRole("group", { name: "Poll" });

  await app.user.click(within(poll).getByText("Rye"));
  await app.user.click(within(poll).getByRole("button", { name: "Vote" }));

  await waitFor(() => {
    expect(app.path()).toMatch(/^\/login(?:\?|$)/);
  });
});
