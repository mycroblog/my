// SPDX-License-Identifier: AGPL-3.0-only
// Fails when an installed npm package has a license outside an allowlist: one compatible with AGPL-3.0-only
// (ADR-0011), or a permissive one for the MIT libraries (ADR-0093 §2).
//
//   bun run src/licenses.ts MIT,Apache-2.0,... [node_modules ...]
//   bun run src/licenses.ts MIT,Apache-2.0,... --deps-of PACKAGE_DIR
//
// The first argument is the allowed SPDX identifiers (i18n:licenses and web:licenses pass
// NPM_ALLOWED_LICENSES of Taskfile.yml, mykit:ts-licenses MYKIT_ALLOWED_LICENSES of tasks/mykit.yml).
// The rest are the node_modules directories to check, tools/i18n's own when none is given. Both
// layouts Bun installs are covered: hoisted packages and the isolated store in node_modules/.bun that
// the client workspace uses (app/bunfig.toml). Symlinks are skipped: in the isolated layout they point
// into the store, and workspace packages are our own code.
//
// --deps-of checks the packages that a library brings to its users instead: the dependencies,
// optional and peer dependencies of PACKAGE_DIR's package.json, and theirs, resolved as Node.js does
// (through the symlinks of the isolated layout), but not its devDependencies.
//
// SPDX expressions are evaluated: "A OR B" needs one allowed side, "A AND B" needs both.
import { existsSync, readdirSync, readFileSync, realpathSync } from "node:fs";
import { basename, dirname, join, resolve } from "node:path";

const allowed = new Set((process.argv[2] ?? "").split(",").filter(Boolean));
if (allowed.size === 0) {
  console.error("usage: bun run src/licenses.ts <comma-separated SPDX identifiers> [node_modules ... | --deps-of DIR]");
  process.exit(2);
}

/**
 * Reviewed packages whose license field is not an SPDX identifier, pinned by version so an
 * upgrade is reviewed again.
 */
const reviewed: Record<string, string> = {
  // Douglas Crockford's JSON parser, dedicated to the public domain; pulled in by
  // @formatjs/cli-lib through json-stable-stringify.
  "jsonify@0.0.1": "Public Domain",
  // Browser support data that browserslist reads at build time (pulled in by @babel/core through
  // @babel/helper-compilation-targets); none of it is bundled into the client. The FSF lists
  // CC BY 4.0 as compatible with GPLv3, and so with AGPLv3.
  "caniuse-lite@1.0.30001814": "CC-BY-4.0",
  // The same data, in the version that MJML pulls in through cssnano (app/mail; compiles the email layout at
  // development time, never shipped).
  "caniuse-lite@1.0.30001815": "CC-BY-4.0",
  // The CSS selector parser of juice, MJML's CSS inliner (app/mail; development time only). Its license field is
  // "MIT (http://mootools.net/license.txt)", the MIT license with a link to its text.
  "slick@1.12.2": "MIT (http://mootools.net/license.txt)",
  // Glob matcher that the lint's ESLint plugins pull in through their peers eslint and
  // @typescript-eslint/typescript-estree (app/ lint only; never bundled into the client). The Blue
  // Oak Model License 1.0.0 is an OSI-approved permissive license with only a notice condition, so it
  // imposes nothing that conflicts with AGPL-3.0.
  "minimatch@10.2.6": "BlueOak-1.0.0",
  // The YAML loader's argument parser (js-yaml, through @redocly/openapi-core), which openapi-typescript
  // runs in `scripts/task.sh api:generate` (app/web devDependency; never bundled into the client). The FSF lists the
  // Python license of 2.0.1 and later as compatible with the GPL, and so with AGPLv3.
  "argparse@2.0.1": "Python-2.0",
};

type Manifest = {
  name?: string;
  version?: string;
  license?: unknown;
  licenses?: unknown;
  dependencies?: Record<string, string>;
  optionalDependencies?: Record<string, string>;
  peerDependencies?: Record<string, string>;
};

function licenseOf(m: Manifest): string | undefined {
  const one = (l: unknown) => (typeof l === "string" ? l : (l as { type?: string } | null)?.type);
  if (m.license !== undefined) return one(m.license);
  if (Array.isArray(m.licenses)) return m.licenses.map(one).join(" OR ");
}

/** Evaluates an SPDX license expression (without WITH exceptions) against the allowlist. */
function permitted(expr: string): boolean {
  const e = expr.trim().replace(/^\((.*)\)$/s, "$1");
  if (/\sOR\s/.test(e)) return e.split(/\s+OR\s+/).some(permitted);
  if (/\sAND\s/.test(e)) return e.split(/\s+AND\s+/).every(permitted);
  return allowed.has(e);
}

const failures: string[] = [];
/** name@version of every package checked, so a package installed in several places counts once. */
const seen = new Set<string>();

/** Checks the license of one package; it reports whether the package was new. */
function check(m: Manifest): boolean {
  const id = `${m.name}@${m.version}`;
  if (seen.has(id)) return false;
  seen.add(id);
  const license = licenseOf(m);
  const ok = license !== undefined && (permitted(license) || reviewed[id] === license);
  if (!ok) failures.push(`${id}: ${license ?? "no license"}`);
  return true;
}

/** The directory of the package `name` that the package in `from` imports, as Node.js resolves it. */
function resolvePackage(from: string, name: string): string | undefined {
  for (let dir = from; ; dir = dirname(dir)) {
    const candidate = basename(dir) === "node_modules" ? join(dir, name) : join(dir, "node_modules", name);
    if (existsSync(join(candidate, "package.json"))) return realpathSync(candidate);
    if (dirname(dir) === dir) return undefined;
  }
}

/** Checks the packages that the package in `dir` depends on at run time, and theirs. */
function walkDeps(dir: string) {
  const m: Manifest = JSON.parse(readFileSync(join(dir, "package.json"), "utf8"));
  const deps = { ...m.peerDependencies, ...m.optionalDependencies, ...m.dependencies };
  for (const name of Object.keys(deps)) {
    const pkg = resolvePackage(dir, name);
    if (pkg === undefined) {
      // An optional or peer dependency need not be installed.
      if (m.dependencies?.[name] !== undefined) failures.push(`${name}: not installed (a dependency of ${m.name})`);
      continue;
    }
    if (check(JSON.parse(readFileSync(join(pkg, "package.json"), "utf8")))) walkDeps(pkg);
  }
}
function walk(dir: string) {
  if (!existsSync(dir)) return;
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;
    if (entry.name === ".bun") {
      // Isolated store: .bun/<name>@<version>[+<hash>]/node_modules/<name>
      const store = join(dir, entry.name);
      for (const pkg of readdirSync(store, { withFileTypes: true })) {
        if (pkg.isDirectory() && pkg.name !== "node_modules") walk(join(store, pkg.name, "node_modules"));
      }
      continue;
    }
    if (entry.name.startsWith(".")) continue;
    const pkg = join(dir, entry.name);
    if (entry.name.startsWith("@")) {
      walk(pkg);
      continue;
    }
    const manifest = join(pkg, "package.json");
    if (existsSync(manifest)) check(JSON.parse(readFileSync(manifest, "utf8")));
    walk(join(pkg, "node_modules"));
  }
}

if (process.argv[3] === "--deps-of") {
  const dir = process.argv[4];
  if (dir === undefined || !existsSync(join(dir, "package.json"))) {
    console.error("error: --deps-of needs a package directory; install the dependencies first");
    process.exit(2);
  }
  walkDeps(realpathSync(dir));
}
const roots =
  process.argv[3] === "--deps-of"
    ? []
    : process.argv.length > 3
      ? process.argv.slice(3)
      : [join(import.meta.dir, "..", "node_modules")];
for (const root of roots) {
  if (!existsSync(root)) {
    console.error(`error: ${root} does not exist; install the dependencies first`);
    process.exit(2);
  }
  walk(resolve(root));
}
for (const f of failures) console.error(`error: incompatible npm license: ${f}`);
console.log(`licenses: ${seen.size} npm package(s) checked, ${failures.length} incompatible`);
if (failures.length > 0) process.exit(1);
