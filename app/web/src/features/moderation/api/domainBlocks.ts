// SPDX-License-Identifier: AGPL-3.0-only
// The domain block list in Mastodon's CSV (FR-706): the export of the domain actions in effect as the file
// Mastodon imports, and the import of a file Mastodon exported in two steps, a preview of what each row would
// issue and the import of the rows the moderator keeps. The import issues domain actions, so it changes what a
// new action changes (cacheIssuedDomainAction).
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import type { components } from "@mycroblog/mykit";
import { cacheIssuedDomainAction } from "./staff";

type Schemas = components["schemas"];

export type DomainBlockImportRow = Schemas["DomainBlockImportRow"];
export type DomainBlockImportStatus = Schemas["DomainBlockImportStatus"];
export type DomainBlockImportPreview = Schemas["DomainBlockImportPreview"];
export type DomainBlockImport = Schemas["ImportDomainBlocksRequest"];
export type DomainBlockImportResult = Schemas["DomainBlockImportResult"];

/** The name Mastodon gives the file, which the export is saved as. */
export const domainBlockFileName = "domain_blocks.csv";

/** The most rows of one import (PreviewDomainBlockImportRequest, ImportDomainBlocksRequest). */
export const domainBlockImportRowLimit = 5000;

/**
 * The most bytes of a CSV, in UTF-8: the server reads at most 1 MiB of a request body, and the JSON escapes of
 * the file fit in what is left.
 */
export const domainBlockImportByteLimit = 900_000;

/** The domain actions in effect as Mastodon's domain block CSV (GET /admin/domain_actions/export). */
export async function fetchDomainBlockExport(api: Api): Promise<Blob> {
  const { data, error } = await api.fetch.GET("/admin/domain_actions/export", { parseAs: "blob" });
  // The middleware throws an ApiError for every error status; this narrows the type.
  if (data === undefined) throw new ApiError(error);
  return data;
}

/** What importing each row of a CSV would do (POST /admin/domain_actions/import/preview), as the options of useMutation. */
export function previewDomainBlockImportMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/domain_actions/import/preview"] as const,
    mutationFn: async (csv: string): Promise<DomainBlockImportPreview> => {
      const { data, error } = await api.fetch.POST("/admin/domain_actions/import/preview", { body: { csv } });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Issues the actions of the chosen rows (POST /admin/domain_actions/import), as the options of useMutation. */
export function importDomainBlocksMutation(api: Api) {
  return {
    mutationKey: ["post", "/admin/domain_actions/import"] as const,
    mutationFn: async (body: DomainBlockImport): Promise<DomainBlockImportResult> => {
      const { data, error } = await api.fetch.POST("/admin/domain_actions/import", { body });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * What an import changes: what each action it issued changes (the lists of domain actions, the audit log and,
 * for a limit or a suspension, the lists of posts). Resolves once those on screen are fetched again.
 */
export async function cacheImportedDomainBlocks(
  queryClient: QueryClient,
  result: DomainBlockImportResult,
): Promise<void> {
  const heaviest =
    result.issued.find((action) => action.kind === "suspend" || action.kind === "limit") ?? result.issued[0];
  if (heaviest !== undefined) await cacheIssuedDomainAction(queryClient, heaviest);
}
