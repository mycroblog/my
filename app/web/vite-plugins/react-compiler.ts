// SPDX-License-Identifier: AGPL-3.0-only
// React Compiler for the client build (ADR-0065): runs oxc-transform-react, Oxc's port of the compiler, on the
// client environment's modules and holds the gate of web-client/README.md R4, which fails `vite build` when the
// compiler reports a diagnostic outside a function that opts out, or when it compiled no function.
import { transform } from "oxc-transform-react";
import { type Plugin, parseSync } from "vite";

/** The files the compiler may change: @vitejs/plugin-react's id filter and code filter. */
const idFilter = { include: /\.[tj]sx?(\?.*)?$/, exclude: /\/node_modules\// };
const codeFilter = /forwardRef|memo|\b(?:[A-Z]|use[A-Z0-9])/;

/** The directives the compiler treats as an opt-out. */
const optOut = /^use no (memo|forget)$/;

interface AstNode {
  type: string;
  start: number;
  end: number;
  [key: string]: unknown;
}

const isNode = (value: unknown): value is AstNode =>
  value != null && typeof value === "object" && typeof (value as AstNode).type === "string";

/** The [start, end) ranges, in UTF-16 code units, of the functions whose body opens with an opt-out directive. */
function optOutRanges(file: string, code: string): Array<[number, number]> {
  if (!/use no (memo|forget)/.test(code)) return [];
  const ranges: Array<[number, number]> = [];
  const visit = (value: unknown): void => {
    if (Array.isArray(value)) {
      for (const item of value) visit(item);
      return;
    }
    if (value == null || typeof value !== "object") return;
    if (
      isNode(value) &&
      value.type.includes("Function") &&
      isNode(value.body) &&
      value.body.type === "BlockStatement"
    ) {
      const statements = value.body.body as unknown[];
      if (statements.some((s) => isNode(s) && typeof s.directive === "string" && optOut.test(s.directive))) {
        ranges.push([value.start, value.end]);
      }
    }
    for (const [key, child] of Object.entries(value)) if (key !== "parent") visit(child);
  };
  visit(parseSync(file, code).program);
  return ranges;
}

/** `line:column` (1-based) of a UTF-16 offset, and the text of its line. */
function position(code: string, offset: number): { at: string; line: string } {
  const before = code.slice(0, offset).split("\n");
  const line = code.split("\n")[before.length - 1] ?? "";
  return { at: `${String(before.length)}:${String((before.at(-1)?.length ?? 0) + 1)}`, line: line.trim() };
}

/** The outcome of compiling one module. */
export interface CompiledModule {
  code: string;
  map: Awaited<ReturnType<typeof transform>>["map"];
  /** How many functions the compiler memoized (the `_c(N)` cache allocations in the output). */
  compiled: number;
  /**
   * Every diagnostic outside an opted-out function, as `id:line:column: reason` and the line's text. The
   * location is in the code the plugin received, which for a route is TanStack Router's split of the file
   * (`?tsr-split=component`), so the line's text is what finds it in the source.
   */
  problems: string[];
}

/**
 * Runs React Compiler on a TypeScript module, leaving JSX in place for Vite's own Oxc transform. `id` is the
 * module's id, which names it in the problems; its path without the query tells the compiler the language.
 */
export async function compileModule(id: string, code: string, sourcemap = true): Promise<CompiledModule> {
  const file = id.split("?")[0] ?? id;
  const result = await transform(file, code, {
    jsx: "preserve",
    sourcemap,
    reactCompiler: { reportDiagnostics: true },
  });
  if (result.fatal)
    throw new Error(
      result.errors.map((e) => e.codeframe ?? e.message).join("\n\n") || `React Compiler failed on ${id}`,
    );
  const problems: string[] = [];
  if (result.errors.length > 0) {
    const ranges = optOutRanges(file, code);
    // Oxc's labels count UTF-8 bytes; the AST and the source string count UTF-16 code units.
    const bytes = new TextEncoder().encode(code);
    const decoder = new TextDecoder();
    for (const error of result.errors) {
      const at = decoder.decode(bytes.subarray(0, error.labels[0]?.start ?? 0)).length;
      if (ranges.some(([start, end]) => start <= at && at < end)) continue;
      const { at: where, line } = position(code, at);
      problems.push(
        `${id}:${where}: ${error.message}${error.helpMessage ? ` (${error.helpMessage})` : ""}\n    > ${line}`,
      );
    }
  }
  return { code: result.code, map: result.map, compiled: (result.code.match(/\b_c\(\d+\)/g) ?? []).length, problems };
}

/**
 * The Vite plugin. It compiles the client environment's modules only (web-client/README.md T6). In `vite build` it fails
 * the build at the end with every problem's location and reason, or when it compiled no function; in the dev
 * server it prints the problems as warnings.
 */
export function reactCompiler(): Plugin {
  // By module id: TanStack Router splits a route file into several modules that share its path.
  const compiled = new Map<string, number>();
  const problems = new Map<string, string[]>();
  return {
    name: "my:react-compiler",
    enforce: "pre",
    transform: {
      filter: { id: idFilter, code: codeFilter },
      async handler(code, id) {
        const { config } = this.environment;
        if (config.consumer !== "client") return;
        let result: CompiledModule;
        try {
          result = await compileModule(id, code, config.command !== "build" || !!config.build.sourcemap);
        } catch (error) {
          this.error(error instanceof Error ? error.message : String(error));
        }
        compiled.set(id, result.compiled);
        if (config.command === "build") problems.set(id, result.problems);
        else for (const problem of result.problems) this.warn(problem);
        return result.map ? { code: result.code, map: result.map } : { code: result.code };
      },
    },
    buildEnd(error) {
      const { config } = this.environment;
      if (error || config.consumer !== "client" || config.command !== "build") return;
      const found = [...problems.values()].flat();
      if (found.length > 0) {
        this.error(
          `React Compiler skipped these functions; rewrite them so that it compiles them (web-client/README.md R4):\n${found.join("\n")}`,
        );
      }
      const total = [...compiled.values()].reduce((sum, n) => sum + n, 0);
      if (total === 0) this.error("React Compiler compiled no function; check reactCompiler() in vite.config.ts");
      this.info(`React Compiler compiled ${String(total)} function(s)`);
    },
  };
}
