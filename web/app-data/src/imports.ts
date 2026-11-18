// SPDX-License-Identifier: AGPL-3.0-only
// Bringing data in from a CSV file (FR-008, ADR-0088): what the viewer follows, the hashtags they follow, and their
// blocks, mutes, muted domains and bookmarks. A preview says what a file would do; starting it carries it out in the
// background, and the list of imports says how far each is. While one is in progress, the list is read again every
// 10 seconds. The CSV to take away is exports.ts's.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import type { AppData } from "./app-data";
import type { CsvKind } from "./exports";
import { useAppData } from "./provider";

/** An import of a file, being carried out or done. */
export type DataImport = components["schemas"]["DataImport"];

/** What to import: the kind of the file, whether it is added to what the viewer has or replaces it, and its text. */
export type ImportRequest = components["schemas"]["DataImportRequest"];

/** Whether an import adds to what the viewer has (`merge`) or replaces it (`overwrite`). */
export type ImportMode = components["schemas"]["DataImportMode"];

/** What an import would do, and the lines of the file it would skip. */
export type ImportPreview = components["schemas"]["DataImportPreview"];

/** The kinds of file that can be imported: the same as the CSV that can be exported. */
export type ImportKind = CsvKind;

/** How often the list is read again while an import is in progress. */
const runningRefetchMs = 10_000;

/** The viewer's latest imports, newest first. */
export function dataImportsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/imports");
}

/** What `request` would do to what the viewer has; nothing changes. It rejects with the server's problem. */
export async function previewDataImport(
  data: AppData,
  request: ImportRequest,
  signal?: AbortSignal,
): Promise<ImportPreview> {
  const { data: preview } = await data.api.POST("/imports/preview", {
    body: request,
    ...(signal === undefined ? {} : { signal }),
  });
  if (preview === undefined) throw new Error("app-data: the preview came without an answer");
  return preview;
}

/**
 * Starts the import of `request`, and resolves with it once the list holds it; what it changes is marked once it
 * finished (markImported). It rejects with the server's problem (an import is already running, too
 * many today, a file that is too large).
 */
export async function startDataImport(data: AppData, request: ImportRequest): Promise<DataImport | undefined> {
  const { data: started } = await data.api.POST("/imports", { body: request });
  await data.queryClient.invalidateQueries({ queryKey: dataImportsQuery(data).queryKey });
  return started;
}

/** Marks what an import that finished may have changed: the viewer's follows, blocks, mutes, hashtags and bookmarks. */
export async function markImported(data: AppData): Promise<void> {
  await data.queryClient.invalidateQueries({
    predicate: ({ queryKey }) =>
      typeof queryKey[1] === "string" &&
      (queryKey[1].startsWith("/timelines/") ||
        ["/blocks", "/mutes", "/domain_mutes", "/followed_tags", "/bookmarks", "/accounts/relationships"].includes(
          queryKey[1],
        )),
    refetchType: "active",
  });
}

export type DataImports = {
  /** The viewer's latest imports, newest first; undefined until the server answered. */
  readonly imports: readonly DataImport[] | undefined;
  readonly preview: (request: ImportRequest, signal?: AbortSignal) => Promise<ImportPreview>;
  readonly start: (request: ImportRequest) => Promise<DataImport | undefined>;
};

/** The viewer's imports, and previewing and starting one, from a screen. */
export function useDataImports(): DataImports {
  const data = useAppData();
  const imports = useQuery({
    ...dataImportsQuery(data),
    refetchInterval: (query) =>
      query.state.data?.items.some((i) => i.state === "in_progress") === true ? runningRefetchMs : false,
  }).data?.items;
  // What an import changed is read again once the list says that none is in progress any more.
  const running = imports?.some((i) => i.state === "in_progress") === true;
  const was = useRef(running);
  useEffect(() => {
    if (was.current && !running) void markImported(data);
    was.current = running;
  }, [data, running]);
  return {
    imports,
    preview: (request, signal) => previewDataImport(data, request, signal),
    start: (request) => startDataImport(data, request),
  };
}
