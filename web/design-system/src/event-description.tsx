// SPDX-License-Identifier: AGPL-3.0-only

// The description of an event (ADR-0161 §5, §6, FR-951): plain text as its organizer wrote it, its lines kept, in its
// own direction, where only the URLs are links. A link leaves the site: it opens in a new tab, tells the page it leads
// to nothing of where it came from, follows in the links' color with the glyph of leaving after it, and says to
// assistive technology that it opens its site in a new tab, as a place's link does (event.tsx).

import { tv } from "tailwind-variants/lite";
import type { OutLink } from "./event";
import { Icon } from "./icons";
import { Box, Spoken } from "./layout";
import { stringsIn, useReader } from "./locale";

/** A part of a description: text as it was written, or a URL it holds, as a link that leaves the site. */
export type DescriptionPart = { readonly text: string } | { readonly link: OutLink };

const description = tv({
  slots: {
    root: "text-body text-text whitespace-pre-line",
    // A URL breaks anywhere, so that a long one never widens the column.
    link: [
      "rounded-control text-accent-text underline underline-offset-[0.15em] outline-hidden [overflow-wrap:anywhere]",
      "focus-visible:shadow-focus",
    ],
  },
});

/** An event's description, `parts` in their order: its text, and each URL in it a link that says it leaves the site. */
export function EventDescription({ parts }: { parts: readonly DescriptionPart[] }) {
  const s = description();
  const { lang } = useReader(undefined);
  const { t } = stringsIn(lang);
  return (
    <Box as="p" className={s.root()} dir="auto">
      {parts.map((part, i) =>
        "text" in part ? (
          part.text
        ) : (
          <a key={i} href={part.link.href} target="_blank" rel="nofollow noopener noreferrer" className={s.link()}>
            {part.link.text}
            <Box as="span" display="inline-block" baseline="glyph" spaceBefore="entry-caption">
              <Icon name="open" place="caption" />
            </Box>
            <Spoken> {t("event.new_tab", { host: part.link.host })}</Spoken>
          </a>
        ),
      )}
    </Box>
  );
}
