// SPDX-License-Identifier: MIT
import { Cron } from "croner";

/** One field of a cron expression: its name and the range of its numbers. */
type Field = { readonly name: string; readonly lo: number; readonly hi: number };

const fields: readonly Field[] = [
  { name: "minute", lo: 0, hi: 59 },
  { name: "hour", lo: 0, hi: 23 },
  { name: "day of the month", lo: 1, hi: 31 },
  { name: "month", lo: 1, hi: 12 },
  { name: "day of the week", lo: 0, hi: 7 },
];

/** The most days of each month, February in a leap year. */
const daysIn = [0, 31, 29, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];

/** How far next and prev look: eight years hold every day of the month and of the week in every combination. */
const searchSpan = (8 * 366 + 1) * 24 * 3_600_000;

const digits = /^[0-9]+$/;

/** The error of parseSchedule and Bot.every for an expression or a time zone that mybot does not take. */
export class ScheduleError extends Error {
  override readonly name = "ScheduleError";
}

/** Parses a number of a field, digits alone. */
function value(text: string, f: Field): number {
  const v = Number(text);
  if (!digits.test(text) || v < f.lo || v > f.hi) throw new Error(`the ${f.name} "${text}" is not a number from ${String(f.lo)} to ${String(f.hi)}`);
  return v;
}

/** Parses one field into the set of its values, as the Go library does. */
function parseField(field: string, f: Field): Set<number> {
  const set = new Set<number>();
  for (const part of field.split(",")) {
    const slash = part.indexOf("/");
    const range = slash < 0 ? part : part.slice(0, slash);
    const stepText = slash < 0 ? null : part.slice(slash + 1);
    let from = f.lo;
    let to = f.hi;
    if (range === "*") {
      // every value
    } else if (range.includes("-")) {
      const dash = range.indexOf("-");
      from = value(range.slice(0, dash), f);
      to = value(range.slice(dash + 1), f);
      if (from > to) throw new Error(`the ${f.name} range "${range}" runs backwards`);
    } else {
      if (stepText !== null) throw new Error(`the ${f.name} step "${part}" needs * or a range before the /`);
      from = to = value(range, f);
    }
    let step = 1;
    if (stepText !== null) {
      const n = Number(stepText);
      const most = f.hi - f.lo + 1;
      if (!digits.test(stepText) || n < 1 || n > most) throw new Error(`the ${f.name} step "${stepText}" is not a number from 1 to ${String(most)}`);
      step = n;
    }
    for (let v = from; v <= to; v += step) set.add(v);
  }
  return set;
}

/** The wall-clock hour and minute of at in timeZone. */
function wallClock(format: Intl.DateTimeFormat, at: number): { hour: number; minute: number } {
  let hour = -1;
  let minute = -1;
  for (const p of format.formatToParts(at)) {
    if (p.type === "hour") hour = Number(p.value) % 24;
    else if (p.type === "minute") minute = Number(p.value);
  }
  return { hour, minute };
}

/**
 * A parsed cron expression of five fields, minute hour day-of-month month day-of-week, in a time zone (ADR-0094 §5).
 * Each field is *, a number, a range a-b, a step * /n or a-b/n, or a list of those separated by commas; the day of the
 * week is 0-7, where 0 and 7 are Sunday. When both the day of the month and the day of the week are other than *, a
 * day matches when either does, as in cron. A field of seconds, the aliases such as @daily, names of months and days,
 * and the other extensions of some crons (?, L, W, #) are refused: mybot takes the same expressions as the Go library,
 * and uses croner to compute the times alone.
 *
 * The times of a Schedule are wall-clock times of its time zone: one that a daylight saving time change skips does not
 * happen, and one that the change repeats happens once, at its first occurrence. Times are milliseconds since the Unix
 * epoch.
 */
export class Schedule {
  /** The expression of the schedule. */
  readonly expr: string;
  /** The IANA time zone of the schedule, "UTC" by default. */
  readonly timeZone: string;
  readonly #cron: Cron;
  readonly #hours: Set<number>;
  readonly #minutes: Set<number>;
  readonly #format: Intl.DateTimeFormat;

  /** Use parseSchedule. */
  private constructor(expr: string, timeZone: string, hours: Set<number>, minutes: Set<number>) {
    this.expr = expr;
    this.timeZone = timeZone;
    this.#hours = hours;
    this.#minutes = minutes;
    this.#format = new Intl.DateTimeFormat("en-US", { timeZone, hour: "2-digit", minute: "2-digit", hourCycle: "h23" });
    // legacyMode: a day matches when the day of the month or the day of the week does, as in cron.
    this.#cron = new Cron(expr, { paused: true, mode: "5-part", timezone: timeZone, legacyMode: true });
  }

  /**
   * Parses the cron expression expr in the IANA time zone timeZone, such as "Asia/Tokyo"; "" or absent is UTC. It
   * throws a ScheduleError for an expression or a time zone it does not take.
   */
  static parse(expr: string, timeZone = ""): Schedule {
    const tz = timeZone === "" ? "UTC" : timeZone;
    try {
      new Intl.DateTimeFormat("en-US", { timeZone: tz });
    } catch {
      throw new ScheduleError(`mybot: the time zone "${timeZone}" is no IANA time zone`);
    }
    if (expr.trim().startsWith("@")) {
      throw new ScheduleError(`mybot: the cron expression "${expr}": aliases such as @daily are not supported: write the five fields`);
    }
    const parts = expr.split(/\s+/).filter((p) => p !== "");
    if (parts.length === 6) {
      throw new ScheduleError(
        `mybot: the cron expression "${expr}": a field of seconds is not supported: write the five fields minute hour day month weekday`,
      );
    }
    if (parts.length !== 5) {
      throw new ScheduleError(`mybot: the cron expression "${expr}" has ${String(parts.length)} fields, not the five minute hour day month weekday`);
    }
    let sets: Set<number>[];
    try {
      sets = parts.map((p, i) => parseField(p, fields[i] as Field));
    } catch (error) {
      throw new ScheduleError(`mybot: the cron expression "${expr}": ${(error as Error).message}`);
    }
    const [minutes, hours, dom, months] = sets as [Set<number>, Set<number>, Set<number>, Set<number>, Set<number>];
    // A day exists unless the day of the month alone decides, and no month has it: "0 0 30 2 *".
    if (parts[2] !== "*" && parts[4] === "*" && ![...months].some((m) => [...dom].some((d) => d <= (daysIn[m] ?? 0)))) {
      throw new ScheduleError(`mybot: the cron expression "${expr}" names no day that exists`);
    }
    try {
      return new Schedule(parts.join(" "), tz, hours, minutes);
    } catch (error) {
      throw new ScheduleError(`mybot: the cron expression "${expr}": ${(error as Error).message}`);
    }
  }

  /** Whether at is a wall-clock time of the schedule, and not one that croner moved out of a skipped hour. */
  #onTime(at: number): boolean {
    const { hour, minute } = wallClock(this.#format, at);
    return this.#hours.has(hour) && this.#minutes.has(minute);
  }

  /** The first time of the schedule after at; null when there is none in the next eight years. */
  next(at: number): number | null {
    const end = at + searchSpan;
    let from = at;
    for (;;) {
      const d = this.#cron.nextRun(new Date(from));
      if (d === null || d.getTime() > end) return null;
      const t = d.getTime();
      if (this.#onTime(t)) return t;
      from = t;
    }
  }

  /** The last time of the schedule at or before at; null when there is none in the eight years before. */
  prev(at: number): number | null {
    const end = at - searchSpan;
    // previousRuns finds the times before a reference in whole seconds: one second on includes at itself.
    let ref = Math.floor(at / 1000) * 1000 + 1000;
    for (;;) {
      const d = this.#cron.previousRuns(1, new Date(ref))[0];
      if (d === undefined || d.getTime() < end) return null;
      const t = d.getTime();
      if (t <= at && this.#onTime(t)) return t;
      // croner moved a skipped time after the gap, and finds it again from any reference in the gap: step back a
      // minute at least, which passes over no time of the schedule, whose times are whole minutes.
      ref = Math.min(ref - 60_000, t);
    }
  }
}

/** Parses the cron expression expr in the IANA time zone timeZone ("" or absent is UTC): Schedule.parse. */
export function parseSchedule(expr: string, timeZone = ""): Schedule {
  return Schedule.parse(expr, timeZone);
}
