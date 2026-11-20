// SPDX-License-Identifier: AGPL-3.0-only

// The test of the lab's scenarios (ADR-0167 §4): the E2E tests' own (specs/test.ts), against the same demo server and
// its scenario, whose page runs with its CPU throttled and the probe of src/ux/probe.ts in each document it loads.
// When a test ends it attaches what the probe heard, by the HTML entry of each document, as `ux-runs`, which the
// reporter (reporter.ts) adds up. A scenario is a test; its title names it in the report.

import { readFileSync } from "node:fs";
import { join } from "node:path";
import type { Manifest } from "../src/ux/bundle";
import { entryOfScripts, runsOf, type EntryOf, type Sample } from "../src/ux/measures";
import { probeBinding } from "../src/ux/probe";
import { buildDir, cpuThrottling } from "./lab";
import { test as base } from "../specs/test";

export { expect, person } from "../specs/test";

let probeScript: Promise<string> | undefined;

/** The probe, built into one script for the browser. */
function probe(): Promise<string> {
  probeScript ??= (async () => {
    const built = await Bun.build({
      entrypoints: [join(import.meta.dirname, "../src/ux/probe.ts")],
      target: "browser",
      format: "iife",
      conditions: ["@mycroblog/source"],
    });
    const output = built.outputs[0];
    if (!built.success || output === undefined)
      throw new Error(`ux: the probe does not build: ${built.logs.map((log) => log.message).join("\n")}`);
    return output.text();
  })();
  return probeScript;
}

let entries: EntryOf | undefined;

function entryOf(): EntryOf {
  entries ??= entryOfScripts(JSON.parse(readFileSync(join(buildDir, ".vite/manifest.json"), "utf8")) as Manifest);
  return entries;
}

export const test = base.extend<{ ux: undefined }>({
  ux: [
    async ({ page }, use, testInfo) => {
      const samples: Sample[] = [];
      await page.exposeBinding(probeBinding, (_source, sample: Sample) => {
        samples.push(sample);
      });
      await page.addInitScript({ content: await probe() });
      const cdp = await page.context().newCDPSession(page);
      await cdp.send("Emulation.setCPUThrottlingRate", { rate: cpuThrottling });
      await use(undefined);
      // The last document's largest paint and what its observers hold, after a frame has painted what the last
      // interaction changed. A page that is already gone has sent them as it went.
      await page
        .evaluate(async () => {
          await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
          (window as Window & { myUxSettle?: () => void }).myUxSettle?.();
        })
        .catch(() => undefined);
      await testInfo.attach("ux-runs", {
        body: JSON.stringify(runsOf(testInfo.title, samples, entryOf())),
        contentType: "application/json",
      });
    },
    { auto: true },
  ],
});
