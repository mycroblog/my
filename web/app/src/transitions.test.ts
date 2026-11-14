// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { navigationKind } from "./transitions";

const at = (pathname: string, index: number) => ({ pathname, state: { __TSR_index: index } });

test("the first screen and a change of the address alone do not move", () => {
  expect(navigationKind({ toLocation: at("/", 0) })).toBe("none");
  expect(navigationKind({ fromLocation: at("/", 0), toLocation: at("/", 1) })).toBe("none");
});

test("one place of the navigation replaces another, forward and back", () => {
  expect(navigationKind({ fromLocation: at("/", 0), toLocation: at("/notifications", 1) })).toBe("replace");
  expect(navigationKind({ fromLocation: at("/notifications", 1), toLocation: at("/", 0) })).toBe("replace");
});

test("elsewhere a navigation pushes, and going back in the history pops", () => {
  expect(navigationKind({ fromLocation: at("/", 0), toLocation: at("/@alice", 1) })).toBe("push");
  expect(navigationKind({ fromLocation: at("/@alice", 1), toLocation: at("/", 0) })).toBe("pop");
});

test("a post and a profile push from the timeline and the notifications, and pop back to them", () => {
  const post = "/@alice@remote.example/status/0190a5d0-0000-7000-8000-000000000001";
  expect(navigationKind({ fromLocation: at("/", 0), toLocation: at(post, 1) })).toBe("push");
  expect(navigationKind({ fromLocation: at(post, 1), toLocation: at("/@alice@remote.example", 2) })).toBe("push");
  expect(navigationKind({ fromLocation: at("/@alice@remote.example", 2), toLocation: at(post, 1) })).toBe("pop");
  expect(navigationKind({ fromLocation: at(post, 1), toLocation: at("/", 0) })).toBe("pop");
  expect(navigationKind({ fromLocation: at("/notifications", 0), toLocation: at("/@bob", 1) })).toBe("push");
  expect(navigationKind({ fromLocation: at("/@bob", 1), toLocation: at("/notifications", 0) })).toBe("pop");
});

test("opening the composer over the screen raises it, and closing it does not move", () => {
  const composing = { ...at("/", 1), search: { compose: "reply", to: "9" } };
  expect(navigationKind({ fromLocation: at("/", 0), toLocation: composing })).toBe("modal");
  expect(navigationKind({ fromLocation: composing, toLocation: at("/", 0) })).toBe("none");
  // A reload with the composer open is a first screen.
  expect(navigationKind({ toLocation: composing })).toBe("none");
});

test("the sign-in opens by a page load, as a first screen, and a move to it within the app pushes", () => {
  expect(navigationKind({ toLocation: at("/login", 0) })).toBe("none");
  expect(navigationKind({ toLocation: at("/login/callback", 0) })).toBe("none");
  expect(navigationKind({ fromLocation: at("/", 0), toLocation: at("/login", 1) })).toBe("push");
});
