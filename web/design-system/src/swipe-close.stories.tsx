// SPDX-License-Identifier: AGPL-3.0-only

// Closing a modal by a swipe down: the composer on a phone (#3523) and the media viewer (#3524), drawn as stand-ins
// that run the motion the build is to have, over a page. Run it with a finger (a phone's viewport) or a mouse.
//
// How it moves (the motion decisions: states, screens; the tokens `swipeClose` and `transition`):
//
// - Where a swipe starts: the composer's head and its row of the action, never what is written (the text, the
//   warning, a description, a poll, which scroll and select), and nothing while a post is in flight (`busy`); the
//   viewer's room while the picture is not zoomed, with one finger.
// - The first `swipeClose.slop` (0.5rem) decides: down more than across, it is a swipe to close for the rest of the
//   touch; across or up first, it is what it was before (the viewer's move between the media on release), never a
//   close. A second finger ends the swipe, which springs back.
// - The surface follows the finger down at once, as far as it goes, never above its place; what is under shows as it
//   comes down: the backing fades by the way, half gone at `swipeClose.distance` (6rem), gone at twice it. In the
//   viewer the bar fades with the backing it stands on, and the picture and its description move together.
// - Let go `distance` down or more, or at a flick down of `swipeClose.velocity` (25 rem/s) over the drag's last
//   `window` (100 ms), it closes as Close does: the composer discards what was written and its notice offers Undo
//   (`onClose`), the viewer gives the focus back to what opened it. A flick up at the release springs back even past
//   the distance. How it leaves is SwipeLeave's options.
// - Else it springs back to its place in a pop's time (250 ms) with the easing of what enters, the backing with it,
//   and nothing else changes: the focus and the keyboard stay.
// - With reduced motion the surface stays in place and fades instead, by the share the backing keeps; let go, it is
//   gone at once or whole again at once.
//
// What the stand-ins are not: the composer's frame is the design system's (a specimen, so its own head and row), the
// page under it a sample; the viewer is drawn bare. The words are samples in English.

import { swipeClose, transition } from "@my/design-tokens";
import { Options } from "@my/devtools/options";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useReducedMotion } from "motion/react";
import { useRef, useState, type CSSProperties, type PointerEvent, type ReactNode } from "react";
import { Button } from "./button";
import { CharCount, ComposerFrame, ComposerText } from "./composer";
import { PlainTextEditor } from "./editor";
import { IconButton } from "./icon-button";
import { wide } from "./viewer.samples";

/**
 * How a modal let go past the distance leaves: `slide`, on down out of the viewport from where it was let go;
 * `fall`, the modal's own leave from there, down `transition.rise` more as it fades out. Either in a modal's time with
 * the easing of what leaves, the backing fading out with it.
 */
type Leave = "slide" | "fall";

type Phase = "rest" | "drag" | "back" | "closing" | "closed";

const remPx = () => Number.parseFloat(getComputedStyle(document.documentElement).fontSize) || 16;

/** The share of the backing's strength left with the surface `down` px down. */
const kept = (down: number) => Math.max(0, 1 - down / (swipeClose.fade * swipeClose.distance * remPx()));

/**
 * A swipe down that closes, by the tokens `swipeClose`: `starts` says whether a pointer down on its target may start
 * one, `height` how far the surface goes to leave the viewport.
 */
function useSwipeClose(leave: Leave, starts: (target: Element) => boolean) {
  const reduced = useReducedMotion() ?? false;
  const [phase, setPhase] = useState<Phase>("rest");
  const [down, setDown] = useState(0);
  const [height, setHeight] = useState(0);
  const touch = useRef<{ id: number; x: number; y: number; swipe: boolean; way: { t: number; y: number }[] } | null>(
    null,
  );
  const settle = (to: Phase, ms: number) => {
    window.setTimeout(() => {
      setPhase(to);
      if (to === "rest") setDown(0);
    }, ms);
  };
  const back = () => {
    touch.current = null;
    setPhase("back");
    setDown(0);
    settle("rest", reduced ? 0 : transition.duration.pop);
  };
  const handlers = {
    onPointerDown: (e: PointerEvent<HTMLElement>) => {
      if (touch.current !== null) {
        if (touch.current.swipe) back();
        touch.current = null;
        return;
      }
      if (phase !== "rest" || !starts(e.target as Element)) return;
      touch.current = { id: e.pointerId, x: e.clientX, y: e.clientY, swipe: false, way: [] };
    },
    onPointerMove: (e: PointerEvent<HTMLElement>) => {
      const t = touch.current;
      if (t?.id !== e.pointerId) return;
      const dx = e.clientX - t.x;
      const dy = e.clientY - t.y;
      if (!t.swipe) {
        if (Math.max(Math.abs(dx), Math.abs(dy)) < swipeClose.slop * remPx()) return;
        if (dy <= Math.abs(dx)) {
          touch.current = null;
          return;
        }
        t.swipe = true;
        e.currentTarget.setPointerCapture(e.pointerId);
        setHeight(e.currentTarget.getBoundingClientRect().height);
        setPhase("drag");
      }
      t.way = [...t.way.filter((p) => e.timeStamp - p.t <= swipeClose.window), { t: e.timeStamp, y: e.clientY }];
      setDown(Math.max(0, dy));
    },
    onPointerUp: (e: PointerEvent<HTMLElement>) => {
      const t = touch.current;
      if (t?.id !== e.pointerId) return;
      touch.current = null;
      if (!t.swipe) return;
      const first = t.way[0];
      const last = t.way.at(-1);
      const pxPerMs = first && last && last.t > first.t ? (last.y - first.y) / (last.t - first.t) : 0;
      const speed = (pxPerMs * 1000) / remPx();
      const far = Math.max(0, e.clientY - t.y) >= swipeClose.distance * remPx();
      if ((far && speed > -swipeClose.velocity) || speed >= swipeClose.velocity) {
        setPhase("closing");
        settle("closed", reduced ? 0 : transition.duration.modal);
      } else back();
    },
    onPointerCancel: (e: PointerEvent<HTMLElement>) => {
      if (touch.current?.id === e.pointerId) back();
    },
  };
  const rem = (n: number) => `${String(n)}rem`;
  const timed = (ms: number, ease: string) =>
    reduced || ms === 0 ? "none" : `transform ${String(ms)}ms ${ease}, opacity ${String(ms)}ms ${ease}`;
  const time =
    phase === "back"
      ? timed(transition.duration.pop, transition.ease.enter)
      : phase === "closing"
        ? timed(transition.duration.modal, transition.ease.exit)
        : "none";
  const share = phase === "closing" ? 0 : kept(down);
  const leaving = phase === "closing";
  const surface: CSSProperties = reduced
    ? { opacity: leaving ? 0 : share, transition: time }
    : {
        transform: leaving
          ? leave === "slide"
            ? `translateY(${String(height)}px)`
            : `translateY(calc(${String(down)}px + ${rem(transition.rise)}))`
          : `translateY(${String(down)}px)`,
        opacity: leaving && leave === "fall" ? 0 : 1,
        transition: time,
      };
  const backing: CSSProperties = { opacity: share, transition: time };
  return {
    closed: phase === "closed",
    reopen: () => {
      setDown(0);
      setPhase("rest");
    },
    handlers,
    surface,
    backing,
  };
}

/** A page under the modal, as a timeline shows it. */
function Page({ closed, onReopen }: { closed: boolean; onReopen: () => void }) {
  return (
    <div className="absolute inset-0 grid content-start gap-entry overflow-hidden bg-surface-raised p-shell-edge text-body text-text">
      {closed && (
        <Button kind="primary" size="timeline" onPress={onReopen}>
          Open it again
        </Button>
      )}
      {["Morning at the river.", "The new bakery opens on Friday.", "Rain all week, and the garden loves it."].map(
        (line) => (
          <p key={line} className="rounded-overlay bg-surface px-shell-edge py-entry">
            {line}
          </p>
        ),
      )}
    </div>
  );
}

/** The composer on a phone, a screen of its own: a swipe starts on its head or its row, never in the body. */
function ComposerSwipe({ leave, busy = false }: { leave: Leave; busy?: boolean }) {
  // The specimen's panel holds the head, the body and the row, in that order: the body is its second child.
  const swipe = useSwipeClose(leave, (target) => {
    const panel = target.closest("[role=dialog]")?.firstElementChild;
    return !busy && panel?.children[1]?.contains(target) === false;
  });
  const [text, setText] = useState("Walked along the river this morning, and the light on the water was worth it.");
  return (
    <div className="relative h-dvh overflow-hidden">
      <Page closed={swipe.closed} onReopen={swipe.reopen} />
      {!swipe.closed && (
        <>
          <div className="absolute inset-0 bg-surface" style={swipe.backing} />
          <div
            className="absolute inset-0 bg-surface [&_[role=dialog]>div>:not(:nth-child(2))]:touch-none [&_[role=dialog]>div]:min-h-dvh"
            style={swipe.surface}
            {...swipe.handlers}
          >
            <ComposerFrame
              title="New post"
              closeLabel="Close"
              onClose={() => undefined}
              onDismiss={() => undefined}
              busy={busy}
              layer={false}
              tools={<IconButton icon="attach" label="Attach" />}
              count={<CharCount left={420} near={20} label="420" />}
              action={
                <Button kind="primary" size="timeline" busy={busy}>
                  Post
                </Button>
              }
            >
              <ComposerText>
                <PlainTextEditor
                  namespace="story"
                  look="composer"
                  value={text}
                  onChange={setText}
                  label="Text"
                  placeholder="What's on your mind?"
                  roomy
                />
              </ComposerText>
            </ComposerFrame>
          </div>
        </>
      )}
    </div>
  );
}

/** The media viewer, bare: the picture and its description move together, the bar fades with the backing. */
function ViewerSwipe({ leave }: { leave: Leave }) {
  const swipe = useSwipeClose(leave, () => true);
  return (
    <div className="relative h-dvh overflow-hidden">
      <Page closed={swipe.closed} onReopen={swipe.reopen} />
      {!swipe.closed && (
        <div className="absolute inset-0 grid grid-rows-[auto_minmax(0,1fr)] text-text [color-scheme:dark]">
          <div className="absolute inset-0 bg-viewer-scrim" style={swipe.backing} />
          <div className="relative flex h-viewer-bar items-center justify-end px-viewer-inset" style={swipe.backing}>
            <IconButton icon="close" label="Close" />
          </div>
          <div
            className="relative grid touch-none content-center justify-items-center gap-viewer-inset"
            {...swipe.handlers}
          >
            <div className="grid justify-items-center gap-viewer-inset px-viewer-inset" style={swipe.surface}>
              <img
                src={wide}
                alt=""
                className="max-h-[60dvh] max-w-full object-contain select-none"
                draggable={false}
              />
              <p className="max-w-full px-viewer-inset text-body text-text">
                A sea under a pale blue sky, with low hills on the horizon.
              </p>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

/** The options of how a modal let go past the distance leaves (`Leave`), the same for the composer and the viewer. */
function SwipeLeave({ of }: { of: (leave: Leave) => ReactNode }) {
  return <Options id="swipe-close-leave" variants={{ slide: of("slide"), fall: of("fall") }} />;
}

const meta = {
  title: "Swipe to close",
  parameters: { layout: "fullscreen", controls: { disable: true } },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The composer on a phone: drag its head or its row down; the text scrolls and selects, never closes. */
export const Composer: Story = { render: () => <SwipeLeave of={(leave) => <ComposerSwipe leave={leave} />} /> };
/** A post in flight: nothing follows the finger, nothing closes. */
export const ComposerSending: Story = { render: () => <ComposerSwipe leave="slide" busy /> };
/** The media viewer: drag the picture down; across, it would move between the media. */
export const Viewer: Story = { render: () => <SwipeLeave of={(leave) => <ViewerSwipe leave={leave} />} /> };
