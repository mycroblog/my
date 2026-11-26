// SPDX-License-Identifier: AGPL-3.0-only

// What the lab's tests (test.ts) and its reporter (reporter.ts) share.

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { gunzipSync } from "node:zlib";
import type { Manifest } from "../src/ux/bundle";
import { root } from "../src/server/paths";

/**
 * The CPU throttling of each page (Chromium's Emulation.setCPUThrottlingRate): four times slower, the slowdown
 * Lighthouse emulates a mid-range phone with on a desktop, so that a page that is slow on a phone is slow here.
 */
export const cpuThrottling = 4;

/** The build that the test build of bin/my embeds, whose Vite manifest names the entries. */
export const buildDir = join(root, "internal/interface/web/dist");

/**
 * The text file at `path` of the build, as Vite wrote it: the build keeps each text file only compressed, and each as
 * `<file>.gz` (web/app/scripts/compress.ts).
 */
export function readBuild(path: string): Buffer {
  return gunzipSync(readFileSync(join(buildDir, `${path}.gz`)));
}

/** The build's Vite manifest. */
export function buildManifest(): Manifest {
  return JSON.parse(readBuild(".vite/manifest.json").toString("utf8")) as Manifest;
}
