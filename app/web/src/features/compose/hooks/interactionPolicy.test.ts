// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, test } from "vitest";
import { visibilityLimit, withApproval, withAutomatic, withChange } from "./interactionPolicy";

const open = { automatic: "everyone" } as const;

describe("withChange", () => {
  test("replaces the kinds the change has and keeps the others", () => {
    expect(withChange({ reply: open, announce: open, quote: open }, { announce: { automatic: "nobody" } })).toEqual({
      reply: open,
      announce: { automatic: "nobody" },
      quote: open,
    });
  });
});

describe("withAutomatic", () => {
  test("keeps the approval while it is wider", () => {
    expect(withAutomatic({ automatic: "followers", manual: "everyone" }, "nobody")).toEqual({
      automatic: "nobody",
      manual: "everyone",
    });
  });

  test.each(["everyone", "followers"] as const)("drops an approval that %s is not narrower than", (automatic) => {
    expect(withAutomatic({ automatic: "mentioned", manual: "followers" }, automatic)).toEqual({ automatic });
  });
});

describe("withApproval", () => {
  test("lets everyone else ask, and stops it", () => {
    expect(withApproval({ automatic: "followers" }, true)).toEqual({ automatic: "followers", manual: "everyone" });
    expect(withApproval({ automatic: "followers", manual: "everyone" }, false)).toEqual({ automatic: "followers" });
  });

  test("keeps a narrower approval another client chose", () => {
    expect(withApproval({ automatic: "nobody", manual: "followers" }, true)).toEqual({
      automatic: "nobody",
      manual: "followers",
    });
  });

  test("leaves no one to ask when everyone is approved at once", () => {
    expect(withApproval(open, true)).toEqual(open);
  });
});

describe("visibilityLimit", () => {
  test.each([
    ["reply", "direct", null],
    ["announce", "public", null],
    ["announce", "private", "author"],
    ["quote", "private", "author"],
    ["quote", "direct", "none"],
  ] as const)("%s of a %s post: %s", (kind, visibility, limit) => {
    expect(visibilityLimit(kind, visibility)).toBe(limit);
  });
});
