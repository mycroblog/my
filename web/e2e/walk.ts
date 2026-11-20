// SPDX-License-Identifier: AGPL-3.0-only

// The walker's browser (ADR-0169 §2): one headless Chromium that stays open between calls, so that an agent walks the
// app one step a call as a person would, and sees after each step what a person sees: the address, the page's
// accessible tree (the roles and names it acts on) and a shot of the viewport.
//
//   scripts/task.sh web:walk -- open <url> [--viewport mobile|tablet|pc] [--lang <locale>]   start at <url>
//   scripts/task.sh web:walk -- look | back | scroll down|up | press <key> | goto <url>
//   scripts/task.sh web:walk -- click <role> <name> [--nth <n>]          the <n>th (from 1) of that role and name
//   scripts/task.sh web:walk -- fill <role> <name> <text> [--nth <n>]
//   scripts/task.sh web:walk -- close                                       stop the browser and remove its profile
//
// The browser and its shots are in .tmp/walk/ of the main checkout, which outlives the worktree; it prints the path
// of each shot, and an open removes the shots of the walk before. A step that finds nothing to act on says so and changes nothing, as a press beside a button would.

import { spawn } from "node:child_process";
import { existsSync } from "node:fs";
import { mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { parseArgs } from "node:util";
import { sizes } from "@my/devtools/sizes";
import { chromium, type Page } from "@playwright/test";

const git = (...args: string[]) => {
  const p = Bun.spawnSync(["git", ...args], { stderr: "inherit" });
  if (p.exitCode !== 0) throw new Error(`git ${args.join(" ")} exited ${p.exitCode}`);
  return p.stdout.toString().trim();
};
const dir = join(dirname(git("rev-parse", "--path-format=absolute", "--git-common-dir")), ".tmp/walk");
const stateFile = join(dir, "browser.json");
const profile = join(dir, "profile");
type State = { pid: number; port: number; shots: number };

// The accessible tree of a long page is cut in its middle, a long list's, so that what comes after the list (the
// bars and the buttons that float over the page) still shows; the shot shows what is in view.
const treeHead = 200;
const treeTail = 80;

const { values, positionals } = parseArgs({
  args: Bun.argv.slice(2),
  options: {
    viewport: { type: "string", default: "mobile" },
    lang: { type: "string", default: "en" },
    nth: { type: "string", default: "1" },
  },
  allowPositionals: true,
});
const [step = "look", ...rest] = positionals;

const readState = async (): Promise<State> => {
  if (!existsSync(stateFile)) throw new Error("no walk is open: start one with `web:walk -- open <url>`");
  return JSON.parse(await readFile(stateFile, "utf8")) as State;
};

/** Starts Chromium detached with a debugging port, which it writes to DevToolsActivePort once it listens. */
async function launch(viewport: { width: number; height: number }, lang: string): Promise<State> {
  await rm(dir, { recursive: true, force: true });
  await mkdir(join(dir, "shots"), { recursive: true });
  const child = spawn(
    chromium.executablePath(),
    [
      "--headless=new",
      "--remote-debugging-port=0",
      `--user-data-dir=${profile}`,
      `--window-size=${viewport.width},${viewport.height}`,
      "--no-first-run",
      "--no-default-browser-check",
      `--lang=${lang}`,
      `--accept-lang=${lang}`,
      "about:blank",
    ],
    { detached: true, stdio: "ignore", env: { ...process.env, LANG: lang, LANGUAGE: lang } },
  );
  child.unref();
  const portFile = join(profile, "DevToolsActivePort");
  for (let i = 0; i < 100 && !existsSync(portFile); i++) await Bun.sleep(100);
  const [port = ""] = (await readFile(portFile, "utf8")).split("\n");
  const state = { pid: child.pid ?? 0, port: Number(port), shots: 0 };
  await writeFile(stateFile, JSON.stringify(state));
  return state;
}

/** Lets the page settle after a step: its document loaded, then a moment for what it fetches and draws. */
async function settle(page: Page) {
  await page.waitForLoadState("domcontentloaded").catch(() => undefined);
  await page.waitForTimeout(800);
}

async function report(page: Page, state: State) {
  const tree = (await page.ariaSnapshot()).split("\n");
  state.shots += 1;
  const shot = join(dir, "shots", `${String(state.shots).padStart(3, "0")}.png`);
  await page.screenshot({ path: shot });
  await writeFile(stateFile, JSON.stringify(state));
  console.log(
    [
      `url: ${page.url()}`,
      `title: ${await page.title()}`,
      `shot: ${shot}`,
      "page:",
      ...(tree.length > treeHead + treeTail
        ? [
            ...tree.slice(0, treeHead),
            `… ${tree.length - treeHead - treeTail} lines left out here`,
            ...tree.slice(-treeTail),
          ]
        : tree),
    ].join("\n"),
  );
}

if (step === "close") {
  if (existsSync(stateFile)) {
    const { pid } = await readState();
    try {
      process.kill(pid);
    } catch {
      // Already gone.
    }
    await Bun.sleep(300);
  }
  await rm(profile, { recursive: true, force: true });
  await rm(stateFile, { force: true });
  console.log(`walk: closed; its shots stay in ${join(dir, "shots")} until the next open`);
  process.exit(0);
}

let state: State;
if (step === "open") {
  if (!Object.hasOwn(sizes, values.viewport)) throw new Error(`--viewport ${values.viewport}: mobile, tablet or pc`);
  const viewport = sizes[values.viewport as keyof typeof sizes];
  if (existsSync(stateFile)) {
    try {
      process.kill((await readState()).pid);
    } catch {
      // Already gone.
    }
  }
  state = await launch(viewport, values.lang);
} else {
  state = await readState();
}

const browser = await chromium.connectOverCDP(`http://127.0.0.1:${state.port}`);
const context = browser.contexts()[0];
if (context === undefined) throw new Error("the walk's browser has no window: open it again");
const page = context.pages().at(-1) ?? (await context.newPage());
const nth = Math.max(1, Number(values.nth)) - 1;
/** The element of that role and name the step acts on: the nth of those the page shows, as a reader counts them. */
async function target(role: string, name: string) {
  const all = await page.getByRole(role as Parameters<Page["getByRole"]>[0], { name, exact: true }).all();
  const found = all[nth];
  if (found === undefined) throw new Error(`no ${role} "${name}" (${all.length} on the page)`);
  return found;
}

try {
  switch (step) {
    case "open":
    case "goto":
      await page.goto(rest[0] ?? "about:blank");
      break;
    case "look":
      break;
    case "back":
      await page.goBack();
      break;
    case "scroll":
      await page.mouse.wheel(0, (rest[0] === "up" ? -1 : 1) * Math.round((page.viewportSize()?.height ?? 600) * 0.8));
      break;
    case "press":
      await page.keyboard.press(rest[0] ?? "Enter");
      break;
    case "click":
      await (await target(rest[0] ?? "", rest[1] ?? "")).click({ timeout: 3000 });
      break;
    case "fill":
      await (await target(rest[0] ?? "", rest[1] ?? "")).fill(rest[2] ?? "", { timeout: 3000 });
      break;
    default:
      throw new Error(`${step}: open, look, click, fill, press, scroll, back, goto or close`);
  }
} catch (error) {
  console.log(`step failed, nothing changed: ${(error as Error).message.split("\n")[0]}`);
}
await settle(page);
await report(page, state);
// Leaves the browser open: closing a browser reached over CDP only disconnects from it.
await browser.close();
