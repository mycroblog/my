// SPDX-License-Identifier: AGPL-3.0-only

// A person adds a keyword filter that folds (#3202, ADR-0157 §2, §10): the server then marks the post of someone they
// follow that holds the filter's word, so their home timeline folds it under the filter's name, with Show to open it,
// at once and after a reload. The reader, the author and the post are fresh ones, and the word is the post's own.
// What the editor checks, sends and says, the list of filters, and where a folded post shows are component tests on
// the fake (app-features, settings/filters-edit.dom.test.tsx and filters.dom.test.tsx; app, filtered.dom.test.tsx).

import { FilterEditor, foldedBy, Frame, homeTimeline, KeywordFilters } from "./screens";
import { expect, test } from "./test";

test("a person adds a filter that folds, and the post on their home timeline that holds its word is folded under the filter's name, at once and after a reload", async ({
  page,
  signIn,
  fresh,
}) => {
  const author = await fresh.person();
  const reader = await fresh.person({ follows: [author] });
  const post = await fresh.post(author, { text: "The levain doubled overnight." });
  // The word that ends the post's text is its own: the filter matches this post and no other.
  const word = post.text.slice(post.text.lastIndexOf(" ") + 1);
  const name = `Fold ${word}`;

  await signIn(reader);
  const home = homeTimeline(page);
  await expect(home.post(author.displayName, post.text).paragraph(post.text)).toBeVisible();
  await expect(foldedBy(home, name)).toHaveCount(0);

  const editor = new FilterEditor(page);
  await editor.goTo();
  await editor.name.fill(name);
  await editor.word(1).fill(word);
  await editor.action.selectOption({ label: "Fold behind the filter's name" });
  await editor.add();

  // The app is at the list, which shows the new filter.
  const filters = new KeywordFilters(page);
  await expect(page).toHaveURL("/settings/filters");
  await expect(filters.heading).toBeVisible();
  await expect(filters.row(name)).toBeVisible();

  // The home timeline follows the filter with no reload: the navigation's link stays in the app.
  await new Frame(page).goTo("Home");
  await expect(foldedBy(home, name)).toHaveCount(1);
  await expect(home.post(author.displayName, post.text).article).toHaveCount(0);
  await foldedBy(home, name).getByRole("button", { name: "Show", exact: true }).click();
  await expect(home.post(author.displayName, post.text).paragraph(post.text)).toBeVisible();

  await page.reload();
  await expect(foldedBy(home, name)).toHaveCount(1);
  await expect(foldedBy(home, name).getByRole("button", { name: "Show", exact: true })).toBeVisible();
  await expect(home.post(author.displayName, post.text).article).toHaveCount(0);
});
