// SPDX-License-Identifier: AGPL-3.0-only
// The user's follows, blocks, mutes, muted domains, and bookmarks as Mastodon's CSV files (FR-008): downloading them, and importing a file
// after a preview, with the progress of the imports, which the server carries out in the background (web-client/README.md S1).
import { type QueryClient, queryOptions } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";

type Schemas = components["schemas"];

export type DataImport = Schemas["DataImport"];
export type DataImportKind = Schemas["DataImportKind"];
export type DataImportMode = Schemas["DataImportMode"];
export type DataImportPreview = Schemas["DataImportPreview"];
export type DataImportRequest = Schemas["DataImportRequest"];
export type DataImportSkipped = Schemas["DataImportSkipped"];

/** The kinds, in the order of the page. */
export const dataImportKinds: readonly DataImportKind[] = ["following", "blocks", "mutes", "domain_mutes", "bookmarks"];

/** The names Mastodon gives the files, which the server's Content-Disposition also names. */
export const csvFileNames: Readonly<Record<DataImportKind, string>> = {
  following: "following_accounts.csv",
  blocks: "blocked_accounts.csv",
  mutes: "muted_accounts.csv",
  domain_mutes: "blocked_domains.csv",
  bookmarks: "bookmarks.csv",
};

/** The largest file the server takes, in UTF-16 code units of its text (the request's `maxLength`). */
export const maxImportTextLength = 1_000_000;

/** The CSV file of what the user has of a kind now (GET /csv_exports/{kind}). */
export async function fetchCsvExport(api: Api, kind: DataImportKind): Promise<Blob> {
  const { data, error } = await api.fetch.GET("/csv_exports/{kind}", { params: { path: { kind } }, parseAs: "blob" });
  // The middleware throws an ApiError for every error status; this narrows the type.
  if (data === undefined) throw new ApiError(error);
  return data;
}

/** What importing a file would do (POST /imports/preview), as the options of useMutation. */
export function previewDataImportMutation(api: Api) {
  return {
    mutationKey: ["post", "/imports/preview"] as const,
    mutationFn: async (body: DataImportRequest): Promise<DataImportPreview> => {
      const { data, error } = await api.fetch.POST("/imports/preview", { body });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Starts importing a file (POST /imports), as the options of useMutation. Returns the import, in progress. */
export function startDataImportMutation(api: Api) {
  return {
    mutationKey: ["post", "/imports"] as const,
    mutationFn: async (body: DataImportRequest): Promise<DataImport> => {
      const { data, error } = await api.fetch.POST("/imports", { body });
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * The query key paths of what a finished import changes: the lists, the profiles' relationships, the bookmarks, and the
 * notifications that muted domains hide.
 */
const importedPaths: ReadonlySet<unknown> = new Set([
  "/blocks",
  "/mutes",
  "/domain_mutes",
  "/notifications",
  "/notifications/marker",
  "/bookmarks",
  "/accounts/relationships",
  "/accounts/{id}/following",
  "/accounts/verify_credentials",
]);

/**
 * The user's latest imports, newest first (GET /imports). When an import that was in progress has finished, the queries of
 * what it changed are marked stale (ADR-0045): the server changed them in the background.
 */
export function dataImportsQueryOptions(api: Api, queryClient: QueryClient) {
  return queryOptions({
    queryKey: ["get", "/imports"] as const,
    queryFn: async ({ signal }) => {
      const { data, error } = await api.fetch.GET("/imports", { signal });
      if (data === undefined) throw new ApiError(error);
      const before = queryClient.getQueryData<Schemas["DataImportList"]>(["get", "/imports"]);
      const wasInProgress = new Set(
        before?.items.filter((item) => item.state === "in_progress").map((item) => item.id),
      );
      if (data.items.some((item) => item.state === "finished" && wasInProgress.has(item.id))) {
        void queryClient.invalidateQueries({
          predicate: (query) => query.queryKey[0] === "get" && importedPaths.has(query.queryKey[1]),
        });
      }
      return data;
    },
  });
}

/** Puts a started import at the top of the cached list, as the server answered it (ADR-0045). */
export function cacheStartedImport(api: Api, queryClient: QueryClient, started: DataImport): void {
  queryClient.setQueryData(
    dataImportsQueryOptions(api, queryClient).queryKey,
    (list: Schemas["DataImportList"] | undefined) => ({
      items: [started, ...(list?.items ?? []).filter((item) => item.id !== started.id)],
    }),
  );
}
