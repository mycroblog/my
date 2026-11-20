// SPDX-License-Identifier: AGPL-3.0-only

// The bundle of each HTML entry of the production build (ADR-0167 §1): the entry's script, the chunks it imports
// statically and their stylesheets, which the server's shell loads with the page (internal/interface/web), read from
// Vite's manifest. The chunks it imports dynamically, a route's or the composer's, load later and are not counted.

import { gzipSync } from "node:zlib";

/** A chunk of Vite's manifest (https://vite.dev/guide/backend-integration). */
type ManifestChunk = { file: string; isEntry?: boolean; imports?: string[]; css?: string[] };

export type Manifest = Readonly<Record<string, ManifestChunk>>;

/** What an entry loads with the page: how many files, and their bytes as built and gzipped. */
export type Bundle = { readonly entry: string; readonly files: number; readonly bytes: number; readonly gzip: number };

/** The bundle of each HTML entry of `manifest`, sorted by entry; `read` gives a file of the build by its path. */
export function bundles(manifest: Manifest, read: (file: string) => Uint8Array): Bundle[] {
  const result: Bundle[] = [];
  for (const [key, entry] of Object.entries(manifest)) {
    if (entry.isEntry !== true || !key.endsWith(".html")) continue;
    const files = new Set<string>();
    const seen = new Set<string>();
    const visit = (k: string) => {
      const chunk = manifest[k];
      if (chunk === undefined || seen.has(k)) return;
      seen.add(k);
      files.add(chunk.file);
      for (const css of chunk.css ?? []) files.add(css);
      for (const imported of chunk.imports ?? []) visit(imported);
    };
    visit(key);
    let bytes = 0;
    let gzip = 0;
    for (const file of files) {
      const content = read(file);
      bytes += content.byteLength;
      gzip += gzipSync(content).byteLength;
    }
    result.push({ entry: key, files: files.size, bytes, gzip });
  }
  return result.sort((a, b) => a.entry.localeCompare(b.entry));
}
