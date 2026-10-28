// SPDX-License-Identifier: AGPL-3.0-only
// The issues of an L4 run (ADR-0114): one per screen whose tasks have lostness above the threshold, with the screen's
// name in the title, each task with the path the agent took and the L1's shortest path.
import type { RunResult, TaskResult } from "./results.ts";
import { screensDir } from "./tasks.ts";

/** Whether `task` was measured and ended without completion: the strongest sign of a lost user (ADR-0110 P1). */
function isNotCompleted(task: TaskResult): boolean {
  return task.status === "measured" && !task.completed;
}

/**
 * The measured tasks of `result` that are over its threshold or not completed, by screen in the order of the run.
 * Tasks that are `unavailable` (the environment cannot set them up) or `error` (the agent failed to run) say nothing
 * about the screen and are left out; see `skippedTasks`.
 */
export function lostByScreen(result: RunResult): Map<string, TaskResult[]> {
  const screens = new Map<string, TaskResult[]>();
  for (const task of result.tasks) {
    const over = task.status === "measured" && task.lostness !== null && task.lostness > result.threshold;
    if (isNotCompleted(task) || over) {
      screens.set(task.screen, [...(screens.get(task.screen) ?? []), task]);
    }
  }
  return screens;
}

/** The tasks that were not measured because of the environment (`unavailable`) or a failed run (`error`). */
export function skippedTasks(result: RunResult): TaskResult[] {
  return result.tasks.filter((task) => task.status === "unavailable" || task.status === "error");
}

/** One line per skipped task for the run summary. */
export function skippedSummary(tasks: readonly TaskResult[]): string[] {
  return tasks.map(
    (task) => `- ${task.status}: ${task.screen}: ${task.task}${task.reason === undefined ? "" : ` (${task.reason})`}`,
  );
}

/** The title of the issue of `screen`, the same every night so that later runs find it. */
export function issueTitle(task: Pick<TaskResult, "screen" | "screenTitle">): string {
  return `L4: lost on ${task.screenTitle} (screens/${task.screen}.md)`;
}

/** One step of a path as a list item: the action, the control, and the page after it. */
function stepLine(step: TaskResult["path"][number], index: number): string {
  const target = step.target === "" ? "" : ` ${step.target}`;
  return `${String(index + 1)}. \`${step.action.replace(/^browser_/, "")}\`${target} → \`${step.url}\``;
}

/**
 * The body of the issue, or of the comment that updates it, for the lost `tasks` of one screen. `runURL` links the
 * workflow run whose artifact holds the full results.
 */
export function issueBody(result: RunResult, tasks: readonly TaskResult[], runURL: string, repo: string): string {
  const [first] = tasks;
  if (first === undefined) {
    return "";
  }
  const l1 = `${screensDir}/${first.screen}.md`;
  const lines = [
    `The nightly L4 task test found tasks of this screen with lostness above ${String(result.threshold)}, or not completed, on ` +
      `${result.commit.slice(0, 12)}${runURL === "" ? "" : ` ([run](${runURL}), artifact \`l4-results\`)`}.`,
    `The tasks and their shortest paths are in the L1: [${l1}](https://github.com/${repo}/blob/${result.commit}/${l1}#2-作業).`,
    "Fix the L1 or the screen, or explain why the path is right (ADR-0110 §7, ADR-0114).",
  ];
  for (const task of tasks) {
    lines.push(
      "",
      `### ${task.task}${isNotCompleted(task) ? " (not completed)" : ""}`,
      "",
      `- Lostness: **${String(task.lostness)}** (steps S = ${String(task.steps)}, distinct places N = ${String(task.distinct)}, ` +
        `L1 shortest path R = ${String(task.shortest)}), backtracks: ${String(task.backtracks)}`,
      `- Completed: ${task.completed ? "yes" : "no"} (${task.ending})${task.summary === "" ? "" : `: ${task.summary}`}`,
      ...(task.judgement === undefined ? [] : [`- Judged by the harness: ${task.judgement.detail}`]),
      ...(task.notes === undefined ? [] : [`- Notes: ${task.notes}`]),
      `- Start: \`${task.start}\`; L1 shortest path: ${String(task.shortest)} step(s)`,
      "",
      "Path taken:",
      "",
      ...task.path.map(stepLine),
    );
    if (task.mailPath.length > 0) {
      lines.push(
        "",
        `Steps in the mailbox (Mailpit's web UI), each run counted as the one step \`mail\` above: ${String(task.mailPath.length)}`,
        "",
        ...task.mailPath.map(stepLine),
      );
    }
  }
  return `${lines.join("\n")}\n`;
}
