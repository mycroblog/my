// SPDX-License-Identifier: AGPL-3.0-only
// The JSON file of an L4 run (run.ts writes it, issues.ts reads it), uploaded as the artifact of the Nightly job.
import type { Task } from "./tasks.ts";
import type { Step } from "./transcript.ts";

/** Lostness above this marks a task as lost (ADR-0110 §7). */
export const lostnessThreshold = 0.4;

/** The result of one task in the JSON file. */
export type TaskResult = Task & {
  /** The path the browser starts at. */
  start: string;
  /** `measured`; `unavailable` when the fixtures cannot set the task up; `dry-run`; `error` when the agent failed to run. */
  status: "measured" | "unavailable" | "dry-run" | "error";
  /** Why the task is unavailable or failed. */
  reason?: string;
  completed: boolean;
  ending: string;
  summary: string;
  steps: number;
  distinct: number;
  backtracks: number;
  lostness: number | null;
  costUSD: number | null;
  path: Step[];
};

/** The JSON file of one run. */
export type RunResult = {
  commit: string;
  startedAt: string;
  dryRun: boolean;
  model: string;
  threshold: number;
  /** How the screens were picked, and the commit of `since`. */
  selection: { mode: "screens" | "since" | "all"; since?: string };
  cap: number;
  tasks: TaskResult[];
};
