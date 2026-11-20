// SPDX-License-Identifier: AGPL-3.0-only

// A person's lists of follows and followers, opened from the counts of their profile: a tab replaces the address. A
// list its owner keeps from the reader says so (FR-311) in the feature (profile/profile.dom.test.tsx). That a follow
// from a row stays is the E2E tests' (web/e2e/specs/profile.spec.ts).

import { expect, test } from "bun:test";
import { screen, waitFor, within } from "@testing-library/react";
import { main, renderApp } from "./test/app";
import { postsLike } from "./test/posts";

/** The screen's heading of the profile of `name` (a display name): the name and the full handle. */
const heading = (name: string) =>
  main().findByRole("heading", { level: 1, name: new RegExp(`^${name} \\(@[^)]+\\)$`) });
/** The link of the profile's card that opens its list `list`, named by the list and its count ("Followers 12"). */
const countLink = (name: string, list: "Following" | "Followers") =>
  within(screen.getByRole("region", { name })).findByRole("link", { name: new RegExp(`^${list} [\\d,]+$`) });
/** The tab of the list `list`, named by the list and its count. */
const tab = (list: "Following" | "Followers") =>
  within(main().getByRole("tablist", { name: "Following and followers" })).getByRole("tab", {
    name: new RegExp(`^${list} [\\d,]+$`),
  });
/** The open list. */
const panel = () => within(main().getByRole("tabpanel"));

test("a tab of the lists replaces the address, and Back returns to the profile", async () => {
  const app = await renderApp("/@sam", { viewer: "marco" });
  expect(await heading("Sam Rivera")).toBeDefined();

  await app.user.click(await countLink("Sam Rivera", "Followers"));
  await main().findByRole("tablist", { name: "Following and followers" });
  expect(app.path()).toBe("/@sam/followers");
  expect(tab("Followers").getAttribute("aria-selected")).toBe("true");
  await app.user.click(tab("Following"));
  expect(await panel().findByRole("link", { name: "Yuki 雪" })).toBeDefined();
  expect(app.path()).toBe("/@sam/following");

  // The tab replaced the list's address, so the browser's back leaves the lists for the profile.
  await app.back();
  expect(await heading("Sam Rivera")).toBeDefined();
  expect(app.path()).toBe("/@sam");
  await app.user.click(await countLink("Sam Rivera", "Following"));
  expect(await panel().findByRole("link", { name: "Yuki 雪" })).toBeDefined();
  await app.user.click(main().getByRole("button", { name: "Back to the profile" }));
  expect(await heading("Sam Rivera")).toBeDefined();
});

/** `body` without the follow counts of its accounts, as the server answers a list of posts. */
function withoutCounts<T>(body: T): T {
  return JSON.parse(JSON.stringify(body), (key, value: unknown) =>
    key === "followers_count" || key === "following_count" ? undefined : value,
  ) as T;
}

test("a profile shows its counts after its author's posts were read in a timeline, the reader's own and another's", async () => {
  const app = await renderApp("/lists", { viewer: "yuki" });
  app.fake.script("GET /timelines/home", { change: withoutCounts });
  await app.navigate("/");
  await waitFor(() => {
    for (const author of ["Sam Rivera", "Yuki 雪"])
      if (postsLike(screen.getByRole("main"), author, "").length === 0)
        throw new Error(`test: no post of ${author} yet`);
  });

  await app.navigate("/profile");
  expect(await countLink("Yuki 雪", "Following")).toBeDefined();
  expect(await countLink("Yuki 雪", "Followers")).toBeDefined();

  await app.navigate("/@sam");
  expect(await countLink("Sam Rivera", "Following")).toBeDefined();
  expect(await countLink("Sam Rivera", "Followers")).toBeDefined();
});
