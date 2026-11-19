// SPDX-License-Identifier: AGPL-3.0-only

// The parts of an event (ADR-0161 §5–§9, FR-951, FR-952): its time, in the reader's zone and, where the event's own
// zone differs, in that one too, with the zone's place name; an event as a line of a list (the events' screen, a
// profile), its date in a box at the start; the reader's answer to it (going, maybe, not going) and to each date of
// its poll, the same three, each with a glyph of its own; the date poll as a table; and its places, one a line each
// by kind, a place the reader may not see showing its kind alone, a link that leaves the site saying so and where.
//
// The app formats every date and time, in the reader's locale and zone (zone.tsx), and gives the words: these parts
// lay them out. Their only words of their own are the frame of the event's own time ("Sat 9:00 PM (Tokyo)") and what a
// link that leaves the site says to assistive technology. Everything is in the text's color on the page's surface,
// with no hue but a link's: a chosen answer is knocked out of the text's color, as a toggle on is; a state the reader
// must know before going (cancelled, the date confirmed) is in the text's color, one that ends nothing (ended) muted.

import { createContext, use, useId, type HTMLAttributes, type ReactElement, type ReactNode } from "react";
import { Link } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Icon, type IconName } from "./icons";
import { Box, Layers, Pin, Row, Spoken, Stack } from "./layout";
import { stringsIn, useReader } from "./locale";
import { HeadcountMark, type Headcount } from "./voice";

export type EventTimeProps = {
  /** The start, as an ISO 8601 string, for the `<time>`. */
  dateTime: string;
  /** The time in the reader's zone, as the app formats it ("Sat, Nov 21, 9:00 PM – 11:00 PM"). */
  shown: string;
  /**
   * Where the event's own zone differs from the reader's: its time there, as the app formats it ("Sun 6:00 AM"), and
   * the zone's place name ("Tokyo").
   */
  own?: { shown: string; zone: string } | undefined;
};

/**
 * An event's time: the reader's, and under it, on a line of its own and muted, the event's own with its zone's place
 * name ("Sun 6:00 AM (Tokyo)"), so that the two never read as one.
 */
export function EventTime({ dateTime, shown, own }: EventTimeProps) {
  const { lang } = useReader(undefined);
  const { t } = stringsIn(lang);
  return (
    <Stack as="span">
      <Box as={<time dateTime={dateTime} />} breaks="phrase">
        {shown}
      </Box>
      {own !== undefined && (
        <Box as="span" tone="muted" breaks="phrase">
          {t("event.own_time", { time: own.shown, zone: own.zone })}
        </Box>
      )}
    </Stack>
  );
}

/** The kinds of an event's place: its voice chat room, a place in person, an address online. */
export type PlaceKind = "voice" | "physical" | "url";

const placeIcon: Record<PlaceKind, IconName> = { voice: "room", physical: "inPerson", url: "link" };

/** How an event ended: cancelled or ended, each marked alike, its glyph and its words in the text's color. */
export type EventState = { kind: "cancelled" | "ended"; label: string };

const line = tv({
  slots: {
    // The whole line is the link: the surface on hover and the ring on focus.
    root: [
      "rounded-control px-screen-row-inline py-screen-row-block text-text no-underline outline-hidden",
      "data-hovered:bg-surface-hover data-focus-visible:shadow-focus",
    ],
    box: "size-event-date-box rounded-control bg-surface-raised",
    month: "text-caption leading-none text-muted-text",
    day: "text-title leading-none tabular-nums",
    title: "text-body font-label [word-break:auto-phrase]",
    caption: "text-caption text-muted-text [word-break:auto-phrase]",
    state: "font-label text-text",
  },
  variants: {
    // Over: its title and its date muted, as what is past is (one color each: no class merges here).
    over: { false: { box: "text-text" }, true: { title: "text-muted-text", box: "text-muted-text" } },
  },
});

export type EventLineProps = {
  /** The event's page. */
  href: string;
  /** Its title, on two lines at most. */
  title: string;
  /**
   * The day it starts on, in the reader's zone, as the app formats it: the month in short ("Nov") and the day ("21").
   * Without it (its date is being polled), the glyph of events stands in the box.
   */
  date?: { month: string; day: string } | undefined;
  /** When it is (an `EventTime`), or that its date is being polled ("Date to be decided"). */
  time: ReactNode;
  /** The kinds of its places, each with its word ("Voice chat", "In person", "Online"), in that order. */
  places?: readonly { kind: PlaceKind; label: string }[] | undefined;
  /** Who organizes it, in the reader's words ("By Mika"). */
  organizer?: ReactNode;
  /** How many are going. */
  going?: Headcount | undefined;
  /** It was cancelled, or it ended. */
  state?: EventState | undefined;
};

/**
 * An event as a line of a list that is one link to it: the day it starts on in a box at the start, the month over the
 * day (the glyph of events while its date is polled); its title, then its state, a glyph and its words, where it was
 * cancelled or ended, and when it is; then the kinds of its places, each a glyph and its word, who organizes it and how
 * many are going. A cancelled or ended event mutes its title and
 * its date.
 */
export function EventLine({ href, title, date, time, places = [], organizer, going, state }: EventLineProps) {
  const s = line({ over: state !== undefined });
  return (
    <Row
      as={<Link href={href} className={s.root()} />}
      align="start"
      gap="screen-row-gap"
      bleedInline="screen-row-inline"
    >
      <Stack as="span" rigid pack="center" justify="center" gap="hair" className={s.box()}>
        {date === undefined ? (
          <Icon name="events" place="action" />
        ) : (
          <>
            <span className={s.month()}>{date.month}</span> <span className={s.day()}>{date.day}</span>
          </>
        )}
      </Stack>
      <Stack as="span" grow gap="hair">
        <Box as="span" lines={2} className={s.title()} dir="auto">
          {title}
        </Box>
        <Row as="span" wrap align="center" gapX="entry-caption" className={s.caption()}>
          {state !== undefined && (
            <Row as="span" rigid align="center" gap="hair" className={s.state()}>
              <Icon name={state.kind === "cancelled" ? "eventCancelled" : "eventEnded"} place="caption" />
              <span>{state.label}</span>
            </Row>
          )}
          {time}
        </Row>
        {(places.length > 0 || organizer !== undefined || going !== undefined) && (
          <Row as="span" wrap align="center" gapX="entry-caption" className={s.caption()}>
            {places.map((p) => (
              <Row key={p.kind} as="span" align="center" gap="hair">
                <Icon name={placeIcon[p.kind]} place="caption" />
                <span>{p.label}</span>
              </Row>
            ))}
            {organizer !== undefined && <span dir="auto">{organizer}</span>}
            {going !== undefined && <HeadcountMark count={going} />}
          </Row>
        )}
      </Stack>
    </Row>
  );
}

/** An answer to an event (going, maybe, not going) or to a date of its poll (can go, maybe, cannot). */
export type Answer = "yes" | "maybe" | "no";

const answers: readonly Answer[] = ["yes", "maybe", "no"];

const answerIcon: Record<Answer, IconName> = { yes: "answerYes", maybe: "answerMaybe", no: "answerNo" };

const choice = tv({
  slots: {
    // As a secondary button, on the raised surface; chosen, knocked out of the text's color, which no hover changes.
    segment: [
      "box-border min-h-control-form cursor-pointer rounded-control px-control-timeline-inline py-control-form-line",
      "bg-surface-raised font-sans text-body font-label text-text outline-hidden",
      "has-[input:enabled:not(:checked)]:hover:bg-surface-raised-hover",
      "has-[input:enabled:not(:checked)]:active:bg-surface-raised-pressed has-[input:focus-visible]:shadow-focus",
      "has-[input:checked]:bg-text has-[input:checked]:text-surface",
      "has-[input:disabled]:cursor-not-allowed has-[input:disabled]:opacity-disabled",
    ],
    note: "text-caption text-muted-text [word-break:auto-phrase]",
    // The note keeps its room while a save state stands in its place.
    hidden: "invisible",
  },
});

/**
 * The radio of an answer, over the whole of its label and not drawn: a press or a click anywhere on the answer lands on
 * the control itself, and the label under it shows its state (chosen, focused, disabled). Radios of one `name` are one
 * group to the browser, which the arrow keys move along.
 */
function AnswerInput({
  name,
  value,
  checked,
  disabled,
  onChange,
}: {
  name: string;
  value: Answer;
  checked: boolean;
  disabled: boolean;
  onChange: (answer: Answer) => void;
}) {
  return (
    <Pin
      cover
      full="both"
      as={
        <input
          type="radio"
          name={name}
          value={value}
          checked={checked}
          disabled={disabled}
          className="cursor-pointer opacity-0 disabled:cursor-not-allowed"
          onChange={() => {
            onChange(value);
          }}
        />
      }
    />
  );
}

/**
 * The three answers, which know their own width: in columns alike, each a third of the row; from a tablet's column, no
 * wider than a few words each, from the row's start.
 */
function ChoicePlace({ children }: { children: ReactNode }) {
  return (
    <div className="@container">
      <div className="grid grid-cols-[repeat(3,minmax(0,1fr))] gap-entry-caption @min-[24rem]:grid-cols-[repeat(3,minmax(0,9rem))]">
        {children}
      </div>
    </div>
  );
}

/**
 * An answer, as `as` (its `<label>`, which its radio covers), as wide as its column: on a phone, its glyph over its word, which a third of
 * the row holds in any language; from a tablet's column, its glyph before its word.
 */
function SegmentPlace({ as, children }: { as: ReactElement<{ className?: unknown }>; children: ReactNode }) {
  return (
    <Row
      as={as}
      full
      anchor
      align="center"
      justify="center"
      gap="hair"
      className="h-full flex-col text-center @min-[24rem]:flex-row @min-[24rem]:gap-entry-caption"
    >
      {children}
    </Row>
  );
}

/** The line under a choice that saves at once: what the reader should know of it, and its save state in its place. */
function ChoiceNote({ id, note, save }: { id: string; note: ReactNode; save: ReactNode | null | undefined }) {
  const s = choice();
  const saying = save !== undefined && save !== null && save !== false;
  return (
    <Layers as="span" className={s.note()}>
      {/* One line of room, said or not, so that a state that comes moves nothing under it. */}
      <span id={id} className={saying || note === undefined ? s.hidden() : ""}>
        {note ?? " "}
      </span>
      {save !== undefined && <span role="status">{saying && save}</span>}
    </Layers>
  );
}

export type AnswerChoiceProps = {
  /** What is answered, for assistive technology ("Your answer"). */
  label: string;
  /** The answers' words: "Going", "Maybe", "Not going". */
  names: Readonly<Record<Answer, string>>;
  /** The reader's answer; null before they answer. */
  value: Answer | null;
  onChange: (answer: Answer) => void;
  /** No answer may be given or changed (no date is fixed yet, the event started, it was cancelled); `note` says why. */
  isDisabled?: boolean | undefined;
  /** The answers that may not be chosen now (going, at capacity); `note` says why. */
  disabled?: readonly Answer[] | undefined;
  /**
   * The line under the answers: why some or all cannot be chosen now ("The event is full"). It describes the answers.
   */
  note?: string | undefined;
  /**
   * Its save state (a `SaveState`), in the note's place while it says something, `null` while it does not; the line
   * keeps its room from the start. Without it nothing is saved here.
   */
  save?: ReactNode | null;
};

/**
 * The reader's answer to an event, one of three side by side, a radio group the arrow keys move along, each radio over the whole of its answer: each a glyph
 * and its word on the raised surface, the chosen one knocked out; the note under them, and the save state in its place.
 */
export function AnswerChoice({
  label,
  names,
  value,
  onChange,
  isDisabled,
  disabled = [],
  note,
  save,
}: AnswerChoiceProps) {
  const s = choice();
  const id = useId();
  const lined = note !== undefined || save !== undefined;
  return (
    <Stack
      gap="entry-line"
      role="radiogroup"
      aria-label={label}
      {...(note === undefined ? {} : { "aria-describedby": id })}
    >
      <ChoicePlace>
        {answers.map((a) => (
          <SegmentPlace key={a} as={<label className={s.segment()} data-selected={value === a || undefined} />}>
            <AnswerInput
              name={id}
              value={a}
              checked={value === a}
              disabled={isDisabled === true || disabled.includes(a)}
              onChange={onChange}
            />
            <Icon name={answerIcon[a]} place="action" />
            <span>{names[a]}</span>
          </SegmentPlace>
        ))}
      </ChoicePlace>
      {lined && <ChoiceNote id={id} note={note} save={save} />}
    </Stack>
  );
}

/**
 * The tracks of a row of the date poll's table, which knows its own width. Narrow (a phone): a date has a line to
 * itself, with its actions at the end; its counts under it, each a glyph and its figure, from the start; and the
 * reader's answer under them, across the row, each answer a glyph and its word. Wide (a tablet's column and wider): a
 * date a row, the dates down the start taking the room left, then a column a count, the reader's answer, glyphs alone,
 * and the actions, each the same width in every row, so that the columns line up.
 */
const TRACKS = {
  plain: [
    "grid-cols-[auto_auto_auto_minmax(0,1fr)]",
    "@min-[32rem]:grid-cols-[minmax(0,1fr)_repeat(3,var(--event-count))_var(--event-answers)]",
  ].join(" "),
  actions: [
    "grid-cols-[auto_auto_auto_minmax(0,1fr)_auto]",
    "@min-[32rem]:grid-cols-[minmax(0,1fr)_repeat(3,var(--event-count))_var(--event-answers)_var(--icon-button-touch)]",
  ].join(" "),
} as const;

/** The table's frame, which knows its own width: its rows one under another. */
function CandidateFrame({ className, ...rest }: HTMLAttributes<HTMLDivElement> & { className: string }) {
  return <div {...rest} className={`${className} @container grid grid-cols-[minmax(0,1fr)]`} />;
}

/**
 * A row of the table. `head`, the row of the columns' names: in a wide frame it heads the columns, and in a narrow one,
 * where every count shows its glyph and every answer its word, it is for assistive technology alone. In a narrow frame
 * a date's lines stand close and the dates apart; in a wide one a row is a line, a little air around it.
 */
function CandidateLine({
  actions,
  head = false,
  className,
  ...rest
}: HTMLAttributes<HTMLDivElement> & { actions: boolean; head?: boolean; className: string }) {
  const shown = head
    ? "sr-only @min-[32rem]:not-sr-only @min-[32rem]:grid @min-[32rem]:pb-screen-air"
    : "gap-y-entry-line py-screen-row-block @min-[32rem]:gap-y-0 @min-[32rem]:py-screen-air";
  return (
    <div
      {...rest}
      className={`${className} grid items-center gap-x-entry-caption ${TRACKS[actions ? "actions" : "plain"]} ${shown}`}
    />
  );
}

const PLACE = {
  // The date: narrow, the first line, up to the actions; wide, the first column.
  time: "col-[1/5] row-[1] min-w-0 @min-[32rem]:col-[1] @min-[32rem]:row-[1]",
  // A count: narrow, the second line, from the start, a glyph and its figure; wide, the middle of its column, its
  // figure alone under the column's glyph and name.
  yes: "col-[1] row-[2] @min-[32rem]:col-[2] @min-[32rem]:row-[1] @min-[32rem]:justify-self-center",
  maybe: "col-[2] row-[2] @min-[32rem]:col-[3] @min-[32rem]:row-[1] @min-[32rem]:justify-self-center",
  no: "col-[3] row-[2] @min-[32rem]:col-[4] @min-[32rem]:row-[1] @min-[32rem]:justify-self-center",
  // The reader's answer: narrow, the third line, across the row; wide, its column.
  answer: "col-[1/-1] row-[3] min-w-0 @min-[32rem]:col-[5] @min-[32rem]:row-[1]",
  // The actions: narrow, at the end of the date's line; wide, the last column.
  actions: "col-[5] row-[1] justify-self-end @min-[32rem]:col-[6]",
  // A glyph that says which count it is, which the column's head says in a wide frame.
  glyph: "inline-flex items-center @min-[32rem]:hidden",
  // An answer's word, which a wide frame says once, at the column's head.
  word: "@min-[32rem]:sr-only",
  // A head of the columns, its glyph over its name.
  head: "flex flex-col items-center gap-[0.125rem]",
} as const;

/** Where a part of a row stands in it. */
function CandidatePlace({
  at,
  className,
  ...rest
}: { at: keyof typeof PLACE; className: string } & HTMLAttributes<HTMLElement>) {
  return <span {...rest} className={`${className} ${PLACE[at]}`} />;
}

/**
 * An answer of the reader's to a date, as `as` (its `<label>`, which its radio covers): narrow, a finger's target tall, a glyph and its
 * word; wide, a finger's target, square, its glyph alone.
 */
function AnswerPlace({ as, children }: { as: ReactElement<{ className?: unknown }>; children: ReactNode }) {
  return (
    <Row
      as={as}
      rigid
      anchor
      align="center"
      justify="center"
      gap="entry-caption"
      className="min-h-icon-button-touch px-control-timeline-inline @min-[32rem]:size-icon-button-touch @min-[32rem]:px-0"
    >
      {children}
    </Row>
  );
}

const table = tv({
  slots: {
    root: "text-body text-text",
    head: "text-center text-caption text-muted-text [word-break:auto-phrase]",
    time: "[word-break:auto-phrase]",
    confirmed: "text-caption font-label",
    count: "tabular-nums",
    glyph: "text-muted-text",
    // As the answers to the event, smaller: on the raised surface, the chosen one knocked out.
    segment: [
      "box-border cursor-pointer rounded-control bg-surface-raised font-sans text-caption font-label text-text",
      "outline-hidden",
      "has-[input:enabled:not(:checked)]:hover:bg-surface-raised-hover",
      "has-[input:enabled:not(:checked)]:active:bg-surface-raised-pressed has-[input:focus-visible]:shadow-focus",
      "has-[input:checked]:bg-text has-[input:checked]:text-surface",
      "has-[input:disabled]:cursor-not-allowed has-[input:disabled]:opacity-disabled",
    ],
    save: "text-caption",
  },
});

/** What the table gives its rows: its answers' names, the ids of the columns' heads, and whether rows have actions. */
const TableContext = createContext<{
  readonly names: Readonly<Record<Answer, string>>;
  readonly headId: (column: Answer | "yours") => string;
  readonly actions: boolean;
}>({ names: { yes: "", maybe: "", no: "" }, headId: (c) => c, actions: false });

export type CandidateTableProps = {
  /** What the table is, for assistive technology ("Date poll"). */
  label: string;
  /** The head of the dates' column, for assistive technology ("Date"). */
  time: string;
  /** The answers' words, each the head of the column of its count: "Can go", "Maybe", "Cannot". */
  names: Readonly<Record<Answer, string>>;
  /** The head of the column of the reader's answer ("Your answer"). */
  yours: string;
  /** The head of the column of each date's actions, for assistive technology ("Actions"): for the organizers alone. */
  actions?: string | undefined;
  /** The save state of the reader's last answer (a `SaveState`), under the table; `null` while it says nothing. */
  save?: ReactNode | null;
  /** The `CandidateRow`s, in the order of their dates. */
  children: ReactNode;
};

/**
 * The table of an event's date poll, named `label`: a date a row, its time the row's head; a column a count of who
 * can go, may, and cannot, then the reader's own answer, which saves as it is pressed, and, for the organizers, the
 * actions of each date. For assistive technology it is a table whose every cell is read with its date and its
 * column's name ("Sat, Nov 21, 9:00 PM, Can go, 5"); its first row the columns' heads, each a glyph over its name.
 * The save state of the reader's answer stands under it, in a polite live region there from the start.
 */
export function CandidateTable({ label, time, names, yours, actions, save, children }: CandidateTableProps) {
  const s = table();
  const id = useId();
  const headId = (column: Answer | "yours") => `${id}-${column}`;
  const hasActions = actions !== undefined;
  return (
    <Stack gap="entry-line">
      <CandidateFrame role="table" aria-label={label} className={s.root()}>
        <CandidateLine role="row" actions={hasActions} head className="">
          <CandidatePlace at="time" role="columnheader" className="">
            <Spoken>{time}</Spoken>
          </CandidatePlace>
          {answers.map((a) => (
            <CandidatePlace key={a} at={a} role="columnheader" id={headId(a)} className={s.head()}>
              <CandidatePlace at="head" className="">
                <Icon name={answerIcon[a]} place="caption" />
                <span>{names[a]}</span>
              </CandidatePlace>
            </CandidatePlace>
          ))}
          <CandidatePlace at="answer" role="columnheader" id={headId("yours")} className={s.head()}>
            {yours}
          </CandidatePlace>
          {hasActions && (
            <CandidatePlace at="actions" role="columnheader" className="">
              <Spoken>{actions}</Spoken>
            </CandidatePlace>
          )}
        </CandidateLine>
        <TableContext value={{ names, headId, actions: hasActions }}>{children}</TableContext>
      </CandidateFrame>
      {save !== undefined && (
        <span role="status" className={s.save()}>
          {save}
        </span>
      )}
    </Stack>
  );
}

export type CandidateRowProps = {
  /** The date's time (an `EventTime`), the head of its row. */
  time: ReactNode;
  /** How many answered each way, as the reader's locale writes the figures. */
  counts: Readonly<Record<Answer, string>>;
  /** The reader's answer to it; null before they answer. */
  answer: Answer | null;
  onAnswer: (answer: Answer) => void;
  /** The answer may not be given or changed (the date was confirmed, the poll closed). */
  isDisabled?: boolean | undefined;
  /** It is the date the organizers confirmed: the words for it ("Confirmed"), under its time. */
  confirmed?: string | undefined;
  /** What the organizers may do with it (a `MenuSurface` on a `square` `IconButton`: confirm, withdraw…). */
  actions?: ReactNode;
};

/**
 * A date of the poll: its time, the head of its row, with the mark of the confirmed date under it; its counts; the
 * reader's answer, a radio group named by the date and the column's name, the chosen answer knocked out; and its
 * actions.
 */
export function CandidateRow({ time, counts, answer, onAnswer, isDisabled, confirmed, actions }: CandidateRowProps) {
  const s = table();
  const id = useId();
  const { names, headId, actions: hasActions } = use(TableContext);
  return (
    <CandidateLine role="row" actions={hasActions} className="">
      <CandidatePlace at="time" role="rowheader" id={id} className={s.time()}>
        <Stack as="span" gap="hair">
          {time}
          {confirmed !== undefined && (
            <Row as="span" align="center" gap="hair" className={s.confirmed()}>
              <Icon name="dateFixed" place="caption" />
              <span>{confirmed}</span>
            </Row>
          )}
        </Stack>
      </CandidatePlace>
      {answers.map((a) => (
        <CandidatePlace key={a} at={a} role="cell" className={s.count()}>
          <Row as="span" align="center" gap="hair">
            <CandidatePlace at="glyph" className={s.glyph()} aria-hidden="true">
              <Icon name={answerIcon[a]} place="caption" />
            </CandidatePlace>
            <span>{counts[a]}</span>
          </Row>
        </CandidatePlace>
      ))}
      <CandidatePlace at="answer" role="cell" className="">
        <Row gap="hair" role="radiogroup" aria-labelledby={`${id} ${headId("yours")}`}>
          {answers.map((a) => (
            <AnswerPlace key={a} as={<label className={s.segment()} data-selected={answer === a || undefined} />}>
              <AnswerInput
                name={id}
                value={a}
                checked={answer === a}
                disabled={isDisabled === true}
                onChange={onAnswer}
              />
              <Icon name={answerIcon[a]} place="caption" />
              <CandidatePlace at="word" className="">
                {names[a]}
              </CandidatePlace>
            </AnswerPlace>
          ))}
        </Row>
      </CandidatePlace>
      {hasActions && (
        <CandidatePlace at="actions" role="cell" className="">
          {actions}
        </CandidatePlace>
      )}
    </CandidateLine>
  );
}

/** A link of an event that leaves the site (a place online, a place's map). */
export type OutLink = {
  href: string;
  /** Its words ("Open the map"); for a place online, the place's name. */
  text: string;
  /** The site it leads to, shown after it and said with it ("meet.example.com"). */
  host: string;
};

const place = tv({
  slots: {
    glyph: "text-muted-text",
    name: "text-body font-label [word-break:auto-phrase]",
    hidden: "text-body text-muted-text",
    detail: "text-body whitespace-pre-line",
    note: "text-caption text-muted-text [word-break:auto-phrase]",
    link: [
      "rounded-control py-target-line text-body text-accent-text underline underline-offset-[0.15em] outline-hidden",
      "data-focus-visible:shadow-focus focus-visible:shadow-focus",
    ],
    host: "text-caption text-muted-text",
  },
});

/**
 * A link that leaves the site: its words in the links' color, the glyph of leaving after them, then the site it leads
 * to, muted. It opens in a new tab and tells the page it leads to nothing of where it came from; to assistive
 * technology it says that it opens that site in a new tab.
 */
function Leaving({ link, name = false }: { link: OutLink; name?: boolean }) {
  const s = place();
  const { lang } = useReader(undefined);
  const { t } = stringsIn(lang);
  return (
    <Row as="span" wrap align="baseline" gapX="entry-caption">
      <Box
        as={<a href={link.href} target="_blank" rel="nofollow noopener noreferrer" dir="auto" />}
        breaks="anywhere"
        className={`${s.link()} ${name ? "font-label" : ""}`}
      >
        {link.text}
        <Box as="span" display="inline-block" baseline="glyph" spaceBefore="entry-caption">
          <Icon name="open" place="caption" />
        </Box>
        <Spoken> {t("event.new_tab", { host: link.host })}</Spoken>
      </Box>
      <Box as="bdi" breaks="anywhere" className={s.host()} aria-hidden="true">
        {link.host}
      </Box>
    </Row>
  );
}

export type PlaceLineProps = {
  kind: PlaceKind;
  /**
   * The kind's words ("Voice chat room", "In person", "Online"): the voice room's name, and what a place the reader may
   * not see shows alone; said before the name of the others.
   */
  label: string;
  /** The place's name; for a place online, the words of its link. None for the voice room. */
  name?: string | undefined;
  /** A place in person's address, its lines as written. */
  address?: string | undefined;
  /** A place online's address, or a place in person's map. */
  link?: OutLink | undefined;
  /** The reader may not see where it is (it is shown to the approved participants): its kind alone shows, muted. */
  hidden?: boolean | undefined;
  /** The voice room could not open: its glyph is the microphone crossed out, and `note` says why. */
  unavailable?: boolean | undefined;
  /** A muted line under it: the voice room's state ("Opens at the start", "Open now", why it could not open). */
  note?: string | undefined;
  /** The way into the voice room while it is open (a `LinkButton`), after the note. */
  action?: ReactNode;
};

/**
 * A place of an event, as a line of the event's places: the glyph of its kind at the start; its name (a place online
 * its link, the voice room the kind's words), its address, its map; a muted note under it, and the way into an open
 * room. A place the reader may not see shows its kind alone. Put them in a `PlaceList`.
 */
export function PlaceLine({
  kind,
  label,
  name,
  address,
  link,
  hidden = false,
  unavailable,
  note,
  action,
}: PlaceLineProps) {
  const s = place();
  const icon: IconName = kind === "voice" && unavailable === true ? "roomUnavailable" : placeIcon[kind];
  const online = kind === "url" && link !== undefined && !hidden;
  return (
    <Row as="li" align="start" gap="entry-caption">
      <Row as="span" line rigid align="center" className={s.glyph()}>
        <Icon name={icon} place="action" />
      </Row>
      <Stack as="span" grow gap="hair">
        {hidden || kind === "voice" ? (
          <span className={hidden ? s.hidden() : s.name()}>{label}</span>
        ) : (
          <>
            <Spoken>{label}: </Spoken>
            {online ? (
              <Leaving link={{ ...link, text: name ?? link.text }} name />
            ) : (
              <Box as="span" className={s.name()} dir="auto">
                {name}
              </Box>
            )}
          </>
        )}
        {!hidden && address !== undefined && (
          <Box as="span" className={s.detail()} dir="auto">
            {address}
          </Box>
        )}
        {!hidden && kind === "physical" && link !== undefined && <Leaving link={link} />}
        {note !== undefined && <span className={s.note()}>{note}</span>}
        {action !== undefined && (
          <Row as="span" spaceAbove="screen-air">
            {action}
          </Row>
        )}
      </Stack>
    </Row>
  );
}

/** The places of an event, named `label` ("Where"): its `PlaceLine`s, the voice room first, a row's gap apart. */
export function PlaceList({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Stack as="ul" plain gap="screen-row-gap" aria-label={label}>
      {children}
    </Stack>
  );
}
