// SPDX-License-Identifier: AGPL-3.0-only

// The table of channels (ADR-0110 §5): what the reader gets, by kind and by the channel it arrives through. A row is a
// kind, a column a channel, a cell a checkbox that saves as it is pressed; the first row turns a whole column on or
// off, and is half on while its column is mixed. A cell that cannot be changed is disabled, and the line with its
// row's name says why. A cell's save state stands in that line too, after its channel's name, in the reason's place
// while it has something to say, so that no checkbox moves: saving, saved for a moment, or not saved with Try again.
//
// The table knows its own width. In a wide frame (a tablet's column and wider) it is a grid: the kinds down the
// start, a column a channel under its name, a checkbox a finger's target. In a narrow one (a phone) a kind's name has a
// line to itself and its checkboxes stand under it, each over its channel's name, in columns alike, so that no head
// scrolls out of sight and nothing runs past the frame at the minimum width.
//
// For assistive technology it is a table named `label`: its first row the channels' names as column headers, a kind
// the header of its row, and each checkbox named by its kind and its channel ("Mentions Email"), described by the
// reason while it is disabled and by its save state while that says something. The state's place is a polite live
// region from the start, and the focus stays where it is; Try again puts it on the checkbox first.

import { createContext, use, useEffect, useId, useRef, useState, type HTMLAttributes, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { tv } from "tailwind-variants/lite";
import { describedBy } from "./field";
import { Layers, Pin, Spoken } from "./layout";
import { SaveControlContext } from "./save-state";

/** How many channels a table shows at most: the app's, Web Push, email and sound. */
type ColumnCount = 1 | 2 | 3 | 4;

/**
 * The tracks of a row. Narrow: its cells in columns alike from the row's start, no wider than a short name, under the
 * kind's name, which runs the row's width (the last track is the room left). Wide: the kind's name takes the room
 * left, and each channel a column of one width, the same in every row.
 */
const TRACKS: Record<ColumnCount, string> = {
  1: "grid-cols-[repeat(1,minmax(0,4.5rem))_minmax(0,1fr)] @min-[28rem]:grid-cols-[minmax(0,1fr)_repeat(1,4.5rem)]",
  2: "grid-cols-[repeat(2,minmax(0,4.5rem))_minmax(0,1fr)] @min-[28rem]:grid-cols-[minmax(0,1fr)_repeat(2,4.5rem)]",
  3: "grid-cols-[repeat(3,minmax(0,4.5rem))_minmax(0,1fr)] @min-[28rem]:grid-cols-[minmax(0,1fr)_repeat(3,4.5rem)]",
  4: "grid-cols-[repeat(4,minmax(0,4.5rem))_minmax(0,1fr)] @min-[28rem]:grid-cols-[minmax(0,1fr)_repeat(4,4.5rem)]",
};

/** The table's frame, which knows its own width: its rows one under another. */
function ChannelFrame({ className, ...rest }: HTMLAttributes<HTMLDivElement> & { className: string }) {
  return <div {...rest} className={`${className} @container grid grid-cols-[minmax(0,1fr)]`} />;
}

/**
 * A row of the table, in the tracks of so many channels. `head`, the row of the channels' names: in a wide frame it
 * heads the columns, and in a narrow one, where every checkbox stands over its channel's name, it is for assistive
 * technology alone.
 */
function ChannelLine({
  columns,
  head = false,
  className,
  ...rest
}: HTMLAttributes<HTMLDivElement> & { columns: ColumnCount; head?: boolean; className: string }) {
  const shown = head ? "sr-only @min-[28rem]:not-sr-only @min-[28rem]:grid" : "";
  return <div {...rest} className={`${className} grid items-center gap-x-screen-air ${TRACKS[columns]} ${shown}`} />;
}

const PLACE = {
  // The kind's name and the line with it: across the row in a narrow frame, the first column in a wide one. The line
  // stands beside the name where there is room, and under it in a wide frame, which holds both in the row's height.
  kind: "col-span-full flex min-w-0 flex-wrap items-baseline gap-x-screen-row-gap @min-[28rem]:col-span-1",
  line: "min-w-0 flex-[1_1_auto] @min-[28rem]:basis-full",
  cell: "min-w-0 @min-[28rem]:justify-self-center",
  // The checkbox's target: narrow, the mark over its channel's name, from the cell's start; wide, the mark alone in
  // the middle of a finger's target.
  target: [
    "relative flex min-h-icon-button-touch flex-col items-start justify-center gap-[0.125rem]",
    "@min-[28rem]:size-icon-button-touch @min-[28rem]:items-center",
  ].join(" "),
  // The channel's name under a mark, which a wide frame says once, at the column's head.
  caption: "max-w-full @min-[28rem]:hidden",
  // A state of the line, one under another where several cells of a row say something. A block, so that an empty one
  // takes no line: the kind's name stays at the middle of its checkboxes.
  state: "flex max-w-full items-start gap-x-entry-caption",
} as const;

/** Where a part of a row stands in it. */
function ChannelPlace({
  at,
  as: Tag = "span",
  className,
  ...rest
}: { at: keyof typeof PLACE; as?: "span" | "label"; className: string } & HTMLAttributes<HTMLElement>) {
  return <Tag {...rest} className={`${className} ${PLACE[at]}`} />;
}

const table = tv({
  slots: {
    root: "text-body text-text",
    // A row is a finger's target tall, which holds a kind's name and one line under it beside its checkboxes.
    row: "min-h-icon-button-touch py-screen-air",
    head: "text-center text-caption text-muted-text [word-break:auto-phrase]",
    kind: "[word-break:auto-phrase]",
    // The row that turns a whole column on or off: its name a label's weight.
    all: "font-label",
    line: "text-caption text-muted-text [word-break:auto-phrase]",
    // The reason while a save state stands in its place: it keeps its room.
    reason: "group-has-[[data-channel-saying]]/line:invisible",
    // A state's channel, before what came of saving it.
    channel: "text-caption text-muted-text",
    // Not drawn: the mark after it shows its state, and the ring of its focus.
    input: "peer cursor-pointer opacity-0 disabled:cursor-not-allowed",
    caption: "text-caption text-muted-text [word-break:auto-phrase] peer-disabled:opacity-disabled",
  },
});

const mark = tv({
  base: [
    "box-border size-form-page-check rounded-[3px] border-[length:var(--composer-mark-line)] border-solid",
    "peer-focus-visible:shadow-focus",
  ],
  variants: {
    on: { true: "text-surface", false: "" },
    disabled: { true: "", false: "" },
  },
  // One border color and one fill each (no class merges here). Off, the box is bounded by a line of the muted text's
  // color, which keeps 3:1 on every surface, and on a raised surface it is filled with the page's, as every neutral
  // control there; on, it is knocked out of the text's color. Disabled, on turns muted, and off dims.
  compoundVariants: [
    { on: true, disabled: false, class: "border-text bg-text" },
    { on: true, disabled: true, class: "border-muted-text bg-muted-text" },
    { on: false, disabled: false, class: "border-muted-text [[data-surface-raised]_&]:bg-surface" },
    { on: false, disabled: true, class: "border-muted-text opacity-disabled [[data-surface-raised]_&]:bg-surface" },
  ],
});

/** A channel of the table: its `id`, by which a cell names its column, and its name. */
export type ChannelColumn = { readonly id: string; readonly label: string };

/** What the table gives its rows: its channels, each with the id of its column's header. */
const TableContext = createContext<{
  readonly columns: readonly ChannelColumn[];
  readonly headId: (id: string) => string;
}>({
  columns: [],
  headId: (id) => id,
});

/** What a row gives its cells: the ids of its name and its reason, and where their save states stand. */
const RowContext = createContext<{
  readonly nameId: string;
  readonly reasonId: string | undefined;
  readonly states: HTMLElement | null;
}>({ nameId: "", reasonId: undefined, states: null });

const countOf = (columns: readonly ChannelColumn[]): ColumnCount =>
  columns.length <= 1 ? 1 : columns.length === 2 ? 2 : columns.length === 3 ? 3 : 4;

/**
 * The table of channels, named `label`: its rows (`ChannelRow`), each with a cell a channel of `columns`, in their
 * order (four at most). `kind` heads the column of the kinds' names, for assistive technology.
 */
export function ChannelTable({
  label,
  kind,
  columns,
  children,
}: {
  label: string;
  /** What a row is, the header of the names' column ("Notification"). */
  kind: string;
  columns: readonly ChannelColumn[];
  children: ReactNode;
}) {
  const s = table();
  const id = useId();
  const headId = (column: string) => `${id}-${column}`;
  return (
    <ChannelFrame role="table" aria-label={label} className={s.root()} data-channel-table="">
      <ChannelLine role="row" columns={countOf(columns)} head className="">
        {/* It keeps the names' column in the head's row, and says what the column holds without showing it. */}
        <ChannelPlace at="kind" role="columnheader" className="">
          <Spoken>{kind}</Spoken>
        </ChannelPlace>
        {columns.map((column) => (
          <ChannelPlace key={column.id} at="cell" role="columnheader" id={headId(column.id)} className={s.head()}>
            {column.label}
          </ChannelPlace>
        ))}
      </ChannelLine>
      <TableContext value={{ columns, headId }}>{children}</TableContext>
    </ChannelFrame>
  );
}

/**
 * A row of the table: a kind's name, the line with it, and its cells, one a channel of the table in the channels'
 * order (a `ChannelCell`, or a `ChannelGap` for a channel the kind does not arrive through). `reason` says why its
 * disabled cells cannot be changed. `all`: the row that turns each column on or off as a whole, the table's first.
 */
export function ChannelRow({
  label,
  reason,
  all = false,
  children,
}: {
  label: string;
  reason?: string | undefined;
  all?: boolean;
  children: ReactNode;
}) {
  const s = table();
  const id = useId();
  const { columns } = use(TableContext);
  const [states, setStates] = useState<HTMLElement | null>(null);
  const reasonId = reason === undefined ? undefined : `${id}-reason`;
  return (
    <ChannelLine role="row" columns={countOf(columns)} className={s.row()}>
      <ChannelPlace at="kind" role="rowheader" className="">
        <span id={id} className={all ? `${s.kind()} ${s.all()}` : s.kind()}>
          {label}
        </span>
        <ChannelPlace at="line" className={s.line()}>
          <Layers as="span" className="group/line">
            {reason !== undefined && (
              <span id={reasonId} className={s.reason()}>
                {reason}
              </span>
            )}
            <span ref={setStates} />
          </Layers>
        </ChannelPlace>
      </ChannelPlace>
      <RowContext value={{ nameId: id, reasonId, states }}>{children}</RowContext>
    </ChannelLine>
  );
}

export type ChannelCellProps = {
  /** The `id` of its channel among the table's `columns`. */
  column: string;
  isSelected: boolean;
  /** Neither on nor off: some of what it stands for is on (the cell of a mixed column, in the `all` row). */
  isMixed?: boolean | undefined;
  /** It cannot be changed now; its row's `reason` says why. */
  isDisabled?: boolean | undefined;
  onChange: (on: boolean) => void;
  /**
   * Its save state (a `SaveState`), which stands in its row's line; `null` while it has nothing to say. A cell saves
   * as it is pressed, so it never goes without.
   */
  save: ReactNode | null;
};

/** A cell of the table: a checkbox, named by its row's kind and its channel, that saves as it is pressed. */
export function ChannelCell({
  column,
  isSelected,
  isMixed = false,
  isDisabled = false,
  onChange,
  save,
}: ChannelCellProps) {
  const s = table();
  const id = useId();
  const input = useRef<HTMLInputElement>(null);
  const { columns, headId } = use(TableContext);
  const { nameId, reasonId, states } = use(RowContext);
  const channel = columns.find((c) => c.id === column)?.label ?? "";
  const saying = save !== undefined && save !== null && save !== false;
  const on = isSelected || isMixed;
  useEffect(() => {
    if (input.current !== null) input.current.indeterminate = isMixed;
  }, [isMixed]);
  return (
    <ChannelPlace at="cell" role="cell" className="">
      <ChannelPlace at="target" as="label" className="">
        {/* The checkbox itself takes the press, over the whole target; the mark under it is what shows. */}
        <Pin
          cover
          full="both"
          as={
            <input
              ref={input}
              className={s.input()}
              type="checkbox"
              checked={isSelected}
              disabled={isDisabled}
              aria-labelledby={`${nameId} ${headId(column)}`}
              aria-describedby={describedBy(isDisabled && reasonId, saying && `${id}-state`)}
              onChange={(event) => {
                onChange(event.target.checked);
              }}
            />
          }
        />
        <Layers as="span" center rigid className={mark({ on, disabled: isDisabled })} aria-hidden="true">
          {on && (
            <svg viewBox="0 0 12 12" className="size-poll-mark-check" fill="none" stroke="currentColor" strokeWidth="2">
              <path
                d={isMixed ? "M3 6 L9 6" : "M2.5 6.25 L5 8.75 L9.5 3.5"}
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            </svg>
          )}
        </Layers>
        <ChannelPlace at="caption" className={s.caption()} aria-hidden="true">
          {channel}
        </ChannelPlace>
      </ChannelPlace>
      {/* The state's place, in the row's line: a live region that is there before the state is. */}
      {states !== null &&
        createPortal(
          <ChannelPlace at="state" id={`${id}-state`} role="status" className="">
            {saying && (
              <>
                {/* Which of the row's cells it is, for the eye: who hears it is on the checkbox it tells of. */}
                <span className={s.channel()} data-channel-saying="" aria-hidden="true">
                  {channel}
                </span>
                <SaveControlContext value={() => input.current?.focus()}>{save}</SaveControlContext>
              </>
            )}
          </ChannelPlace>,
          states,
        )}
    </ChannelPlace>
  );
}

/** The place of a cell in a row whose kind does not arrive through that channel: nothing to choose. */
export function ChannelGap() {
  return <ChannelPlace at="cell" role="cell" className="" />;
}
