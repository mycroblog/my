// SPDX-License-Identifier: AGPL-3.0-only
// What the about page and the server's documents read (FR-705, FR-718; ADR-0072 §13), for everyone, signed in or not:
// the instance, with the way to read it again where it could not be; a document the instance published (its terms, its
// privacy policy, its commerce disclosure), a 404 where it has none; and the domains it moderates, which it lists only
// to whom `domain_actions_visibility` lets read them.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { instanceQuery, type Instance } from "./instance";
import { pagedQuery, usePagedList, type PagedList } from "./paged";
import { useAppData } from "./provider";

type Schemas = components["schemas"];

/** A document of the instance: its kind, its HTML as the server sanitized it, and when it was last revised. */
export type InstanceDocument = Schemas["InstanceDocument"];
/** Which document: the terms of service, the privacy policy, or the commerce disclosure of the supporter plans. */
export type InstanceDocumentKind = Schemas["InstanceDocumentKind"];
/** A domain the instance moderates, as it lists it publicly: limited or suspended, with its public comment. */
export type InstanceDomainAction = Schemas["InstanceDomainAction"];

/**
 * Whether `instance` lists its moderated domains to the reader: to everyone, or, with `signed_in`, to a reader with a
 * session (`signedIn`).
 */
export function domainActionsShown(
  instance: Pick<Instance, "domain_actions_visibility"> | undefined,
  signedIn: boolean,
): boolean {
  const visibility = instance?.domain_actions_visibility;
  return visibility === "everyone" || (visibility === "signed_in" && signedIn);
}

/** The document of `kind` that the instance published (a 404 where it has none). */
export function instanceDocumentQuery({ queries }: AppData, kind: InstanceDocumentKind) {
  return queries.queryOptions("get", "/instance/documents/{kind}", { params: { path: { kind } } });
}

/**
 * A document as a page reads it: until the server answered, when it could not be read (with the reader's Try again),
 * when the instance has not published it (a 404), or the document.
 */
export type InstanceDocumentRead =
  | { readonly status: "pending" }
  | { readonly status: "error"; readonly retry: () => void }
  | { readonly status: "unpublished" }
  | { readonly status: "success"; readonly document: InstanceDocument };

/** The document of `kind`, for everyone, signed in or not. */
export function useInstanceDocument(kind: InstanceDocumentKind): InstanceDocumentRead {
  const query = useQuery(instanceDocumentQuery(useAppData(), kind));
  // A 404 after an earlier answer means the instance took the document down: the page says so, not what it kept.
  if (query.error instanceof ProblemError && query.error.status === 404) return { status: "unpublished" };
  if (query.data !== undefined) return { status: "success", document: query.data };
  if (query.status === "error")
    return {
      status: "error",
      retry: () => {
        void query.refetch();
      },
    };
  return { status: "pending" };
}

/** The domains the instance moderates, as it lists them publicly, newest first, a page at a time. */
export function instanceDomainActionsQuery({ api, queries }: AppData) {
  return pagedQuery<InstanceDomainAction>(
    queries.queryOptions("get", "/instance/domain_actions").queryKey,
    async (cursor, signal) =>
      (
        await api.GET("/instance/domain_actions", {
          params: { query: cursor === undefined ? {} : { cursor } },
          signal,
        })
      ).data,
  );
}

/**
 * The domains the instance moderates; nothing is read while `enabled` is false (where it lists them to nobody, or only
 * to signed-in readers and the reader has no session: `domainActionsShown`).
 */
export function useInstanceDomainActions(enabled = true): PagedList<InstanceDomainAction> {
  return usePagedList(instanceDomainActionsQuery(useAppData()), enabled);
}

/**
 * The instance as the about page reads it: undefined until the server answered, and `retry` once it could not be read,
 * which reads it again.
 */
export function useAboutInstance(): {
  readonly instance: Instance | undefined;
  readonly retry: (() => void) | undefined;
} {
  const query = useQuery(instanceQuery(useAppData()));
  const retry =
    query.data === undefined && query.status === "error"
      ? () => {
          void query.refetch();
        }
      : undefined;
  return { instance: query.data, retry };
}
