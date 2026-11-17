// SPDX-License-Identifier: AGPL-3.0-only

// A person reads another person's profile. The reader is Marco, who follows Sam and not Yuki. That a follow stays is
// the E2E tests' (web/e2e/specs/profile.spec.ts).

import { expect, test } from "bun:test";
import { scenario } from "@my/e2e/fake";
import { screen, within } from "@testing-library/react";
import { findPost, main, renderApp, type TestApp } from "./test/app";

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

test("a profile opened from a post in the home timeline shows its author at once, and the rest after", async () => {
  const app = await home();
  await findPost(sam, "Day 3 of the sourdough");
  // Every read of the profile waits: of the account itself, by its handle and by its ID, and of the rest.
  const rest = app.fake.script("GET /accounts/relationships", { hold: "request" });
  const held = [
    app.fake.script("GET /accounts/lookup", { hold: "request" }),
    app.fake.script("GET /accounts/{id}", { hold: "request" }),
    app.fake.script("GET /accounts/{id}/statuses", { hold: "request" }),
    rest,
  ];

  await openSam(app);

  expect(await heading(sam)).toBeDefined();
  expect(card(sam).getByText(noteOf("sam"))).toBeDefined();
  expect(card(sam).getByRole("term").textContent).toContain("Blog");
  // Shown from what the post said of its author, with no answer yet: the rest is still asked for.
  await rest.arrived();
  expect(card(sam).queryByRole("button", { name: "Following" })).toBeNull();
  for (const read of held) read.release();
  expect(await card(sam).findByRole("button", { name: "Following" })).toBeDefined();
  const posts = await main().findByRole("feed", { name: "Posts of the account" });
  expect(await within(posts).findByText(/^Day 3 of the sourdough/)).toBeDefined();
});

test("another person's profile shows their note, fields and posts", async () => {
  const app = await home();

  await openSam(app);

  expect(await heading(sam)).toBeDefined();
  expect(app.path()).toBe("/@sam");
  expect(card(sam).getByText(noteOf("sam"))).toBeDefined();
  expect(card(sam).getByRole("term").textContent).toContain("Blog");
  expect(await card(sam).findByRole("button", { name: "Following" })).toBeDefined();
  const posts = await main().findByRole("feed", { name: "Posts of the account" });
  expect(await within(posts).findByText(/^Day 3 of the sourdough/)).toBeDefined();
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
