// SPDX-License-Identifier: AGPL-3.0-only
// Spike #2906: the plugins of spike/bun-dev.ts for bunfig's [serve.static]: Tailwind, and the condition
// @mycroblog/source, which Bun's dev server does not take from `--conditions` or bunfig: @mycroblog/mykit from its
// sources.
import type { BunPlugin } from "bun";
import { reactCompiler, tailwind } from "./plugins-impl.ts";

const source: BunPlugin = {
  name: "my:source-condition",
  setup(build) {
    build.onResolve({ filter: /^@mycroblog\/mykit$/ }, () => ({
      path: new URL("../../../pkg/mykit/ts/src/index.ts", import.meta.url).pathname,
    }));
  },
};

const all: BunPlugin = {
  name: "my:spike-dev",
  async setup(build) {
    await source.setup(build);
    await reactCompiler.setup(build);
    await tailwind.setup(build);
  },
};
export default all;
