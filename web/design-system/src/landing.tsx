// SPDX-License-Identifier: AGPL-3.0-only

// The parts of a server's front page for a visitor (the landing page), besides those it shares with the pages of its
// own outside the app (form-page.tsx): its frame, one reading column at every width with the server's banner at the
// top; a section of what it tells under a heading; the figure of the fediverse; the samples of the screens of what a
// member can do there; and the low-key line of the places a visitor may go to without an account. Like the frame of
// the form pages, the frame carries the page's surface onto <html>.

import { figureMotion } from "@my/design-tokens";
import {
  domAnimation,
  LazyMotion,
  m,
  useReducedMotionConfig,
  type TargetAndTransition,
  type Transition,
} from "motion/react";
import type { ReactNode } from "react";
import { Link } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Icon, type IconName } from "./icons";
import { Box, Row, Stack } from "./layout";
import { curve } from "./modal";

const frame = tv({
  slots: {
    root: [
      "box-border flex min-h-dvh justify-center bg-surface px-shell-edge pt-form-page-top pb-form-page-bottom font-sans",
      "text-body text-text [body:has(&)]:m-0 [html:has(&)]:bg-surface tablet:pt-form-page-top-wide",
    ],
    body: "grid w-full max-w-[var(--shell-column)] grid-cols-[minmax(0,1fr)] content-start gap-form-page-gap",
    intro: "grid grid-cols-[minmax(0,1fr)] content-start gap-form-page-intro-gap",
    banner: "aspect-banner w-full rounded-overlay object-cover",
    join: "grid grid-cols-[minmax(0,1fr)] content-start gap-form-page-form-gap",
    main: "grid grid-cols-[minmax(0,1fr)] content-start gap-form-page-gap",
  },
});

/**
 * The frame of the landing page, one reading column at every width: the introduction, the account's part, what the
 * page tells, and the footer.
 */
export function LandingFrame({
  banner,
  intro,
  join,
  footer,
  label,
  children,
}: {
  /** The server's banner (its thumbnail), at the top of the introduction; it says nothing the name does not. */
  banner?: string | undefined;
  /** The server's introduction: its mark and name, its summary, its counts. */
  intro: ReactNode;
  /** The account's part: joining, or signing in. */
  join: ReactNode;
  footer?: ReactNode;
  /** The name of the account's part for assistive technology. */
  label: string;
  children: ReactNode;
}) {
  const s = frame();
  return (
    <div className={s.root()}>
      <div className={s.body()}>
        <header className={s.intro()}>
          {banner !== undefined && banner !== "" && <img className={s.banner()} src={banner} alt="" />}
          {intro}
        </header>
        <section className={s.join()} aria-label={label}>
          {join}
        </section>
        <main className={s.main()}>{children}</main>
        {footer}
      </div>
    </div>
  );
}

/**
 * The head of the introduction: the server's mark and name, which wraps under itself, and what stands at the end of
 * its line (Sign in), which keeps its size and stays on the line however long the name.
 */
export function LandingHead({ mark, end }: { mark: ReactNode; end?: ReactNode }) {
  return (
    <Row align="center" justify="between" gap="form-page-mark-gap">
      <Row grow>{mark}</Row>
      {end !== undefined && end !== false && <Row rigid>{end}</Row>}
    </Row>
  );
}

const parts = tv({
  slots: {
    heading: "text-title",
    // In a right-to-left page it keeps to the right, whatever its own direction.
    summary: "rtl:text-right",
    places: "text-caption text-muted-text",
    // A place reads as the words around it; its target reaches the minimum around its text without moving the line
    // (WCAG 2.5.8).
    place: "rounded-control py-target-em text-muted-text no-underline outline-hidden data-focus-visible:shadow-focus",
  },
});

/**
 * The server's summary, the operator's words in their own direction; in a right-to-left page from its right side,
 * where the column starts, whatever their direction.
 */
export function LandingSummary({ children }: { children: string }) {
  return (
    <p className={parts().summary()} dir="auto">
      {children}
    </p>
  );
}

/** A part of what the page tells, under its heading. */
export function LandingSection({ id, heading, children }: { id: string; heading: string; children: ReactNode }) {
  return (
    <Stack as="section" gap="form-page-head-gap" aria-labelledby={id}>
      <h2 id={id} className={parts().heading()} dir="auto">
        {heading}
      </h2>
      {children}
    </Stack>
  );
}

const figure = tv({
  slots: {
    // Drawn toward the start of the line: this server stands at the start, in either direction.
    root: "rtl:-scale-x-100",
    follow: "fill-none stroke-muted-text",
    here: "fill-text",
    hereMark: "fill-surface",
    other: "fill-surface-raised-pressed",
    otherMark: "fill-muted-text",
    post: "fill-surface stroke-text",
    postMark: "fill-text",
  },
});

type Point = { readonly x: number; readonly y: number };

/** A server of the figure, a box in its own units, with the people on it. */
type Place = { readonly x: number; readonly y: number; readonly width: number; readonly height: number };

/** The servers, each a place of its own size: this one, the largest, at the start, and three others. */
const servers = {
  here: { x: 16, y: 60, width: 136, height: 88 },
  north: { x: 196, y: 14, width: 116, height: 72 },
  east: { x: 350, y: 92, width: 114, height: 74 },
  south: { x: 196, y: 138, width: 100, height: 62 },
} as const satisfies Record<string, Place>;

/** The people of the servers, one or two on each. */
const people = {
  hereA: { x: 58, y: 116 },
  hereB: { x: 110, y: 116 },
  northA: { x: 234, y: 58 },
  northB: { x: 276, y: 58 },
  east: { x: 407, y: 137 },
  south: { x: 246, y: 177 },
} as const satisfies Record<string, Point>;

/** Who follows whom, across servers: the lines a post travels along. */
const follows: readonly (readonly [Point, Point])[] = [
  [people.hereB, people.northA],
  [people.hereB, people.south],
  [people.northB, people.east],
  [people.hereA, people.east],
];

/** The posts of a cycle: one of someone here to a follower on another server, and one from another server back. */
const journeys: readonly { readonly from: Point; readonly to: Point; readonly start: number }[] = [
  { from: people.hereB, to: people.northA, start: 0 },
  { from: people.east, to: people.hereA, start: 0.5 },
];

/** The size of a person, and of a post, in the figure's units. */
const person = 26;
const post = { width: 30, height: 22 };

/** A server: a box with the bar of a window, a place of its own. */
function Server({ place, className, markClassName }: { place: Place; className: string; markClassName: string }) {
  return (
    <g>
      <rect className={className} x={place.x} y={place.y} width={place.width} height={place.height} rx={14} />
      {[0, 1, 2].map((i) => (
        <circle key={i} className={markClassName} cx={place.x + 14 + i * 9} cy={place.y + 13} r={2.5} />
      ))}
    </g>
  );
}

/** A person: a head over shoulders. */
function Person({ at, className }: { at: Point; className: string }) {
  const shoulders = person * 0.38;
  return (
    <g className={className}>
      <circle cx={at.x} cy={at.y - person * 0.18} r={person * 0.2} />
      <path
        d={`M ${at.x - shoulders} ${at.y + person * 0.4} a ${shoulders} ${shoulders * 0.9} 0 0 1 ${shoulders * 2} 0 Z`}
      />
    </g>
  );
}

/** A post: a card with its author's face and two lines of text. */
function Post({ className, markClassName }: { className: string; markClassName: string }) {
  return (
    <>
      <rect
        className={className}
        x={-post.width / 2}
        y={-post.height / 2}
        width={post.width}
        height={post.height}
        rx={4}
        strokeWidth={1.5}
      />
      <circle className={markClassName} cx={-8} cy={-4} r={3} />
      <rect className={markClassName} x={-3} y={-5.5} width={13} height={2.5} rx={1.25} />
      <rect className={markClassName} x={-10} y={2.5} width={20} height={2.5} rx={1.25} />
    </>
  );
}

const seconds = (ms: number) => ms / 1000;

/**
 * One post's cycle, without end: once `start` of the cycle is past it comes out at its author, travels the follow,
 * and goes into the follower it reaches, then waits, unseen, for the next cycle.
 */
function journey(from: Point, to: Point, start: number): { animate: TargetAndTransition; transition: Transition } {
  const { cycle, fade, travel, ease } = figureMotion;
  const out = start + fade / cycle;
  const reach = out + travel / cycle;
  const gone = reach + fade / cycle;
  return {
    animate: {
      x: [from.x, from.x, from.x, to.x, to.x, to.x],
      y: [from.y, from.y, from.y, to.y, to.y, to.y],
      opacity: [0, 0, 1, 1, 0, 0],
    },
    transition: {
      duration: seconds(cycle),
      times: [0, start, out, reach, gone, 1],
      ease: ["linear", "linear", curve(ease), "linear", "linear"],
      repeat: Infinity,
    },
  };
}

/** Where a post stands on the still figure: half way along its line. */
const halfWay = (from: Point, to: Point) => ({ x: (from.x + to.x) / 2, y: (from.y + to.y) / 2, opacity: 1 });

/**
 * The fediverse in a figure, which says nothing its sentence beside it does not: servers, each a place of its own with
 * its people, this one at the start and marked; follows from people here to people on the others; and a post
 * travelling along a follow to another server while another comes back, cycle after cycle at a calm pace
 * (figureMotion). With reduced motion it stands still, each post half way along its line. As wide as its column.
 */
export function FediverseFigure() {
  const s = figure();
  const reduced = useReducedMotionConfig() ?? false;
  const others = [servers.north, servers.east, servers.south];
  return (
    <Box full as={<svg className={s.root()} viewBox="0 0 480 210" aria-hidden="true" focusable="false" />}>
      <Server place={servers.here} className={s.here()} markClassName={s.hereMark()} />
      {others.map((place, i) => (
        <Server key={i} place={place} className={s.other()} markClassName={s.otherMark()} />
      ))}
      {follows.map(([a, b], i) => (
        <line key={i} className={s.follow()} x1={a.x} y1={a.y} x2={b.x} y2={b.y} strokeWidth={1.5} />
      ))}
      <Person at={people.hereA} className={s.hereMark()} />
      <Person at={people.hereB} className={s.hereMark()} />
      {[people.northA, people.northB, people.east, people.south].map((at, i) => (
        <Person key={i} at={at} className={s.otherMark()} />
      ))}
      <LazyMotion features={domAnimation} strict>
        {journeys.map(({ from, to, start }, i) => {
          const run = journey(from, to, start);
          return (
            <m.g
              key={i}
              initial={reduced ? halfWay(from, to) : { x: from.x, y: from.y, opacity: 0 }}
              animate={reduced ? halfWay(from, to) : run.animate}
              transition={run.transition}
            >
              <Post className={s.post()} markClassName={s.postMark()} />
            </m.g>
          );
        })}
      </LazyMotion>
    </Box>
  );
}

const samples = tv({
  slots: {
    grid: "m-0 grid list-none grid-cols-[repeat(auto-fill,minmax(min(100%,var(--landing-sample)),1fr))] gap-form-page-gap p-0",
    // A sample and its words share the rows of the samples beside it: the frames of a row as tall as its tallest, each
    // sample in the middle of its frame, and their names on one line; the words keep to the top of their row.
    cell: "row-span-2 grid grid-rows-subgrid gap-y-form-page-head-gap [&>:last-child]:self-start",
    frame: "rounded-overlay bg-surface-raised p-landing-sample-padding",
    // The screen in miniature, drawn at its scale; its layout takes the room it is drawn in.
    screen: "rounded-control bg-surface p-landing-sample-padding [zoom:var(--landing-sample-scale)]",
    name: "font-label",
  },
});

/** The samples, in a grid of as many of the least width side by side as the column holds. */
function SampleGrid({ children }: { children: ReactNode }) {
  return <ul className={samples().grid()}>{children}</ul>;
}

/** A sample over its words, in the rows it shares with the samples beside it. */
function SampleCell({ sample, words }: { sample: ReactNode; words: ReactNode }) {
  return (
    <li className={samples().cell()}>
      {sample}
      {words}
    </li>
  );
}

/**
 * What a member can do on the server, each a sample of its screen in miniature, which shows and does nothing for
 * assistive technology or the pointer, over its name and a sentence that say what it is.
 */
export function LandingSamples({
  items,
}: {
  items: readonly { key: string; name: string; text: string; sample: ReactNode }[];
}) {
  const s = samples();
  return (
    <SampleGrid>
      {items.map((item) => (
        <SampleCell
          key={item.key}
          sample={
            <Stack pack="center" clip as={<div className={s.frame()} aria-hidden="true" inert />}>
              <div className={s.screen()}>{item.sample}</div>
            </Stack>
          }
          words={
            <Stack gap="hair">
              <h3 className={s.name()} dir="auto">
                {item.name}
              </h3>
              <p dir="auto">{item.text}</p>
            </Stack>
          }
        />
      ))}
    </SampleGrid>
  );
}

/**
 * The places a visitor may go to without an account, low-key: `heading`, and the places under it, a line each, as far
 * apart as the heading is from the first.
 */
export function LandingPlaces({
  heading,
  items,
}: {
  heading: string;
  items: readonly { href: string; icon: IconName; label: string }[];
}) {
  const s = parts();
  if (items.length === 0) return null;
  return (
    <Stack as="nav" gap="form-page-head-gap" justify="start" aria-label={heading} className={s.places()}>
      <span>{heading}</span>
      {items.map((item) => (
        <Row key={item.href} align="center" gap="entry-caption" as={<Link href={item.href} className={s.place()} />}>
          <Icon name={item.icon} place="caption" />
          {item.label}
        </Row>
      ))}
    </Stack>
  );
}
