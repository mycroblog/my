// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, test } from "vitest";
import { viewerFixture, viewerStatusFixture } from "../../../test/fixtures";
import type { PostingLimits } from "../api/compose";
import { fitWithin } from "./prepareImage";
import {
  countCharacters,
  defaultVisibility,
  domainMentions,
  narrower,
  postedVisibility,
  replyMentions,
} from "./policy";

describe("narrower", () => {
  test.each([
    ["public", "local", "local"],
    ["local", "unlisted", "unlisted"],
    ["private", "public", "private"],
    ["direct", "private", "direct"],
    ["unlisted", "unlisted", "unlisted"],
  ] as const)("of %s and %s is %s", (a, b, expected) => {
    expect(narrower(a, b)).toBe(expected);
  });
});

describe("defaultVisibility", () => {
  test("is the preference for a new post", () => {
    expect(defaultVisibility("unlisted", undefined)).toBe("unlisted");
  });

  test("of a reply is the narrower of the preference and the replied-to post's", () => {
    expect(defaultVisibility("public", viewerStatusFixture({ visibility: "local" }))).toBe("local");
    expect(defaultVisibility("private", viewerStatusFixture({ visibility: "public" }))).toBe("private");
  });
});

describe("replyMentions", () => {
  const bob = { ...viewerFixture, id: "bob-id", username: "bob", acct: "bob" };

  test("mentions the author and the mentioned accounts once, without the viewer", () => {
    const post = viewerStatusFixture({
      account: bob,
      mentions: [
        { id: viewerFixture.id, username: "alice", acct: "alice", url: "https://my.example/@alice" },
        { id: "bob-id", username: "bob", acct: "bob", url: "https://my.example/@bob" },
        { id: "carol-id", username: "carol", acct: "carol@remote.example", url: "https://remote.example/@carol" },
      ],
    });
    expect(replyMentions(post, viewerFixture.id)).toBe("@bob @carol@remote.example ");
  });

  test("is empty for a reply to the viewer's own post that mentions nobody else", () => {
    expect(replyMentions(viewerStatusFixture(), viewerFixture.id)).toBe("");
  });
});

describe("postedVisibility", () => {
  const silenced: PostingLimits = { visibility_ceiling: "private", visibilities: ["private", "direct"] };

  test("keeps a choice that the posting limits keep", () => {
    expect(postedVisibility("direct", silenced)).toBe("direct");
    expect(postedVisibility("public", undefined)).toBe("public");
  });

  test("is the ceiling for a wider choice", () => {
    expect(postedVisibility("local", silenced)).toBe("private");
  });
});

describe("domainMentions", () => {
  test("finds the mentions with a domain, lowercase and once each", () => {
    expect(
      domainMentions("Hi @Carol@Remote.Example, @bob and @carol@remote.example. Also @dave@other.example."),
    ).toEqual(["carol@remote.example", "dave@other.example"]);
  });

  test("skips e-mail addresses and URLs, as the server does", () => {
    expect(domainMentions("mail me@my.example or see https://remote.example/@carol@remote.example")).toEqual([]);
  });
});

test("countCharacters counts code points, as the server does", () => {
  expect(countCharacters("👍🏽 é")).toBe(4);
});

describe("fitWithin", () => {
  test("keeps an image that is small enough", () => {
    expect(fitWithin(1920, 1080)).toEqual({ width: 1920, height: 1080 });
  });

  test("scales a larger image down to 3840 × 2160 pixels of area, keeping the aspect ratio", () => {
    const { width, height } = fitWithin(8000, 2000);
    expect(width * height).toBeLessThanOrEqual(3840 * 2160);
    expect(width / height).toBeCloseTo(4, 2);
  });
});
