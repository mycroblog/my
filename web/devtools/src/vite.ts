// SPDX-License-Identifier: AGPL-3.0-only

// The Vite plugin of @my/devtools (ADR-0168 §3). On the dev server it adds data-source to the JSX, loads the
// overlay, and takes the batches of notes; a production build refuses any import of @my/devtools.

import { mkdirSync, watch } from "node:fs";
import type { IncomingMessage } from "node:http";
import { relative } from "node:path";
import { fileURLToPath } from "node:url";
import { searchForWorkspaceRoot, type Plugin } from "vite";
import { endpoint, updateEvent, type BatchInput } from "./note";
import { addSources } from "./source";
import { feedbackDir, readBatches, writeBatch } from "./store";

const self = fileURLToPath(new URL(".", import.meta.url));
const overlay = fileURLToPath(new URL("./overlay.tsx", import.meta.url));

async function body(req: IncomingMessage): Promise<string> {
  const chunks: Buffer[] = [];
  for await (const chunk of req) chunks.push(chunk as Buffer);
  return Buffer.concat(chunks).toString("utf8");
}

function serve(): Plugin {
  let root = "";
  return {
    name: "my-devtools",
    apply: "serve",
    enforce: "pre",
    configResolved(config) {
      root = searchForWorkspaceRoot(config.root);
    },
    transform(code, id) {
      if (!/\.[jt]sx$/.test(id) || id.includes("/node_modules/") || id.startsWith(self)) return null;
      return addSources(code, id, relative(root, id));
    },
    transformIndexHtml() {
      return [{ tag: "script", attrs: { type: "module", src: `/@fs${overlay}` }, injectTo: "body" }];
    },
    configureServer(server) {
      const dir = feedbackDir(root);
      mkdirSync(dir, { recursive: true });
      // A batch the dev server wrote or `agentkit feedback reply` answered: the overlay reads the batches again.
      const watcher = watch(dir, () => {
        server.ws.send({ type: "custom", event: updateEvent });
      });
      server.httpServer?.on("close", () => {
        watcher.close();
      });
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
        this.error(`${importer ?? "a module"} imports ${id}: only the dev server loads @my/devtools (ADR-0168 §3).`);
      }
      return null;
    },
  };
}

/** The plugins of @my/devtools: list them first in the plugins of a Vite configuration. */
export function devtools(): Plugin[] {
  return [serve(), refuse()];
}
