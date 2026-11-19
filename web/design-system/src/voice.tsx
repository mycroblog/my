// SPDX-License-Identifier: AGPL-3.0-only

// The parts of a voice chat room (ADR-0100 §5, §8, §10): who is in it, as tiles on its stage (the host, the co-hosts
// and the speakers) and as compact tiles in the long list of its listeners; the emoji reactions that rise over the
// stage and are gone; a room as a line of a list (the rooms' screen, a hashtag's page, a profile); and the room's
// bar of controls (the microphone, a raised hand, a reaction, leaving). The call's bar, which crosses the screens, is
// the frame's (shell.tsx).
//
// Everything is drawn in the text's color on the page's surface, with no hue: who speaks has a ring of the text's
// color around their avatar, its halo swelling while they speak, still with reduced motion; a microphone off and a
// hand raised are badges knocked out of the text's color on the avatar's corners; a role and the mark of an account
// the reader blocks are muted words under the name, the block's glyph before its words, so that a participant the
// reader blocks shows, marked, and is not hidden (ADR-0100 §10). A control that is on (muted, a hand raised) is
// knocked out as a toggle on is. The words are the screen's: every label comes in a prop.

import type { ReactNode } from "react";
import { Children } from "react";
import { Button as AriaButton, Link, ToggleButton, Toolbar } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Avatar } from "./avatar";
import { Faces } from "./faces";
import { Icon, icons, type IconName } from "./icons";
import { Box, Pin, Row, Stack } from "./layout";

/** Where a participant's tile sits: on the stage (who may speak), or in the list of listeners, compact. */
export type ParticipantSize = "stage" | "listener";

const tile = tv({
  slots: {
    // The ring of who speaks and the badges' cut take the surface under them, hovered or not.
    root: [
      "box-border cursor-pointer rounded-control border-0 bg-transparent p-voice-tile-padding font-sans text-text",
      "outline-hidden data-focus-visible:shadow-focus",
      "[--face-cut-color:var(--color-surface)] [--voice-cut:var(--color-surface)]",
      "data-hovered:bg-surface-hover data-hovered:[--face-cut-color:var(--color-surface-hover)]",
      "data-hovered:[--voice-cut:var(--color-surface-hover)]",
      "data-pressed:bg-surface-pressed data-pressed:[--face-cut-color:var(--color-surface-pressed)]",
      "data-pressed:[--voice-cut:var(--color-surface-pressed)]",
    ],
    face: "rounded-full",
    badge: "rounded-full bg-text text-surface shadow-face-cut",
    name: "text-caption font-label",
    caption: "text-caption text-muted-text [word-break:auto-phrase]",
  },
  variants: {
    size: {
      stage: { badge: "size-voice-badge" },
      listener: { badge: "size-voice-badge-small" },
    },
    // The halo swells and settles while they speak; with reduced motion the ring stays still.
    speaking: { true: { face: "shadow-speaking animate-speaking motion-reduce:animate-none" } },
  },
});

export type ParticipantTileProps = {
  /** The account's display name, under the avatar, cut to one line. */
  name: string;
  /** The avatar's image. */
  src?: string | undefined;
  size: ParticipantSize;
  /** Their role, while they have one that is not a speaker's or a listener's: "Host", "Co-host". */
  role?: string | undefined;
  /** While they speak (the stream says so; nothing here listens): the words for it ("Speaking"). */
  speaking?: string | undefined;
  /** While their microphone is off: the words for it ("Microphone off"). */
  muted?: string | undefined;
  /** While their hand is raised to speak: the words for it ("Hand raised"). */
  hand?: string | undefined;
  /** While the reader blocks them, or they the reader (ADR-0100 §10): the mark's words ("Blocked"). */
  blocked?: string | undefined;
  /** What a press does (opens what the reader may do with them); or the tile is a `MenuSurface`'s trigger. */
  onPress?: (() => void) | undefined;
};

/** A badge knocked out of the text's color on a corner of the avatar, its glyph in its on shape when `on`. */
function Badge({ size, icon, on = false }: { size: ParticipantSize; icon: IconName; on?: boolean }) {
  return (
    <Row as="span" inline rigid align="center" justify="center" className={tile({ size }).badge()}>
      <Icon name={icon} place="caption" on={on} />
    </Row>
  );
}

/**
 * A participant of a room, pressed as one: their avatar, ringed while they speak, a hand raised on its top corner at
 * the end and the microphone off on its bottom one; their name under it, then their role and the block's mark, muted.
 * Its name for assistive technology says all of them, the name first.
 */
export function ParticipantTile({
  name,
  src,
  size,
  role,
  speaking,
  muted,
  hand,
  blocked,
  onPress,
}: ParticipantTileProps) {
  const s = tile({ size, speaking: speaking !== undefined });
  const label = [name, role, blocked, speaking, muted, hand].filter((w) => w !== undefined && w !== "").join(", ");
  return (
    <Stack
      as={<AriaButton className={s.root()} aria-label={label} {...(onPress === undefined ? {} : { onPress })} />}
      justify="center"
      gap="hair"
    >
      <Box as="span" anchor rigid display="inline-block" className={s.face()}>
        <Avatar name={name} src={src} size={size === "stage" ? "preview" : "post"} decorative />
        {hand !== undefined && (
          <Pin as="span" top="0" end="0">
            <Badge size={size} icon="hand" />
          </Pin>
        )}
        {muted !== undefined && (
          <Pin as="span" bottom="0" end="0">
            <Badge size={size} icon="microphone" on />
          </Pin>
        )}
      </Box>
      {/* The words shown are apart by spaces, so that they read as words of its name (WCAG 2.5.3). */}
      <Box as="span" lines={1} className={s.name()} dir="auto">
        {name}
      </Box>
      {role !== undefined && " "}
      {role !== undefined && <span className={s.caption()}>{role}</span>}
      {blocked !== undefined && " "}
      {blocked !== undefined && (
        <Row as="span" inline align="center" gap="hair" className={s.caption()}>
          <Icon name="block" place="caption" />
          <span>{blocked}</span>
        </Row>
      )}
    </Stack>
  );
}

const grid = tv({
  base: "m-0 grid list-none gap-voice-tile-gap p-0 [&>li]:grid [&>li]:min-w-0",
  variants: {
    size: {
      stage: "grid-cols-[repeat(auto-fill,minmax(var(--voice-stage),1fr))]",
      listener: "grid-cols-[repeat(auto-fill,minmax(var(--voice-listener),1fr))]",
    },
  },
});

/**
 * The tiles of one kind of participant (the stage's, or the listeners'), in as many columns alike as fit, each at
 * least a tile's width, row after row.
 */
export function ParticipantGrid({
  label,
  size,
  children,
}: {
  /** What the list is, for assistive technology ("Speakers"). */
  label: string;
  size: ParticipantSize;
  /** The `ParticipantTile`s, in order. */
  children: ReactNode;
}) {
  return (
    <ul className={grid({ size })} aria-label={label}>
      {Children.map(children, (child) => (
        <li>{child}</li>
      ))}
    </ul>
  );
}

/** A reaction sent in the room: who sent it is not shown; `emoji` is a Unicode emoji, or a `CustomEmoji`. */
export type Reaction = { id: string; emoji: ReactNode };

/** The lanes reactions rise in, from the stage's end. */
const lanes = 4;

/** The lane of a reaction, by its id, so that one keeps its lane while those around it come and go. */
function laneOf(id: string): number {
  let sum = 0;
  for (const c of id) sum = (sum * 31 + (c.codePointAt(0) ?? 0)) % 9973;
  return sum % lanes;
}

const flow = tv({
  slots: {
    root: "relative isolate",
    // Over what it flows over, taking none of its presses.
    over: "pointer-events-none absolute inset-0 z-1",
    lane: "absolute bottom-0",
  },
  variants: {
    lane: {
      0: { lane: "end-0" },
      1: { lane: "end-voice-lane" },
      2: { lane: "end-[calc(var(--voice-lane)*2)]" },
      3: { lane: "end-[calc(var(--voice-lane)*3)]" },
    },
  },
});

/** What reactions rise over, and the layer over it they rise in, hidden from assistive technology. */
function ReactionPlace({ over, children }: { over: ReactNode; children: ReactNode }) {
  const s = flow();
  return (
    <div className={s.root()}>
      {children}
      <div className={s.over()} aria-hidden="true">
        {over}
      </div>
    </div>
  );
}

/** A lane of reactions, from the bottom of what they rise over, so many lanes' widths from its end. */
function ReactionLane({ lane, children }: { lane: number; children: ReactNode }) {
  return <span className={flow({ lane: lane as 0 | 1 | 2 | 3 }).lane()}>{children}</span>;
}

const bubble = tv({
  base: "text-[length:var(--voice-reaction)] leading-none animate-reaction motion-reduce:animate-reaction-still",
});

/** One reaction: it rises and fades, or, with reduced motion, fades in and out where it is; then it is done. */
function Rising({ onDone, children }: { onDone: () => void; children: ReactNode }) {
  return (
    <Box as="span" display="inline-block" className={bubble()} onAnimationEnd={onDone}>
      {children}
    </Box>
  );
}

/**
 * What the room's reactions rise over (its stage), and the reactions rising from its bottom at the end, in a few
 * lanes, each once, over what is under them without taking its presses. The owner adds a reaction as it arrives and
 * removes it at `onDone`. They are hidden from assistive technology: they come too fast to be heard, and say nothing a
 * participant must know.
 */
export function ReactionFlow({
  reactions,
  onDone,
  children,
}: {
  reactions: readonly Reaction[];
  /** A reaction has risen and faded: the owner removes it. */
  onDone: (id: string) => void;
  children: ReactNode;
}) {
  return (
    <ReactionPlace
      over={reactions.map((r) => (
        <ReactionLane key={r.id} lane={laneOf(r.id)}>
          <Rising
            onDone={() => {
              onDone(r.id);
            }}
          >
            {r.emoji}
          </Rising>
        </ReactionLane>
      ))}
    >
      {children}
    </ReactionPlace>
  );
}

const line = tv({
  slots: {
    // The whole line is the link: the surface on hover and the ring on focus. The ring that cuts a face out takes
    // the surface under it.
    root: [
      "rounded-control px-screen-row-inline py-screen-row-block text-text no-underline outline-hidden",
      "[--face-cut-color:var(--color-surface)]",
      "data-hovered:bg-surface-hover data-hovered:[--face-cut-color:var(--color-surface-hover)]",
      "data-focus-visible:shadow-focus",
    ],
    box: "size-voice-room-box text-[length:var(--voice-room-emoji)] leading-none",
    glyph: "text-muted-text",
    name: "text-body font-label [word-break:auto-phrase]",
    caption: "text-caption text-muted-text [word-break:auto-phrase]",
  },
});

export type RoomLineProps = {
  /** The room's screen. */
  href: string;
  /** The room's name, on two lines at most. */
  name: string;
  /** Its icon emoji (FR-923): a Unicode emoji, or a `CustomEmoji`; without one, the room's glyph stands in. */
  icon?: ReactNode;
  /** Who hosts it and how many are in it, in the reader's words ("Hosted by Mika · 12 in the room"). */
  caption: ReactNode;
  /** Who speaks in it now, the first first; the first three show. */
  speakers?: readonly { name: string; src?: string | undefined }[] | undefined;
};

/**
 * A room going on, as a line of a list that is one link to it: its icon emoji (or the room's glyph) at the start, its
 * name over who hosts it and how many are in it, after the glyph of a call going on, and the faces of who speaks at
 * the end.
 */
export function RoomLine({ href, name, icon, caption, speakers = [] }: RoomLineProps) {
  const s = line();
  return (
    <Row
      as={<Link href={href} className={s.root()} />}
      align="center"
      gap="screen-row-gap"
      bleedInline="screen-row-inline"
    >
      <Row as="span" inline rigid align="center" justify="center" className={s.box()} aria-hidden="true">
        {icon ?? (
          <Row as="span" inline className={s.glyph()}>
            <Icon name="room" place="action" />
          </Row>
        )}
      </Row>
      <Stack as="span" grow gap="hair">
        <Box as="span" lines={2} className={s.name()} dir="auto">
          {name}
        </Box>
        <Row as="span" align="start" gap="entry-caption" className={s.caption()}>
          <Row as="span" line rigid align="center">
            <Icon name="live" place="caption" />
          </Row>
          <span>{caption}</span>
        </Row>
      </Stack>
      {speakers.length > 0 && (
        <Faces limit={3}>
          {speakers.map((p, i) => (
            <Avatar key={i} name={p.name} src={p.src} size="row" decorative />
          ))}
        </Faces>
      )}
    </Row>
  );
}

const control = tv({
  slots: {
    root: [
      "group box-border min-w-voice-control cursor-pointer rounded-control border-0 bg-transparent p-0",
      "font-sans text-caption font-label text-text outline-hidden data-focus-visible:shadow-focus",
      "data-disabled:cursor-not-allowed data-disabled:opacity-disabled",
    ],
    tile: [
      "size-voice-control rounded-overlay bg-surface-raised",
      "group-data-hovered:bg-surface-raised-hover group-data-pressed:bg-surface-raised-pressed",
    ],
    label: "[word-break:auto-phrase]",
  },
  variants: {
    // On, it is knocked out: the text's color as its fill, with no hue.
    on: {
      true: {
        tile: "bg-text text-surface group-data-hovered:bg-text group-data-pressed:bg-text",
      },
    },
  },
});

/** Whether a glyph has an on shape of its own, which a knocked-out tile shows; else the tile alone says it is on. */
function ownOn(name: IconName): boolean {
  const entry = icons[name];
  return "on" in entry && entry.on !== "tile";
}

export type RoomControlProps = {
  icon: IconName;
  /** Its name, shown under its glyph; a toggle's is the same on and off ("Mute", pressed while muted). */
  label: string;
  /** A toggle's state; none for an action that is no toggle (React, Leave). */
  pressed?: boolean | undefined;
  /** What a press does; or it is a trigger (the emoji picker's popover, a `MenuSurface`). */
  onPress?: (() => void) | undefined;
  isDisabled?: boolean | undefined;
};

/**
 * A control of the room's bar: its glyph on a raised tile a finger's size, its name under it. A toggle that is on is
 * knocked out, its glyph in its on shape where it has one of its own (the microphone crossed out).
 */
export function RoomControl({ icon, label, pressed, onPress, isDisabled }: RoomControlProps) {
  const on = pressed === true;
  const s = control({ on });
  const glyph = (
    <Row as="span" inline rigid align="center" justify="center" className={s.tile()}>
      <Icon name={icon} place="navigation" on={on && ownOn(icon)} />
    </Row>
  );
  const props = {
    className: s.root(),
    ...(onPress === undefined ? {} : { onPress }),
    ...(isDisabled === undefined ? {} : { isDisabled }),
  };
  return (
    <Stack
      as={pressed === undefined ? <AriaButton {...props} /> : <ToggleButton {...props} isSelected={pressed} />}
      justify="center"
      gap="hair"
    >
      {glyph}
      <span className={s.label()}>{label}</span>
    </Stack>
  );
}

/**
 * The room's bar of controls, one toolbar that the arrow keys move along: the reader's controls side by side (the
 * microphone or a raised hand, a reaction), and `end` (leaving) last and apart, at the end. Put it in the screen's
 * `FrameFoot`, so that it stays in reach while the room scrolls.
 */
export function RoomControls({ label, children, end }: { label: string; children: ReactNode; end?: ReactNode }) {
  return (
    <Row as={<Toolbar aria-label={label} />} align="start" gap="voice-control-gap">
      {children}
      {end !== undefined && (
        <Row push="end" rigid gap="voice-control-gap">
          {end}
        </Row>
      )}
    </Row>
  );
}
