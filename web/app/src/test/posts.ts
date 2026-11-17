// SPDX-License-Identifier: AGPL-3.0-only

// The posts of the screen in place, for the component tests of the app's routes: a feed by its name, and a post in it
// by its author's display name, which names its article, and a part of its text.

import { screen, within } from "@testing-library/react";
import type { TestApp } from "./app";

/** The feed named `name` of the screen in place, once it shows. */
export function feedOf(name: string): Promise<HTMLElement> {
  return within(screen.getByRole("main")).findByRole("feed", { name });
}

/** The posts of `author` (a display name) in `feed` that hold `text`, boosts of them included. */
const postsOf = (feed: HTMLElement, author: string, text: string): HTMLElement[] =>
  within(feed)
    .queryAllByRole("article", { name: author })
    .filter((post) => post.textContent.includes(text));

/** The one of `posts`; a test that finds none, or several, fails saying which post it looked for. */
function one(posts: HTMLElement[], what: string): HTMLElement {
  const [post] = posts;
  if (post === undefined || posts.length !== 1)
    throw new Error(`test: ${String(posts.length)} posts are ${what}, where one was expected`);
  return post;
}

/** The post of `author` in `feed` that holds `text`, itself and not a boost of it. */
export function postOf(feed: HTMLElement, author: string, text: string): HTMLElement {
  return one(
    postsOf(feed, author, text).filter((post) => !post.textContent.includes(" boosted")),
    `${author}'s "${text}"`,
  );
}

/** The posts of `author` in `feed` that hold `text`, themselves and not boosts of them: none once it is gone. */
export function postsLike(feed: HTMLElement, author: string, text: string): HTMLElement[] {
  return postsOf(feed, author, text).filter((post) => !post.textContent.includes(" boosted"));
}

/** The boost by `booster` of the post of `author` in `feed` that holds `text`. */
export function boostOf(feed: HTMLElement, booster: string, author: string, text: string): HTMLElement {
  return one(
    postsOf(feed, author, text).filter((post) => post.textContent.includes(`${booster} boosted`)),
    `${booster}'s boost of ${author}'s "${text}"`,
  );
}

/** Opens the More menu of `post`, and returns the menu once it is open. */
export async function openMore(app: TestApp, post: HTMLElement): Promise<HTMLElement> {
  await app.user.click(within(post).getByRole("button", { name: "More" }));
  return screen.findByRole("menu", { name: "More" });
}
