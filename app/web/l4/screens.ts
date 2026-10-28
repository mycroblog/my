// SPDX-License-Identifier: AGPL-3.0-only
// What each screen's L4 tasks need on the demo server (`node e2e/server.ts demo`, the scenario of demo/scenario.ts),
// read from the front matter of the screen's L1 file (docs/architecture/web-client/screens/<screen>.md, whose
// README.md documents the keys): the address the agent starts at, the person of the scenario it is signed in as (or
// nobody, for the screen or for one task), the code of the screen that --since watches, the changes to the scenario that the tasks need, the tasks
// that read email in Mailpit, and the tasks the demo server cannot set up, with the reason. Each line of the front
// matter is `<key>: <JSON value>`, so the block is YAML and needs no YAML parser. A task that turns Web Push on names
// the person whose reply the harness sends after it (push.ts judges it), and a task may carry notes for its result. screens.test.ts checks every L1
// file: that its tasks named here are rows of its task table, and that its people are of the scenario.

/** What a screen's setup can reach on the demo server. */
export type SetupContext = {
  /** Runs `bin/my` with `args` against the demo server's database and returns its standard output. */
  runMy: (args: readonly string[], input?: string) => string;
  /**
   * Calls the client API (`/api/v1` + `path`) as the person `username` of the scenario and returns the JSON of the
   * response (undefined when it has none); a failure throws.
   */
  api: (username: string, method: ApiMethod, path: string, body?: unknown) => Promise<unknown>;
};

type ApiMethod = "GET" | "PUT" | "PATCH" | "POST" | "DELETE";

/** One change to the scenario: a command of `bin/my`, or a call of the client API as a person of the scenario. */
export type SeedStep = { my: string[] } | { api: string; method: ApiMethod; path: string; body?: unknown };

export type ScreenSetup = {
  /** The path the browser opens before the agent starts. */
  start: string;
  /** The person of the scenario the tasks begin signed in as, by username; null begins signed out. */
  persona: string | null;
  /** The code of the screen (context-map patterns: `*` within a segment, `**` across): --since measures the screen when they change. */
  paths: readonly string[];
  /** The paths of the tasks (by their text in the L1) that start elsewhere than `start`. */
  starts?: Readonly<Record<string, string>>;
  /** The tasks (by their text in the L1) that begin signed in as another person than `persona`, or signed out (null). */
  personas?: Readonly<Record<string, string | null>>;
  /** Changes the scenario as the tasks need, in order; they run once per run, before the first task of the screen. */
  seed?: readonly SeedStep[];
  /**
   * The tasks that read an email: the username of the person of the scenario whose mailbox the agent reads in
   * Mailpit's web UI, in a second tab. The harness removes that person's earlier email from Mailpit before the task.
   */
  mail?: Readonly<Record<string, string>>;
  /**
   * The tasks that turn Web Push on: the username of the person of the scenario who replies to the signed-in person
   * after the task. The browser subscribes with the fake push service, and the harness judges the task (push.ts).
   */
  push?: Readonly<Record<string, string>>;
  /** Notes on tasks (by their text in the L1), such as what L4 cannot measure of them; the result carries them. */
  notes?: Readonly<Record<string, string>>;
  /** The tasks (by their text in the L1) that the demo server cannot set up, and why; they are recorded, not measured. */
  unavailable?: Readonly<Record<string, string>>;
};

const methods = new Set<string>(["GET", "PUT", "PATCH", "POST", "DELETE"]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isStrings(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((item) => typeof item === "string");
}

function isStringRecord(value: unknown): value is Record<string, string> {
  return isRecord(value) && Object.values(value).every((item) => typeof item === "string");
}

function isSeedStep(value: unknown): value is SeedStep {
  if (!isRecord(value)) {
    return false;
  }
  if ("my" in value) {
    return Object.keys(value).length === 1 && isStrings(value["my"]);
  }
  return (
    typeof value["api"] === "string" &&
    typeof value["method"] === "string" &&
    methods.has(value["method"]) &&
    typeof value["path"] === "string" &&
    Object.keys(value).every((key) => ["api", "method", "path", "body"].includes(key))
  );
}

/** The check of each key of the front matter, and the form it expects. */
const keys: Readonly<Record<keyof ScreenSetup, [(value: unknown) => boolean, string]>> = {
  start: [(v) => typeof v === "string" && v.startsWith("/"), "a path"],
  persona: [(v) => v === null || typeof v === "string", "a username or null"],
  paths: [isStrings, "a list of patterns"],
  starts: [isStringRecord, "an object of tasks and paths"],
  personas: [
    (v) => isRecord(v) && Object.values(v).every((item) => item === null || typeof item === "string"),
    "an object of tasks and usernames or null",
  ],
  seed: [
    (v) => Array.isArray(v) && v.every(isSeedStep),
    'a list of {"my": [...]} or {"api", "method", "path", "body"}',
  ],
  mail: [isStringRecord, "an object of tasks and usernames"],
  push: [isStringRecord, "an object of tasks and usernames"],
  notes: [isStringRecord, "an object of tasks and notes"],
  unavailable: [isStringRecord, "an object of tasks and reasons"],
};

/**
 * Parses the front matter of the L1 file `markdown` of `screen`: the lines between a first line `---` and the next
 * `---`, each `<key>: <JSON value>`. `start`, `persona`, and `paths` are required; an unknown key or a value of the
 * wrong form is an error, so that a broken L1 fails the run instead of measuring the wrong thing.
 */
export function parseSetup(screen: string, markdown: string): ScreenSetup {
  const file = `screens/${screen}.md`;
  const lines = markdown.split("\n");
  const end = lines.indexOf("---", 1);
  if (lines[0] !== "---" || end < 0) {
    throw new Error(`${file}: no front matter (a block between two "---" lines at the top)`);
  }
  const values: Record<string, unknown> = {};
  for (const line of lines.slice(1, end)) {
    const match = /^([a-z]+):\s*(.+)$/.exec(line);
    if (match === null) {
      throw new Error(`${file}: a front matter line is not "<key>: <JSON value>": ${line}`);
    }
    const [, key = "", raw = ""] = match;
    const check = keys[key as keyof ScreenSetup] as (typeof keys)[keyof ScreenSetup] | undefined;
    if (check === undefined) {
      throw new Error(`${file}: unknown front matter key "${key}"`);
    }
    let value: unknown;
    try {
      value = JSON.parse(raw);
    } catch {
      throw new Error(`${file}: the value of "${key}" is not JSON: ${raw}`);
    }
    if (!check[0](value)) {
      throw new Error(`${file}: "${key}" must be ${check[1]}`);
    }
    values[key] = value;
  }
  for (const key of ["start", "persona", "paths"]) {
    if (!(key in values)) {
      throw new Error(`${file}: the front matter has no "${key}"`);
    }
  }
  return values as ScreenSetup;
}

/** Applies the seed steps of a screen to the demo server. */
export async function applySeed(steps: readonly SeedStep[], { runMy, api }: SetupContext): Promise<void> {
  for (const step of steps) {
    if ("my" in step) {
      runMy(step.my);
    } else {
      await api(step.api, step.method, step.path, step.body);
    }
  }
}
