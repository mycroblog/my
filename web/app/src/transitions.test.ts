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
