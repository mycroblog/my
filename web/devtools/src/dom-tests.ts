#!/usr/bin/env bun
// SPDX-License-Identifier: AGPL-3.0-only

// Runs the component tests of one DOM of the package it is called in (ADR-0171 §4), side by side: `bun test` runs the
// files of a process one after another, so this deals the files to several processes, each with the DOM's preload
// (happy-dom.ts, jsdom.ts). A test file shares nothing with another: each renders on a fake API of its own. A
// process's output is printed whole, when it and the ones before it have ended.
//
//   dom-tests happy-dom|jsdom [an option of bun test]...
//
// There are as many processes as cores, among every package whose run goes beside this one: web:test runs the packages
// side by side and says how many of them run tests on a DOM (DOM_TESTS_BESIDE), and each takes its share of the cores,
// one process at least. A package run alone takes them all. With a process per core and package, a test's own work
// waited for a core four times as long as it took, and passed the limit of a test.
//
// A package calls it from its `test` script and declares @my/devtools, where scripts/changed-web-packages.sh reads
// that a change to a preload reaches the package's tests.

import { availableParallelism } from "node:os";
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

/** The processes of a run: its share of `cores` among the `beside` packages that run side by side, one at least. */
export const share = (cores: number, beside: number): number =>
  Math.max(1, Math.floor(cores / Math.max(1, Number.isFinite(beside) ? beside : 1)));

if (import.meta.main) {
  const [dom = "", ...options] = process.argv.slice(2);
  const suffix = suffixes[dom];
  if (suffix === undefined) {
    console.error("usage: dom-tests happy-dom|jsdom [an option of bun test]...");
    process.exit(2);
  }
  const files = [...new Bun.Glob(`**/*${suffix}`).scanSync(".")].filter((f) => !f.includes("node_modules/")).sort();
  const preload = fileURLToPath(new URL(`./${dom}.ts`, import.meta.url));
  const processes = share(availableParallelism(), Number(process.env["DOM_TESTS_BESIDE"] ?? 1));
  const runs = deal(files, processes).map((list) =>
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
  process.exit(failed ? 1 : 0);
}
