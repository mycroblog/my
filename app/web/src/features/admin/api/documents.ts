// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the terms and policies screen (web-client/README.md S1): the instance texts (GET
// /admin/instance_texts), one value each that every locale shows, writing one (PUT /admin/instance_texts/{key}),
// and the instance, whose links to the public pages of the documents change with them.
import type { QueryClient } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import type { components } from "@mycroblog/mykit";
import { refreshAuditLog } from "../../moderation";

type Schemas = components["schemas"];

export type InstanceText = Schemas["AdminInstanceText"];

/** The instance texts that are documents, written in Markdown. */
export type DocumentKey = Extract<Schemas["InstanceTextKey"], "terms" | "privacy_policy" | "commerce_disclosure">;

/** The most characters of a document (internal/domain/instance, text.go). */
export const documentTextLimit = 100_000;

/** Every current instance text, by key. */
export function instanceTextsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/admin/instance_texts");
}

/** The instance, whose `terms_url`, `privacy_policy_url` and `commerce_disclosure_url` lead to the documents' public pages. */
export function instanceQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/instance");
}

/** A new text of a document; an empty body removes it. */
export type DocumentRevision = { key: DocumentKey; body: string };

/** Records a document's text, as the options of useMutation. Returns the text as recorded. */
export function setDocumentMutation(api: Api) {
  return {
    mutationKey: ["put", "/admin/instance_texts/{key}"] as const,
    mutationFn: async ({ key, body }: DocumentRevision): Promise<InstanceText> => {
      const { data, error } = await api.fetch.PUT("/admin/instance_texts/{key}", { params: { path: { key } }, body: { body } });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/**
 * Marks what a new text changes (ADR-0045): the texts and the instance's links to the documents' pages (which
 * every page shows); the queries shown now are fetched again and awaited, the others by their route's loader when
 * opened. The audit log records the change, and its route reads its cache as it is (`staleTime: "static"`), so it
 * is fetched again at once, like after every audited write (refreshAuditLog).
 */
export async function cacheSavedDocument(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/instance_texts"] }),
    queryClient.invalidateQueries({ queryKey: ["get", "/instance"] }),
    refreshAuditLog(queryClient),
  ]);
}
