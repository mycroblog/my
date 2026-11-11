// SPDX-License-Identifier: AGPL-3.0-only
// Regenerates pseudo-locales, typed accessors (Go and TS), and the parity golden file
// from the source catalogs. Run after editing locales/: `scripts/task.sh i18n:gen`.
//
//   bun run src/gen.ts        write every generated file
//   bun run src/gen.ts --ts   write only the files the TS clients read (`scripts/task.sh i18n:gen-ts`)
//
// The files Go reads are committed, so building the Go server never needs Bun; the TS-only files are
// not (ADR-0081). A file is written only when its content changes, through a rename, so the checks and
// builds that read these files while a generator runs beside them never see a partial file.
import { basename } from "node:path";
import { isPseudo, loadCatalog, loadCatalogFiles, rel, removeCatalogFile, writeFileAtomic } from "./catalog";
import { generate, type Part, pseudoPart } from "./generate";

/** Writes the generated files of `parts` and returns the paths it wrote, relative to the repository. */
export async function writeGenerated(parts: Part[] = ["go", "ts"]): Promise<string[]> {
  const catalogFiles = loadCatalogFiles();
  const files = await generate(loadCatalog(catalogFiles), parts);
  // Pseudo-locale directories are fully generated; drop the files of these parts that no longer have a source
  // (a removed area or namespace, or a catalog outside a namespace directory), and the directories left empty.
  for (const { path, locale, ns } of catalogFiles) {
    if (isPseudo(locale) && parts.includes(pseudoPart(ns || basename(path, ".json"))) && !files.has(path)) {
      removeCatalogFile(path, locale);
    }
  }
  const written: string[] = [];
  for (const [path, content] of files) {
    if (writeFileAtomic(path, content)) written.push(rel(path));
  }
  return written;
}

if (import.meta.main) {
  for (const path of await writeGenerated(process.argv.includes("--ts") ? ["ts"] : ["go", "ts"])) {
    console.log(`wrote ${path}`);
  }
}
