// SPDX-License-Identifier: AGPL-3.0-only
// Taking the viewer's data away (FR-008, ADR-0088 §2), which is all a closed server still does: the archive of
// everything, requested, built in the background, listed with its state and downloaded once ready; and the CSV of what
// they follow, which another server imports. While an archive is being built, the list is read again every 10 seconds.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";

/** An export of the viewer's data, built in the background. */
export type DataExport = components["schemas"]["DataExport"];

/** A file to save: its contents and the name the server gives it. */
export type SavedFile = { readonly blob: Blob; readonly name: string };

/** What the viewer may export as CSV. */
export type CsvKind = components["schemas"]["DataImportKind"];

/** How often the list is read again while an archive is being built. */
const buildingRefetchMs = 10_000;

/** The viewer's latest exports, newest first. */
export function dataExportsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/exports");
}

/**
 * Starts building the archive of the viewer's data, and reads the list again; it rejects with the problem (one is
 * already being built, too many today).
 */
export async function requestDataExport(data: AppData): Promise<DataExport | undefined> {
  const made = (await data.api.POST("/exports")).data;
  await data.queryClient.invalidateQueries({ queryKey: dataExportsQuery(data).queryKey });
  return made;
}

/** The name in a Content-Disposition header, else `fallback`. */
function fileName(disposition: string | null, fallback: string): string {
  const name = /filename="?([^";]+)"?/.exec(disposition ?? "")?.[1];
  return name === undefined || name === "" ? fallback : name;
}

/** The archive of the export of `id`, once it is ready; it rejects with the problem (not ready, gone). */
export async function downloadDataExport({ api }: AppData, id: string): Promise<SavedFile> {
  const { data, response } = await api.GET("/exports/{id}/archive", { params: { path: { id } }, parseAs: "blob" });
  return {
    blob: data ?? new Blob([]),
    name: fileName(response.headers.get("Content-Disposition"), `export-${id}.zip`),
  };
}

/** The CSV of what the viewer has of `kind` now; it rejects with the problem. */
export async function exportCsv({ api }: AppData, kind: CsvKind): Promise<SavedFile> {
  const { data, response } = await api.GET("/csv_exports/{kind}", {
    params: { path: { kind } },
    parseAs: "blob",
  });
  return { blob: data ?? new Blob([]), name: fileName(response.headers.get("Content-Disposition"), `${kind}.csv`) };
}

export type DataExports = {
  /** The viewer's latest exports, newest first; undefined until the server answered. */
  readonly exports: readonly DataExport[] | undefined;
  readonly request: () => Promise<DataExport | undefined>;
  readonly download: (id: string) => Promise<SavedFile>;
  readonly csv: (kind: CsvKind) => Promise<SavedFile>;
};

/** The viewer's exports, and requesting and downloading them and the CSV files, from a screen. */
export function useDataExports(): DataExports {
  const data = useAppData();
  const exports = useQuery({
    ...dataExportsQuery(data),
    refetchInterval: (query) =>
      query.state.data?.items.some((e) => e.state === "pending") === true ? buildingRefetchMs : false,
  }).data?.items;
  return {
    exports,
    request: () => requestDataExport(data),
    download: (id) => downloadDataExport(data, id),
    csv: (kind) => exportCsv(data, kind),
  };
}
