// SPDX-License-Identifier: AGPL-3.0-only

// A feed's pull from its top: on a phone's touch screen, a reader at the very top of the page who pulls the feed down
// past a threshold and lets go has it read anew (its owner's `onPull`), as in other mobile clients. Whether its feeds
// may be pulled is the app's to say, as it alone reads the viewport (`FeedPulls`, ADR-0025 §4); a mouse or a pen
// never pulls, since only a touch is heard. The feed's own scrolling is the browser's throughout: the listeners are
// passive, a touch that starts away from the top, moves sideways first or up first is no pull, and a pull ends where
// the page scrolls. While a feed that pulls is on the page, the page has no pull of its own at its top (styles.css:
// no reload, no stretch or bounce under the mark).
//
// The mark comes down from above the feed into the room over it as the pull nears the threshold, clear as it comes,
// its arc growing toward a ring, muted until the pull is past the threshold, then a closed ring in the accent; it goes
// no lower, past the threshold nor while the feed is read, when it turns a three-quarter arc, so that it covers none
// of the entries' words. Where the room above the feed is shorter than the mark (no title or tabs over it), it stands
// as high as the view lets it, over the top of the first entry. It leaves up the way it came. With reduced motion it
// stands at its rest from the start of the pull, still: its arc says how far the pull is, the closed ring that it is
// past the threshold, and the three-quarter arc that the feed is read. While the feed is read, a polite live region
// says so; the mark itself is hidden from assistive technology.

import { feed as tokens, motion, transition } from "@my/design-tokens";
import { AnimatePresence, domMin, LazyMotion, m, useReducedMotion } from "motion/react";
import { createContext, use, useEffect, useEffectEvent, useRef, useState, type ReactNode, type RefObject } from "react";
import { tv } from "tailwind-variants/lite";
import { Layers, Spoken } from "./layout";
import { stringsIn, useReader } from "./locale";
import { curve } from "./modal";
import { scrollerOf, viewTopOf } from "./scroll";

const Pulls = createContext(false);

/** Whether the feeds inside may be pulled from their top: the app says so on a phone's viewport. */
export function FeedPulls({ on, children }: { on: boolean; children: ReactNode }) {
  return <Pulls value={on}>{children}</Pulls>;
}

/** Whether the feeds here may be pulled from their top (`FeedPulls`). */
export const useFeedPulls = (): boolean => use(Pulls);

/**
 * Where a pull stands: `pulling` short of the threshold, `armed` past it, `loading` once let go past it while the
 * feed is read; `distance` is how far the mark has come down, in rem.
 */
export type Pull = { readonly phase: "pulling" | "armed" | "loading"; readonly distance: number };

/** The lengths a pull is measured by, in rem (feed of @my/design-tokens), and the share of the finger's way. */
export type PullMeasure = {
  readonly slop: number;
  readonly threshold: number;
  readonly most: number;
  readonly resistance: number;
};

export const pullMeasure: PullMeasure = {
  slop: tokens.pullSlop,
  threshold: tokens.pullThreshold,
  most: tokens.pullMost,
  resistance: tokens.pullResistance,
};

/** What a pull answers to the touches it is told of. */
export type PullGesture = {
  /** A touch began at `y` (rem) and `x`, with `touches` fingers on the screen. */
  start: (x: number, y: number, touches: number) => void;
  /** The finger moved: the pull it makes, or undefined while it makes none (and undefined from then on, once not). */
  move: (x: number, y: number, touches: number) => Pull | undefined;
  /** The finger left the screen: whether it let go past the threshold. */
  end: () => boolean;
  /** The browser took the touch over (a gesture of its own): nothing is let go. */
  cancel: () => void;
};

/**
 * The pull of one touch at a time, by `measure`, while `atTop` says the page is at its very top: it starts with one
 * finger at the top, and is a pull once the finger has gone `slop` and more down than sideways. A touch that goes up
 * or sideways first, a second finger, or a page that left its top ends it, and nothing is pulled until the next touch.
 */
export function pullGesture(measure: PullMeasure, atTop: () => boolean): PullGesture {
  let from: { x: number; y: number } | undefined;
  let pulled = false;
  let last: Pull | undefined;
  const stop = () => {
    from = undefined;
    pulled = false;
    last = undefined;
  };
  return {
    start: (x, y, touches) => {
      stop();
      if (touches === 1 && atTop()) from = { x, y };
    },
    move: (x, y, touches) => {
      if (from === undefined) return undefined;
      if (touches !== 1 || !atTop()) {
        stop();
        return undefined;
      }
      const down = y - from.y;
      if (!pulled) {
        const across = Math.abs(x - from.x);
        if (Math.max(across, Math.abs(down)) < measure.slop) return undefined;
        if (down <= across) {
          stop();
          return undefined;
        }
        pulled = true;
      }
      const distance = Math.min(measure.most, Math.max(0, down - measure.slop) * measure.resistance);
      last = { phase: distance >= measure.threshold ? "armed" : "pulling", distance };
      return last;
    },
    end: () => {
      const armed = last?.phase === "armed";
      stop();
      return armed;
    },
    cancel: stop,
  };
}

// The mark: a disc of the raised surface, bounded by a line so that it stands off the page in light, holding an arc
// that grows with the pull toward a full ring, muted until the pull is past the threshold, then whole in the accent;
// while the feed is read, a three-quarter arc that turns (and stands still with reduced motion).
const pull = tv({
  slots: {
    mark: [
      "size-feed-pull-mark rounded-full bg-surface-raised shadow-overlay",
      "outline-1 -outline-offset-1 outline-solid outline-surface-raised-pressed",
    ],
    ring: motion.spin.reduced === "still" ? "motion-reduce:animate-none" : "",
    track: "fill-none stroke-spinner-track",
    arc: "fill-none stroke-current",
  },
  variants: {
    phase: {
      // In dark, the muted arc on the raised disc reads faintly: it is drawn in the text's color there.
      pulling: {
        ring: "text-muted-text [:root[data-theme=dark]_&]:text-text dark:[:root:not([data-theme=light])_&]:text-text",
      },
      armed: { ring: "text-accent-text" },
      loading: { ring: "animate-spin text-accent-text" },
    },
  },
});

/** The share of the ring the arc draws while the feed is read. */
const loadingArc = 0.75;

const follow = { duration: 0 };
const settle = { duration: transition.duration.pop / 1000, ease: curve(transition.ease.enter) };
const leave = { duration: transition.duration.pop / 1000, ease: curve(transition.ease.exit) };

/** The ring of a pull: its track, and over it from the top the share `arc` of the ring, a closed circle when whole. */
function Ring({ phase, arc }: { phase: Pull["phase"]; arc: number }) {
  const s = pull({ phase });
  const size = tokens.pullRing;
  const r = (size - tokens.pullStroke) / 2;
  const middle = size / 2;
  return (
    <svg
      width={`${String(size)}rem`}
      height={`${String(size)}rem`}
      viewBox={`0 0 ${String(size)} ${String(size)}`}
      className={s.ring()}
    >
      <circle className={s.track()} cx={middle} cy={middle} r={r} strokeWidth={tokens.pullStroke} />
      <circle
        className={s.arc()}
        cx={middle}
        cy={middle}
        r={r}
        strokeWidth={tokens.pullStroke}
        {...(arc < 1
          ? {
              pathLength: 1,
              strokeDasharray: `${String(arc)} 1`,
              transform: `rotate(-90 ${String(middle)} ${String(middle)})`,
            }
          : {})}
      />
    </svg>
  );
}

/** Where the mark rests by default: in the room above the feed, its bottom at the feed's top. */
export const pullRest = -tokens.pullMark;

/**
 * The mark of a pull: it comes down a diameter to its `rest` (where its top stands from the top of what holds it, in
 * rem) as the pull nears the threshold, clear as it comes, and stays there past it and while the feed is read, so
 * that it never covers an entry's words; its arc is the share of the threshold pulled. With reduced motion it stands
 * at its rest, whole, from the start.
 */
function PullMark({ phase, distance, rest }: Pull & { rest: number }) {
  const s = pull({ phase });
  const reduced = useReducedMotion() ?? false;
  const share = Math.min(1, distance / pullMeasure.threshold);
  const away = reduced || phase !== "pulling" ? 0 : 1 - share;
  const y = `${String(rest - away * tokens.pullMark)}rem`;
  const above = `${String(rest - tokens.pullMark)}rem`;
  const arc = phase === "loading" ? loadingArc : phase === "armed" ? 1 : share;
  return (
    <Layers
      center
      as={
        <m.div
          aria-hidden="true"
          initial={reduced || phase !== "pulling" ? false : { y: above, opacity: 0 }}
          animate={{ y, opacity: reduced || phase !== "pulling" ? 1 : share }}
          exit={reduced ? { opacity: 0, transition: follow } : { y: above, opacity: 0, transition: leave }}
          transition={phase === "loading" ? settle : follow}
        />
      }
      className={s.mark()}
    >
      <Ring phase={phase} arc={arc} />
    </Layers>
  );
}

/**
 * The mark of `pull` while there is one, which leaves by its motion as the pull ends; `rest` is where its top stands
 * from the feed's top, in rem (`pullRest`: in the room above the feed).
 */
export function PullIndicator({ pull: shown, rest = pullRest }: { pull: Pull | undefined; rest?: number }) {
  return (
    <LazyMotion features={domMin} strict>
      <AnimatePresence>{shown && <PullMark key="mark" {...shown} rest={rest} />}</AnimatePresence>
    </LazyMotion>
  );
}

/**
 * Where a feed's pull shows, in its first row: a strip of no height at its top, over its entries, that holds the mark
 * at its middle and takes no touch; a polite live region, which says that the feed is read anew.
 */
export function PullPlace({ children }: { children: ReactNode }) {
  return (
    <div role="status" data-feed-pulls="" className="pointer-events-none z-2 flex h-0 justify-center self-start">
      {children}
    </div>
  );
}

/** The root's font size in px, by which a touch's way is told in rem. */
function remOf(el: Element): number {
  const view = el.ownerDocument.defaultView;
  const size = view ? Number.parseFloat(view.getComputedStyle(el.ownerDocument.documentElement).fontSize) : NaN;
  return Number.isFinite(size) && size > 0 ? size : 16;
}

/**
 * The pull of the feed whose root is `target`, which `onPull` reads anew when let go past the threshold: it hears the
 * touches that start on the feed, the mark showing until what `onPull` returns has settled, and says while it is read.
 */
export function FeedPull({
  target,
  onPull,
}: {
  target: RefObject<HTMLElement | null>;
  onPull: () => Promise<unknown>;
}) {
  const [shown, show] = useState<Pull>();
  // Where the mark rests: above the feed, or as high as the view lets it where the room above the feed is short.
  const [rest, setRest] = useState(pullRest);
  const reading = useRef(false);
  const { t, lang } = stringsIn(useReader(undefined).lang);
  const read = useEffectEvent(() => {
    reading.current = true;
    show({ phase: "loading", distance: pullMeasure.threshold });
    const done = () => {
      reading.current = false;
      show(undefined);
    };
    onPull().then(done, done);
  });
  useEffect(() => {
    const el = target.current;
    if (!el) return;
    const scroller = scrollerOf(el);
    const gesture = pullGesture(pullMeasure, () => scroller.scrollTop <= 0);
    let rem = 16;
    const point = (e: TouchEvent) => e.touches[0];
    const onStart = (e: TouchEvent) => {
      const touch = point(e);
      if (reading.current || !touch) return;
      rem = remOf(el);
      const room = (el.getBoundingClientRect().top - viewTopOf(scroller)) / rem;
      setRest(Math.max(pullRest, tokens.pullInset - room));
      gesture.start(touch.clientX / rem, touch.clientY / rem, e.touches.length);
    };
    const onMove = (e: TouchEvent) => {
      if (reading.current) return;
      const touch = point(e);
      const now = touch && gesture.move(touch.clientX / rem, touch.clientY / rem, e.touches.length);
      show(now);
    };
    const onEnd = () => {
      if (reading.current) return;
      if (gesture.end()) read();
      else show(undefined);
    };
    const onCancel = () => {
      if (reading.current) return;
      gesture.cancel();
      show(undefined);
    };
    const options = { passive: true } as const;
    el.addEventListener("touchstart", onStart, options);
    el.addEventListener("touchmove", onMove, options);
    el.addEventListener("touchend", onEnd, options);
    el.addEventListener("touchcancel", onCancel, options);
    return () => {
      el.removeEventListener("touchstart", onStart);
      el.removeEventListener("touchmove", onMove);
      el.removeEventListener("touchend", onEnd);
      el.removeEventListener("touchcancel", onCancel);
    };
  }, [target]);
  return (
    <PullPlace>
      <PullIndicator pull={shown} rest={rest} />
      {shown?.phase === "loading" && (
        <Spoken>
          <span lang={lang}>{t("feed.loading_newer")}</span>
        </Spoken>
      )}
    </PullPlace>
  );
}
