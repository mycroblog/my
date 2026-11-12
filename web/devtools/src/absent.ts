// SPDX-License-Identifier: AGPL-3.0-only

// Fails when a production build contains @my/devtools (ADR-0168 §3): every module of the package names its endpoint
// or its attribute, which contain "my-devtools", and no other code of the workspace does.
//
//   bun src/absent.ts <build directory>...

import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

const marker = "my-devtools";

function files(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((e) =>
    e.isDirectory() ? files(join(dir, e.name)) : [join(dir, e.name)],
  );
}

/** The files of the build directories that contain @my/devtools; throws on a directory that holds no file. */
export function findDevtools(dirs: string[]): string[] {
  return dirs.flatMap((dir) => {
    const all = statSync(dir).isDirectory() ? files(dir) : [];
    if (all.length === 0) throw new Error(`${dir} holds no build`);
    return all.filter((f) => readFileSync(f, "latin1").includes(marker)).map((f) => relative(process.cwd(), f));
  });
}

if (import.meta.main) {
  const dirs = process.argv.slice(2);
  if (dirs.length === 0) {
    console.error("usage: bun src/absent.ts <build directory>...");
    process.exit(2);
  }
  const found = findDevtools(dirs);
  if (found.length > 0) {
    console.error(`@my/devtools is in a production build, which only the dev server may load (ADR-0168 §3):`);
    for (const f of found) console.error(`  ${f}`);
    process.exit(1);
  }
  console.log(`@my/devtools is in none of ${dirs.join(", ")}`);
}
