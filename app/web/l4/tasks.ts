// SPDX-License-Identifier: AGPL-3.0-only
// The tasks of the L4 task tests (ADR-0110 §7, ADR-0114): the rows of the "作業" table of each screen's L1 file
// (docs/architecture/web-client/screens/<screen>.md), and which screens a change touches: a screen is touched when
// its L1 file changed, or a path of the `paths` of its front matter (screens.ts).

/** One task of a screen's L1: the row of its table and the number of steps of the shortest path. */
export type Task = {
  /** The L1 file's name without `.md`, as `badges`. */
  screen: string;
  /** The screen's name, the first heading of the L1 file without the route. */
  screenTitle: string;
  /** The text of the row, which the agent gets as its task. */
  task: string;
  /** The steps of the shortest path (R of the lostness). */
  shortest: number;
};

/** The directory of the L1 files, relative to the repository's root. */
export const screensDir = "docs/architecture/web-client/screens";

/** The heading of the L1 section that holds the task table (screens/README.md). */
const taskSection = /^###\s+2\.\s/;

/**
 * Parses the tasks of the L1 file `markdown` of `screen`: the rows of the first table under the heading
 * "### 2. 作業", whose second column is the number of steps of the shortest path. A row whose count is not a
 * positive integer is an error, so that a broken L1 fails the run instead of dropping a task.
 */
export function parseTasks(screen: string, markdown: string): Task[] {
  const lines = markdown.split("\n");
  const title = (lines.find((line) => line.startsWith("## ")) ?? `## ${screen}`)
    .slice(3)
    .replace(/[（(][^（(]*[)）]\s*$/, "")
    .trim();
  const start = lines.findIndex((line) => taskSection.test(line));
  if (start < 0) {
    return [];
  }
  const tasks: Task[] = [];
  let row = 0;
  for (const line of lines.slice(start + 1)) {
    if (line.startsWith("#")) {
      break;
    }
    if (!line.startsWith("|")) {
      if (row > 0) {
        break;
      }
      continue;
    }
    row++;
    if (row <= 2) {
      // The header row and the delimiter row.
      continue;
    }
    const [task = "", count = ""] = line
      .split("|")
      .slice(1, -1)
      .map((cell) => cell.trim());
    const shortest = Number(count);
    if (!Number.isInteger(shortest) || shortest < 1) {
      throw new Error(`${screensDir}/${screen}.md: the task "${task}" has no shortest path count ("${count}")`);
    }
    tasks.push({ screen, screenTitle: title, task, shortest });
  }
  return tasks;
}

/**
 * Converts a path pattern to a regular expression: `*` matches within one path segment, `**` any number of
 * segments (as the patterns of tools/agentkit/context-map.yaml).
 */
export function globToRegExp(pattern: string): RegExp {
  let source = "";
  for (let i = 0; i < pattern.length; i++) {
    const c = pattern.charAt(i);
    if (c === "*" && pattern.charAt(i + 1) === "*") {
      const slash = pattern.charAt(i + 2) === "/";
      source += slash ? "(?:.*/)?" : ".*";
      i += slash ? 2 : 1;
    } else if (c === "*") {
      source += "[^/]*";
    } else {
      source += c.replace(/[.+?^${}()|[\]\\]/g, "\\$&");
    }
  }
  return new RegExp(`^${source}$`);
}

/** Whether `changed` (repository paths) touch `screen`: its L1 file or one of `paths`, the code of its front matter. */
export function screenChanged(screen: string, changed: readonly string[], paths: readonly string[]): boolean {
  const patterns = [`${screensDir}/${screen}.md`, ...paths].map(globToRegExp);
  return changed.some((path) => patterns.some((pattern) => pattern.test(path)));
}

/**
 * Picks at most `cap` tasks of `tasks`, taking the screens in turn (the first task of every screen, then the
 * second, ...) so that a cap below the total still measures every selected screen.
 */
export function capTasks(tasks: readonly Task[], cap: number): Task[] {
  const byScreen = new Map<string, Task[]>();
  for (const task of tasks) {
    byScreen.set(task.screen, [...(byScreen.get(task.screen) ?? []), task]);
  }
  const queues = [...byScreen.values()];
  const picked: Task[] = [];
  for (let round = 0; picked.length < cap; round++) {
    const row = queues.map((queue) => queue[round]).filter((task) => task !== undefined);
    if (row.length === 0) {
      break;
    }
    picked.push(...row.slice(0, cap - picked.length));
  }
  return picked;
}
