// SPDX-License-Identifier: AGPL-3.0-only
// The queries and mutations of the link card domains screen (web-client/screens/admin-link-card-domains.md,
// manage_settings): the rules of the link card crawler, by domain (GET /admin/link_card_domains); setting the rule of
// a domain and removing it. Every change is recorded in the audit log.
import type { QueryClient } from "@tanstack/react-query";
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import { refreshAuditLog } from "../../moderation";

type Schemas = components["schemas"];

export type LinkCardDomainRule = Schemas["LinkCardDomainRule"];
export type LinkCardDomainMode = Schemas["LinkCardDomainMode"];
export type NewLinkCardDomainRule = Schemas["SetLinkCardDomainRuleRequest"] & { domain: string };

/** The longest note of a rule, in characters (SetLinkCardDomainRuleRequest). */
export const linkCardDomainNoteLimit = 5000;

/** The longest domain (SetLinkCardDomainRuleRequest). */
export const linkCardDomainLimit = 253;

/** The modes of a rule, in the order of the form: the one that stops less first. */
export const linkCardDomainModes = ["no_images", "no_fetch"] as const satisfies readonly LinkCardDomainMode[];

/** The rules, by domain. */
export function linkCardDomainsQueryOptions(api: Api) {
  return api.query.queryOptions("get", "/admin/link_card_domains");
}

/** Sets the rule of a domain, replacing the one it has (PUT /admin/link_card_domains/{domain}), as the options of useMutation. */
export function setLinkCardDomainRuleMutation(api: Api) {
  return {
    mutationKey: ["put", "/admin/link_card_domains/{domain}"] as const,
    mutationFn: async ({ domain, ...body }: NewLinkCardDomainRule): Promise<LinkCardDomainRule> => {
      const { data, error } = await api.fetch.PUT("/admin/link_card_domains/{domain}", {
        params: { path: { domain } },
        body,
      });
      // The middleware throws an ApiError for every error status; this narrows the type.
      if (data === undefined) throw new ApiError(error);
      return data;
    },
  };
}

/** Removes the rule of a domain, as the options of useMutation. */
export function removeLinkCardDomainRuleMutation(api: Api) {
  return {
    mutationKey: ["delete", "/admin/link_card_domains/{domain}"] as const,
    mutationFn: async (domain: string): Promise<void> => {
      // The middleware throws for every error status; a 204 has no body to read.
      await api.fetch.DELETE("/admin/link_card_domains/{domain}", { params: { path: { domain } } });
    },
  };
}

/**
 * Marks what a change of a rule changes (ADR-0045): the list and the audit log; the queries shown now are fetched
 * again and awaited.
 */
export async function refreshLinkCardDomainRules(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["get", "/admin/link_card_domains"] }),
    refreshAuditLog(queryClient),
  ]);
}
