// SPDX-License-Identifier: AGPL-3.0-only

// A log of a record (the audit log, a webhook's deliveries, a job's attempts), as the moderation, administration and
// operations screens show it: its entries, the newest first, as the notices are. Each is the glyph of its kind, the
// sentence of who did what to what, the app's, its names isolated (`SentenceName`), with the time at the end of its
// first line; the muted line about it under it (a reason, a response's status); and, where it has more (the payload,
// the request), Details at that line's end, a button of the text with a chevron that turns as it opens, which shows
// that under it. Entries stand apart by space alone, no line between them.
//
// For assistive technology: the log is an ordered list named by the screen; Details is a disclosure's button, its
// state read with it, and what it folds stays out of the page while folded.

import type { ReactNode, RefCallback } from "react";
import { Disclosure as AriaDisclosure, DisclosurePanel } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { TextButton } from "./button";
import { Stamp } from "./entry";
import { Icon, type IconName } from "./icons";
import { Row, Stack } from "./layout";
import { stringsIn, useReader } from "./locale";
import { usePreviewExpanded } from "./preview";
import { actionJoint } from "./sentence";

const log = tv({
  slots: {
    list: "list-none p-0",
    // At the icon's width, so that every sentence starts in one column, as a notice's.
    glyph: "w-icon-action text-muted-text",
    // A sentence that wraps breaks between phrases, never inside a word of ja, its lines kept even.
    text: "[text-wrap:pretty] [word-break:auto-phrase]",
    about: "text-caption text-muted-text [word-break:auto-phrase]",
    toggle: "whitespace-nowrap",
    // The chevron after Details points down while folded and up while open, turned at once as the panel shows.
    chevron: "[[aria-expanded=true]_&]:rotate-180",
    // What it folds stands a little apart from the lines over it, as a group of its own.
    panel: "pt-screen-air [&[hidden]]:hidden",
  },
});

/** A log's entries, the newest first, each a `LogEntry`, named `label` (Audit log, Deliveries). */
export function LogList({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Stack as="ol" gap="feed-tight" aria-label={label} className={log().list()}>
      {children}
    </Stack>
  );
}

export type LogEntryProps = {
  /** The glyph of its kind (moderation, delete, error), muted at its start. */
  icon: IconName;
  /** When it happened, as a Date or an ISO 8601 string: the relative time, the full date in its tooltip. */
  date: Date | string;
  /** The sentence of who did what to what, the app's, its names isolated (`SentenceName`). */
  children: ReactNode;
  /** What it says further, muted under the sentence: a reason, a response's status and how long it took. */
  about?: ReactNode;
  /** What more there is, shown under Details: the payload, the request and its response, the facts of the target. */
  detail?: ReactNode;
  /** The ref of the entry, for a log that grows under Show more (`useAddedFocus`). */
  itemRef?: RefCallback<HTMLElement> | undefined;
};

/**
 * An entry of a log: its glyph, the sentence of who did what to what with the time at the end of its first line, the
 * muted line about it with Details at its end, which shows what more there is under it.
 */
export function LogEntry({ icon, date, children, about, detail, itemRef }: LogEntryProps) {
  const s = log();
  const { lang } = useReader(undefined);
  const strings = stringsIn(lang);
  const previewExpanded = usePreviewExpanded();
  const said = about !== undefined && about !== null;
  const more = detail !== undefined && detail !== null;
  const lines = (
    <>
      <p className={s.text()}>
        {/* The time floats at the first line's end, and the lines under it run to the column's end. */}
        <Row as="span" inline line align="center" float="end" spaceBefore="entry-caption">
          <Stamp date={date} />
        </Row>
        {children}
      </p>
      {(said || more) && (
        <p className={s.about()}>
          {about}
          {more && (
            <>
              {/* The dot between the line and Details is shown alone: they are read apart already. */}
              {said && <span aria-hidden="true">{actionJoint(about)}</span>}
              {/* Its few words hold together: the line wraps before it, never inside it. */}
              <span className={s.toggle()} lang={strings.lang}>
                <TextButton slot="trigger">
                  <Row as="span" inline align="center" gap="hair">
                    {strings.t("log.details")}
                    <Row as="span" inline rigid className={s.chevron()}>
                      <Icon name="expand" place="caption" />
                    </Row>
                  </Row>
                </TextButton>
              </span>
            </>
          )}
        </p>
      )}
    </>
  );
  return (
    <Row as={<li ref={itemRef} />} align="start" gap="entry-caption">
      <Row as="span" inline line rigid align="center" justify="center" className={s.glyph()}>
        <Icon name={icon} place="action" />
      </Row>
      {more ? (
        <Stack grow gap="entry-line" as={<AriaDisclosure defaultExpanded={previewExpanded} />}>
          {lines}
          <DisclosurePanel className={s.panel()}>{detail}</DisclosurePanel>
        </Stack>
      ) : (
        <Stack grow gap="entry-line">
          {lines}
        </Stack>
      )}
    </Row>
  );
}
