// SPDX-License-Identifier: AGPL-3.0-only
// Spike #2906, not for main: the Bun plugins of the spike's build and dev server: React Compiler through the gate of
// vite-plugins/react-compiler.ts, and Tailwind v4 through @tailwindcss/node as @tailwindcss/vite runs it.
import { dirname } from "node:path";
import { compile } from "@tailwindcss/node";
import { Scanner } from "@tailwindcss/oxide";
import type { BunPlugin } from "bun";
import { compileModule } from "../vite-plugins/react-compiler.ts";

/** The same filters as vite-plugins/react-compiler.ts. */
const codeFilter = /forwardRef|memo|\b(?:[A-Z]|use[A-Z0-9])/;
export const stats = { compiled: 0, problems: [] as string[] };

export const reactCompiler: BunPlugin = {
  name: "my:react-compiler",
  setup(build) {
    build.onLoad({ filter: /\.[tj]sx?$/ }, async ({ path }) => {
      if (path.includes("/node_modules/")) return undefined;
      const code = await Bun.file(path).text();
      if (!codeFilter.test(code)) return undefined;
      const result = await compileModule(path, code, false);
      stats.compiled += result.compiled;
      stats.problems.push(...result.problems);
      return { contents: result.code, loader: path.endsWith("x") ? "jsx" : "js" };
    });
  },
};

/** Tailwind v4 the way @tailwindcss/vite runs it, on a stylesheet that uses Tailwind's at-rules. */
export const tailwind: BunPlugin = {
  name: "my:tailwind",
  setup(build) {
    build.onLoad({ filter: /\.css$/ }, async ({ path }) => {
      const css = await Bun.file(path).text();
      if (!/@import\s+["']tailwindcss|@source|@plugin|@theme|@apply|@utility/.test(css)) return undefined;
      const compiler = await compile(css, { base: dirname(path), from: path, onDependency: () => {} });
      const root =
        compiler.root === "none"
          ? []
          : compiler.root === null
            ? [{ base: dirname(path), pattern: "**/*", negated: false }]
            : [{ ...compiler.root, negated: false }];
      const scanner = new Scanner({ sources: [...root, ...compiler.sources] });
      return { contents: compiler.build(scanner.scan()), loader: "css" };
    });
  },
};
