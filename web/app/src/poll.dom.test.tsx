// SPDX-License-Identifier: AGPL-3.0-only

// A post's poll, as one screen shows it: a vote in flight, a vote that gets no answer, a visitor's vote, and the line
// under it in the reader's language. The author is Yuki, on a server that holds the test's post alone; that a vote
// stays is the E2E tests' (web/e2e/specs/poll.spec.ts).

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { findPost, renderApp, words, type AppOptions } from "./test/app";
import { fakeApp } from "./test/server";

const yuki = "Yuki 雪";

/** A server that holds one public post of Yuki's, a poll of `options`, which Sam, who follows Yuki, reads. */
const polled = (text: string, options: readonly string[], language = "en"): AppOptions => ({
  viewer: "sam",
  seed: {
    people: scenario.people,
    follows: [{ follower: "sam", followee: "yuki" }],
    posts: [{ key: "poll", author: "yuki", text, language, visibility: "public", poll: { options, days: 1 } }],
  },
});

/** The results of a poll, an item per option: its title, "(your vote)" after the reader's choice, and its share. */
const results = (poll: HTMLElement) =>
  within(poll)
    .queryAllByRole("listitem")
    .map((item) => item.textContent);

test("a vote in flight holds its choice; one that gets no answer shows the poll again, and Try again sends it", async () => {
  const text = "Indoors or outdoors this time?";
  const on = fakeApp(polled(text, ["Indoors", "Outdoors"]));
  const held = on.fake.script("POST /statuses/{id}/poll/votes", { times: 1, hold: "request", fail: "network" });
  const app = await renderApp("/", { on });
  const poll = within(await findPost(yuki, text)).getByRole("group", { name: "Poll" });
  const outdoors = () => within(poll).getByRole<HTMLInputElement>("radio", { name: "Outdoors" });

  await app.user.click(within(poll).getByText("Outdoors"));
  await app.user.click(within(poll).getByRole("button", { name: "Vote" }));
  await held.arrived();
  await waitFor(() => {
    expect(within(poll).getByRole("button", { name: "Vote" }).getAttribute("aria-busy")).toBe("true");
  });
  expect(outdoors().checked).toBe(true);
  expect(outdoors().disabled).toBe(true);
  expect(results(poll)).toEqual([]);

  held.release();
  expect(await screen.findByText("The vote did not finish.")).toBeDefined();
  expect(results(poll)).toEqual([]);
  expect(outdoors().disabled).toBe(false);

  await app.user.click(screen.getByRole("button", { name: "Try again" }));
  await waitFor(() => {
    expect(results(poll)).toEqual(["Indoors0%", "Outdoors (your vote)100%"]);
  });
});

test("a visitor who votes is taken to sign in", async () => {
  const text = "Which bread for Saturday?";
  const app = await renderApp("/local", { ...polled(text, ["Rye", "Sourdough"]), visitor: true });
  const poll = within(await findPost(yuki, text)).getByRole("group", { name: "Poll" });
  expect(words(within(poll).getByRole("paragraph"))).toMatch(/^0 people voted · Ends in /);

  await app.user.click(within(poll).getByText("Rye"));
  await app.user.click(within(poll).getByRole("button", { name: "Vote" }));

  await waitFor(() => {
    expect(app.path()).toMatch(/^\/login(?:\?|$)/);
  });
});

test("the line under a poll reads in the reader's language", async () => {
  const text = "土曜のパンはどれ？";
  await renderApp("/local", { ...polled(text, ["ライ麦", "サワー種"], "ja"), visitor: true, locale: "ja" });

  const poll = within(await findPost(yuki, text)).getByRole("group", { name: "投票" });
  expect(words(within(poll).getByRole("paragraph"))).toMatch(/^0人が投票 · .+に締切$/);
  expect(within(poll).getByRole("button", { name: "投票する" })).toBeDefined();
});
