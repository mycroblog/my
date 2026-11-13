// SPDX-License-Identifier: AGPL-3.0-only

// The disclosure, for a post's content warning, on React Aria Components' Disclosure: a summary on a raised surface,
// an optional muted glyph at its start, and a quiet button at its end that shows or hides what follows it. The
// button carries aria-expanded and aria-controls, Enter and Space toggle it, and it is described by the summary, so
// that "Show" is read with what it shows. What it folds stays out of the layout, of assistive technology and of the
// page's search while folded: a reader who chose not to see it is not shown it by a find. The button's label is the
// design system's own string, in the reader's locale.

import type { ReactNode } from "react";
import { useId } from "react";
import { Disclosure as AriaDisclosure, DisclosurePanel } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Button } from "./button";
import { Icon, type IconName } from "./icons";
import { pageLocale, stringsIn } from "./locale";

export type DisclosureProps = {
  /** What the folded part is about (a content warning's text); it stays in sight. */
  summary: ReactNode;
  /** A glyph before the summary, by its meaning (report for a content warning). */
  icon?: IconName | undefined;
  /** Shown at first; folded by default. */
  defaultExpanded?: boolean | undefined;
  /** The reader's locale, for the button's label; the page's language by default. */
  locale?: string | undefined;
  /** What the disclosure folds. */
  children: ReactNode;
};

const disclosure = tv({
  slots: {
    root: "grid grid-cols-1 gap-disclosure-gap",
    // The glyph and the toggle line up with the summary's first line, so a long warning wraps below them. On a narrow
    // screen (under 40rem, mobile) the toggle's column is its label's width and little more, so the warning wraps less.
    summary: [
      "flex items-start gap-disclosure-gap rounded-control bg-surface-raised",
      "py-disclosure-padding ps-disclosure-start pe-disclosure-padding",
      "max-[40rem]:[&>button]:px-disclosure-padding",
    ],
    glyph: "inline-flex h-control-timeline items-center text-muted-text",
    // Centred on the toggle's height: its first line sits level with the glyph and the label.
    text: "min-w-0 flex-1 text-body py-[calc((var(--control-timeline-height)-1lh)/2)]",
    panel: "[&[hidden]]:hidden",
  },
});

export function Disclosure({ summary, icon, defaultExpanded = false, locale, children }: DisclosureProps) {
  const { t, lang } = stringsIn(locale ?? pageLocale());
  const summaryId = useId();
  const s = disclosure();
  return (
    <AriaDisclosure defaultExpanded={defaultExpanded} className={s.root()}>
      {({ isExpanded }) => (
        <>
          <div className={s.summary()}>
            {icon !== undefined && (
              <span className={s.glyph()}>
                <Icon name={icon} place="caption" />
              </span>
            )}
            <div id={summaryId} className={s.text()}>
              {summary}
            </div>
            <Button slot="trigger" kind="quiet" size="timeline" aria-describedby={summaryId}>
              <span lang={lang}>{t(isExpanded ? "disclosure.hide" : "disclosure.show")}</span>
            </Button>
          </div>
          <DisclosurePanel className={s.panel()}>{children}</DisclosurePanel>
        </>
      )}
    </AriaDisclosure>
  );
}
