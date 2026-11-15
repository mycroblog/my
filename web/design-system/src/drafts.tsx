// SPDX-License-Identifier: AGPL-3.0-only

// The parts of the reader's drafts and scheduled posts (ADR-0156): a list of them, a row each, which opens the draft
// in the composer and holds its other actions in a menu at its end; the time a post goes out, in the reader's locale
// and time zone; and the form that sets that time, within the server's limits. Everything sits on the page's surface,
// apart by space; a row takes a surface on hover and the ring on focus, as a row that opens something does. Every word
// is the caller's.

import { useId, type ReactNode } from "react";
import { Button as AriaButton, useLocale } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Button } from "./button";
import { FieldError } from "./form-page";
import { Icon, type IconName } from "./icons";
import { useReader } from "./locale";

const sections = tv({
  slots: {
    // Apart from the back link above by a group's space, as the sections are from each other.
    root: "mt-screen-group-gap grid gap-entry-room",
    section: "grid gap-entry-line",
    // The heading and the line under it (the zone) together, tight.
    head: "grid gap-[0.125rem]",
    title: "m-0 text-body font-strong text-text",
    // How many, muted beside the heading, as a list's title says how many members it has.
    count: "font-normal whitespace-nowrap text-muted-text tabular-nums",
    empty: "m-0 text-muted-text",
  },
});

/** How many a section of the drafts' list holds, muted, in its heading's sentence (Scheduled 3). */
export function DraftCount({ count }: { count: number }) {
  const { locale } = useLocale();
  return <span className={sections().count()}>{new Intl.NumberFormat(locale).format(count)}</span>;
}

/** The sections of the drafts' list, one under another, a group's space apart and from what is above them. */
export function DraftSections({ children }: { children: ReactNode }) {
  return <div className={sections().root()}>{children}</div>;
}

/**
 * A section of the drafts' list: its heading (with a `DraftCount`), a step above its rows, with a line under it (the
 * zone), then its rows.
 */
export function DraftSection({ title, note, children }: { title: ReactNode; note?: ReactNode; children: ReactNode }) {
  const s = sections();
  return (
    <section className={s.section()}>
      <div className={s.head()}>
        <h3 className={s.title()}>{title}</h3>
        {note}
      </div>
      {children}
    </section>
  );
}

/** That a list of drafts has none, muted, where its first heading would stand. */
export function DraftsEmpty({ children }: { children: string }) {
  return <p className={sections().empty()}>{children}</p>;
}

const list = tv({ base: "m-0 grid list-none gap-entry-line p-0" });

/** The rows of drafts or of scheduled posts, one under another. */
export function DraftList({ label, children }: { label: string; children: ReactNode }) {
  return (
    <ul className={list()} aria-label={label}>
      {children}
    </ul>
  );
}

const row = tv({
  slots: {
    root: "flex items-start gap-entry-caption",
    open: [
      "-ms-screen-row-inline grid min-w-0 flex-1 cursor-pointer gap-entry-line rounded-control border-0 bg-transparent",
      "px-screen-row-inline py-screen-row-block text-start font-sans text-body text-text outline-hidden",
      "data-hovered:bg-surface-hover data-pressed:bg-surface-pressed data-focus-visible:shadow-focus",
    ],
    lead: "flex items-start gap-entry-caption text-caption",
    leadGlyph: "inline-flex h-[1lh] flex-none items-center",
    text: "line-clamp-2 [overflow-wrap:anywhere] whitespace-pre-line",
    warning: "text-muted-text [&>svg]:me-[0.25rem] [&>svg]:inline [&>svg]:align-[-0.125em]",
    blank: "text-muted-text",
    foot: "flex flex-wrap items-center gap-x-entry-avatar gap-y-0 text-caption text-muted-text",
    mark: "inline-flex items-center gap-[0.25rem]",
    more: "pt-[0.25rem]",
  },
  variants: {
    // A line that says what went wrong (a failed publish) in the text's color, by its glyph; a time in the text's
    // color too, at the label's weight: the one thing a scheduled post is found by.
    lead: {
      failed: { lead: "text-text", leadGlyph: "text-text" },
      scheduled: { lead: "font-label text-text", leadGlyph: "text-muted-text" },
    },
  },
});

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
  const s = row({ lead: lead?.kind });
  return (
    <li className={s.root()}>
      {/* Named by what it says after what it does, as it shows, so that each row reads apart and its words once. */}
      <AriaButton className={s.open()} onPress={onOpen}>
        <span className="sr-only">{openLabel}</span>
        {lead !== undefined && (
          <span className={s.lead()}>
            <span className={s.leadGlyph()}>
              <Icon name={lead.kind === "failed" ? "error" : "schedule"} place="caption" />
            </span>
            <span>{lead.text}</span>
          </span>
        )}
        <span className={s.text()} lang={lang} dir="auto">
          {warning !== undefined && warning !== "" && (
            <span className={s.warning()}>
              <Icon name="hidden" place="caption" />
              {warning}
            </span>
          )}
          {warning !== undefined && warning !== "" && text !== "" && " "}
          {text === "" ? (
            warning === undefined || warning === "" ? (
              <span className={s.blank()}>{blank}</span>
            ) : null
          ) : (
            text
          )}
        </span>
        <span className={s.foot()}>
          {marks.map((m) => (
            <span key={m.icon} className={s.mark()} role="img" aria-label={m.label} title={m.label}>
              <Icon name={m.icon} place="caption" />
              {m.count !== undefined && m.count > 1 && <span aria-hidden="true">{m.count}</span>}
            </span>
          ))}
          <span>{saved}</span>
        </span>
      </AriaButton>
      <span className={s.more()}>{menu}</span>
    </li>
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
    <bdi className="whitespace-nowrap">
      <time dateTime={new Date(date).toISOString()}>{scheduleTime(date, locale)}</time>
    </bdi>
  );
}

/** The reader's time zone (`readerZone`), isolated and never broken inside, as `ScheduleTime` is. */
export function ScheduleZone() {
  const { locale } = useLocale();
  return <bdi className="whitespace-nowrap">{readerZone(locale)}</bdi>;
}

const form = tv({
  slots: {
    root: "grid min-w-[min(17rem,100%)] gap-entry-line p-[0.25rem]",
    caption: "m-0 inline-flex items-center gap-entry-caption text-caption text-muted-text",
    field: "grid gap-[0.25rem]",
    input: [
      "box-border h-control-form w-full min-w-0 rounded-control border-0 bg-surface-raised px-control-timeline-inline",
      "font-sans text-body text-text tabular-nums outline-hidden [color-scheme:inherit] focus-visible:shadow-focus",
      "aria-invalid:outline-solid aria-invalid:outline-[length:var(--form-page-invalid)]",
      "aria-invalid:-outline-offset-[var(--form-page-invalid)] aria-invalid:outline-text",
    ],
    hint: "m-0 text-caption text-muted-text",
    row: "flex flex-wrap items-center justify-end gap-entry-caption pt-[0.25rem]",
  },
});

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
 * The time a post goes out: under its caption, one field of the date and the time, the reader's zone and the server's
 * limits (or what is wrong with the time) under it, then a clear and the confirmation at the end.
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
  const s = form();
  const id = useId();
  const { direction } = useReader(undefined);
  return (
    <div className={s.root()} dir={direction}>
      <p className={s.caption()}>
        <Icon name="schedule" place="caption" />
        {title}
      </p>
      <div className={s.field()}>
        <input
          type="datetime-local"
          className={s.input()}
          aria-label={label}
          aria-invalid={error ? true : undefined}
          aria-describedby={`${id}-zone ${id}-note`}
          value={value}
          min={localInput(min)}
          max={localInput(max)}
          onChange={(e) => {
            onChange(e.target.value);
          }}
        />
        <p id={`${id}-zone`} className={s.hint()}>
          {zone}
        </p>
        {error ? (
          <FieldError id={`${id}-note`}>{error}</FieldError>
        ) : (
          <p id={`${id}-note`} className={s.hint()}>
            {hint}
          </p>
        )}
      </div>
      <div className={s.row()}>
        {clear && (
          <Button kind="quiet" size="timeline" onPress={clear.onPress}>
            {clear.label}
          </Button>
        )}
        <Button kind="secondary" size="timeline" onPress={confirm.onPress}>
          {confirm.label}
        </Button>
      </div>
    </div>
  );
}
