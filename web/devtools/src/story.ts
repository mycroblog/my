// SPDX-License-Identifier: AGPL-3.0-only

// The story a window shows, as the Storybook addon (storybook-preview.ts) publishes it on the window, and the page a
// note is on: the story's, which stays while Storybook switches stories without loading the page again, else the
// window's URL.

import { storyEvent, type Story } from "./note";

type Shown = { myDevtoolsStory?: Story };

/** The story the window shows, or null on a page that is no story. */
export const storyOf = (win: Window): Story | null => (win as Shown).myDevtoolsStory ?? null;

/** Records the story the window shows and tells the overlay, after the render that shows it. */
export function showStory(win: Window, story: Story): void {
  (win as Shown).myDevtoolsStory = story;
  queueMicrotask(() => {
    win.dispatchEvent(new CustomEvent(storyEvent));
  });
}

/** The page of a window: its story's, else its URL's path, search and hash. */
export function pageOf(win: Window): string {
  const story = storyOf(win);
  if (story) return `/iframe.html?id=${story.id}`;
  const { location } = win;
  return location.pathname + location.search + location.hash;
}
