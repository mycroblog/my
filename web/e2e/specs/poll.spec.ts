// SPDX-License-Identifier: AGPL-3.0-only

// A person votes in a post's poll, one choice or several, and sees its results, which the server keeps: they show
// after a reload. A vote that gets no answer shows the poll as it was, with a notice to try again. A visitor who votes
// is taken to sign in.

import type { Route } from "@playwright/test";
import { homeTimeline, Landing, localTimeline, Notice, SignIn } from "./screens";
import { expect, test } from "./test";

const votes = "**/api/v1/statuses/*/poll/votes";

test("a vote of one choice shows the results, the reader's choice marked, and stays after a reload", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const { text } = await fresh.post(author, {
    text: "Tea or coffee at the next meetup?",
    poll: { options: ["Tea", "Coffee", "Juice"], days: 1 },
  });
  await signIn(await fresh.person({ follows: [author] }));
  const post = homeTimeline(page).post(author.displayName, text);
  await expect(post.voteButton()).toBeDisabled();
  await post.choose("Coffee");
  await expect(post.option("Coffee")).toBeChecked();
  await post.vote();
  await expect(post.results).toHaveText(["Tea0%", "Coffee (your vote)100%", "Juice0%"]);
  await expect(post.pollLine()).toHaveText(/^1 person voted · Ends in /);
  await expect(post.voteButton()).toHaveCount(0);
  await page.reload();
  await expect(post.results).toHaveText(["Tea0%", "Coffee (your vote)100%", "Juice0%"]);
});

test("a vote of several choices marks each of them", async ({ page, signIn, fresh }) => {
  const author = await fresh.person();
  const { text } = await fresh.post(author, {
    text: "Which snacks should we bring?",
    poll: { options: ["Crackers", "Fruit", "Cake"], multiple: true, days: 1 },
  });
  await signIn(await fresh.person({ follows: [author] }));
  const post = homeTimeline(page).post(author.displayName, text);
  await expect(post.pollLine()).toHaveText(/^Several choices allowed · /);
  await post.choose("Crackers");
  await post.choose("Cake");
  await expect(post.option("Crackers", "checkbox")).toBeChecked();
  await expect(post.option("Fruit", "checkbox")).not.toBeChecked();
  await expect(post.option("Cake", "checkbox")).toBeChecked();
  await post.vote();
  await expect(post.results).toHaveText(["Crackers (your vote)100%", "Fruit0%", "Cake (your vote)100%"]);
  await expect(post.pollLine()).toHaveText(/^1 person voted · Ends in /);
  await page.reload();
  await expect(post.results).toHaveText(["Crackers (your vote)100%", "Fruit0%", "Cake (your vote)100%"]);
});

test("a vote in flight holds its choice; one that gets no answer shows the poll again, and Try again sends it", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const { text } = await fresh.post(author, {
    text: "Indoors or outdoors this time?",
    poll: { options: ["Indoors", "Outdoors"], days: 1 },
  });
  await signIn(await fresh.person({ follows: [author] }));
  const post = homeTimeline(page).post(author.displayName, text);
  const held = Promise.withResolvers<Route>();
  await page.route(votes, (route) => {
    held.resolve(route);
  });
  await post.choose("Outdoors");
  await post.voteButton().click();
  const route = await held.promise;
  await expect(post.voteButton()).toHaveAttribute("aria-busy", "true");
  await expect(post.option("Outdoors")).toBeChecked();
  await expect(post.option("Outdoors")).toBeDisabled();
  await expect(post.results).toHaveCount(0);
  await route.abort("failed");
  const notice = new Notice(page);
  await expect(notice.text("The vote did not finish.")).toBeVisible();
  await expect(post.results).toHaveCount(0);
  await expect(post.option("Outdoors")).toBeEnabled();
  await page.unroute(votes);
  await post.voteAgain(notice.tryAgain);
  await expect(post.results).toHaveText(["Indoors0%", "Outdoors (your vote)100%"]);
});

// The visitor's and the Japanese reader's polls are their own, the newest of the local timeline: the posts the other
// tests write would push a seeded one off its first page.
test("a visitor who votes is taken to sign in", async ({ page, fresh }) => {
  const author = await fresh.person();
  const { text } = await fresh.post(author, {
    text: "Which bread for Saturday?",
    poll: { options: ["Rye", "Sourdough"], days: 1 },
  });
  const landing = new Landing(page);
  await landing.open();
  await landing.place("Read the local timeline").click();
  const post = localTimeline(page).post(author.displayName, text);
  await expect(post.pollLine()).toHaveText(/^0 people voted · Ends in /);
  await post.choose("Rye");
  await post.voteButton().click();
  await expect(new SignIn(page).heading).toBeVisible();
});

test.describe("in Japanese", () => {
  test.use({ locale: "ja" });

  test("the line under a poll reads in the reader's language", async ({ page, fresh }) => {
    const author = await fresh.person();
    const { text } = await fresh.post(author, {
      text: "土曜のパンはどれ？",
      poll: { options: ["ライ麦", "サワー種"], days: 1 },
    });
    await page.goto("/local");
    const post = localTimeline(page, "ローカルタイムライン").post(author.displayName, text);
    await expect(post.pollLine("投票")).toHaveText(/^0人が投票 · .+に締切$/);
    await expect(post.poll("投票").getByRole("button", { name: "投票する", exact: true })).toBeVisible();
  });
});
