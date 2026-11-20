// SPDX-License-Identifier: AGPL-3.0-only

// A visitor without a session reads a profile, its lists of follows and a post with its thread, as the server shows
// them to everyone, and the first page of a one-person server, its operator's profile (ADR-0152 §7); what needs an
// account (following, reporting, a post's actions) leads them to the app's sign-in, which comes back to where they
// were (ADR-0090 §16). What the server keeps from a visitor reads as the server answers it, never as "sign in". The
// app of a visitor never asks for what needs a session (renderApp fails a test that does).

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { screen, within } from "@testing-library/react";
import { main, renderApp, type TestApp } from "./test/app";
import { feedOf, findPost, postOf } from "./test/posts";

/** Where the sign-in the app went to comes back to. */
const returnTo = (app: TestApp) => new URLSearchParams(app.path().split("?")[1]).get("return_to");

/** Waits for the app's sign-in, and returns where it comes back to. */
async function signedInBack(app: TestApp): Promise<string | null> {
  expect(await main().findByRole("heading", { level: 1, name: "Sign in" })).toBeDefined();
  expect(app.path()).toStartWith("/login?");
  return returnTo(app);
}

/** The screen's heading of the profile of `name` (a display name): the name and the full handle. */
const heading = (name: string) =>
  main().findByRole("heading", { level: 1, name: new RegExp(`^${name} \\(@[^)]+\\)$`) });

test("a visitor reads a profile and its posts, with nothing of a reader's own", async () => {
  await renderApp("/@sam", { visitor: true });
  expect(await heading("Sam Rivera")).toBeDefined();
  const profile = screen.getByRole("region", { name: "Sam Rivera" });
  expect(within(profile).getByRole("button", { name: "Follow" })).toBeDefined();
  expect(within(profile).queryByText("Follows you")).toBeNull();
  expect(within(profile).queryByText("Add a note only you can see")).toBeNull();
  expect(postOf(await feedOf("Posts of the account"), "Sam Rivera", "Day 3 of the sourdough")).toBeDefined();
  expect(main().queryByText("Sign in to see this screen.")).toBeNull();
});

test("Follow on a profile leads a visitor to the sign-in, which comes back to the profile", async () => {
  const app = await renderApp("/@sam", { visitor: true });
  await heading("Sam Rivera");
  await app.user.click(
    within(screen.getByRole("region", { name: "Sam Rivera" })).getByRole("button", { name: "Follow" }),
  );
  expect(await signedInBack(app)).toBe("/@sam");
});

test("Report in a profile's menu leads a visitor to the sign-in, which comes back to the profile", async () => {
  const app = await renderApp("/@sam", { visitor: true });
  await heading("Sam Rivera");
  await app.user.click(
    within(screen.getByRole("region", { name: "Sam Rivera" })).getByRole("button", { name: "More" }),
  );
  const menu = await screen.findByRole("menu");
  expect(within(menu).queryByRole("menuitem", { name: "Mute" })).toBeNull();
  expect(within(menu).queryByRole("menuitem", { name: "Block" })).toBeNull();
  await app.user.click(within(menu).getByRole("menuitem", { name: "Report…" }));
  expect(await signedInBack(app)).toBe("/@sam");
});

test("a locked account's profile offers a visitor Request to follow", async () => {
  await renderApp("/@ren", { visitor: true });
  const region = await screen.findByRole("region", { name: /^Ren/ });
  expect(await within(region).findByRole("button", { name: "Request to follow" })).toBeDefined();
});

test("a post's Favourite on a profile leads a visitor to the sign-in, which comes back to the post", async () => {
  const app = await renderApp("/@sam", { visitor: true });
  const post = postOf(await feedOf("Posts of the account"), "Sam Rivera", "Day 3 of the sourdough");
  await app.user.click(within(post).getByRole("button", { name: /^Favourite/ }));
  expect(await signedInBack(app)).toBe(app.fake.postPath("sam-bread"));
});

test("a visitor reads a list of follows, and a row's Follow leads to the sign-in, which comes back to the list", async () => {
  const app = await renderApp("/@sam/following", { visitor: true });
  const list = within(await main().findByRole("tabpanel"));
  expect(await list.findByRole("link", { name: "Yuki 雪" })).toBeDefined();
  await app.user.click(list.getAllByRole("button", { name: /^(Follow|Request to follow)$/ })[0] as HTMLElement);
  expect(await signedInBack(app)).toBe("/@sam/following");
});

test("a list its owner shows to their followers alone says so to a visitor", async () => {
  await renderApp("/@mio/followers", { visitor: true });
  expect(await main().findByText(/does not show this list to you\.$/)).toBeDefined();
  expect(main().queryByText("Sign in to see this screen.")).toBeNull();
});

test("a visitor reads a post in its thread, and Reply leads to the sign-in, which comes back to the post", async () => {
  const app = await renderApp("/", { visitor: true });
  const path = app.fake.postPath("yuki-ridge-reply");
  await app.navigate(path);
  const feed = await feedOf("Thread");
  expect(postOf(feed, "Sam Rivera", "That light! Which trail is this?")).toBeDefined();
  const focused = postOf(feed, "Yuki 雪", "北側の尾根道です。駐車場から");
  await app.user.click(within(focused).getByRole("button", { name: /^Reply/ }));
  expect(await signedInBack(app)).toBe(path);
});

test("a followers-only post opened by a visitor is not found, with nothing to try again", async () => {
  const app = await renderApp("/", { visitor: true });
  await app.navigate(app.fake.postPath("ren-followers"));
  expect(await main().findByText(/^This post was not found\./)).toBeDefined();
  expect(main().queryByRole("button", { name: "Try again" })).toBeNull();
  expect(main().queryByText("Sign in to see this screen.")).toBeNull();
});

test("a visitor opens who reacted to a post from its thread, and reads the list", async () => {
  const text = "Our choir sings at the market on Saturday.";
  const app = await renderApp("/", {
    visitor: true,
    seed: {
      people: scenario.people,
      emojis: scenario.emojis,
      posts: [{ key: "post", author: "yuki", text, language: "en", visibility: "public" }],
      interactions: [
        { kind: "reaction", by: "hana", post: "post", emoji: ":teacup:" },
        { kind: "reaction", by: "sam", post: "post", emoji: ":teacup:" },
      ],
    },
  });
  await app.navigate(app.fake.postPath("post"));
  const focused = await findPost("Yuki 雪", text);
  await app.user.click(within(focused).getByRole("link", { name: ":teacup: 2 people reacted" }));
  expect(await main().findByRole("heading", { level: 2, name: "Reacted with :teacup: 2" })).toBeDefined();
  expect(app.path()).toBe(`${app.fake.postPath("post")}/reactions/teacup`);
  expect(main().getByRole("link", { name: "Sam Rivera" })).toBeDefined();
  expect(main().getByRole("link", { name: "Hana 花" })).toBeDefined();
  expect(main().queryByText("Sign in to see this screen.")).toBeNull();
});

test("who reacted to a post a visitor may not see is not found, with nothing to try again", async () => {
  const app = await renderApp("/", { visitor: true });
  await app.navigate(`${app.fake.postPath("ren-followers")}/reactions/${encodeURIComponent("🎉")}`);
  expect(await main().findByText(/^This post was not found\./)).toBeDefined();
  expect(main().queryByRole("button", { name: "Try again" })).toBeNull();
});

test("the first page of a one-person server shows a visitor its operator's profile, whose Follow leads to the sign-in", async () => {
  const app = await renderApp("/", { visitor: true, singleUser: true });
  const follow = await main().findByRole("button", { name: "Follow" });
  await app.user.click(follow);
  const back = await signedInBack(app);
  expect(back).toStartWith("/@");
});
