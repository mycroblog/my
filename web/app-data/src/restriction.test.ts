// SPDX-License-Identifier: AGPL-3.0-only

// Which refusals are a partial restriction of the reader's account (#3216): the server answers a moderation action that
// keeps the account from posting or from interacting with a problem type of its own, and the client tells it from any
// other refusal so that it can lead the reader to Account status. The errors are those a write of the app's data
// rejects with, as the fake answers them.

import { expect, test } from "bun:test";
import { scenario, type FakeApi } from "@my/e2e/fake";
import { restrictionRefused } from ".";
import { markStatus } from "./interactions";
import { fakeData } from "./test/data";

/** A problem of the server of `type` with `status`, in its words. */
const problem = (type: string, status: number) => ({
  type,
  title: "This could not be done.",
  status,
  detail: "A moderation action keeps your account from this for now.",
});

/** What a favourite of sam's post rejects with, when the server answers it as `answer` says. */
async function refusalOf(answer: Parameters<FakeApi["script"]>[1]): Promise<unknown> {
  const { fake, data } = fakeData({
    seed: {
      people: scenario.people,
      posts: [{ key: "loaf", author: "sam", text: "The first loaf.", visibility: "public", language: "en" }],
    },
  });
  fake.script("POST /statuses/{id}/favourite", answer);
  try {
    await markStatus(data, fake.idOf("loaf"), { type: "favourite", on: true });
  } catch (error: unknown) {
    return error;
  }
  throw new Error("test: the favourite was not refused");
}

test("a refusal of posting is a restriction's", async () => {
  const error = await refusalOf({ fail: problem("/api/v1/problems/posting-restricted", 403) });

  expect(restrictionRefused(error)).toBe(true);
});

test("a refusal of interacting is a restriction's", async () => {
  const error = await refusalOf({ fail: problem("/api/v1/problems/interaction-restricted", 403) });

  expect(restrictionRefused(error)).toBe(true);
});

for (const [what, fail] of [
  ["a 403 that means no more than its status", problem("about:blank", 403)],
  ["a problem of another type", problem("/api/v1/problems/status-rejected", 422)],
  ["a failure of the server", 500],
  ["no answer of the server", "network"],
] as const)
  test(`${what} is not a restriction's`, async () => {
    const error = await refusalOf({ fail });

    expect(restrictionRefused(error)).toBe(false);
  });

test("what is not a problem of the server is not a restriction's, even when it reads like one", () => {
  for (const error of [
    new Error("/api/v1/problems/posting-restricted"),
    problem("/api/v1/problems/posting-restricted", 403),
    "/api/v1/problems/interaction-restricted",
    undefined,
    null,
  ])
    expect(restrictionRefused(error)).toBe(false);
});
