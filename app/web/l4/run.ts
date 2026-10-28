// SPDX-License-Identifier: AGPL-3.0-only
// Runs the L4 task tests (ADR-0110 §7, ADR-0114): an agent (Claude Code, non-interactive) does each task of the
// screens' L1 files in a browser it sees only through ARIA snapshots (the Playwright MCP server), against the demo
// server (`node e2e/server.ts demo`, ADR-0117 §4: the test build with the demo scenario of demo/scenario.ts and its
// email in Mailpit), signed in as the person of the scenario that screens.ts names for the screen, and the run writes
// the steps, backtracks, and lostness of every task to one JSON file (transcript.ts says how they are measured). A task
// that reads email gets the person's mailbox, Mailpit's web UI, in a second tab, and its steps there are recorded
// apart from the measures. `scripts/task.sh web:l4` runs it, with Mailpit and the test build of `bin/my`; the Nightly
// workflow runs it on main with the agent authenticated by Workload Identity Federation (ADR-0115).
//
//   node l4/run.ts [--screen <name>]... [--since <commit>] [--cap <n>] [--dry-run] [--out <file>]
//
// --screen picks screens by the name of their L1 file; otherwise --since picks the screens whose L1 file or
// context-map paths changed since that commit (tasks.ts), and neither picks every screen. --cap bounds the tasks of
// the run. --dry-run does everything but call the agent: it selects the tasks, starts and seeds the demo server,
// signs in, and opens each task's start (and mailbox) through the Playwright MCP server as the agent would, then
// records the tasks with no measures. L4_MODEL, L4_MAX_TURNS, L4_BUDGET_USD and L4_TIMEOUT_MINUTES tune the agent;
// L4_NO_SANDBOX=1 starts the agent's Chromium without its sandbox (mcp.ts).
import { execFileSync, spawn } from "node:child_process";
import { createWriteStream, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { parseArgs } from "node:util";
import { chromium } from "@playwright/test";
import { type Persona, personas } from "../demo/scenario.ts";
import { mailpit, runMy } from "../e2e/server-cli.ts";
import { demoServerDir, demoServerURL, serverDir, voiceServerDir } from "../e2e/server-origin.ts";
import { MCPClient, mcpCommand } from "./mcp.ts";
import { type SetupContext, setupOf } from "./screens.ts";
import { capTasks, parseContextMap, parseTasks, screenChanged, screensDir, type Task } from "./tasks.ts";
import { lostnessThreshold, type RunResult, type TaskResult } from "./results.ts";
import { failureOf, mcpPrefix, measure, parseTranscript, readTools, stepTools, tabTool } from "./transcript.ts";

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
const sandbox = process.env["L4_NO_SANDBOX"] !== "1";

/** The origin of Mailpit's web UI, the mailbox of the tasks that read email. */
const mailOrigin = `http://127.0.0.1:${String(mailpit.uiPort)}`;

function git(...args: string[]): string {
  return execFileSync("git", args, { cwd: root, encoding: "utf8" }).trim();
}

function personaOf(username: string): Persona {
  const p = personas.find((x) => x.username === username);
  if (p === undefined) {
    throw new Error(`l4/screens.ts: no person ${username} in demo/scenario.ts`);
  }
  return p;
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

/**
 * Starts the demo server (`node e2e/server.ts demo`) and waits until it has applied the scenario: the line
 * "demo: ready" of its output.
 */
async function startServer(): Promise<() => Promise<void>> {
  const log = createWriteStream(join(workDir, "server.log"));
  // server-origin.ts wrote the ports, the directories, and the KEK to process.env, which the child inherits.
  const server = spawn(process.execPath, [join(webDir, "e2e/server.ts"), "demo"], {
    cwd: webDir,
    env: process.env,
    stdio: ["ignore", "pipe", "pipe"],
  });
  server.stderr.pipe(log);
  const ready = new Promise<void>((done) => {
    server.stdout.on("data", (chunk: Buffer) => {
      log.write(chunk);
      if (chunk.toString("utf8").includes("demo: ready")) {
        done();
      }
    });
  });
  const exited = new Promise<void>((done) => {
    server.on("exit", () => {
      done();
    });
  });
  let timer: NodeJS.Timeout | undefined;
  const outcome = await Promise.race([
    ready.then(() => "ready" as const),
    exited.then(() => "exited" as const),
    new Promise<"late">((done) => {
      timer = setTimeout(() => {
        done("late");
      }, 300_000);
    }),
  ]);
  clearTimeout(timer);
  if (outcome === "exited") {
    throw new Error(`e2e/server.ts demo exited with ${String(server.exitCode)}; see ${join(workDir, "server.log")}`);
  }
  if (outcome === "late") {
    server.kill("SIGTERM");
    throw new Error(`the demo server was not ready within 5 minutes; see ${join(workDir, "server.log")}`);
  }
  return async () => {
    server.kill("SIGTERM");
    await exited;
    // server-origin.ts made a directory for each server of e2e/server.ts; the run started only the demo server.
    for (const dir of [serverDir, voiceServerDir, demoServerDir]) {
      rmSync(dir, { recursive: true, force: true });
    }
  };
}

/**
 * Signs `p` in to the web client as e2e/demo-smoke.spec.ts does and sets the display language to Japanese (the
 * language of the L1 tasks); with `storageState`, saves the browser's storage state for the Playwright MCP server.
 * Returns the Authorization header of the client's API calls.
 */
async function signIn(p: Persona, storageState?: string): Promise<string> {
  const browser = await chromium.launch();
  try {
    const page = await browser.newPage({ locale: "ja-JP" });
    await page.goto(`${demoServerURL}/login`);
    const authorized = page.waitForRequest(
      (request) =>
        new URL(request.url()).pathname.startsWith("/api/") &&
        request.headers()["authorization"]?.startsWith("Bearer ") === true,
    );
    // The client's sign-in page leads to the server's with a button, unless it goes there at once.
    const email = page.locator('input[type="email"][autocomplete="username"]');
    const toServer = page.getByRole("main").getByRole("button").first();
    await email.or(toServer).first().waitFor();
    if (!(await email.isVisible())) {
      await toServer.click();
    }
    await email.fill(p.email);
    await page.locator('input[type="password"][autocomplete="current-password"]').fill(p.password);
    await page.locator('main button[type="submit"]').first().click();
    const authorization = (await authorized).headers()["authorization"] ?? "";
    await page.waitForURL(`${demoServerURL}/home`);
    const response = await page.request.patch(`${demoServerURL}/api/v1/preferences`, {
      headers: { Authorization: authorization },
      data: { locale: "ja" },
    });
    if (!response.ok()) {
      throw new Error(`PATCH /api/v1/preferences as ${p.username}: ${String(response.status())}`);
    }
    if (storageState !== undefined) {
      // The client keeps the language it loaded with until it loads again.
      await page.reload();
      await page.locator('html[lang="ja"]').waitFor({ state: "attached" });
      await page.context().storageState({ path: storageState });
    }
    return authorization;
  } finally {
    await browser.close();
  }
}

/** The context of the screens' setups: `bin/my` and the client API as the people of the scenario. */
function setupContext(): SetupContext {
  const tokens = new Map<string, string>();
  return {
    runMy: (args, input) => runMy(args, input, "demo"),
    api: async (username, method, path, body) => {
      let authorization = tokens.get(username);
      if (authorization === undefined) {
        authorization = await signIn(personaOf(username));
        tokens.set(username, authorization);
      }
      const response = await fetch(`${demoServerURL}/api/v1${path}`, {
        method,
        headers: { Authorization: authorization, "Content-Type": "application/json" },
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      });
      if (!response.ok) {
        throw new Error(`${method} /api/v1${path} as ${username}: ${String(response.status)}`);
      }
    },
  };
}

/** Removes the email to `p` from Mailpit, so that the mailbox of a task holds only what the task sends. */
async function clearMailbox(p: Persona): Promise<void> {
  const query = new URLSearchParams({ query: `to:${p.email}` });
  const response = await fetch(`${mailOrigin}/api/v1/search?${query.toString()}`, { method: "DELETE" });
  if (!response.ok) {
    throw new Error(`Mailpit: removing the email to ${p.email} answered ${String(response.status)}`);
  }
}

/** Who the agent is in a task: the person signed in, and the person whose mailbox it reads. */
type Roles = { signedIn: Persona | undefined; mail: Persona | undefined };

/** The instructions of the agent for `task`. */
function prompt(task: Task, roles: Roles): string {
  const { signedIn, mail } = roles;
  return [
    "You are a first-time user of a web app, testing whether its screens are easy to find your way through.",
    signedIn === undefined
      ? "A browser is open at the app's start page; you are not signed in."
      : `A browser is open at the app's start page, signed in as ${signedIn.displayName}.`,
    "You see the page only through its accessibility tree: call browser_snapshot to read it, and act with the other",
    "browser tools. You cannot type an address.",
    ...(mail === undefined
      ? []
      : [
          `You are ${mail.displayName}, and your email address is ${mail.email}. Your mailbox, a web mail app, is open in`,
          "the browser's second tab: switch tabs with browser_tabs. A link in an email may open in a new tab.",
        ]),
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
async function runAgent(task: Task, roles: Roles, dir: string, mcpArgs: string[]): Promise<string> {
  const config = join(dir, "mcp.json");
  writeFileSync(config, JSON.stringify({ mcpServers: { playwright: { command: "npx", args: mcpArgs } } }, null, 2));
  const tools = [...stepTools, ...readTools, ...(roles.mail === undefined ? [] : [tabTool])].map(
    (tool) => mcpPrefix + tool,
  );
  const args = [
    "-p",
    prompt(task, roles),
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

/**
 * Opens `start` through the Playwright MCP server as the agent's first snapshot would, and checks it, and the
 * mailbox's tab of a task that reads email.
 */
async function probe(dir: string, mcpArgs: string[], start: string, mail: boolean): Promise<void> {
  const client = new MCPClient("npx", mcpArgs, dir);
  try {
    await client.start();
    const tools = await client.tools();
    for (const tool of [...stepTools, ...readTools, ...(mail ? [tabTool] : [])]) {
      if (!tools.includes(tool)) {
        throw new Error(`the Playwright MCP server has no tool ${tool}`);
      }
    }
    const snapshot = await client.call("browser_snapshot");
    writeFileSync(join(dir, "probe-snapshot.md"), snapshot);
    const url = /- Page URL: (\S+)/.exec(snapshot)?.[1] ?? "";
    if (new URL(url).pathname !== start) {
      throw new Error(`the browser opened ${url}, not ${start}: the sign-in (or its absence) did not carry over`);
    }
    if (mail) {
      const tabs = await client.call(tabTool, { action: "list" });
      if (!tabs.includes(`${mailOrigin}/`)) {
        throw new Error(`the browser has no tab of the mailbox ${mailOrigin}: ${tabs}`);
      }
    }
    // A screenshot would be the agent's only way to an image; the server must leave it out.
    await client.call("browser_take_screenshot");
    console.log(
      `l4: ${start} opens (${String(snapshot.split("\n").length)} lines of ARIA snapshot)${mail ? ", with the mailbox in a second tab" : ""}.`,
    );
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
    mailPath: [],
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
    const context = setupContext();
    const seeded = new Set<string>();
    for (const [index, task] of tasks.entries()) {
      const setup = setupOf(task.screen);
      const start = setup.starts?.[task.task] ?? setup.start;
      const unavailable = setup.unavailable?.[task.task];
      if (unavailable !== undefined) {
        result.tasks.push(emptyResult(task, start, "unavailable", unavailable));
        continue;
      }
      console.log(`l4: ${task.screen}: ${task.task}`);
      const dir = join(workDir, `${String(index + 1).padStart(2, "0")}-${task.screen}`);
      rmSync(dir, { recursive: true, force: true });
      mkdirSync(dir, { recursive: true });
      try {
        if (!seeded.has(task.screen)) {
          seeded.add(task.screen);
          await setup.seed?.(context);
        }
        const mailUser = setup.mail?.[task.task];
        const roles: Roles = {
          signedIn: setup.persona === null ? undefined : personaOf(setup.persona),
          mail: mailUser === undefined ? undefined : personaOf(mailUser),
        };
        if (roles.mail !== undefined) {
          await clearMailbox(roles.mail);
        }
        // A sign-in of its own for every task: the client rotates the refresh token, so a storage state works once.
        let storageState: string | undefined;
        if (roles.signedIn !== undefined) {
          storageState = join(dir, "storage-state.json");
          await signIn(roles.signedIn, storageState);
        }
        const mcpArgs = mcpCommand({
          dir,
          startURL: demoServerURL + start,
          ...(storageState === undefined ? {} : { storageState }),
          ...(roles.mail === undefined ? {} : { mailURL: `${mailOrigin}/` }),
          executablePath,
          sandbox,
        });
        if (dryRun) {
          await probe(dir, mcpArgs, start, roles.mail !== undefined);
          result.tasks.push(emptyResult(task, start, "dry-run"));
          continue;
        }
        const transcript = parseTranscript(
          await runAgent(task, roles, dir, mcpArgs),
          roles.mail === undefined ? undefined : mailOrigin,
        );
        const failure = failureOf(transcript);
        if (failure !== null) {
          // Not a measure of the screen: recorded as `error` (no issue), and the run exits non-zero.
          throw new Error(failure);
        }
        const measures = measure(transcript.path, task.shortest, start);
        result.tasks.push({
          ...emptyResult(task, start, "measured"),
          ...measures,
          completed: transcript.completed,
          ending: transcript.ending,
          summary: transcript.summary,
          costUSD: transcript.costUSD,
          path: transcript.path,
          mailPath: transcript.mailPath,
        });
        console.log(
          `l4:   ${transcript.completed ? "done" : "not done"} in ${String(measures.steps)} step(s), lostness ${String(measures.lostness)}` +
            `${transcript.mailPath.length > 0 ? `, and ${String(transcript.mailPath.length)} step(s) in the mailbox` : ""}.`,
        );
      } catch (error) {
        failed = true;
        const reason = error instanceof Error ? error.message : String(error);
        console.error(`l4:   error: ${reason}`);
        result.tasks.push(emptyResult(task, start, "error", reason));
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
