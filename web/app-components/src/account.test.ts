// SPDX-License-Identifier: AGPL-3.0-only

// The handle a sentence names an account by (#3174): an account of this server by its username alone, one of another
// server with its server, so that two accounts of one name on two servers read differently. The sentences that take it
// are held where a reader sees them: post.dom.test.tsx (a reply's line) and, in @my/app-features,
// profile/from-post.dom.test.tsx (a mute's line, a block's confirmation and line, the post menu's group). And which
// account the server marks as one the reader may not see (#3387), whose lines unavailable.dom.test.tsx holds.

import { expect, test } from "bun:test";
import { isUnavailable, sentenceHandleOf } from "./account";

test("a sentence names an account of this server by its username alone", () => {
  expect(sentenceHandleOf({ username: "sora", acct: "sora", domain: "demo.test" })).toBe("@sora");
  expect(sentenceHandleOf({ username: "sora", acct: "sora" })).toBe("@sora");
});

test("a sentence names an account of another server with its server", () => {
  expect(sentenceHandleOf({ username: "sora", acct: "sora@photos.example", domain: "photos.example" })).toBe(
    "@sora@photos.example",
  );
});

test("an account without a confirmed username is named by its domain alone", () => {
  expect(sentenceHandleOf({ username: "", acct: "", domain: "far.example" })).toBe("far.example");
});

test("an account is unavailable only where the server marks it so", () => {
  expect(isUnavailable({ unavailable: true })).toBe(true);
  expect(isUnavailable({ unavailable: false })).toBe(false);
  expect(isUnavailable({})).toBe(false);
  expect(isUnavailable({ unavailable: undefined })).toBe(false);
});
