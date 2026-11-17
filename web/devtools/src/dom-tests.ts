#!/usr/bin/env bun
// SPDX-License-Identifier: AGPL-3.0-only

// Runs the component tests of one DOM of the package it is called in (ADR-0171 §4), side by side: `bun test` runs the
// files of a process one after another, so this deals the files to several processes, each with the DOM's preload
// (happy-dom.ts, jsdom.ts). A test file shares nothing with another: each renders on a fake API of its own. A
// process's output is printed whole, when it and the ones before it have ended.
//
//   dom-tests happy-dom|jsdom [an option of bun test]...
//
// There are as many processes as cores, and one package's at a time in the workspace (web:test runs the packages side
// by side): a process that waits for a core is slow to import the package, and a test that waits a second for what it
// imports then fails.
//
// A package calls it from its `test` script and declares @my/devtools, where scripts/changed-web-packages.sh reads
// that a change to a preload reaches the package's tests.

import { Database } from "bun:sqlite";
import { existsSync, mkdirSync } from "node:fs";
import { availableParallelism } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

/** The files a process runs at least: it pays the preload and the package's imports once, about a second. */
const perProcess = 4;

/** The suffix of the test files of each DOM. */
const suffixes: Record<string, string> = { "happy-dom": ".dom.test.tsx", jsdom: ".jsdom.test.tsx" };

/** `files` dealt in turn to `count` lists at most, each of `perProcess` files or more while there are that many. */
export function deal(files: string[], count: number): string[][] {
  const length = Math.max(1, Math.min(count, Math.floor(files.length / perProcess)));
  const lists = Array.from({ length: Math.min(length, files.length) }, (): string[] => []);
  files.forEach((file, i) => lists[i % lists.length]?.push(file));
  return lists;
}

/** The directory of the workspace the package is in, which holds bun.lock; nothing outside one. */
function workspace(dir: string): string | undefined {
  if (existsSync(join(dir, "bun.lock"))) return dir;
  return dirname(dir) === dir ? undefined : workspace(dirname(dir));
}

/**
 * Waits for the other packages' runs in the workspace to end, and holds the others off until `close()` or the end of
 * this process: an exclusive lock on a database file, which SQLite waits for and the system frees with the process.
 */
function alone(): { close(): void } {
  const root = workspace(process.cwd());
  if (root === undefined) return { close: () => {} };
  mkdirSync(join(root, "node_modules/.cache"), { recursive: true });
  const lock = new Database(join(root, "node_modules/.cache/dom-tests.lock"));
  lock.run("PRAGMA busy_timeout = 3600000");
  lock.run("BEGIN EXCLUSIVE");
  return lock;
}

if (import.meta.main) {
  const [dom = "", ...options] = process.argv.slice(2);
  const suffix = suffixes[dom];
  if (suffix === undefined) {
    console.error("usage: dom-tests happy-dom|jsdom [an option of bun test]...");
    process.exit(2);
  }
  const files = [...new Bun.Glob(`**/*${suffix}`).scanSync(".")].filter((f) => !f.includes("node_modules/")).sort();
  const preload = fileURLToPath(new URL(`./${dom}.ts`, import.meta.url));
  const lock = alone();
  const runs = deal(files, availableParallelism()).map((list) =>
    Bun.spawn([process.execPath, "test", ...options, "--preload", preload, ...list.map((f) => `./${f}`)], {
      stdout: "pipe",
      stderr: "pipe",
    }),
  );
  let failed = false;
  for (const run of runs) {
    const [out, err, code] = await Promise.all([
      new Response(run.stdout).text(),
      new Response(run.stderr).text(),
      run.exited,
    ]);
    process.stdout.write(out);
    process.stderr.write(err);
    if (code !== 0) failed = true;
  }
  lock.close();
  process.exit(failed ? 1 : 0);
}
