// SPDX-License-Identifier: AGPL-3.0-only
// Regenerates the pseudo-locales, the Go typed accessors, and the parity golden file from the source catalogs.
// Run after editing locales/: `scripts/task.sh i18n:gen`. The repository commits what it writes, so building the
// Go server never needs Bun. A file is written only when its content changes, through a rename, so the checks and
// builds that read these files while a generator runs beside them never see a partial file.
import { isPseudo, loadCatalog, loadCatalogFiles, rel, removeCatalogFile, writeFileAtomic } from "./catalog";
import { generate } from "./generate";

/** Writes the generated files and returns the paths it wrote, relative to the repository. */
export async function writeGenerated(): Promise<string[]> {
  const catalogFiles = loadCatalogFiles();
  const files = await generate(loadCatalog(catalogFiles));
  // Pseudo-locale directories are fully generated; drop the files that no longer have a source (a removed area or
  // namespace, or a catalog outside a namespace directory), and the directories left empty.
  for (const { path, locale, ns } of catalogFiles) {
    if (isPseudo(locale) && !files.has(path)) removeCatalogFile(path, locale, ns);
  }
  const written: string[] = [];
  for (const [path, content] of files) {
    if (writeFileAtomic(path, content)) written.push(rel(path));
  }
  return written;
}

if (import.meta.main) {
  for (const path of await writeGenerated()) console.log(`wrote ${path}`);
}
