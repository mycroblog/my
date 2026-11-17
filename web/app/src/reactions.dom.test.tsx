// SPDX-License-Identifier: AGPL-3.0-only

// A reader's emoji reaction to a post (FR-116), as the screens show it: a reaction that does not finish, and the rows
// of who reacted on the post's own page, with the list each opens. The reader is Sam, who follows Yuki, on a server
// that holds the test's post alone; that the server keeps a reaction is the E2E tests' (web/e2e/specs/reactions.spec.ts).

import { expect, test } from "bun:test";
import { scenario, type FakeSeed } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, type AppOptions, type TestApp } from "./test/app";
import { findPost } from "./test/posts";
import { fakeApp } from "./test/server";

const yuki = "Yuki 雪";

/** A server that holds one public post of Yuki's, with the reactions of others, which Sam reads. */
const posted = (text: string, interactions: FakeSeed["interactions"] = []): AppOptions => ({
  viewer: "sam",
  seed: {
    people: scenario.people,
    emojis: scenario.emojis,
    follows: [{ follower: "sam", followee: "yuki" }],
    posts: [{ key: "post", author: "yuki", text, language: "en", visibility: "public" }],
    interactions,
  },
});

/** The reader's own reaction to `post`, once it shows: its chip, pressed, named "{emoji}, {count}, including yours". */
const ownReaction = (post: HTMLElement, name: string) =>
  waitFor(() =>
    within(within(post).getByRole("group", { name: "Reactions" })).getByRole("button", { name, pressed: true }),
  );
/** The reader's favourite of `post`: the favourite button, pressed, named with its count; null without one. */
const favourited = (post: HTMLElement) => within(post).queryByRole("button", { name: "Favourite, 1", pressed: true });

/** Opens the emoji picker from the React button of `post`, and picks the emoji `name` of `section` in it. */
async function react({ user }: TestApp, post: HTMLElement, section: string, name: string) {
  await user.click(within(post).getByRole("button", { name: "React" }));
  const grid = await within(await screen.findByRole("dialog", { name: "React" })).findByRole("grid", { name: "Emoji" });
  await user.click(within(within(grid).getByRole("rowgroup", { name: section })).getByRole("gridcell", { name }));
}

test("a reaction that does not finish puts back the favourite it took, and Try again sends it", async () => {
  const text = "Lost: one blue umbrella, near the station.";
  const on = fakeApp(posted(text));
  const app = await renderApp("/", { on });
  const post = await findPost(yuki, text);
  await app.user.click(within(post).getByRole("button", { name: "Favourite" }));
  await waitFor(() => {
    expect(favourited(post)).not.toBeNull();
  });

  const held = app.fake.script("PUT /statuses/{id}/reaction", { times: 1, hold: "request", fail: "network" });
  await react(app, post, "Smileys & emotion", "grinning face");
  await held.arrived();
  // Shown at once, before the server answers.
  expect(await ownReaction(post, "😀, 1, including yours")).toBeDefined();
  expect(favourited(post)).toBeNull();

  held.release();
  expect(await screen.findByText("The reaction did not finish.")).toBeDefined();
  expect(within(post).queryByRole("group", { name: "Reactions" })).toBeNull();
  expect(favourited(post)).not.toBeNull();

  await app.user.click(screen.getByRole("button", { name: "Try again" }));
  await waitFor(() => {
    expect(held.seen).toHaveLength(2);
  });
  expect(await ownReaction(post, "😀, 1, including yours")).toBeDefined();
  expect(favourited(post)).toBeNull();
});

test("the post's own page has a row per emoji of who reacted, which opens the list of them", async () => {
  const text = "Our choir sings at the market on Saturday.";
  const app = await renderApp(
    "/",
    posted(text, [
      { kind: "reaction", by: "hana", post: "post", emoji: ":teacup:" },
      { kind: "reaction", by: "layla", post: "post", emoji: "🎉" },
    ]),
  );
  const listed = await findPost(yuki, text);
  await react(app, listed, "Custom emoji", ":teacup:");
  expect(await ownReaction(listed, ":teacup:, 2, including yours")).toBeDefined();

  await app.navigate(app.fake.postPath("post"));
  expect(await main().findByRole("heading", { level: 1, name: "Post" })).toBeDefined();
  const focused = await findPost(yuki, text);
  expect(within(focused).getByRole("link", { name: "🎉 1 person reacted" })).toBeDefined();
  await app.user.click(within(focused).getByRole("link", { name: ":teacup: 2 people reacted" }));

  expect(await main().findByRole("heading", { level: 1, name: "Reactions" })).toBeDefined();
  expect(app.path()).toBe(`${app.fake.postPath("post")}/reactions/teacup`);
  expect(await main().findByRole("heading", { level: 2, name: "Reacted with :teacup: 2" })).toBeDefined();
  expect(main().getByRole("link", { name: "Sam Rivera" })).toBeDefined();
  expect(main().getByRole("link", { name: "Hana 花" })).toBeDefined();

  await app.user.click(main().getByRole("button", { name: "Back to the post" }));
  expect(await main().findByRole("heading", { level: 1, name: "Post" })).toBeDefined();
  const again = await findPost(yuki, text);
  expect(within(again).getByRole("link", { name: ":teacup: 2 people reacted" })).toBeDefined();
  // Taking the reaction back on the post's page leaves one person in its row.
  await react(app, again, "Custom emoji", ":teacup:, your reaction");
  expect(await within(again).findByRole("link", { name: ":teacup: 1 person reacted" })).toBeDefined();
});
