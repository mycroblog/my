// SPDX-License-Identifier: AGPL-3.0-only
// The user's own account (FR-008): the archives of the user's data, and deleting the account (web-client/README.md S1).
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import type { components } from "@mycroblog/mykit";
import { ApiError } from "../../../lib/errors";
import { type Confirmation, confirmationBody } from "./linkedIdentities";

type Schemas = components["schemas"];

export type DataExport = Schemas["DataExport"];
export type DataExportState = DataExport["state"];

/** The user's latest data exports, newest first (GET /exports). */
export function dataExportsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/exports");
}

/** Asks for an archive of the user's data (POST /exports), as the options of useMutation. Returns the pending export. */
export function requestDataExportMutation(api: Api) {
  return {
    mutationKey: ["post", "/exports"] as const,
    mutationFn: async (): Promise<DataExport> => {
      const { data, error } = await api.fetch.POST("/exports");
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Puts a requested export at the top of the cached list, as the server answered it (ADR-0028). */
export function cacheRequestedExport(api: Api, queryClient: QueryClient, requested: DataExport): void {
  queryClient.setQueryData(dataExportsQueryOptions(api).queryKey, (list: Schemas["DataExportList"] | undefined) => ({
    items: [requested, ...(list?.items ?? []).filter((item) => item.id !== requested.id)],
  }));
}

/** The archive of a ready export (GET /exports/{id}/archive). */
export async function fetchExportArchive(api: Api, id: string): Promise<Blob> {
  const { data, error } = await api.fetch.GET("/exports/{id}/archive", { params: { path: { id } }, parseAs: "blob" });
  // The middleware throws an ApiError for every error status; this narrows the type.
  if (data === undefined) throw new ApiError(error);
  return data;
}

export type DeactivationPreview = Schemas["DeactivationPreview"];

/** What deactivating the account would do now (GET /accounts/deactivation). */
export function deactivationPreviewQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/accounts/deactivation");
}

/** What confirms a deactivation: what confirms the other operations, or a code of the second factor. */
export type DeactivationConfirmation = Confirmation | { kind: "code"; code: string };

/** The problem type of a wrong code of the second factor (api/openapi.yaml, Problem). */
export const invalidCodeType = "/api/v1/problems/invalid-two-factor-code";

/** Deactivates the user's own account (POST /accounts/deactivation), as the options of useMutation. */
export function deactivateOwnAccountMutation(api: Api) {
  return {
    mutationKey: ["post", "/accounts/deactivation"] as const,
    mutationFn: async (confirmation: DeactivationConfirmation): Promise<Schemas["Deactivation"]> => {
      const body =
        confirmation.kind === "code"
          ? { two_factor_code: confirmation.code }
          : await confirmationBody(api, confirmation);
      const { data, error } = await api.fetch.POST("/accounts/deactivation", { body });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * Deletes the user's own account after checking the password, or a passkey when `password` is undefined (a user
 * without a password), (POST /accounts/delete), as the options of useMutation.
 */
export function deleteOwnAccountMutation(api: Api) {
  return {
    mutationKey: ["post", "/accounts/delete"] as const,
    mutationFn: async (confirmation: Confirmation): Promise<void> => {
      await api.fetch.POST("/accounts/delete", { body: await confirmationBody(api, confirmation) });
    },
  };
}
