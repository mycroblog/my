// SPDX-License-Identifier: AGPL-3.0-only

// Screenshots of the catalog page for the visual evaluator (ADR-0169 §2), which sees the rendered page and nothing of
// its source: `scripts/task.sh web:shot -- [--options <id>] [--landscape] [--out <dir>] [<url>]`. Each shot is a full
// page at the device viewports of @my/devtools/sizes (ADR-0169 §1), which the overlay's viewport control shows,
// without the overlay. They are as listed, portrait but for PC; with --landscape, each is in landscape (mobile
// 812×375). With --options, it takes one set
// per variant of that <Options>, so the evaluator can answer before the decider picks. The shots go to
// .tmp/visual/<id> of the main checkout, which outlives the worktree, for the agreement check to replay
// (scripts/visual-agreement.sh). It prints the path of each shot.

import { mkdir } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { parseArgs } from "node:util";
import { sizes } from "@my/devtools/sizes";
import { chromium, type Page } from "playwright";

const optionsAttribute = "data-my-devtools-options";

const run = (cmd: string[]) => {
  const p = Bun.spawnSync(cmd, { stderr: "inherit" });
  if (p.exitCode !== 0) throw new Error(`${cmd.join(" ")} exited ${p.exitCode}`);
  return p.stdout.toString().trim();
};

const repo = run(["git", "rev-parse", "--show-toplevel"]);
const main = dirname(run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"]));

const { values, positionals } = parseArgs({
  args: Bun.argv.slice(2),
  options: { options: { type: "string" }, landscape: { type: "boolean" }, out: { type: "string" } },
  allowPositionals: true,
});
const url = positionals[0] ?? run([join(repo, "scripts/agentkit.sh"), "feedback", "url"]);
const out = resolve(values.out ?? join(main, ".tmp/visual", values.options ?? "page"));
await mkdir(out, { recursive: true });

async function shoot(page: Page, name: string) {
  for (const [size, { width, height }] of Object.entries(sizes)) {
    const viewport = values.landscape
      ? { width: Math.max(width, height), height: Math.min(width, height) }
      : { width, height };
    await page.setViewportSize(viewport);
    await page.addStyleTag({ content: "[data-my-devtools] { display: none !important; }" });
    const path = join(out, `${name}-${size}${values.landscape ? "-landscape" : ""}.png`);
    await page.screenshot({ path, fullPage: true, animations: "disabled" });
    console.log(path);
  }
}

const browser = await chromium.launch();
try {
  const page = await browser.newPage();
  await page.goto(url, { waitUntil: "networkidle" });
  const id = values.options;
  if (id === undefined) {
    await shoot(page, "page");
  } else {
    const names = await page
      .locator(`[${optionsAttribute}="${id}"]`)
      .getAttribute("data-variants", { timeout: 5000 })
      .catch(() => null);
    if (names === null) throw new Error(`no <Options id="${id}"> on ${url}`);
    for (const variant of JSON.parse(names) as string[]) {
      await page.evaluate(
        ({ key, value }) => {
          sessionStorage.setItem(key, value);
        },
        { key: `my-devtools:option:${id}`, value: variant },
      );
      await page.reload({ waitUntil: "networkidle" });
      await shoot(page, variant);
    }
  }
} finally {
  await browser.close();
}
