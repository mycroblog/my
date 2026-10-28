// SPDX-License-Identifier: AGPL-3.0-only
// The poll of a new post (FR-112): its draft, the limits the server applies (PollInput in api/openapi.yaml),
// and the request it becomes.
import type { CreateStatusRequest } from "../api/compose";

/** One option of the draft's poll; `key` is a client-side ID, the React key. */
type PollOptionDraft = { key: string; title: string };

/** The poll of the draft. */
export type PollDraft = {
  options: readonly PollOptionDraft[];
  /** How long the poll takes votes, in seconds: one of `pollDurations`. */
  expiresIn: number;
  multiple: boolean;
  hideTotals: boolean;
};

/** The fewest and the most options of a poll (PollInput.options). */
export const minPollOptions = 2;
export const maxPollOptions = 4;

/** The longest option, in characters (PollInput.options.items.maxLength). */
export const maxPollOptionLength = 50;

const minute = 60;
const hour = 60 * minute;
const day = 24 * hour;

/** The durations the form offers, in seconds, within the server's 5 minutes to 30 days (PollInput.expires_in). */
export const pollDurations: readonly number[] = [
  5 * minute,
  30 * minute,
  hour,
  6 * hour,
  12 * hour,
  day,
  3 * day,
  7 * day,
];

/** A duration as a count of its largest whole unit, for its label. */
export function durationParts(seconds: number): { unit: "minutes" | "hours" | "days"; count: number } {
  if (seconds % day === 0) return { unit: "days", count: seconds / day };
  if (seconds % hour === 0) return { unit: "hours", count: seconds / hour };
  return { unit: "minutes", count: Math.round(seconds / minute) };
}

function option(): PollOptionDraft {
  return { key: crypto.randomUUID(), title: "" };
}

/** A new poll: two empty options, for a day, single-choice, with its counts shown. */
export function newPoll(): PollDraft {
  return { options: [option(), option()], expiresIn: day, multiple: false, hideTotals: false };
}

/** The poll with one more empty option, up to `maxPollOptions`. */
export function withOption(poll: PollDraft): PollDraft {
  return poll.options.length >= maxPollOptions ? poll : { ...poll, options: [...poll.options, option()] };
}

/** The poll without the option `key`, keeping at least `minPollOptions`. */
export function withoutOption(poll: PollDraft, key: string): PollDraft {
  return poll.options.length <= minPollOptions ? poll : { ...poll, options: poll.options.filter((o) => o.key !== key) };
}

/**
 * What keeps the poll from being sent, as the server would refuse it: an empty option, or two options that are the same
 * once trimmed; null when it can be sent. The length is kept by the fields themselves.
 */
export function pollProblem(poll: PollDraft): "empty" | "duplicate" | null {
  const titles = poll.options.map((o) => o.title.trim());
  if (titles.some((title) => title === "")) return "empty";
  return new Set(titles).size === titles.length ? null : "duplicate";
}

/** The `poll` of the request a draft's poll becomes. */
export function pollRequest(poll: PollDraft): NonNullable<CreateStatusRequest["poll"]> {
  return {
    options: poll.options.map((o) => o.title.trim()),
    expires_in: poll.expiresIn,
    ...(poll.multiple ? { multiple: true } : {}),
    ...(poll.hideTotals ? { hide_totals: true } : {}),
  };
}
