// SPDX-License-Identifier: AGPL-3.0-only
import { describe, expect, test } from "vitest";
import {
  durationParts,
  maxPollOptions,
  minPollOptions,
  newPoll,
  pollDurations,
  pollProblem,
  pollRequest,
  withOption,
  withoutOption,
} from "./poll";

describe("the poll of a new post", () => {
  test("starts with two empty options for a day, and keeps between the fewest and the most options", () => {
    const poll = newPoll();
    expect(poll.options).toHaveLength(minPollOptions);
    expect(poll.expiresIn).toBe(86400);
    expect(withoutOption(poll, poll.options[0]?.key ?? "")).toBe(poll);
    let more = poll;
    for (let i = 0; i < 10; i++) more = withOption(more);
    expect(more.options).toHaveLength(maxPollOptions);
    expect(new Set(more.options.map((o) => o.key)).size).toBe(maxPollOptions);
    const removed = withoutOption(more, more.options[1]?.key ?? "");
    expect(removed.options.map((o) => o.key)).toEqual(more.options.filter((_, i) => i !== 1).map((o) => o.key));
  });

  test("is sent only with options that are filled in and different once trimmed", () => {
    const [a, b] = newPoll().options;
    const poll = (titles: [string, string]) => ({
      ...newPoll(),
      options: [
        { key: a?.key ?? "a", title: titles[0] },
        { key: b?.key ?? "b", title: titles[1] },
      ],
    });
    expect(pollProblem(poll(["Tea", " "]))).toBe("empty");
    expect(pollProblem(poll(["Tea", " Tea "]))).toBe("duplicate");
    expect(pollProblem(poll(["Tea", "Coffee"]))).toBeNull();
    expect(pollRequest({ ...poll([" Tea ", "Coffee"]), expiresIn: 300 })).toEqual({
      options: ["Tea", "Coffee"],
      expires_in: 300,
    });
    expect(pollRequest({ ...poll(["Tea", "Coffee"]), multiple: true, hideTotals: true })).toEqual({
      options: ["Tea", "Coffee"],
      expires_in: 86400,
      multiple: true,
      hide_totals: true,
    });
  });

  test("offers durations within the server's limits, each named by its largest whole unit", () => {
    expect(pollDurations.every((s) => s >= 300 && s <= 2592000)).toBe(true);
    expect(durationParts(300)).toEqual({ unit: "minutes", count: 5 });
    expect(durationParts(6 * 3600)).toEqual({ unit: "hours", count: 6 });
    expect(durationParts(3 * 86400)).toEqual({ unit: "days", count: 3 });
  });
});
