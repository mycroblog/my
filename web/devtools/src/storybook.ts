// SPDX-License-Identifier: AGPL-3.0-only

// The Storybook addon of @my/devtools (ADR-0168 §3): on Storybook's dev server only, the plugin of vite.ts adds
// data-source to the JSX and takes the batches of notes, recording the URL Storybook listens at; the manager
// (manager.ts) shows the notes, a toolbar tool and a Notes panel beside the canvas; and the preview
// (storybook-preview.ts) loads the overlay, which picks on the story and marks its notes, and tells it the story shown.
// A static build of Storybook and the story tests have none of it.
// The host lists it in its addons: `addons: ["@my/devtools/storybook"]`.

import { fileURLToPath } from "node:url";
import { mergeConfig, type InlineConfig } from "vite";
import { devtools } from "./vite";

/** What Storybook hands a preset: whether it serves or builds, and the port it listens at. */
interface Options {
  configType?: "DEVELOPMENT" | "PRODUCTION";
  port?: number;
  host?: string;
  https?: boolean;
}

/** Storybook's dev server, not a static build nor the story tests (whose Vitest serves the stories too). */
const dev = (options: Options) => options.configType === "DEVELOPMENT" && process.env["VITEST"] === undefined;

export const viteFinal = (config: InlineConfig, options: Options): InlineConfig => {
  if (!dev(options) || options.port === undefined) return config;
  const url = `${options.https === true ? "https" : "http"}://${options.host ?? "localhost"}:${String(options.port)}/`;
  return mergeConfig(config, { plugins: [devtools({ url })] });
};

export const previewAnnotations = (entries: string[] = [], options: Options): string[] =>
  dev(options) ? [...entries, fileURLToPath(new URL("./storybook-preview.ts", import.meta.url))] : entries;

export const managerEntries = (entries: string[] = [], options: Options): string[] =>
  dev(options) ? [...entries, fileURLToPath(new URL("./manager.ts", import.meta.url))] : entries;
