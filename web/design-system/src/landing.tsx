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
    places: "text-caption text-muted-text",
    // A place reads as the words around it; its target reaches the minimum around its text without moving the line
    // (WCAG 2.5.8).
    place: "rounded-control py-target-em text-muted-text no-underline outline-hidden data-focus-visible:shadow-focus",
  },
});

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
    hereFace: "fill-surface",
    other: "fill-surface-raised-pressed",
    otherFace: "fill-muted-text",
    post: "fill-text stroke-surface",
  },
});

type Point = { readonly x: number; readonly y: number };

/** The servers of the figure, in its own units: this one, larger, at the start, and three others. */
const here = { x: 60, y: 72, size: 48 };
const others = [
  { x: 200, y: 30, size: 36 },
  { x: 270, y: 104, size: 36 },
  { x: 156, y: 122, size: 30 },
] as const;
const [north, east, south] = others;

/** The follows between people of the servers, the lines a post travels along. */
const follows: readonly (readonly [Point, Point])[] = [
  [here, north],
  [here, east],
  [north, east],
  [here, south],
];

/** The posts of a cycle: one of this server's to a follower on another, and one of another's back here. */
const journeys: readonly { readonly from: Point; readonly to: Point; readonly start: number }[] = [
  { from: here, to: north, start: 0 },
  { from: east, to: here, start: 0.5 },
];

const post = { width: 14, height: 10 };

/** A server, a rounded square with the face of a person on it. */
function Server({
  x,
  y,
  size,
  faceClassName,
  tileClassName,
}: {
  x: number;
  y: number;
  size: number;
  faceClassName: string;
  tileClassName: string;
}) {
  const r = size * 0.16;
  const shoulders = size * 0.26;
  return (
    <g>
      <rect className={tileClassName} x={x - size / 2} y={y - size / 2} width={size} height={size} rx={size * 0.24} />
      <circle className={faceClassName} cx={x} cy={y - size * 0.1} r={r} />
      <path
        className={faceClassName}
        d={`M ${x - shoulders} ${y + size * 0.3} a ${shoulders} ${shoulders * 0.85} 0 0 1 ${shoulders * 2} 0 Z`}
      />
    </g>
  );
}

const seconds = (ms: number) => ms / 1000;

/**
 * One post's cycle, without end: it sets out from under the server it leaves once `start` of the cycle is past,
 * travels the line, and waits under the one it reaches for the next cycle, which takes it back under the first at
 * once, unseen.
 */
function journey(from: Point, to: Point, start: number): { animate: TargetAndTransition; transition: Transition } {
  const { cycle, travel, ease } = figureMotion;
  const reach = start + travel / cycle;
  return {
    animate: { x: [from.x, from.x, to.x, to.x], y: [from.y, from.y, to.y, to.y] },
    transition: {
      duration: seconds(cycle),
      times: [0, start, reach, 1],
      ease: ["linear", curve(ease), "linear"],
      repeat: Infinity,
    },
  };
}

/** Where a post stands on the still figure: half way along its line. */
const halfWay = (from: Point, to: Point) => ({ x: (from.x + to.x) / 2, y: (from.y + to.y) / 2 });

/**
 * The fediverse in a figure, which says nothing its sentence beside it does not: this server among others, joined by
 * the follows of their people, and a post travelling along a follow to another server while another comes back, cycle
 * after cycle at a calm pace (figureMotion); with reduced motion it stands still, each post half way along its line.
 * Only the posts move, by their position alone.
 */
export function FediverseFigure() {
  const s = figure();
  const reduced = useReducedMotionConfig() ?? false;
  return (
    <Box
      full
      max="landing-figure"
      as={<svg className={s.root()} viewBox="0 0 320 148" aria-hidden="true" focusable="false" />}
    >
      {follows.map(([a, b], i) => (
        <line key={i} className={s.follow()} x1={a.x} y1={a.y} x2={b.x} y2={b.y} strokeWidth={1.5} />
      ))}
      <LazyMotion features={domAnimation} strict>
        {journeys.map(({ from, to, start }, i) => {
          const run = journey(from, to, start);
          return (
            <m.g
              key={i}
              initial={reduced ? halfWay(from, to) : { x: from.x, y: from.y }}
              animate={reduced ? halfWay(from, to) : run.animate}
              transition={run.transition}
            >
              <rect
                className={s.post()}
                x={-post.width / 2}
                y={-post.height / 2}
                width={post.width}
                height={post.height}
                rx={2}
                strokeWidth={1.5}
              />
            </m.g>
          );
        })}
      </LazyMotion>
      {others.map((o, i) => (
        <Server key={i} {...o} tileClassName={s.other()} faceClassName={s.otherFace()} />
      ))}
      <Server {...here} tileClassName={s.here()} faceClassName={s.hereFace()} />
    </Box>
  );
}

const samples = tv({
  slots: {
    grid: "m-0 grid list-none grid-cols-[repeat(auto-fill,minmax(min(100%,var(--landing-sample)),1fr))] gap-form-page-gap p-0",
    // A sample and its words share the rows of the samples beside it: the frames of a row as tall as its tallest, and
    // their names on one line; the words keep to the top of their row.
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
            <Box clip as={<div className={s.frame()} aria-hidden="true" inert />}>
              <div className={s.screen()}>{item.sample}</div>
            </Box>
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
