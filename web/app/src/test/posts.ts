// SPDX-License-Identifier: AGPL-3.0-only

// The posts of the screen in place, for the component tests of the app's routes: a feed by its name, and a post by its
// author's display name, which names its article, and a part of its text, in a feed or anywhere on the screen.

import { screen, waitFor, within } from "@testing-library/react";
import type { TestApp } from "./app";

/** The feed named `name` of the screen in place, once it shows. */
export function feedOf(name: string): Promise<HTMLElement> {
  return within(screen.getByRole("main")).findByRole("feed", { name });
}

/** The articles of `author` (a display name) in `scope` that hold `text`, boosts of them included. */
const articlesOf = (scope: HTMLElement, author: string, text: string): HTMLElement[] =>
  within(scope)
    .queryAllByRole("article", { name: author })
    .filter((post) => post.textContent.includes(text));

/** The one of `posts`; a test that finds none, or several, fails saying which post it looked for. */
function one(posts: HTMLElement[], what: string): HTMLElement {
  const [post] = posts;
  if (post === undefined || posts.length !== 1)
    throw new Error(`test: ${String(posts.length)} posts are ${what}, where one was expected`);
  return post;
}

/** The posts of `author` in `scope` that hold `text`, themselves and not boosts of them: none once it is gone. */
export function postsLike(scope: HTMLElement, author: string, text: string): HTMLElement[] {
  return articlesOf(scope, author, text).filter((post) => !post.textContent.includes(" boosted"));
}

/** The post of `author` in `scope` (a feed, a thread) that holds `text`, itself and not a boost of it. */
export function postOf(scope: HTMLElement, author: string, text: string): HTMLElement {
  return one(postsLike(scope, author, text), `${author}'s "${text}"`);
}

/** The post of `author` that holds `text` in the main landmark, itself and not a boost of it, once the screen shows it. */
export const findPost = (author: string, text: string): Promise<HTMLElement> =>
  waitFor(() => postOf(screen.getByRole("main"), author, text));

/** Opens the More menu of `post`, and returns the menu once it is open. */
export async function openMore(app: TestApp, post: HTMLElement): Promise<HTMLElement> {
  await app.user.click(within(post).getByRole("button", { name: "More" }));
  return screen.findByRole("menu", { name: "More" });
}
