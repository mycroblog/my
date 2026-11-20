// SPDX-License-Identifier: AGPL-3.0-only

// The scenarios the lab measures (ADR-0167 §4), each a test whose title names it in the report: what a person does
// most, on the screens of the scenario's people (hana's home follows the scenario's posts and pictures), with the
// operations of the E2E tests' screens. A scenario that writes acts on its own fresh people and posts, as a spec does.

import { Frame, homeTimeline, Notifications, Profile, Thread } from "../../specs/screens";
import { expect, person, test } from "../test";

const hana = person("hana");

test("sign-in", async ({ signIn }) => {
  await signIn(hana);
});

test("home and the composer", async ({ page, signedIn }) => {
  await signedIn(page, hana);
  await page.goto("/");
  const home = homeTimeline(page);
  await expect(home.posts).not.toHaveCount(0);
  const composer = await new Frame(page).newPost();
  await expect(composer.text).toBeFocused();
  await composer.write("A post the lab writes and never sends.");
  await composer.holds("A post the lab writes and never sends.");
});

test("a thread opened from home", async ({ page, signedIn, fresh }) => {
  const author = await fresh.person();
  const post = await fresh.post(author, { text: "A post the lab opens from home." });
  const replier = await fresh.person();
  const reply = await fresh.post(replier, { text: "A reply in the lab's thread.", replyTo: post.id });
  const reader = await fresh.person({ follows: [author] });
  await signedIn(page, reader);
  await page.goto("/");
  await homeTimeline(page).post(author.displayName, post.text).open();
  const thread = new Thread(page);
  await expect(thread.post(replier.displayName, reply.text).article).toBeVisible();
  const opened = thread.post(author.displayName, post.text);
  await opened.favourite();
  await expect(opened.favourited).toBeVisible();
});

test("notifications", async ({ page, signedIn }) => {
  await signedIn(page, hana);
  await page.goto("/");
  await expect(homeTimeline(page).posts).not.toHaveCount(0);
  await new Frame(page).goTo("Notifications");
  await expect(new Notifications(page).feed.getByRole("article")).not.toHaveCount(0);
});

test("a profile read signed out", async ({ page }) => {
  const profile = new Profile(page, hana);
  await profile.open();
  await expect(profile.heading).toBeVisible();
  await expect(profile.posts.posts).not.toHaveCount(0);
});
