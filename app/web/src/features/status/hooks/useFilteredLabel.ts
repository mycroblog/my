// SPDX-License-Identifier: AGPL-3.0-only
// What a post that the viewer's keyword filters fold says in its place (ADR-0157 §2, §10): the server matched the post
// and put the filters with the action `warn` in its `filtered`; the client only shows their names. It returns no JSX.
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import type { Status } from "../api/status";

/**
 * A filter's name inside a sentence of the UI's language. It is the user's text in any script, so it is wrapped in
 * Unicode directional isolates (FSI … PDI), and a right-to-left name does not reorder the sentence (web-client Y10).
 */
function isolate(title: string): string {
  return `⁨${title}⁩`;
}

/**
 * The line that stands in for a post its filters fold ("Filtered: Spoilers and Politics"), or null for a post that
 * matched none. Several filters' names are joined as the UI's language joins a list.
 */
export function useFilteredLabel(filtered: Status["filtered"]): string | null {
  const t = useT();
  const intl = useIntl();
  if (filtered.length === 0) return null;
  const titles = intl.formatList(
    filtered.map((result) => isolate(result.filter.title)),
    { type: "conjunction" },
  );
  return t("client:filters.folded", { titles });
}
