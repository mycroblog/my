// SPDX-License-Identifier: AGPL-3.0-only
// The rules of the link card crawler as a holder of `manage_settings` keeps them (ADR-0062, ADR-0058 §1): per domain
// and its subdomains, whether the crawler fetches nothing from it or no images, with a note of the administrators';
// setting a rule (a new one, or the domain's again) and removing one. The list is read whole (the server keeps at most
// 1,000 rules), and every write reads it again.
import type { components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";
import { readOf, type ScreenRead } from "./reads";
import { removeCached, writeCached } from "./written";

/** The rule of a domain. */
export type LinkCardRule = components["schemas"]["LinkCardDomainRule"];
/** What the crawler does not do on a rule's domain. */
export type LinkCardMode = components["schemas"]["LinkCardDomainMode"];
/** The rule to set on a domain. */
export type LinkCardRuleInput = components["schemas"]["SetLinkCardDomainRuleRequest"];

/** The modes of a rule, the stronger first. */
export const linkCardModes = ["no_fetch", "no_images"] as const satisfies readonly LinkCardMode[];
/** The most characters of a rule's note, and of a domain. */
export const linkCardNoteLimit = 5000;
export const linkCardDomainLimit = 253;

/** The rules, by domain. */
function rulesQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/link_card_domains");
}

/** Reads the rules, for a route's loader. */
export async function loadLinkCardRules(data: AppData): Promise<void> {
  await data.queryClient.query({ ...rulesQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** The rules, and where their read is. */
export function useLinkCardRules(): ScreenRead & { readonly rules: readonly LinkCardRule[] | undefined } {
  const query = useQuery(rulesQuery(useAppData()));
  return { ...readOf([query]), rules: query.data?.items };
}

/** The rule of `domain`, found in the list; `missing` once the list is read without it. */
export function useLinkCardRule(
  domain: string,
): ScreenRead & { readonly rule: LinkCardRule | undefined; readonly missing: boolean } {
  const query = useQuery(rulesQuery(useAppData()));
  const rule = query.data?.items.find((r) => r.domain === domain);
  return { ...readOf([query]), rule, missing: query.data !== undefined && rule === undefined };
}

/** The writes of the rules; each rejects with the server's problem (400 for a domain that is not a host). */
export type LinkCardRuleWrites = {
  /** Sets the rule of `domain`, replacing the one it has; resolves with it, its domain in canonical form. */
  readonly set: (domain: string, rule: LinkCardRuleInput) => Promise<LinkCardRule>;
  /** Removes the rule of `domain`; a domain without one is done all the same. */
  readonly remove: (domain: string) => Promise<void>;
};

type RuleList = components["schemas"]["LinkCardDomainRuleList"];

/** The writes of the rules, for a screen. */
export function useLinkCardRuleWrites(): LinkCardRuleWrites {
  const data = useAppData();
  const key = rulesQuery(data).queryKey;
  return {
    set: async (domain, input) => {
      const { data: rule } = await data.api.PUT("/admin/link_card_domains/{domain}", {
        params: { path: { domain } },
        body: input,
      });
      if (rule === undefined) throw new Error("app-data: a rule of the link cards answered without the rule");
      await writeCached<RuleList>(data, key, (now) => ({
        items: [...now.items.filter((r) => r.domain !== rule.domain), rule].toSorted((a, b) =>
          a.domain.localeCompare(b.domain),
        ),
      }));
      void data.queryClient.invalidateQueries({ queryKey: key });
      return rule;
    },
    remove: async (domain) => {
      await removeCached<RuleList>(
        data,
        key,
        data.api.DELETE("/admin/link_card_domains/{domain}", { params: { path: { domain } } }),
        (now) => ({ items: now.items.filter((r) => r.domain !== domain) }),
      );
      void data.queryClient.invalidateQueries({ queryKey: key });
    },
  };
}
