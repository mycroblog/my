// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { shownScreen } from "./screen";

const home = { routeId: "/", pathname: "/" };
const root = { routeId: "__root__", pathname: "/" };

test("the shown screen is the screen of the matches the router renders, whatever its location says", () => {
  // As a navigation starts, the router's location is the new address while the matches are still the old screen's:
  // the focus and the position wait for the matches.
  const starting = { location: { pathname: "/notifications" }, matches: [root, home] };
  expect(shownScreen(starting)).toBe("/ /");
  const shown = { ...starting, matches: [root, { routeId: "/notifications", pathname: "/notifications" }] };
  expect(shownScreen(shown)).toBe("/notifications /notifications");
});

test("an address with no screen is told from the home screen, and no matches are no screen", () => {
  expect(shownScreen({ matches: [root] })).not.toBe(shownScreen({ matches: [root, home] }));
  expect(shownScreen({ matches: [] })).toBe("");
});
