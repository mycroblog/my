// SPDX-License-Identifier: AGPL-3.0-only
// Runs the React Compiler plugin on small modules as `vite build` would and proves that the gate of
// web-client/README.md R4 fails on a function the compiler skips, prints the compiler's reason and location, lets a
// "use no memo" opt-out through, and fails when the compiler compiled nothing.
import { describe, expect, it } from "vitest";
import { compileModule, reactCompiler } from "./react-compiler.ts";

const environment = { config: { consumer: "client", command: "build", build: { sourcemap: false } } };

/** Transforms each module with the plugin and calls its buildEnd; returns the message the build fails with. */
async function build(modules: Record<string, string>): Promise<string | undefined> {
  const plugin = reactCompiler();
  const context = {
    environment,
    error(message: string): never {
      throw new Error(message);
    },
    warn() {},
    info() {},
  };
  const transform = plugin.transform;
  const buildEnd = plugin.buildEnd;
  if (typeof transform !== "object" || typeof buildEnd !== "function") throw new Error("unexpected plugin shape");
  try {
    for (const [file, code] of Object.entries(modules))
      await transform.handler.call(context as never, code, file, {} as never);
    await buildEnd.call(context as never, undefined);
  } catch (error) {
    return (error as Error).message;
  }
  return undefined;
}

const counter = `
  import { useState } from "react";
  export function useCount() {
    const [count, setCount] = useState(0);
    return { count, increment: () => setCount(count + 1) };
  }
`;

// openapi-react-query's hooks are methods of the client (`api.query.useMutation`), which the compiler cannot
// prove to be the same function on every render.
const dynamicHook = `
  import { useApi } from "./api";
  export function useSend() {
    const api = useApi();
    return api.query.useMutation("post", "/statuses");
  }
`;

describe("compileModule", () => {
  it("memoizes a hook the compiler compiles", async () => {
    const result = await compileModule("/src/useCount.ts", counter);
    expect(result.compiled).toBe(1);
    expect(result.problems).toEqual([]);
  });

  it("reports a skipped function with its location and the compiler's reason", async () => {
    const result = await compileModule("/src/useSend.ts", dynamicHook);
    expect(result.compiled).toBe(0);
    expect(result.problems).toHaveLength(1);
    expect(result.problems[0]).toMatch(/^\/src\/useSend\.ts:5:\d+: .*[Hh]ook/);
  });

  it("locates the problem by characters, not bytes, after non-ASCII text", async () => {
    const result = await compileModule("/src/useSend.ts", `// 日本語のコメント\n${dynamicHook}`);
    const column = /^\/src\/useSend\.ts:6:(\d+):/.exec(result.problems[0] ?? "")?.[1];
    const plain = /^\/src\/useSend\.ts:5:(\d+):/.exec(
      (await compileModule("/src/useSend.ts", dynamicHook)).problems[0] ?? "",
    )?.[1];
    expect(column).toBeDefined();
    expect(column).toBe(plain);
  });

  it('ignores the diagnostics of a function that opts out with "use no memo"', async () => {
    const result = await compileModule(
      "/src/useSend.ts",
      dynamicHook.replace("const api = useApi();", '"use no memo";\n    const api = useApi();'),
    );
    expect(result.problems).toEqual([]);
  });

  // The compiler reports a library it cannot compile even inside a function that opts out (oxc#26140); the
  // virtualizer hook of @my/ui's Feed is the one documented opt-out (web-client/README.md R4), and the useVirtual
  // example of app/oxlint/violations is the same call without it.
  const virtual = (body: string) => `
    import { useVirtualizer } from "@tanstack/react-virtual";
    export function useVirtual(count: number) {
      ${body}
      const virtualizer = useVirtualizer({ count, getScrollElement: () => null, estimateSize: () => 10 });
      return virtualizer.getTotalSize();
    }
  `;

  it("reports an incompatible library outside an opt-out", async () => {
    const result = await compileModule("/src/useVirtual.ts", virtual(""));
    expect(result.problems).toEqual([expect.stringMatching(/^\/src\/useVirtual\.ts:5:\d+: .*incompatible library/)]);
  });

  it("lets an incompatible library through inside an opt-out", async () => {
    expect((await compileModule("/src/useVirtual.ts", virtual('"use no memo";'))).problems).toEqual([]);
  });
});

describe("reactCompiler", () => {
  it("passes a build where every function compiles", async () => {
    expect(await build({ "/src/useCount.ts": counter })).toBeUndefined();
  });

  it("fails the build on a skipped function, with the compiler's reason", async () => {
    expect(await build({ "/src/useCount.ts": counter, "/src/useSend.ts": dynamicHook })).toMatch(
      /React Compiler skipped these functions[\s\S]*\/src\/useSend\.ts:5:/,
    );
  });

  it("keeps the problems of each module that TanStack Router splits from one route file", async () => {
    expect(
      await build({
        "/src/routes/a.tsx?tsr-split=component": dynamicHook,
        "/src/routes/a.tsx?tsr-split=errorComponent": counter,
      }),
    ).toMatch(/\/src\/routes\/a\.tsx\?tsr-split=component:5:\d+: [^\n]*\n {4}> return api\.query\.useMutation/);
  });

  it("fails when the compiler compiled nothing", async () => {
    expect(await build({ "/src/Values.ts": "export const X = 1;" })).toMatch(/compiled no function/);
  });

  it("leaves the modules of other environments alone", async () => {
    const plugin = reactCompiler();
    const transform = plugin.transform;
    if (typeof transform !== "object") throw new Error("unexpected plugin shape");
    const context = { environment: { config: { ...environment.config, consumer: "server" } } };
    expect(await transform.handler.call(context as never, dynamicHook, "/src/useSend.ts", {} as never)).toBeUndefined();
  });
});
