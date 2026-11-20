// SPDX-License-Identifier: AGPL-3.0-only
// Spike #2906, not for main: the production build of web/app with Bun.build instead of Vite, to compare. It builds the
// three entries with code splitting, React Compiler (oxc-transform-react through the gate of vite-plugins/, or Bun's
// own `reactCompiler`), Tailwind through @tailwindcss/node, and writes a manifest in Vite's shape from Bun's metafile
// so that scripts/payment-scripts.ts, scripts/etags.ts and scripts/build-check.ts, and the Go server, can read it.
// Usage: bun spike/bun-build.ts <out dir> [--bun-compiler] [--min-chunk <bytes>]
import { mkdirSync, rmSync, writeFileSync, copyFileSync } from "node:fs";
import { basename, dirname, join, relative, resolve } from "node:path";
import { reactCompiler, stats, tailwind } from "./plugins-impl.ts";

const args = process.argv.slice(2);
const out = resolve(args[0] ?? "../../.tmp/bun-build");
const bunCompiler = args.includes("--bun-compiler");
const minChunk = args.includes("--min-chunk") ? Number(args[args.indexOf("--min-chunk") + 1]) : 0;
const here = dirname(new URL(import.meta.url).pathname);
const app = resolve(here, "..");

const entries = { "index.html": "src/main.tsx", "auth.html": "src/auth.tsx", "pay.html": "src/pay.tsx" } as const;

rmSync(out, { recursive: true, force: true });
mkdirSync(out, { recursive: true });
const started = performance.now();
const result = await Bun.build({
  entrypoints: Object.values(entries).map((e) => join(app, e)),
  outdir: out,
  root: app,
  target: "browser",
  format: "esm",
  splitting: true,
  minify: true,
  ...(minChunk > 0 ? { minChunkSize: minChunk } : {}),
  conditions: ["@mycroblog/source"],
  define: { "import.meta.env.PROD": "true", "process.env.NODE_ENV": '"production"' },
  loader: { ".woff2": "file", ".svg": "file", ".png": "file" },
  naming: {
    entry: "assets/[name]-[hash].[ext]",
    chunk: "assets/[name]-[hash].[ext]",
    asset: "assets/[name]-[hash].[ext]",
  },
  metafile: true,
  plugins: bunCompiler ? [tailwind] : [reactCompiler, tailwind],
  ...(bunCompiler ? { reactCompiler: true } : {}),
});
const ms = performance.now() - started;
if (!result.success) {
  for (const log of result.logs) console.error(log);
  process.exit(1);
}
if (stats.problems.length > 0)
  console.error(`React Compiler skipped ${stats.problems.length} function(s):\n${stats.problems.join("\n")}`);

// The manifest in Vite's shape (https://vite.dev/guide/backend-integration): an entry under its HTML's name, every
// other output under its own file.
const meta = result.metafile!;
// Bun's metafile names outputs, and the imports between them, relative to the outdir.
const fileOf = (p: string) => relative(out, resolve(out, p));
const keyOf = new Map<string, string>();
for (const [p, o] of Object.entries(meta.outputs)) {
  const entry = o.entryPoint && Object.entries(entries).find(([, src]) => resolve(o.entryPoint!) === join(app, src));
  keyOf.set(p, entry ? entry[0] : `_${basename(p)}`);
}
const manifest: Record<string, unknown> = {};
for (const [p, o] of Object.entries(meta.outputs)) {
  if (!p.endsWith(".js")) continue;
  const key = keyOf.get(p)!;
  const imports = o.imports.filter((i) => i.kind === "import-statement" && i.path.endsWith(".js"));
  const lazy = o.imports.filter((i) => i.kind === "dynamic-import");
  const resolveImport = (i: { path: string }) => {
    const hit = [...keyOf.keys()].find((k) => fileOf(k) === fileOf(i.path));
    if (hit === undefined) throw new Error(`${p} imports ${i.path}, which is no output`);
    return keyOf.get(hit)!;
  };
  manifest[key] = {
    file: fileOf(p),
    ...(key.endsWith(".html") ? { isEntry: true, src: key } : {}),
    ...(imports.length ? { imports: imports.map(resolveImport) } : {}),
    ...(lazy.length ? { dynamicImports: lazy.map(resolveImport) } : {}),
    ...(o.cssBundle ? { css: [fileOf(o.cssBundle)] } : {}),
  };
}
mkdirSync(join(out, ".vite"), { recursive: true });
writeFileSync(join(out, ".vite/manifest.json"), `${JSON.stringify(manifest, null, 2)}\n`);
writeFileSync(join(out, ".vite/bun-metafile.json"), JSON.stringify(meta));
mkdirSync(join(out, "licenses"), { recursive: true });
for (const f of [
  "../app-features/src/emoji/unicode/LICENSE-Unicode.txt",
  "../design-system/src/fonts/emoji/OFL.txt",
  "../design-system/src/fonts/cjk/OFL-NotoSansCJK.txt",
  "../design-system/src/fonts/noto-sans/OFL-NotoSans.txt",
])
  copyFileSync(join(app, f), join(out, "licenses", basename(f)));

const js = result.outputs.filter((o) => o.path.endsWith(".js"));
console.log(
  JSON.stringify({
    ms: Math.round(ms),
    compiler: bunCompiler ? "bun" : "oxc-transform-react",
    compiledFunctions: bunCompiler ? null : stats.compiled,
    problems: stats.problems.length,
    outputs: result.outputs.length,
    jsFiles: js.length,
    jsBytes: js.reduce((s, o) => s + o.size, 0),
    largestJs: Math.max(...js.map((o) => o.size)),
  }),
);
