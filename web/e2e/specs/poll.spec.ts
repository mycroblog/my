// SPDX-License-Identifier: AGPL-3.0-only

// A person votes in a post's poll and sees its results, which the server keeps: they show after a reload. That a vote
// of several choices sends each of them is the features' (web/app-features/src/post-actions.dom.test.tsx).

import { homeTimeline } from "./screens";
import { expect, test } from "./test";

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
