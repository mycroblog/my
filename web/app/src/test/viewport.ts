// SPDX-License-Identifier: AGPL-3.0-only

// The viewport's width, for the component tests of the app's routes. No simulation lays a page out, so the rail and
// the tab bar both render whatever the width; what the app itself asks of the width (whether its menus are sheets) it
// asks through a media query, which a test answers here. Whether a width is under a breakpoint, and what CSS shows at
// it, is the browser's to say.

import { onTestFinished } from "bun:test";
import { screen } from "@testing-library/react";

/**
 * Makes the viewport a phone's for the app rendered next, until the test ends: the media queries of a width under a
 * tablet's (40rem) match, and no other.
 */
export function onAPhone(): void {
  const matchMedia = window.matchMedia;
  window.matchMedia = (query: string) => {
    const list = matchMedia.call(window, query);
    Object.defineProperty(list, "matches", { value: /^\(width < 40rem\)$/.test(query) });
    return list;
  };
  onTestFinished(() => {
    window.matchMedia = matchMedia;
  });
}

/** The rail, the main navigation of a wide viewport: the first of the two navigations named Main that render. */
export function rail(): HTMLElement {
  const [navigation] = screen.getAllByRole("navigation", { name: "Main" });
  if (navigation === undefined) throw new Error("test: the shell shows no main navigation");
  return navigation;
}

/** The tab bar, the main navigation of a phone, in the dock: the last of the two navigations named Main that render. */
export function tabBar(): HTMLElement {
  const navigation = screen.getAllByRole("navigation", { name: "Main" }).at(-1);
  if (navigation === undefined) throw new Error("test: the shell shows no main navigation");
  return navigation;
}
