// SPDX-License-Identifier: AGPL-3.0-only
// The `client` message catalogs of locales/ (ADR-0009) as one module per locale. A locale's catalog is a directory
// of area files (ADR-0082); importing each file on its own would make a chunk per file, so this plugin merges them:
//   virtual:locales/<locale>/client   the default export holds every message of the locale's area files
//   virtual:locales                   the default export maps each locale with a client catalog to a dynamic
//                                     import of its module, which the build puts in a chunk of its own
// The dev server reloads the page when a catalog file is added, changed, or removed.
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { join, relative, sep } from "node:path";
import type { Plugin } from "vite";

const indexId = "virtual:locales";
const catalogId = /^virtual:locales\/([^/]+)\/client$/;

/** The locales with a client catalog, sorted. */
function catalogLocales(localesDir: string): string[] {
  return readdirSync(localesDir, { withFileTypes: true })
    .filter((d) => d.isDirectory() && existsSync(join(localesDir, d.name, "client")))
    .map((d) => d.name)
    .toSorted();
}

/** The area files of a locale's client catalog, sorted. */
function areaFiles(localesDir: string, locale: string): string[] {
  const dir = join(localesDir, locale, "client");
  return readdirSync(dir, { recursive: true, encoding: "utf8" })
    .filter((f) => f.endsWith(".json"))
    .toSorted()
    .map((f) => join(dir, f));
}

export function clientCatalogs(localesDir: string): Plugin {
  return {
    name: "my:client-catalogs",
    resolveId(id) {
      return id === indexId || catalogId.test(id) ? `\0${id}` : undefined;
    },
    load(id) {
      if (!id.startsWith("\0")) return undefined;
      const virtual = id.slice(1);
      if (virtual === indexId) {
        const loaders = catalogLocales(localesDir).map(
          (l) => `${JSON.stringify(l)}: () => import("virtual:locales/${l}/client")`,
        );
        return `export default {${loaders.join(", ")}};\n`;
      }
      const locale = catalogId.exec(virtual)?.[1];
      if (locale === undefined) return undefined;
      const messages: Record<string, unknown> = {};
      for (const file of areaFiles(localesDir, locale)) {
        this.addWatchFile(file);
        Object.assign(messages, JSON.parse(readFileSync(file, "utf8")));
      }
      return `export default JSON.parse(${JSON.stringify(JSON.stringify(messages))});\n`;
    },
    configureServer(server) {
      const reload = (file: string) => {
        const [, namespace] = relative(localesDir, file).split(sep);
        if (file.startsWith(localesDir + sep) && namespace?.startsWith("client")) {
          const graph = server.environments.client.moduleGraph;
          for (const [id, mod] of graph.idToModuleMap) if (id.startsWith(`\0${indexId}`)) graph.invalidateModule(mod);
          server.ws.send({ type: "full-reload" });
        }
      };
      server.watcher.add(localesDir);
      server.watcher.on("add", reload).on("change", reload).on("unlink", reload);
    },
  };
}
