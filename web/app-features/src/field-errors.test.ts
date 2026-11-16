// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { NetworkError, ProblemError } from "@mycroblog/mykit";
import { failedPhase } from "./compose/use-composer";
import { fieldError, inputOf, placeRefusal, type FieldMembers } from "./field-errors";

type Input = "text" | "cw" | `option:${string}` | "nested";

const members: FieldMembers<Input> = {
  text: "text",
  spoiler_text: "cw",
  "poll.options[]": (i) => `option:${String(i)}`,
  "groups[].items[]": (i) => (i < 2 ? "nested" : undefined),
};

const title = "This could not be done.";
const detail = "This value is not valid. Check what you entered.";

/** A problem the server answered with `status`, naming `field` when given. */
const problem = (field: string | undefined, status = 422, more: Record<string, unknown> = {}) =>
  new ProblemError(
    { type: "about:blank", title, status, detail, ...(field === undefined ? {} : { field }), ...more },
    new Headers(),
  );

test("a member answers with its input; a member of a list with the input of its index", () => {
  expect(inputOf("text", members)).toBe("text");
  expect(inputOf("spoiler_text", members)).toBe("cw");
  expect(inputOf("poll.options[0]", members)).toBe("option:0");
  expect(inputOf("poll.options[3]", members)).toBe("option:3");
  // In a list of lists, the function gets the first index.
  expect(inputOf("groups[1].items[7]", members)).toBe("nested");
  expect(inputOf("groups[2].items[0]", members)).toBeUndefined();
});

test("a member no input answers to has no input: the list itself, another member, a header", () => {
  expect(inputOf("poll.options", members)).toBeUndefined();
  expect(inputOf("poll.expires_in", members)).toBeUndefined();
  expect(inputOf("language", members)).toBeUndefined();
  expect(inputOf("Idempotency-Key", members)).toBeUndefined();
  expect(inputOf("text[0]", members)).toBeUndefined();
});

test("a refusal of a member an input answers to stands beside it, and the form's summary says the title alone", () => {
  expect(placeRefusal(problem("poll.options[1]"), members)).toEqual({
    reason: title,
    field: { input: "option:1", text: detail },
  });
  expect(placeRefusal(problem("text"), members)).toEqual({ reason: title, field: { input: "text", text: detail } });
});

test("a refusal no input answers to stays the form's, in the server's words", () => {
  expect(placeRefusal(problem("Idempotency-Key"), members)).toEqual({ reason: detail });
  expect(placeRefusal(problem("poll.options"), members)).toEqual({ reason: detail });
  expect(placeRefusal(problem(undefined), members)).toEqual({ reason: detail });
  // Not a 422: the server's words, whatever it names.
  expect(placeRefusal(problem("text", 409), members)).toEqual({ reason: detail });
  // No answer: nothing of the server's, which the form words itself.
  expect(placeRefusal(new NetworkError("POST", "/statuses", new Error("offline")), members).reason).toBeUndefined();
  expect(placeRefusal(new NetworkError("POST", "/statuses", new Error("offline")), members).field).toBeUndefined();
});

test("what a refusal says beside an input is its own, and nothing beside the others", () => {
  const refused = placeRefusal(problem("poll.options[1]"), members);
  expect(fieldError(refused, "option:1")).toBe(detail);
  expect(fieldError(refused, "option:0")).toBeUndefined();
  expect(fieldError(refused, "text")).toBeUndefined();
  expect(fieldError(placeRefusal(problem(undefined), members), "text")).toBeUndefined();
  expect(fieldError(undefined, "text")).toBeUndefined();
});

test("a refused post fails beside the input the server named, else with the server's words", () => {
  expect(failedPhase(problem("spoiler_text"), members)).toEqual({
    kind: "failed",
    reason: title,
    field: { input: "cw", text: detail },
  });
  const unplaced = failedPhase(problem("media"), members);
  expect(unplaced).toMatchObject({ kind: "failed", reason: detail });
  expect(unplaced).not.toHaveProperty("field.input");
});
