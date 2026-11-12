// SPDX-License-Identifier: AGPL-3.0-only
// Renders stories to PNG files, so that an agent can look at a change to the UI
// (`scripts/task.sh web:shot -- <target> ...`). It needs no Docker and no Storybook build: it starts the Storybook
// dev server of app/web or app/ui on a free port, leaves it running for the next call (it stops itself after 30
// minutes without one), and opens each story's frame in the Chromium of Playwright.
//
//   node scripts/shot.ts <target> ... [--width 390,1440] [--theme light,dark] [--locale en,ja]
//   node scripts/shot.ts --changed     the stories of the features and components changed since origin/redesign-web
//   node scripts/shot.ts --stop        stops the servers
//
// A target is a story id (screens-admin-rules--normal), a story file (app/ui/src/Button.stories.tsx), a route of a
// screen (/admin/rules) or the screen's L1 file. A route, an L1 file and --changed shoot at most `cap` stories.
//
// It prints the path of each PNG, full page, .tmp/agentkit/shots/<story id>-<width>[-<theme>][-<locale>].png, and
// after a path one `overflow: <selector> (<what>)` line for each thing a look would find broken at that width: the
// page scrolls sideways, an element reaches past the viewport, or a box scrolls sideways. manifest.json beside the
// PNGs lists them ({"shots": [{"file", "story", "width", "theme", "locale", "takenAt"}]}, `file` from the
// repository's root); `agentkit pr open` reads it.
import { execFileSync, spawn } from "node:child_process";
import { existsSync, mkdirSync, openSync, readFileSync, rmSync, statSync, utimesSync, writeFileSync } from "node:fs";
import { createServer } from "node:net";
import { join, relative, resolve } from "node:path";
import { chromium, type Page } from "playwright";
import { readScreens, screensOf } from "../l4/budget.ts";
import { screensDir } from "../l4/tasks.ts";

const root = resolve(import.meta.dirname, "../../..");
const out = join(root, ".tmp/agentkit/shots");
/** The most stories of a route, an L1 file or --changed. */
const cap = 12;
/** How long a server waits for the next call before it stops, in ms. */
const idle = 30 * 60_000;

type Package = "web" | "ui";
type IndexEntry = { id: string; type: string; importPath: string };
type Shot = { file: string; story: string; width: number; theme: string; locale: string; takenAt: string };

class UsageError extends Error {}

const stateFile = (pkg: Package) => join(out, `server-${pkg}.json`);

// ---- the server ----

function freePort(): Promise<number> {
  return new Promise((done, fail) => {
    const server = createServer();
    server.once("error", fail);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      server.close(() => {
        if (address !== null && typeof address === "object") done(address.port);
        else fail(new Error("no free port"));
      });
    });
  });
}

/** `--serve <package> <port>`: runs the Storybook dev server until no call has used it for `idle`. */
function serve(pkg: Package, port: string): void {
  const child = spawn(
    join(root, "app", pkg, "node_modules/.bin/storybook"),
    ["dev", "--port", port, "--host", "127.0.0.1", "--exact-port", "--ci", "--no-version-updates"],
    { cwd: join(root, "app", pkg), stdio: "inherit" },
  );
  const stop = () => {
    child.kill();
    rmSync(stateFile(pkg), { force: true });
    process.exit(0);
  };
  child.once("exit", stop);
  process.once("SIGTERM", stop);
  setInterval(() => {
    // Each call touches the file; --stop removes it.
    if (!existsSync(stateFile(pkg)) || Date.now() - statSync(stateFile(pkg)).mtimeMs > idle) stop();
  }, 15_000);
}

async function index(origin: string): Promise<IndexEntry[] | undefined> {
  try {
    const response = await fetch(`${origin}/index.json`, { signal: AbortSignal.timeout(5_000) });
    if (!response.ok) return undefined;
    const { entries } = (await response.json()) as { entries: Record<string, IndexEntry> };
    return Object.values(entries).filter((entry) => entry.type === "story");
  } catch {
    return undefined;
  }
}

/** The origin and the stories of the package's server, starting it when none answers. */
async function server(pkg: Package): Promise<{ origin: string; stories: IndexEntry[] }> {
  if (existsSync(stateFile(pkg))) {
    const { port } = JSON.parse(readFileSync(stateFile(pkg), "utf8")) as { port: number };
    const origin = `http://127.0.0.1:${port}`;
    const stories = await index(origin);
    if (stories !== undefined) return { origin, stories };
  }
  if (!existsSync(join(root, "app/node_modules"))) throw new UsageError("run scripts/task.sh web:generate first");
  const port = await freePort();
  const log = join(out, `server-${pkg}.log`);
  const child = spawn(process.execPath, [import.meta.filename, "--serve", pkg, String(port)], {
    detached: true,
    stdio: ["ignore", openSync(log, "w"), openSync(log, "a")],
  });
  child.unref();
  writeFileSync(stateFile(pkg), JSON.stringify({ port, pid: child.pid }));
  const origin = `http://127.0.0.1:${port}`;
  for (const end = Date.now() + 180_000; Date.now() < end;) {
    const stories = await index(origin);
    if (stories !== undefined) return { origin, stories };
    if (child.exitCode !== null) break;
    await new Promise((done) => setTimeout(done, 300));
  }
  throw new UsageError(`the Storybook of app/${pkg} did not start: see ${relative(root, log)}`);
}

function stopServers(): void {
  for (const pkg of ["web", "ui"] as const) {
    if (!existsSync(stateFile(pkg))) continue;
    const { pid } = JSON.parse(readFileSync(stateFile(pkg), "utf8")) as { pid?: number };
    rmSync(stateFile(pkg));
    try {
      if (pid !== undefined) process.kill(pid, "SIGTERM");
    } catch {
      // It had stopped already.
    }
  }
}

// ---- the page ----

/** Waits until Storybook has rendered the story and run its play function; answers how it ended. */
async function storyDone(page: Page): Promise<string> {
  for (let attempt = 0; ; attempt++) {
    try {
      const phase = await page.waitForFunction(
        () => {
          const preview = (window as { __STORYBOOK_PREVIEW__?: { currentRender?: { phase?: string } } })
            .__STORYBOOK_PREVIEW__;
          const current = preview?.currentRender?.phase;
          return current === "finished" || current === "errored" || current === "aborted" ? current : undefined;
        },
        undefined,
        { timeout: 120_000 },
      );
      return (await phase.jsonValue()) ?? "aborted";
    } catch (error) {
      // A cold dev server reloads the page once it has optimized the dependencies the first story imports.
      const reloaded = error instanceof Error && /context was destroyed|navigation/i.test(error.message);
      if (!reloaded || attempt >= 5) throw error;
    }
  }
}

/** Waits until the fonts and the pictures have loaded, and every animation has ended. */
async function drawn(page: Page): Promise<void> {
  await page.evaluate(() => document.fonts.ready);
  await page
    .waitForFunction(
      () => [...document.images].filter((image) => image.checkVisibility()).every((image) => image.complete),
      undefined,
      { timeout: 15_000 },
    )
    .catch(() => undefined);
}

/**
 * Makes the page as tall as what it shows, for a full-page capture of a layout that scrolls inside a box (the app
 * shell): the viewport grows by what the tallest scrolling box hides.
 */
async function growToContent(page: Page, width: number, height: number): Promise<void> {
  const hidden = await page.evaluate(() => {
    let most = 0;
    for (const element of document.querySelectorAll("body *")) {
      const { overflowY } = getComputedStyle(element);
      if ((overflowY === "auto" || overflowY === "scroll") && element.clientHeight > window.innerHeight / 2)
        most = Math.max(most, element.scrollHeight - element.clientHeight);
    }
    return most;
  });
  if (hidden > 0) await page.setViewportSize({ width, height: Math.min(height + hidden, 12_000) });
}

/** What a look at the page would find broken sideways, as `<selector> (<what>)`. */
function overflows(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const name = (element: Element): string => {
      const parts: string[] = [];
      for (let at: Element | null = element; at !== null && at !== document.body && parts.length < 3;) {
        const label = at.id !== "" ? `#${at.id}` : (at.getAttribute("aria-label") ?? at.getAttribute("role"));
        const classes = [...at.classList].slice(0, 2).map((c) => `.${c}`);
        parts.unshift(
          at.tagName.toLowerCase() + (label === null ? classes.join("") : label.startsWith("#") ? label : `[${label}]`),
        );
        at = at.parentElement;
      }
      return parts.join(" > ");
    };
    const found: string[] = [];
    const page = document.documentElement;
    const width = page.clientWidth;
    if (page.scrollWidth > width) found.push(`html (the page scrolls sideways: ${page.scrollWidth} px of content)`);
    const outside: Element[] = [];
    for (const element of document.querySelectorAll("body *")) {
      if (!element.checkVisibility({ visibilityProperty: true })) continue;
      const box = element.getBoundingClientRect();
      if (box.width <= 1 || box.height <= 1) continue;
      const style = getComputedStyle(element);
      if (
        (style.overflowX === "auto" || style.overflowX === "scroll") &&
        element.scrollWidth > element.clientWidth + 1
      ) {
        found.push(`${name(element)} (the box scrolls sideways: ${element.scrollWidth} px in ${element.clientWidth})`);
      }
      // A focus ring or a hairline may sit a few px outside.
      if (box.right <= width + 3 && box.left >= -3) continue;
      // Inside a box that clips or scrolls, an element past the edge is not seen past it.
      let clipped = false;
      for (let at = element.parentElement; at !== null && at !== document.body; at = at.parentElement) {
        if (getComputedStyle(at).overflowX !== "visible") clipped = true;
      }
      if (!clipped && !outside.some((outer) => outer.contains(element))) {
        outside.push(element);
        found.push(`${name(element)} (reaches ${Math.round(box.left)}–${Math.round(box.right)} px of ${width})`);
      }
    }
    return found.slice(0, 8);
  });
}

// ---- the targets ----

type Target = { pkg: Package; origin: string; id: string };

const servers = new Map<Package, Promise<{ origin: string; stories: IndexEntry[] }>>();

function serverOf(pkg: Package) {
  const started = servers.get(pkg) ?? server(pkg);
  servers.set(pkg, started);
  return started;
}

/** The stories of the file `path` (from the repository's root), in the file's order. */
async function storiesOfFile(path: string): Promise<Target[]> {
  const pkg: Package = path.startsWith("app/ui/") ? "ui" : "web";
  const { origin, stories } = await serverOf(pkg);
  const importPath = `./${relative(join("app", pkg), path)}`;
  return stories.filter((story) => story.importPath === importPath).map(({ id }) => ({ pkg, origin, id }));
}

/** The path each screen story of app/web opens, by the story's id, read from the stories in the browser. */
async function storyPaths(origin: string): Promise<Map<string, string>> {
  const browser = await chromium.launch();
  try {
    const page = await browser.newPage();
    await page.goto(`${origin}/iframe.html?viewMode=story`);
    await page.waitForFunction(() => "__STORYBOOK_PREVIEW__" in window);
    const paths = await page.evaluate(async () => {
      type Story = { id: string; parameters?: { screen?: { path?: unknown } } };
      const preview = (
        window as unknown as { __STORYBOOK_PREVIEW__: { extract: () => Promise<Record<string, Story>> } }
      ).__STORYBOOK_PREVIEW__;
      return Object.values(await preview.extract()).flatMap((story): [string, string][] => {
        const path = story.parameters?.screen?.path;
        return typeof path === "string" ? [[story.id, path]] : [];
      });
    });
    return new Map(paths);
  } finally {
    await browser.close();
  }
}

/** The stories of the screens named `names` (L1 files), in the index's order. */
async function storiesOfScreens(names: ReadonlySet<string>): Promise<Target[]> {
  const { origin, stories } = await serverOf("web");
  const screens = readScreens(root);
  const paths = await storyPaths(origin);
  return stories
    .filter((story) => {
      const path = paths.get(story.id);
      return path !== undefined && screensOf(path, screens).some((screen) => names.has(screen.name));
    })
    .map(({ id }) => ({ pkg: "web" as const, origin, id }));
}

async function resolveTarget(target: string): Promise<{ stories: Target[]; capped: boolean }> {
  const file = [resolve(target), resolve(root, target)].find((path) => existsSync(path) && statSync(path).isFile());
  if (file !== undefined) {
    const path = relative(root, file);
    if (path.startsWith(`${screensDir}/`) && path.endsWith(".md")) {
      return { stories: await storiesOfScreens(new Set([path.slice(screensDir.length + 1, -3)])), capped: true };
    }
    return { stories: await storiesOfFile(path), capped: false };
  }
  if (target.startsWith("/")) {
    const names = new Set(screensOf(target, readScreens(root)).map((screen) => screen.name));
    if (names.size === 0) throw new UsageError(`no L1 file of ${screensDir} has the route ${target}`);
    return { stories: await storiesOfScreens(names), capped: true };
  }
  for (const pkg of target.startsWith("screens-") ? (["web", "ui"] as const) : (["ui", "web"] as const)) {
    const { origin, stories } = await serverOf(pkg);
    if (stories.some((story) => story.id === target)) return { stories: [{ pkg, origin, id: target }], capped: false };
  }
  throw new UsageError(`"${target}" is not a story id, a story file, a route of a screen or an L1 file`);
}

function git(...args: string[]): string[] {
  return execFileSync("git", args, { cwd: root, encoding: "utf8" }).split("\n").filter(Boolean);
}

/** The story files of the features of app/web and the components of app/ui changed since origin/redesign-web. */
function changedStoryFiles(): string[] {
  const [base = "HEAD"] = git("merge-base", "origin/redesign-web", "HEAD");
  const changed = [...git("diff", "--name-only", base), ...git("ls-files", "--others", "--exclude-standard")];
  const files = new Set<string>();
  for (const path of changed) {
    const feature = /^app\/web\/src\/features\/[^/]+\//.exec(path)?.[0];
    if (feature !== undefined) {
      for (const story of git("ls-files", "--cached", "--others", "--exclude-standard", `${feature}*.stories.tsx`))
        files.add(story);
    }
    const component = /^(app\/ui\/src\/[^/.]+)\.(stories\.)?tsx$/.exec(path)?.[1];
    if (component !== undefined && existsSync(join(root, `${component}.stories.tsx`)))
      files.add(`${component}.stories.tsx`);
  }
  return [...files].toSorted();
}

/** At most `cap` of the stories of `groups`, taking the groups in turn, and the ids left out. */
function capStories(groups: Target[][]): { picked: Target[]; skipped: string[] } {
  const all: Target[] = [];
  for (let round = 0; groups.some((group) => round < group.length); round++) {
    for (const group of groups) {
      const story = group[round];
      if (story !== undefined && !all.some((other) => other.id === story.id)) all.push(story);
    }
  }
  return { picked: all.slice(0, cap), skipped: all.slice(cap).map((story) => story.id) };
}

// ---- the run ----

function list(value: string | undefined, flag: string): string[] {
  if (value === undefined || value.startsWith("--")) throw new UsageError(`${flag} needs a value`);
  return value.split(",").filter(Boolean);
}

async function main(args: string[]): Promise<void> {
  let widths = [390, 1440];
  let themes = ["light"];
  let locales = ["en"];
  let changed = false;
  const targets: string[] = [];
  for (let i = 0; i < args.length; i++) {
    const arg = args[i] ?? "";
    if (arg === "--width") widths = list(args[++i], arg).map(Number);
    else if (arg === "--theme") themes = list(args[++i], arg);
    else if (arg === "--locale") locales = list(args[++i], arg);
    else if (arg === "--changed") changed = true;
    else if (arg === "--") continue;
    else if (arg.startsWith("--")) throw new UsageError(`unknown option ${arg}`);
    else targets.push(arg);
  }
  if (widths.some((width) => !Number.isInteger(width) || width < 200 || width > 4000))
    throw new UsageError("--width takes widths in CSS px, as 390,1440");
  if (themes.some((theme) => theme !== "light" && theme !== "dark")) throw new UsageError("--theme is light, dark");
  if (!changed && targets.length === 0)
    throw new UsageError(
      "usage: scripts/task.sh web:shot -- <story id | story file | route | L1 file> ... | --changed",
    );
  if (!existsSync(chromium.executablePath()))
    throw new UsageError("Playwright's Chromium is not installed: run scripts/task.sh ui:test-install");
  mkdirSync(out, { recursive: true });

  const stories: Target[] = [];
  const cappedGroups: Target[][] = [];
  for (const target of targets) {
    const found = await resolveTarget(target);
    if (found.stories.length === 0) throw new UsageError(`${target} has no story`);
    if (found.capped) cappedGroups.push(found.stories);
    else stories.push(...found.stories);
  }
  if (changed) {
    const files = changedStoryFiles();
    if (files.length === 0 && targets.length === 0) {
      console.error("web:shot: no feature of app/web and no component of app/ui changed since origin/redesign-web");
      return;
    }
    for (const file of files) cappedGroups.push(await storiesOfFile(file));
  }
  const { picked, skipped } = capStories(cappedGroups);
  if (skipped.length > 0)
    console.error(`web:shot: at most ${cap} stories; name these to shoot them: ${skipped.join(" ")}`);
  stories.push(...picked.filter((story) => !stories.some((other) => other.id === story.id)));

  const jobs = stories.flatMap((story) =>
    locales.flatMap((locale) => themes.flatMap((theme) => widths.map((width) => ({ story, locale, theme, width })))),
  );
  const results: { shot: Shot; lines: string[] }[] = [];
  const failures: string[] = [];
  const browser = await chromium.launch();
  try {
    let next = 0;
    const worker = async () => {
      for (let job = jobs[next++]; job !== undefined; job = jobs[next++]) {
        const { story, locale, theme, width } = job;
        const height = width < 600 ? 844 : 900;
        // A context of its own: the stories keep the session in localStorage.
        const context = await browser.newContext({
          viewport: { width, height },
          colorScheme: theme === "dark" ? "dark" : "light",
          reducedMotion: "reduce",
        });
        try {
          const page = await context.newPage();
          await page.goto(
            `${story.origin}/iframe.html?id=${story.id}&viewMode=story&globals=theme:${theme};locale:${locale}`,
          );
          const phase = await storyDone(page);
          if (phase !== "finished") failures.push(`${story.id} at ${width} px: the story ${phase}`);
          await drawn(page);
          const lines = await overflows(page);
          await growToContent(page, width, height);
          const name = `${story.id}-${width}${theme === "light" ? "" : `-${theme}`}${locale === "en" ? "" : `-${locale}`}.png`;
          await page.screenshot({ path: join(out, name), fullPage: true, animations: "disabled", caret: "hide" });
          results[jobs.indexOf(job)] = {
            shot: {
              file: relative(root, join(out, name)),
              story: story.id,
              width,
              theme,
              locale,
              takenAt: new Date().toISOString(),
            },
            lines: lines.map((line) => `overflow: ${line}`),
          };
        } catch (error) {
          failures.push(`${story.id} at ${width} px: ${error instanceof Error ? error.message : String(error)}`);
        } finally {
          await context.close();
        }
      }
    };
    await Promise.all(Array.from({ length: Math.min(4, jobs.length) }, worker));
  } finally {
    await browser.close();
    for (const pkg of servers.keys()) {
      if (existsSync(stateFile(pkg))) utimesSync(stateFile(pkg), new Date(), new Date());
    }
  }

  // The manifest keeps the shots of earlier calls whose files are still there.
  const manifest = join(out, "manifest.json");
  const taken = results.filter(Boolean).map((result) => result.shot);
  const earlier = existsSync(manifest) ? (JSON.parse(readFileSync(manifest, "utf8")) as { shots: Shot[] }).shots : [];
  const kept = earlier.filter(
    (shot) => existsSync(join(root, shot.file)) && !taken.some((other) => other.file === shot.file),
  );
  writeFileSync(manifest, `${JSON.stringify({ shots: [...kept, ...taken] }, null, 2)}\n`);
  for (const result of results.filter(Boolean)) {
    console.log(result.shot.file);
    for (const line of result.lines) console.log(line);
  }
  if (failures.length > 0) {
    console.error(`web:shot: ${failures.join("\nweb:shot: ")}`);
    process.exitCode = 1;
  }
}

const [mode, pkg, port] = process.argv.slice(2);
if (mode === "--serve" && (pkg === "web" || pkg === "ui") && port !== undefined) {
  serve(pkg, port);
} else if (mode === "--stop") {
  stopServers();
} else {
  try {
    await main(process.argv.slice(2));
  } catch (error) {
    if (!(error instanceof UsageError)) throw error;
    console.error(`web:shot: ${error.message}`);
    process.exitCode = 2;
  }
}
