// SPDX-License-Identifier: AGPL-3.0-only

// The parts of the reader's drafts and scheduled posts (ADR-0156): a list of them, a row each, which opens the draft
// in the composer and holds its other actions in a menu at its end; the time a post goes out, in the reader's locale
// and time zone; and the form that sets that time, within the server's limits. Everything sits on the page's surface,
// apart by space; a row takes a surface on hover and the ring on focus, as a row that opens something does. Every word
// is the caller's. It is composed of the design system's parts alone, laid out and painted by their props (ADR-0025
// §4).

import { useId, type ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { Button } from "./button";
import { DateTimeField, FieldError } from "./form-page";
import { Icon, type IconName } from "./icons";
import { Box, Row, Spoken, Stack } from "./layout";
import { PressRow } from "./list";
import { useReader } from "./locale";
import { useHeaded } from "./menu";

/** How many a section of the drafts' list holds, muted, in its heading's sentence (Scheduled 3). */
export function DraftCount({ count }: { count: number }) {
  const { locale } = useLocale();
  return (
    <Box as="span" weight="normal" nowrap tone="muted" numeric>
      {new Intl.NumberFormat(locale).format(count)}
    </Box>
  );
}

/** The sections of the drafts' list, one under another, a group's space apart and from what is above them. */
export function DraftSections({ children }: { children: ReactNode }) {
  return (
    <Stack gap="entry-room" spaceAbove="screen-group-gap">
      {children}
    </Stack>
  );
}

/**
 * A section of the drafts' list: its heading (with a `DraftCount`), a step above its rows, with a line under it (the
 * zone), then its rows.
 */
export function DraftSection({ title, note, children }: { title: ReactNode; note?: ReactNode; children: ReactNode }) {
  return (
    <Stack as="section" gap="entry-line">
      <Stack gap="hair">
        <Box as="h3" text="body" weight="strong" tone="text">
          {title}
        </Box>
        {note}
      </Stack>
      {children}
    </Stack>
  );
}

/** That a list of drafts has none, muted, where its first heading would stand. */
export function DraftsEmpty({ children }: { children: string }) {
  return (
    <Box as="p" tone="muted">
      {children}
    </Box>
  );
}

/** The rows of drafts or of scheduled posts, one under another. */
export function DraftList({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Stack as="ul" gap="entry-line" plain aria-label={label}>
      {children}
    </Stack>
  );
}

export type DraftMark = {
  icon: IconName;
  /** What it is, for assistive technology and on hover (2 images, a poll). */
  label: string;
  /** How many, when more than one may be (images). */
  count?: number | undefined;
};

export type DraftRowProps = {
  /** A line over the text: when it is scheduled for, or why it was not posted. */
  lead?: { kind: "scheduled" | "failed"; text: ReactNode } | undefined;
  /** The start of the text; empty for a draft of media or a poll alone. */
  text: string;
  /** What stands for an empty text (No text). */
  blank: string;
  /** The content warning, which stands before the text as a post's band does. */
  warning?: string | undefined;
  lang?: string | undefined;
  /** What the draft carries besides its text: images, a poll. */
  marks: readonly DraftMark[];
  /** When it was last saved, in a sentence whose time is a `RelativeTime` (Saved 2h ago, 2時間前に保存). */
  saved: ReactNode;
  /** What the row does, read before what it shows (Open this draft). */
  openLabel: string;
  onOpen: () => void;
  /** The menu of its other actions, at the row's end (a `MenuButton`). */
  menu: ReactNode;
};

/**
 * A draft or a scheduled post: the time it goes out or why it did not, the start of its text after its warning, then
 * what it carries and when it was saved, muted; the whole row opens it in the composer, and its menu sits at the end.
 */
export function DraftRow({ lead, text, blank, warning, lang, marks, saved, openLabel, onOpen, menu }: DraftRowProps) {
  const hasWarning = warning !== undefined && warning !== "";
  return (
    <Row as="li" align="start" gap="entry-caption">
      {/* Named by what it says after what it does, as it shows, so that each row reads apart and its words once. */}
      <PressRow bleed="start" gap="entry-line" grow onPress={onOpen}>
        <Spoken>{openLabel}</Spoken>
        {lead !== undefined && (
          <Row
            as="span"
            align="start"
            gap="entry-caption"
            text="caption"
            tone="text"
            weight={lead.kind === "scheduled" ? "label" : undefined}
          >
            <Row as="span" inline line rigid align="center" tone={lead.kind === "scheduled" ? "muted" : "text"}>
              <Icon name={lead.kind === "failed" ? "error" : "schedule"} place="caption" />
            </Row>
            <span>{lead.text}</span>
          </Row>
        )}
        <Box as="span" lines={2} breaks="anywhere" keeps="lines" lang={lang} dir="auto">
          {hasWarning && (
            <Box as="span" tone="muted">
              <Row as="span" inline baseline="glyph" spaceAfter="screen-tab-gap">
                <Icon name="hidden" place="caption" />
              </Row>
              {warning}
            </Box>
          )}
          {hasWarning && text !== "" && " "}
          {text === "" ? (
            hasWarning ? null : (
              <Box as="span" tone="muted">
                {blank}
              </Box>
            )
          ) : (
            text
          )}
        </Box>
        <Row as="span" wrap align="center" gapX="entry-avatar" gapY="0" text="caption" tone="muted">
          {marks.map((m) => (
            <Row
              key={m.icon}
              as="span"
              inline
              align="center"
              gap="screen-tab-gap"
              role="img"
              aria-label={m.label}
              title={m.label}
            >
              <Icon name={m.icon} place="caption" />
              {m.count !== undefined && m.count > 1 && <span aria-hidden="true">{m.count}</span>}
            </Row>
          ))}
          <span>{saved}</span>
        </Row>
      </PressRow>
      <Box as="span" padTop="screen-tab-gap">
        {menu}
      </Box>
    </Row>
  );
}

/**
 * A moment as a scheduled post's time reads: the weekday, the date and the time in the reader's locale and zone; the
 * zone itself is `readerZone`'s, written once where the times are.
 */
export function scheduleTime(date: Date | string, locale: string): string {
  return new Intl.DateTimeFormat(locale, {
    weekday: "short",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(new Date(date));
}

/** The reader's time zone, as its name and its offset read (Asia/Tokyo, GMT+9). */
export function readerZone(locale: string, at = new Date()): string {
  const name = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const offset =
    new Intl.DateTimeFormat(locale, { timeZoneName: "shortOffset" })
      .formatToParts(at)
      .find((p) => p.type === "timeZoneName")?.value ?? "";
  return offset === "" || offset === name ? name : `${name} · ${offset}`;
}

/**
 * The time a post goes out, in the reader's locale; isolated, so that it keeps its order in either direction, and
 * never broken inside, so that a line wraps around it.
 */
export function ScheduleTime({ date }: { date: Date | string }) {
  const { locale } = useLocale();
  return (
    <Box as="bdi" nowrap>
      <time dateTime={new Date(date).toISOString()}>{scheduleTime(date, locale)}</time>
    </Box>
  );
}

/** The reader's time zone (`readerZone`), isolated and never broken inside, as `ScheduleTime` is. */
export function ScheduleZone() {
  const { locale } = useLocale();
  return (
    <Box as="bdi" nowrap>
      {readerZone(locale)}
    </Box>
  );
}

/** A local date and time as a `datetime-local` field holds it (2026-11-16T09:00). */
export const localInput = (d: Date) => {
  const p = (n: number) => String(n).padStart(2, "0");
  return `${String(d.getFullYear())}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
};

export type ScheduleFormProps = {
  /** The form's caption (Schedule). */
  title: string;
  /** The field's label, for assistive technology (Date and time). */
  label: string;
  /** The time in the field, as a `datetime-local` holds it (2026-11-16T09:00); empty for none yet. */
  value: string;
  onChange: (value: string) => void;
  /** The nearest and the farthest time the server takes, now. */
  min: Date;
  max: Date;
  /** The reader's time zone, under the field (Asia/Tokyo · GMT+9). */
  zone: string;
  /** The limits, in words (From 5 minutes to 366 days from now). */
  hint: string;
  /** What is wrong with the time chosen, under the field, in place of the hint. */
  error?: string | undefined;
  /** Sets the time, and its label (Set time). */
  confirm: { label: string; onPress: () => void };
  /** Takes the time away, while one is set, and its label (Post now instead). */
  clear?: { label: string; onPress: () => void } | undefined;
};

/**
 * The time a post goes out: under its caption (none in a sheet, whose head names it), one field of the date and the
 * time, the reader's zone and the server's limits (or what is wrong with the time) under it, then a clear and the
 * confirmation at the end.
 */
export function ScheduleForm({
  title,
  label,
  value,
  onChange,
  min,
  max,
  zone,
  hint,
  error,
  confirm,
  clear,
}: ScheduleFormProps) {
  // In a sheet, its head names the form; else it is of its own width, which a popover sized to what it holds takes.
  const headed = useHeaded();
  const id = useId();
  const { direction } = useReader(undefined);
  return (
    <Stack
      gap="entry-line"
      pad="screen-tab-gap"
      full={headed}
      max={headed ? undefined : "popover-form"}
      dir={direction}
    >
      {!headed && (
        <Row as="p" inline align="center" gap="entry-caption" text="caption" tone="muted">
          <Icon name="schedule" place="caption" />
          {title}
        </Row>
      )}
      <Stack gap="screen-tab-gap">
        <DateTimeField
          label={label}
          value={value}
          onChange={onChange}
          min={localInput(min)}
          max={localInput(max)}
          invalid={Boolean(error)}
          describedBy={`${id}-zone ${id}-note`}
        />
        <Box as="p" id={`${id}-zone`} text="caption" tone="muted">
          {zone}
        </Box>
        {error ? (
          <FieldError id={`${id}-note`}>{error}</FieldError>
        ) : (
          <Box as="p" id={`${id}-note`} text="caption" tone="muted">
            {hint}
          </Box>
        )}
      </Stack>
      <Row wrap align="center" justify="end" gap="entry-caption" padTop="screen-tab-gap">
        {clear && (
          <Button kind="quiet" size="timeline" onPress={clear.onPress}>
            {clear.label}
          </Button>
        )}
        <Button kind="secondary" size="timeline" onPress={confirm.onPress}>
          {confirm.label}
        </Button>
      </Row>
    </Stack>
  );
}
