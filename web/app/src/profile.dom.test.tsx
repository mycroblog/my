// SPDX-License-Identifier: AGPL-3.0-only

// A person reads another person's profile. The reader is Marco, who follows Sam and not Yuki. What the profile shows
// is the feature's (profile.dom.test.tsx); that a follow stays is the E2E tests' (web/e2e/specs/profile.spec.ts).

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { screen, within } from "@testing-library/react";
import { main, renderApp, type TestApp } from "./test/app";
import { findPost } from "./test/posts";

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
