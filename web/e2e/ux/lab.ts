// SPDX-License-Identifier: AGPL-3.0-only

// What the lab's tests (test.ts) and its reporter (reporter.ts) share.

import { join } from "node:path";
import { root } from "../src/server/paths";

/**
 * The CPU throttling of each page (Chromium's Emulation.setCPUThrottlingRate): four times slower, the slowdown
 * Lighthouse emulates a mid-range phone with on a desktop, so that a page that is slow on a phone is slow here.
 */
export const cpuThrottling = 4;

/** The build that the test build of bin/my embeds, whose Vite manifest names the entries. */
export const buildDir = join(root, "internal/interface/web/dist");
