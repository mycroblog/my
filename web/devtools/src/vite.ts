// SPDX-License-Identifier: AGPL-3.0-only

// The Vite plugin of @my/devtools (ADR-0168 §3). On the dev server it adds data-source to the JSX, takes the batches
// of notes, and records its URL; a production build refuses any import of @my/devtools. The notes' UI is the Storybook
// addon's (storybook.ts): inside Storybook's dev server, which runs Vite in middleware mode without a server of its
// own, the plugin records the URL Storybook listens at.

import { mkdirSync, watch } from "node:fs";
import type { IncomingMessage } from "node:http";
import { basename, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { searchForWorkspaceRoot, type Plugin, type ViteDevServer } from "vite";
import { endpoint, updateEvent, type BatchInput } from "./note";
import { addSources } from "./source";
import { feedbackDir, readBatches, removeServer, writeBatch, writeServer } from "./store";

const self = fileURLToPath(new URL(".", import.meta.url));

async function body(req: IncomingMessage): Promise<string> {
  const chunks: Buffer[] = [];
  for await (const chunk of req) chunks.push(chunk as Buffer);
  return Buffer.concat(chunks).toString("utf8");
}

/** Where the dev server is: its own (by default), or the server that serves its middlewares at `url`. */
export interface DevtoolsOptions {
  url?: string;
}

/** Records the URL of a server in middleware mode until its process ends, which no server of its own closes. */
function recordUntilExit(server: ViteDevServer, root: string, url: string, stop: () => void) {
  writeServer(root, url, process.pid);
  server.config.logger.info(`${basename(server.config.root)}: ${url}`);
  process.once("exit", stop);
  for (const [signal, code] of [
    ["SIGINT", 130],
    ["SIGTERM", 143],
    ["SIGHUP", 129],
  ] as const)
    process.once(signal, () => {
      stop();
      process.exit(code);
    });
}

function serve({ url }: DevtoolsOptions): Plugin {
  let root = "";
  return {
    name: "my-devtools",
    apply: "serve",
    enforce: "pre",
    // Vite's watcher (chokidar) drops a change to a file within 50 ms of its last change, and a raw event within 5 ms
    // of the last. So when an agent saves a file twice in a row while the page fetches the module in between, the
    // server keeps the first save until the file changes again. Waiting until a file holds still reports its last
    // save instead, a few tens of milliseconds later.
    config: () => ({ server: { watch: { awaitWriteFinish: { stabilityThreshold: 50, pollInterval: 10 } } } }),
    configResolved(config) {
      root = searchForWorkspaceRoot(config.root);
    },
    transform(code, id) {
      if (!/\.[jt]sx$/.test(id) || id.includes("/node_modules/") || id.startsWith(self)) return null;
      return addSources(code, id, relative(root, id));
    },
    configureServer(server) {
      const dir = feedbackDir(root);
      mkdirSync(dir, { recursive: true });
      // A batch the dev server wrote or `agentkit feedback reply` answered: the overlay reads the batches again.
      const watcher = watch(dir, () => {
        server.ws.send({ type: "custom", event: updateEvent });
      });
      const stop = () => {
        watcher.close();
        removeServer(root);
      };
      if (url !== undefined) recordUntilExit(server, root, url, stop);
      // Vite resolves its URLs on "listening" before this listener runs. The page is named after its directory,
      // on one plain line ahead of Vite's banner.
      server.httpServer?.on("listening", () => {
        const local = server.resolvedUrls?.local[0];
        if (local === undefined) return;
        writeServer(root, local, process.pid);
        server.config.logger.info(`${basename(server.config.root)}: ${local}`);
      });
      server.httpServer?.on("close", stop);
      server.middlewares.use(`${endpoint}/batches`, (req, res) => {
        const send = (status: number, value: unknown) => {
          res.statusCode = status;
          res.setHeader("content-type", "application/json");
          res.end(JSON.stringify(value));
        };
        if (req.method === "GET") {
          send(200, readBatches(root));
          return;
        }
        if (req.method !== "POST") {
          send(405, { error: "GET or POST" });
          return;
        }
        body(req)
          .then((text) => {
            const batch = writeBatch(root, JSON.parse(text) as BatchInput);
            server.config.logger.info(`my-devtools: batch ${batch.id}, ${batch.notes.length} notes`, {
              timestamp: true,
            });
            send(201, batch);
          })
          .catch((err: unknown) => {
            send(400, { error: String(err) });
          });
      });
    },
  };
}

function refuse(): Plugin {
  return {
    name: "my-devtools:refuse",
    apply: "build",
    enforce: "pre",
    resolveId(id, importer) {
      if (id === "@my/devtools" || id.startsWith("@my/devtools/")) {
        this.error(
          `${importer ?? "a module"} imports ${id}: only the dev server loads @my/devtools (ADR-0168 §3). ` +
            "Keep the variant the decider picked of each <Options> and remove the rest.",
        );
      }
      return null;
    },
  };
}

/** The plugins of @my/devtools: list them first in the plugins of a Vite configuration. */
export function devtools(options: DevtoolsOptions = {}): Plugin[] {
  return [serve(options), refuse()];
}
