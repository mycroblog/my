// SPDX-License-Identifier: AGPL-3.0-only
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useT } from "../../../lib/i18n";
import { shellQueryReader } from "../../../lib/query";
import { instanceQueryOptions } from "../api/documents";

/** A link to a public page of the server's documents. */
export type DocumentLink = { href: string; label: string };

/**
 * The links to the documents that the server publishes for the reader's language (GET /instance): the terms of
 * service, then the privacy policy. A document the server has not published has no link, and there are none
 * while the instance loads or when it cannot be loaded. The shell reads the same query on every page, so this
 * reader does not fetch again what the shell already knows to fail (shellQueryReader).
 */
export function useDocumentLinks(): DocumentLink[] {
  const api = useApi();
  const t = useT();
  const { data } = useQuery({ ...instanceQueryOptions(api), ...shellQueryReader });
  if (data === undefined) return [];
  const links: DocumentLink[] = [];
  if (data.terms_url !== undefined) links.push({ href: data.terms_url, label: t("client:documents.terms") });
  if (data.privacy_policy_url !== undefined)
    links.push({ href: data.privacy_policy_url, label: t("client:documents.privacy_policy") });
  return links;
}
