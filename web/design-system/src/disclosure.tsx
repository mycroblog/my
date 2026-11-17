// SPDX-License-Identifier: AGPL-3.0-only

// The disclosure, for a post's content warning, on React Aria Components' Disclosure: a summary on a raised surface,
// an optional muted glyph at its start, and a quiet button at its end that shows or hides what follows it. The
// button carries aria-expanded and aria-controls, Enter and Space toggle it, and it is described by the summary, so
// that "Show" is read with what it shows. What it folds stays out of the layout, of assistive technology and of the
// page's search while folded: a reader who chose not to see it is not shown it by a find. The button's label is the
// design system's own string, in the reader's locale; it lays out in the reader's direction (locale.ts).

import type { ComponentProps, HTMLAttributes, ReactNode } from "react";
import { useId } from "react";
import { Disclosure as AriaDisclosure, DisclosurePanel } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Button } from "./button";
import { Icon, type IconName } from "./icons";
import { Box, Row } from "./layout";
import { stringsIn, useReader } from "./locale";
import { usePreviewExpanded } from "./preview";

export type DisclosureProps = {
  /** What the folded part is about (a content warning's text); it stays in sight. */
  summary: ReactNode;
  /** A glyph before the summary, by its meaning (report for a content warning). */
  icon?: IconName | undefined;
  /** Shown at first; folded by default. */
  defaultExpanded?: boolean | undefined;
  /** The reader's locale, for the button's label; the I18nProvider's by default. */
  locale?: string | undefined;
  /** What the disclosure folds. */
  children: ReactNode;
};

const disclosure = tv({
  slots: {
    // Beside the toggle's label, what the frame of the page gives it: on a phone the app's frame makes the toggle's
    // column its label's width and little more, so that the warning wraps less (`--disclosure-toggle-inline`, shell.tsx).
    summary: [
      "rounded-control bg-surface-raised",
      "py-disclosure-padding ps-disclosure-start pe-disclosure-padding",
      "[&>button]:px-disclosure-toggle",
    ],
    glyph: "h-control-timeline text-muted-text",
    // Centred on the toggle's height: its first line sits level with the glyph and the label.
    text: "text-body py-disclosure-text",
    panel: "[&[hidden]]:hidden",
  },
});

/** The disclosure's grid: the summary over what it folds. */
function DisclosureFrame(props: Omit<ComponentProps<typeof AriaDisclosure>, "className">) {
  return <AriaDisclosure {...props} className="grid grid-cols-1 gap-disclosure-gap" />;
}

/** The summary's line: the glyph and the toggle line up with its first line, so a long warning wraps below them. */
function DisclosureSummary({ className, ...rest }: HTMLAttributes<HTMLDivElement> & { className: string }) {
  return <div {...rest} className={`${className} flex items-start gap-disclosure-gap`} />;
}

export function Disclosure({ summary, icon, defaultExpanded = false, locale, children }: DisclosureProps) {
  const reader = useReader(locale);
  const { t, lang } = stringsIn(reader.lang);
  const summaryId = useId();
  const previewExpanded = usePreviewExpanded();
  const s = disclosure();
  return (
    <DisclosureFrame defaultExpanded={defaultExpanded || previewExpanded} dir={reader.direction}>
      {({ isExpanded }) => (
        <>
          <DisclosureSummary className={s.summary()}>
            {icon !== undefined && (
              <Row as="span" align="center" className={s.glyph()}>
                <Icon name={icon} place="caption" />
              </Row>
            )}
            <Box id={summaryId} grow className={s.text()}>
              {summary}
            </Box>
            <Button slot="trigger" kind="quiet" size="timeline" aria-describedby={summaryId}>
              <span lang={lang}>{t(isExpanded ? "disclosure.hide" : "disclosure.show")}</span>
            </Button>
          </DisclosureSummary>
          <DisclosurePanel className={s.panel()}>{children}</DisclosurePanel>
        </>
      )}
    </DisclosureFrame>
  );
}
