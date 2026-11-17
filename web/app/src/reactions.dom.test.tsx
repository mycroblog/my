// SPDX-License-Identifier: AGPL-3.0-only

// A reader's emoji reaction to a post (FR-116), as the screens lead to it: a row of who reacted on the post's own page
// opens the list of them. What the rows say is the post's (who-reacted.dom.test.tsx), and a reaction that does not
// finish is the features' (post-actions.dom.test.tsx). The reader is Sam, who follows Yuki, on a server that holds the
// test's post alone; that the server keeps a reaction is the E2E tests' (web/e2e/specs/reactions.spec.ts).

import { expect, test } from "bun:test";
import { scenario, type FakeSeed } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, type AppOptions, type TestApp } from "./test/app";
import { findPost } from "./test/posts";

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
/** Opens the emoji picker from the React button of `post`, and picks the emoji `name` of `section` in it. */
async function react({ user }: TestApp, post: HTMLElement, section: string, name: string) {
  await user.click(within(post).getByRole("button", { name: "React" }));
  const grid = await within(await screen.findByRole("dialog", { name: "React" })).findByRole("grid", { name: "Emoji" });
  await user.click(within(within(grid).getByRole("rowgroup", { name: section })).getByRole("gridcell", { name }));
}

test("a row of who reacted on the post's own page opens the list of them, and Back returns to the post", async () => {
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
