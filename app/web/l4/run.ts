// SPDX-License-Identifier: AGPL-3.0-only
// Runs the L4 task tests (ADR-0110 §7, ADR-0114): an agent (Claude Code, non-interactive) does each task of the
// screens' L1 files in a browser it sees only through ARIA snapshots (the Playwright MCP server), against `my serve`
// with the web E2E fixtures (e2e/server.ts), and the run writes the steps, backtracks, and lostness of every task to
// one JSON file (transcript.ts says how they are measured). `scripts/task.sh web:l4` runs it; the Nightly workflow
// runs it on main with the agent authenticated by Workload Identity Federation (ADR-0115).
//
//   node l4/run.ts [--screen <name>]... [--since <commit>] [--cap <n>] [--dry-run] [--out <file>]
//
// --screen picks screens by the name of their L1 file; otherwise --since picks the screens whose L1 file or
// context-map paths changed since that commit (tasks.ts), and neither picks every screen. --cap bounds the tasks of
// the run. --dry-run does everything but call the agent: it selects the tasks, starts the server with the fixtures,
// signs in, and opens each screen's start through the Playwright MCP server as the agent would, then records the
// tasks with no measures. L4_MODEL, L4_MAX_TURNS, L4_BUDGET_USD and L4_TIMEOUT_MINUTES tune the agent.
import { execFileSync, spawn } from "node:child_process";
import { createWriteStream, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { parseArgs } from "node:util";
import { chromium } from "@playwright/test";
import { runMy, serverDatabaseOf } from "../e2e/server-cli.ts";
import { admin, serverURL, voiceServerDir } from "../e2e/server-origin.ts";
import { MCPClient, mcpCommand } from "./mcp.ts";
import { setupOf } from "./screens.ts";
import { capTasks, parseContextMap, parseTasks, screenChanged, screensDir, type Task } from "./tasks.ts";
import { lostnessThreshold, type RunResult, type TaskResult } from "./results.ts";
import { mcpPrefix, measure, parseTranscript, readTools, stepTools } from "./transcript.ts";

const webDir = resolve(import.meta.dirname, "..");
const root = resolve(webDir, "../..");

const { values } = parseArgs({
  options: {
    screen: { type: "string", multiple: true, default: [] },
    since: { type: "string", default: "" },
    cap: { type: "string", default: "10" },
    "dry-run": { type: "boolean", default: false },
    out: { type: "string", default: join(root, ".tmp/l4/results.json") },
  },
});

const dryRun = values["dry-run"];
const cap = Number(values.cap);
const out = resolve(values.out);
const workDir = dirname(out);
const model = process.env["L4_MODEL"] ?? "sonnet";
const maxTurns = process.env["L4_MAX_TURNS"] ?? "40";
const budgetUSD = process.env["L4_BUDGET_USD"] ?? "1";
const timeoutMs = Number(process.env["L4_TIMEOUT_MINUTES"] ?? "10") * 60_000;

function git(...args: string[]): string {
  return execFileSync("git", args, { cwd: root, encoding: "utf8" }).trim();
}

/** The tasks of the run, by --screen, --since, or every screen. */
function selectTasks(): { tasks: Task[]; selection: RunResult["selection"] } {
  const all = readdirSync(join(root, screensDir))
    .filter((name) => name.endsWith(".md") && name !== "README.md")
    .map((name) => name.slice(0, -3))
    .toSorted();
  let screens = all;
  let selection: RunResult["selection"] = { mode: "all" };
  const named = values.screen.flatMap((value) => value.split(",")).filter((name) => name !== "");
  if (named.length > 0) {
    const unknown = named.filter((name) => !all.includes(name));
    if (unknown.length > 0) {
      throw new Error(`no L1 file in ${screensDir} for: ${unknown.join(", ")}`);
    }
    screens = named;
    selection = { mode: "screens" };
  } else if (values.since !== "") {
    let changed: string[] | undefined;
    try {
      changed = git("diff", "--name-only", `${values.since}...HEAD`).split("\n");
    } catch {
      console.log(`l4: ${values.since} is not in the history; measuring every screen.`);
    }
    if (changed !== undefined) {
      const entries = parseContextMap(readFileSync(join(root, "tools/agentkit/context-map.yaml"), "utf8"));
      screens = all.filter((screen) => screenChanged(screen, changed, entries));
      selection = { mode: "since", since: values.since };
    }
  }
  const tasks = screens.flatMap((screen) =>
    parseTasks(screen, readFileSync(join(root, screensDir, `${screen}.md`), "utf8")),
  );
  return { tasks: capTasks(tasks, cap), selection };
}

/** Starts `my serve` with the E2E fixtures (e2e/server.ts) and waits until it answers. */
async function startServer(): Promise<() => Promise<void>> {
  const log = createWriteStream(join(workDir, "server.log"));
  // server-origin.ts wrote the ports, the directories, and the KEK to process.env, which the child inherits.
  const server = spawn(process.execPath, [join(webDir, "e2e/server.ts")], {
    cwd: webDir,
    env: process.env,
    stdio: ["ignore", "pipe", "pipe"],
  });
  server.stdout.pipe(log);
  server.stderr.pipe(log);
  const exited = new Promise<void>((done) => {
    server.on("exit", () => {
      done();
    });
  });
  const deadline = Date.now() + 120_000;
  for (;;) {
    if (server.exitCode !== null) {
      throw new Error(`e2e/server.ts exited with ${String(server.exitCode)}; see ${join(workDir, "server.log")}`);
    }
    try {
      await fetch(serverURL);
      break;
    } catch {
      if (Date.now() > deadline) {
        throw new Error(`my serve did not answer at ${serverURL}; see ${join(workDir, "server.log")}`);
      }
      await new Promise((wait) => setTimeout(wait, 500));
    }
  }
  return async () => {
    server.kill("SIGTERM");
    await exited;
    rmSync(voiceServerDir, { recursive: true, force: true });
  };
}

/**
 * Signs alice in to the web client as e2e/main-flow.spec.ts does, sets her display language to Japanese (the
 * language of the L1 tasks), and saves the browser's storage state for the Playwright MCP server.
 */
async function signIn(storageState: string): Promise<void> {
  const browser = await chromium.launch();
  try {
    const page = await browser.newPage({ locale: "ja-JP" });
    await page.goto(`${serverURL}/login`);
    const authorized = page.waitForRequest(
      (request) =>
        new URL(request.url()).pathname.startsWith("/api/") &&
        request.headers()["authorization"]?.startsWith("Bearer ") === true,
    );
    await page.getByRole("main").getByRole("button").click();
    await page.locator('input[type="email"][autocomplete="username"]').fill(admin.email);
    await page.locator('input[type="password"][autocomplete="current-password"]').fill(admin.password);
    await page.locator('main button[type="submit"]').first().click();
    const authorization = (await authorized).headers()["authorization"] ?? "";
    await page.waitForURL(`${serverURL}/home`);
    const response = await page.request.patch(`${serverURL}/api/v1/preferences`, {
      headers: { Authorization: authorization },
      data: { locale: "ja" },
    });
    if (!response.ok()) {
      throw new Error(`PATCH /api/v1/preferences: ${String(response.status())}`);
    }
    // The client keeps the language it loaded with until it loads again.
    await page.reload();
    await page.locator('html[lang="ja"]').waitFor({ state: "attached" });
    await page.context().storageState({ path: storageState });
  } finally {
    await browser.close();
  }
}

/** The instructions of the agent for `task`. */
function prompt(task: Task): string {
  return [
    "You are a first-time user of a web app, testing whether its screens are easy to find your way through.",
    "A browser is open at the app's start page, signed in. You see the page only through its accessibility tree:",
    "call browser_snapshot to read it, and act with the other browser tools. You cannot type an address.",
    "",
    `Your task (in the app's language, Japanese): ${task.task}`,
    "",
    "Do it as a user would: read the page, choose what looks right, and stop as soon as the task is done.",
    "Do not explore beyond what the task needs and do not undo the task to check it.",
    "If you cannot find the way, stop and say where you got stuck.",
    'Finish with the structured result: "completed" is true only when the task is done.',
  ].join("\n");
}

const verdictSchema = JSON.stringify({
  type: "object",
  properties: {
    completed: { type: "boolean" },
    summary: { type: "string", description: "What you did, or where you got stuck, in one or two sentences." },
  },
  required: ["completed", "summary"],
  additionalProperties: false,
});

/** Runs the agent on `task` in `dir` and returns its stream-json transcript. */
async function runAgent(task: Task, dir: string, mcpArgs: string[]): Promise<string> {
  const config = join(dir, "mcp.json");
  writeFileSync(config, JSON.stringify({ mcpServers: { playwright: { command: "npx", args: mcpArgs } } }, null, 2));
  const tools = [...stepTools, ...readTools].map((tool) => mcpPrefix + tool);
  const args = [
    "-p",
    prompt(task),
    "--output-format",
    "stream-json",
    "--verbose",
    "--model",
    model,
    "--max-turns",
    maxTurns,
    "--max-budget-usd",
    budgetUSD,
    "--json-schema",
    verdictSchema,
    "--mcp-config",
    config,
    "--strict-mcp-config",
    // No built-in tool (no shell, no files): only the browser tools allowed here. Print mode denies every other tool.
    "--tools",
    "",
    "--allowedTools",
    tools.join(","),
    "--disallowedTools",
    `${mcpPrefix}browser_take_screenshot,${mcpPrefix}browser_navigate,${mcpPrefix}browser_evaluate,${mcpPrefix}browser_run_code_unsafe`,
    "--setting-sources",
    "project",
    "--no-session-persistence",
  ];
  const transcript = join(dir, "transcript.jsonl");
  const output = createWriteStream(transcript);
  const agent = spawn("claude", args, { cwd: dir, env: process.env, stdio: ["ignore", "pipe", "inherit"] });
  agent.stdout.pipe(output);
  const timer = setTimeout(() => agent.kill("SIGTERM"), timeoutMs);
  const code = await new Promise<number | null>((done) => agent.on("exit", done));
  clearTimeout(timer);
  await new Promise((done) => output.end(done));
  const text = readFileSync(transcript, "utf8");
  if (code !== 0 && !text.includes('"type":"result"')) {
    throw new Error(`claude exited with ${String(code)} without a result; see ${transcript}`);
  }
  return text;
}

/** Opens `startURL` through the Playwright MCP server as the agent's first snapshot would, and checks it. */
async function probe(dir: string, mcpArgs: string[], start: string): Promise<void> {
  const client = new MCPClient("npx", mcpArgs, dir);
  try {
    await client.start();
    const tools = await client.tools();
    for (const tool of [...stepTools, ...readTools]) {
      if (!tools.includes(tool)) {
        throw new Error(`the Playwright MCP server has no tool ${tool}`);
      }
    }
    const snapshot = await client.call("browser_snapshot");
    writeFileSync(join(dir, "probe-snapshot.md"), snapshot);
    const url = /- Page URL: (\S+)/.exec(snapshot)?.[1] ?? "";
    if (new URL(url).pathname !== start) {
      throw new Error(`the browser opened ${url}, not ${start}: the sign-in did not carry over`);
    }
    // A screenshot would be the agent's only way to an image; the server must leave it out.
    await client.call("browser_take_screenshot");
    console.log(`l4: ${start} opens signed in (${String(snapshot.split("\n").length)} lines of ARIA snapshot).`);
  } finally {
    client.stop();
  }
}

function emptyResult(task: Task, start: string, status: TaskResult["status"], reason?: string): TaskResult {
  return {
    ...task,
    start,
    status,
    ...(reason === undefined ? {} : { reason }),
    completed: false,
    ending: "none",
    summary: "",
    steps: 0,
    distinct: 0,
    backtracks: 0,
    lostness: null,
    costUSD: null,
    path: [],
  };
}

async function main(): Promise<number> {
  mkdirSync(workDir, { recursive: true });
  const { tasks, selection } = selectTasks();
  const result: RunResult = {
    commit: git("rev-parse", "HEAD"),
    startedAt: new Date().toISOString(),
    dryRun,
    model,
    threshold: lostnessThreshold,
    selection,
    cap,
    tasks: [],
  };
  const write = () => {
    writeFileSync(out, `${JSON.stringify(result, null, 2)}\n`);
  };
  console.log(
    `l4: ${String(tasks.length)} task(s) of ${[...new Set(tasks.map((task) => task.screen))].join(", ") || "no screen"}.`,
  );
  if (tasks.length === 0) {
    write();
    return 0;
  }

  const stop = await startServer();
  let failed = false;
  try {
    const executablePath = chromium.executablePath();
    const seeded = new Set<string>();
    for (const [index, task] of tasks.entries()) {
      const setup = setupOf(task.screen);
      const unavailable = setup.unavailable?.[task.task];
      if (unavailable !== undefined) {
        result.tasks.push(emptyResult(task, setup.start, "unavailable", unavailable));
        continue;
      }
      console.log(`l4: ${task.screen}: ${task.task}`);
      const dir = join(workDir, `${String(index + 1).padStart(2, "0")}-${task.screen}`);
      rmSync(dir, { recursive: true, force: true });
      mkdirSync(dir, { recursive: true });
      try {
        if (!seeded.has(task.screen)) {
          seeded.add(task.screen);
          setup.seed?.({ runMy: (args, input) => runMy(args, input), database: serverDatabaseOf("main") });
        }
        // A sign-in of its own for every task: the client rotates the refresh token, so a storage state works once.
        const storageState = join(dir, "storage-state.json");
        await signIn(storageState);
        const mcpArgs = mcpCommand({ dir, startURL: serverURL + setup.start, storageState, executablePath });
        if (dryRun) {
          await probe(dir, mcpArgs, setup.start);
          result.tasks.push(emptyResult(task, setup.start, "dry-run"));
          continue;
        }
        const transcript = parseTranscript(await runAgent(task, dir, mcpArgs));
        if (transcript.apiError !== null) {
          // Not a measure of the screen: recorded as `error` (no issue), and the run exits non-zero.
          throw new Error(`agent run failed: ${transcript.apiError}`);
        }
        const measures = measure(transcript.path, task.shortest, setup.start);
        result.tasks.push({
          ...emptyResult(task, setup.start, "measured"),
          ...measures,
          completed: transcript.completed,
          ending: transcript.ending,
          summary: transcript.summary,
          costUSD: transcript.costUSD,
          path: transcript.path,
        });
        console.log(
          `l4:   ${transcript.completed ? "done" : "not done"} in ${String(measures.steps)} step(s), lostness ${String(measures.lostness)}.`,
        );
      } catch (error) {
        failed = true;
        const reason = error instanceof Error ? error.message : String(error);
        console.error(`l4:   error: ${reason}`);
        result.tasks.push(emptyResult(task, setup.start, "error", reason));
      } finally {
        write();
      }
    }
  } finally {
    write();
    await stop();
  }
  console.log(`l4: wrote ${out}`);
  return failed ? 1 : 0;
}

process.exitCode = await main();
