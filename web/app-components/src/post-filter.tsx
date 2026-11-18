// SPDX-License-Identifier: AGPL-3.0-only

// The fold of what one of the reader's keyword filters matched (ADR-0157 §10): the server matches and marks the status
// (`filtered`), and the client folds it under a line that names the filters, with the content warning's own button
// (Disclosure), which shows it and folds it again. Nothing of what it folds is drawn until the reader shows it: none
// of its media is asked for, and none of its actions answers a key. The line is the one sentence wherever a status is
// drawn: in a post's place, in a quoted post's, and in the place of the excerpt of a notice about one.

import { Box, Disclosure, useLocale } from "@my/design-system";
import { Fragment, type ReactNode } from "react";
import { format } from "./format";
import { stringsIn } from "./locale";
import type { PostFiltered } from "./status";

/**
 * The sentence that names the filters a status matched ("Matches filter “Spoilers”"), in the reader's language: every
 * filter by the name the reader gave it, isolated in its own direction, as a list of the language. `id` lets it name
 * what it stands for.
 */
export function FilterSentence({ filtered, id }: { filtered: readonly PostFiltered[]; id?: string | undefined }) {
  const { t, lang } = stringsIn(useLocale().locale);
  const names = filtered.map(({ filter }) => format(t, "post.filter_name", { name: <bdi>{filter.title}</bdi> }));
  // The list's own words ("and", "、") stand between the names, each name in the place of its index.
  const list = new Intl.ListFormat(lang, { type: "conjunction" })
    .formatToParts(names.map((_, i) => String(i)))
    .map((part, i) => <Fragment key={i}>{part.type === "element" ? names[Number(part.value)] : part.value}</Fragment>);
  return (
    // Where it wraps, it breaks between its phrases, never inside a word of ja ("に一致").
    <Box as="span" id={id} lang={lang} breaks="phrase">
      {format(t, "post.filtered", { names: list }, [], { count: filtered.length })}
    </Box>
  );
}

export type FilterFoldProps = {
  /** The filters the status matched (its `filtered`), one at least. */
  filtered: readonly PostFiltered[];
  /** The id of its line, by which the line names what it folds. */
  id?: string | undefined;
  /** Whether what it folds shows, and what its button asks for. */
  shown: boolean;
  onShownChange: (shown: boolean) => void;
  /** What the filters matched (a post, a quoted post), drawn only while shown. */
  children: ReactNode;
};

/** What a status's filters matched, folded under the line that names them until the reader shows it. */
export function FilterFold({ filtered, id, shown, onShownChange, children }: FilterFoldProps) {
  return (
    <Disclosure
      summary={<FilterSentence filtered={filtered} id={id} />}
      icon="hidden"
      expanded={shown}
      onExpandedChange={onShownChange}
    >
      {shown && children}
    </Disclosure>
  );
}
