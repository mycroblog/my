// SPDX-License-Identifier: AGPL-3.0-only

// A person reads another person's profile. The reader is Marco, who follows Sam and not Yuki. What the profile shows
// is the feature's (profile.dom.test.tsx); that a follow stays is the E2E tests' (web/e2e/specs/profile.spec.ts). And a
// reader mutes a server from a post at home, which the home they come back to no longer shows.

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, type TestApp } from "./test/app";
import { feedOf, findPost, openMore, postOf } from "./test/posts";

const sam = "Sam Rivera";
const yuki = "Yuki 雪";
const noteOf = (username: string) => scenario.people.find((p) => p.username === username)?.note ?? "";

/** Marco's home on a server that holds Sam's post about the sourdough alone, so that no boost shows it twice. */
const home = () =>
  renderApp("/", {
    viewer: "marco",
    seed: {
      people: scenario.people,
      follows: [{ follower: "marco", followee: "sam" }],
      posts: scenario.posts.filter((post) => post.key === "sam-bread"),
    },
  });

/** The screen's heading of the profile of `name` (a display name): the name and the full handle. */
const heading = (name: string) =>
  main().findByRole("heading", { level: 1, name: new RegExp(`^${name} \\(@[^)]+\\)$`) });
/** The profile's card. */
const card = (name: string) => within(screen.getByRole("region", { name }));

/** Opens Sam's profile from the author's name on a post of the home timeline. */
async function openSam({ user }: TestApp) {
  const post = await findPost(sam, "Day 3 of the sourdough");
  await user.click(within(post).getByRole("link", { name: sam }));
}

test("the author's name on a post in the home timeline leads to their profile, at their address", async () => {
  const app = await home();

  await openSam(app);

  expect(await heading(sam)).toBeDefined();
  expect(app.path()).toBe("/@sam");
});

test("the profile of a person who writes in Japanese keeps the reader's language", async () => {
  await renderApp("/@yuki", { viewer: "marco" });

  expect(await heading(yuki)).toBeDefined();
  expect(card(yuki).getByText(noteOf("yuki"))).toBeDefined();
  // The navigation and the card's own words stay in the reader's English.
  for (const place of ["Home", "Notifications", "Profile"])
    expect(screen.getAllByRole("link", { name: place }).length).toBeGreaterThan(0);
  expect(await card(yuki).findByRole("button", { name: "Follow" })).toBeDefined();
});

test("once the reader muted a server from a post at home, left and came back, the boost of that server's post has left the home timeline, and the booster's own post stays", async () => {
  const film = "Developed the first roll from the coast.";
  // Yuki follows Sam alone, who posted a loaf and boosted a post of Sora's, of photos.example.
  const app = await renderApp("/", {
    seed: {
      people: scenario.people,
      follows: [{ follower: "yuki", followee: "sam" }],
      posts: [
        { key: "loaf", author: "sam", text: "The first loaf.", visibility: "public", language: "en" },
        { key: "film", author: "sora@photos.example", text: film, visibility: "public", language: "en" },
      ],
      interactions: [{ kind: "boost", by: "sam", post: "film" }],
    },
  });
  const boosted = within(await feedOf("Home timeline")).getByRole("article", { name: "Sora そら" });

  await app.user.click(within(await openMore(app, boosted)).getByRole("menuitem", { name: "Mute server" }));
  expect(await main().findByText(/^You muted the server ⁨?photos\.example⁩?\.$/)).toBeDefined();
  await app.navigate("/lists");
  await app.navigate("/");

  const home = await feedOf("Home timeline");
  await waitFor(() => {
    if (home.textContent.includes(film) || home.textContent.includes("You muted the server"))
      throw new Error("the home timeline still shows the muted server's post");
  });
  expect(postOf(home, sam, "The first loaf.")).toBeDefined();
});
