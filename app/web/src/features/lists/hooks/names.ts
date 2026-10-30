// SPDX-License-Identifier: AGPL-3.0-only
import { useIntl } from "react-intl";
import { type Account, listNameLimit, maxListMembers, maxLists } from "../api/lists";

/**
 * A name inside a sentence of the UI's language (a list's or an account's). It is the user's text in any script, so
 * it is wrapped in Unicode directional isolates (FSI … PDI): a right-to-left name does not reorder the sentence around
 * it (web-client/README.md Y10). The same as features/profile's, which this feature does not import, since the profile
 * shows this feature's dialog.
 */
export function isolate(name: string): string {
  return `⁨${name}⁩`;
}

/** The limits of lists (ADR-0122 §5) as the messages take them: numbers formatted in the UI's locale ("1,000"). */
export function useLimits(): { lists: string; members: string; name: string } {
  const intl = useIntl();
  return {
    lists: intl.formatNumber(maxLists),
    members: intl.formatNumber(maxListMembers),
    name: intl.formatNumber(listNameLimit),
  };
}

/** The name an account goes by in the UI. */
export function accountName(account: Account): string {
  return account.display_name || account.username;
}
