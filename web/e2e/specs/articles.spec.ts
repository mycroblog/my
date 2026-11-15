// SPDX-License-Identifier: AGPL-3.0-only

// A person reads an article (ADR-0160): its card in the home timeline opens its page, which reads its whole text and
// its replies; and a profile lists its articles on a tab of their own.

import { ArticleText, homeTimeline, Profile, Thread } from "./screens";
import { expect, person, test } from "./test";

const title = "A sourdough starter in seven days";
const summary = "What I fed it, when it rose, and the three mistakes that cost me a week. A diary of a jar of flour.";

test("an article opens from its card in the home timeline and reads in full, its replies under it", async ({
  page,
  signIn,
}) => {
  const sam = person("sam");
  const yuki = person("yuki");
  await signIn(person("marco"));
  const card = homeTimeline(page).post(sam.displayName, title);
  await expect(card.articleCard(title)).toHaveAccessibleName(`Article ${title} ${summary}`);
  await expect(card.image("A round loaf on a wooden board")).toBeVisible();
  await card.openArticle(title);

  const thread = new Thread(page);
  await expect(page).toHaveURL(/\/@sam\/status\/[^/]+$/);
  const text = new ArticleText(thread.post(sam.displayName, title));
  await expect(text.title(title)).toBeVisible();
  await expect(text.paragraph(summary)).toBeVisible();
  await expect(text.image("A round loaf on a wooden board")).toBeVisible();
  await expect(
    text.paragraph("Seven days ago I mixed flour and water in a jar. This is what happened next, day by day."),
  ).toBeVisible();
  await expect(text.headings(2)).toHaveText(["The first three days", "What I fed it"]);
  await expect(text.headings(3)).toHaveText(["The mistakes"]);
  await expect(text.listItems).toHaveText([
    "Day 1: 50 g flour, 50 g water",
    "Day 2: the same, at the same hour",
    "Day 3: half of it out, then the same",
  ]);
  await expect(text.quote).toHaveText("A starter is ready when it doubles within six hours of a feed.");
  await expect(text.columnHeaders).toHaveText(["Day", "Flour", "Water"]);
  await expect(text.rows).toHaveCount(4);
  // An image from another host is never fetched by the reader's browser: the server wrote a link of its alt text.
  await expect(text.link("the shop on a map")).toBeVisible();
  await expect(text.image("the shop on a map")).toHaveCount(0);
  await expect(text.code("feed: 1 starter : 1 flour : 1 water")).toBeVisible();
  await expect(text.rules).toHaveCount(1);
  await expect(text.paragraph("Next week: the first loaf.")).toBeVisible();

  const reply = thread.post(yuki.displayName, "三日目のにおい、うちでも同じでした");
  await expect(reply.article).toBeVisible();
  await expect(reply.article).not.toContainText("Replying to");
});

test("a profile's Articles tab lists its articles and none of its other posts", async ({ page, signIn }) => {
  const sam = person("sam");
  await signIn(person("marco"));
  const profile = new Profile(page, sam);
  await profile.open();
  await expect(profile.posts.post(sam.displayName, "Day 3 of the sourdough").article).toBeVisible();
  await expect(profile.tabs).toHaveText(["Posts", "Posts and replies", "Media", "Articles"]);
  await profile.openTab("Articles");
  await expect(profile.tab("Articles")).toHaveAttribute("aria-selected", "true");
  const article = profile.posts.post(sam.displayName, title);
  await expect(article.articleCard(title)).toBeVisible();
  await expect(profile.posts.posts).toHaveCount(1);
  await article.openArticle(title);
  await expect(new ArticleText(new Thread(page).post(sam.displayName, title)).title(title)).toBeVisible();
});

test("a profile's pinned posts do not lead its Articles tab", async ({ page, signIn }) => {
  const hana = person("hana");
  await signIn(person("marco"));
  const profile = new Profile(page, hana);
  await profile.open();
  await expect(profile.posts.pinned(hana.displayName, "mycroblog のデモサーバーへようこそ").article).toBeVisible();
  await profile.openTab("Articles");
  await expect(profile.tab("Articles")).toHaveAttribute("aria-selected", "true");
  await expect(profile.noPosts("Articles")).toBeVisible();
  await expect(profile.posts.posts).toHaveCount(0);
});
