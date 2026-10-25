// SPDX-License-Identifier: AGPL-3.0-only
// The mirror of a text field (ADR-0092 §3 and §4, C1): an aria-hidden box behind the native textarea (or input) that
// lays out the same text with the control's own computed font, letter and word spacing, line height, padding, border
// widths and tab size, under `white-space: pre-wrap`, `overflow-wrap: break-word` and `dir="auto"` (`white-space: pre`
// on one line, centred in the box's height as an input centres its line), so every character lands where the control
// draws it. Its text is transparent: the control draws the text, and the mirror only gives positions (the caret line
// for the autocomplete) and paints the backgrounds of the highlighted ranges (the highlight layer, §4). Forced-colors
// mode drops backgrounds, so a highlighted range is underlined there. The mirror opts out of forced colors, so that its
// transparent text stays transparent and the control's text is never drawn twice, and it moves in front of the control
// there, because Chromium paints a backplate of the Canvas color behind each line of the control's text, which would
// hide the lines under it.
// It follows the control's scroll and is measured again when the control resizes and when web fonts finish loading
// (Noto Color Emoji swaps in after the first layout). A fractional line height lets the scrolled mirror drift by up to a
// pixel in Chromium and WebKit (#1152), so a control with a mirror has a whole-pixel line height (`layered` in
// TextArea.tsx and TextField.tsx).
import { type ReactNode, type Ref, type RefObject, useEffect, useEffectEvent, useLayoutEffect, useRef } from "react";
import { tv } from "tailwind-variants";

/** The computed properties of the control that decide where each character lands; the mirror copies them. */
const copiedProperties = [
  "font-family",
  "font-size",
  "font-weight",
  "font-style",
  "font-variant",
  "font-stretch",
  "font-feature-settings",
  "font-variation-settings",
  "font-kerning",
  "font-optical-sizing",
  "letter-spacing",
  "word-spacing",
  "line-height",
  "text-transform",
  "text-indent",
  "text-rendering",
  "tab-size",
  "padding-block-start",
  "padding-block-end",
  "padding-inline-start",
  "padding-inline-end",
  "border-block-start-width",
  "border-block-end-width",
  "border-inline-start-width",
  "border-inline-end-width",
  "box-sizing",
] as const;

// The scrollbar gutter is kept on both boxes of a text area, so a scrollbar that appears in the textarea does not
// narrow its lines without narrowing the mirror's. The borders are transparent: only their widths matter.
// One line (an input) is centred in the box's height, as the input centres its line. Its text sits in one inline box,
// the flex item, and runs past the box's end where the input scrolls it.
const mirror = tv({
  slots: {
    box: [
      "pointer-events-none absolute inset-0 overflow-hidden border-solid border-transparent text-transparent select-none",
      "forced-color-adjust-none forced-colors:z-1",
    ],
    line: "",
    // The room after the text: an input may scroll a caret's width past its text, so the mirror can scroll as far.
    end: "",
  },
  variants: {
    singleLine: {
      true: { box: "flex items-center whitespace-pre", line: "flex-none", end: "inline-block w-2" },
      false: { box: "scrollbar-gutter-stable wrap-break-word whitespace-pre-wrap" },
    },
  },
});

/** What a highlighted range is: a mention, hashtag, URL or `:shortcode:` (ADR-0092 §4), or text over the character limit. */
export type TextHighlightKind = "mention" | "hashtag" | "url" | "shortcode" | "over";

/** A highlighted range of the text, [start, end) in UTF-16 units. */
export interface TextHighlight {
  readonly kind: TextHighlightKind;
  readonly start: number;
  readonly end: number;
}

// Backgrounds only: a mark changes no metrics (no padding, inline border or font), so the text lays out as it does
// without one. Forced colors drop backgrounds, so marks are underlined there, dashed over the limit. The line is a
// block-end border, which an inline box draws below its text without moving any line.
const mark = tv({
  base: "rounded-sm forced-colors:rounded-none forced-colors:border-be-2 forced-colors:border-be-[CanvasText] forced-colors:bg-transparent",
  variants: {
    kind: {
      mention: "bg-highlight-mention",
      hashtag: "bg-highlight-hashtag",
      url: "bg-highlight-url",
      shortcode: "bg-highlight-shortcode",
      over: "bg-highlight-over forced-colors:border-dashed",
    },
  },
});

/** A run of the text with one look: its range and the kind of highlight on it, or null for plain text. */
type Run = { start: number; end: number; kind: TextHighlightKind | null };

/**
 * Splits text of `length` UTF-16 units into runs at every edge of a highlight and at the caret. A run takes the kind of
 * the last highlight that covers it, except that text over the limit stays `over`.
 */
export function highlightRuns(length: number, highlights: readonly TextHighlight[], caret: number | null): Run[] {
  const clamp = (offset: number) => Math.min(Math.max(offset, 0), length);
  const edges = new Set([0, length]);
  if (caret !== null) edges.add(clamp(caret));
  for (const { start, end } of highlights) {
    edges.add(clamp(start));
    edges.add(clamp(end));
  }
  const sorted = [...edges].toSorted((a, b) => a - b);
  const runs: Run[] = [];
  for (const [i, start] of sorted.entries()) {
    const end = sorted[i + 1];
    if (end === undefined) break;
    let kind: TextHighlightKind | null = null;
    for (const highlight of highlights) {
      if (highlight.start <= start && end <= highlight.end && kind !== "over") kind = highlight.kind;
    }
    runs.push({ start, end, kind });
  }
  return runs;
}

/** The text with its highlighted runs as marks, and the caret marker at the caret. */
function mirroredText(text: string, highlights: readonly TextHighlight[], caret: number | null, caretRef: Ref<HTMLSpanElement> | undefined) {
  const marker = <span key="caret" ref={caretRef} />;
  const nodes: ReactNode[] = [];
  for (const run of highlightRuns(text.length, highlights, caret)) {
    if (run.start === caret) nodes.push(marker);
    const part = text.slice(run.start, run.end);
    nodes.push(
      run.kind === null ? (
        part
      ) : (
        <span key={run.start} data-highlight={run.kind} className={mark({ kind: run.kind })}>
          {part}
        </span>
      ),
    );
  }
  if (caret === text.length) nodes.push(marker);
  return nodes;
}

/** No highlights: a mirror for positions only (the autocomplete). */
const noHighlights: readonly TextHighlight[] = [];

/** Gives a final line break of the text its line, as the textarea does. */
const zeroWidthSpace = "​";

/** Copies the control's layout properties onto the mirror and its scroll offset. */
function copyLayout(control: HTMLTextAreaElement | HTMLInputElement, box: HTMLElement) {
  const computed = getComputedStyle(control);
  for (const property of copiedProperties) box.style.setProperty(property, computed.getPropertyValue(property));
  box.scrollTop = control.scrollTop;
  box.scrollLeft = control.scrollLeft;
}

export type TextAreaMirrorProps = {
  /** The control it mirrors; the mirror sits in the same positioned box (`inset-0`), right before it. */
  textareaRef: RefObject<HTMLTextAreaElement | HTMLInputElement | null>;
  /** The control's value. */
  text: string;
  /** Where to put the caret marker, in UTF-16 units, or null for none. */
  caret: number | null;
  /** The zero-width marker at the caret; its box is on the caret's line. */
  caretRef?: Ref<HTMLSpanElement>;
  /** The ranges to paint (the highlight layer, ADR-0092 §4). */
  highlights?: readonly TextHighlight[] | undefined;
  /** The control is an input: one line, which it scrolls sideways. */
  singleLine?: boolean;
  /** Called after the mirror has copied the layout or the scroll again (resize, scroll, fonts). */
  onSync?: () => void;
};

/** The aria-hidden mirror behind a text area or a text input. */
export function TextAreaMirror({ textareaRef, text, caret, caretRef, highlights = noHighlights, singleLine = false, onSync }: TextAreaMirrorProps) {
  const boxRef = useRef<HTMLDivElement>(null);
  const synced = useEffectEvent(() => {
    onSync?.();
  });

  // A render can change the control's classes (invalid, disabled), so the layout is copied after each one.
  useLayoutEffect(() => {
    const control = textareaRef.current;
    const box = boxRef.current;
    if (control !== null && box !== null) copyLayout(control, box);
  });

  useEffect(() => {
    const control = textareaRef.current;
    const box = boxRef.current;
    if (control === null || box === null) return;
    const sync = () => {
      copyLayout(control, box);
      synced();
    };
    const onScroll = () => {
      box.scrollTop = control.scrollTop;
      box.scrollLeft = control.scrollLeft;
      synced();
    };
    control.addEventListener("scroll", onScroll, { passive: true });
    // An input scrolls its line to keep the caret in view as the caret moves, which not every engine tells with a
    // scroll event; the caret's moves come as selectionchange, after which the scroll is read in the next frame.
    let scrollFrame = 0;
    const onSelectionChange = () => {
      cancelAnimationFrame(scrollFrame);
      scrollFrame = requestAnimationFrame(onScroll);
    };
    const doc = control.ownerDocument;
    if (control instanceof HTMLInputElement) doc.addEventListener("selectionchange", onSelectionChange);
    // What follows a resize (the autocomplete moves its anchor, which React Aria observes) waits for the next frame, so
    // it does not resize observed elements inside this observer's own delivery.
    // jsdom (the component tests) has neither a ResizeObserver nor document.fonts.
    let frame = 0;
    const observer =
      typeof ResizeObserver === "undefined"
        ? null
        : new ResizeObserver(() => {
            copyLayout(control, box);
            cancelAnimationFrame(frame);
            frame = requestAnimationFrame(synced);
          });
    observer?.observe(control);
    const fonts = "fonts" in doc ? doc.fonts : null;
    fonts?.addEventListener("loadingdone", sync);
    // Fonts already loading when this mounts may finish without a loadingdone for this mirror, and a style that settles
    // after the first copy (a metric tied to the font) changes no size the observer sees: copy once more when they are ready.
    let disposed = false;
    void fonts?.ready.then(() => {
      if (!disposed) sync();
    });
    return () => {
      disposed = true;
      control.removeEventListener("scroll", onScroll);
      doc.removeEventListener("selectionchange", onSelectionChange);
      observer?.disconnect();
      cancelAnimationFrame(frame);
      cancelAnimationFrame(scrollFrame);
      fonts?.removeEventListener("loadingdone", sync);
    };
  }, [textareaRef]);

  const styles = mirror({ singleLine });
  const at = caret === null ? null : Math.min(Math.max(caret, 0), text.length);
  const content = mirroredText(text, highlights, at, caretRef);
  return (
    <div ref={boxRef} aria-hidden dir="auto" className={styles.box()}>
      {singleLine ? (
        <span className={styles.line()}>
          {content}
          <span className={styles.end()} />
        </span>
      ) : (
        <>
          {content}
          {/* A final line break takes up a line in the textarea only when something follows it; a zero-width space does. */}
          {zeroWidthSpace}
        </>
      )}
    </div>
  );
}
