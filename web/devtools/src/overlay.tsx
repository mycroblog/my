// SPDX-License-Identifier: AGPL-3.0-only

// The part of the notes (ADR-0168 §1) that must be on the story's page, in Storybook's preview: picking the element
// or the area a note is on, with its highlight, and the markers of the notes the manager shows. It draws nothing while
// the decider picks nothing and no marker is shown, so that it never covers the story (a phone's bottom navigation).
// The rest, the notes' list, their writing, sending and answers, and the live options, is the manager's (manager.tsx),
// beside the canvas; the two talk over Storybook's channel (channel.ts). It also tells the manager the <Options> on
// the page (options.tsx), switches them, and makes the note of a pick; and passes on the dev server's word that a batch
// changed. It is a developer tool: plain, in a shadow root of its own above the story, and it sets nothing on the page.

import { useEffect, useState, type PointerEvent } from "react";
import { createRoot } from "react-dom/client";
import { addons } from "storybook/preview-api";
import { noteOf } from "./capture";
import { events, type Marker, type OptionSet } from "./channel";
import { optionsAttribute, optionsEvent, pickNote, storyEvent, switchEvent, updateEvent, type Rect } from "./note";
import { pageOf } from "./story";

// The overlay's own stacking context, above any the page makes (a sticky header's z-index). It reads left-to-right in
// English on a page of any locale, so that a box placed from screen coordinates at its inline start is placed from the
// left.
const host = document.createElement("div");
host.setAttribute("data-my-devtools", "");
host.lang = "en";
host.dir = "ltr";
host.style.cssText = "position: relative; z-index: 2147483647";

// A document and those of its same-origin iframes, whose elements take notes too.
function tree(doc: Document): Document[] {
  const inner = Array.from(doc.querySelectorAll("iframe")).flatMap((f) => {
    try {
      return f.contentDocument ? tree(f.contentDocument) : [];
    } catch {
      return [];
    }
  });
  return [doc, ...inner];
}

const documents = () => tree(document);

// How much an iframe is scaled on screen.
const scaleOf = (frame: Element) => {
  const width = (frame as HTMLElement).offsetWidth;
  return width > 0 ? frame.getBoundingClientRect().width / width : 1;
};

interface Offset {
  x: number;
  y: number;
  scale: number;
}

// Where a document's viewport starts in the overlay's viewport, and its scale: the content box of each iframe it is
// in, up to the overlay's document.
function offsetOf(doc: Document): Offset {
  const frame = doc === document ? null : doc.defaultView?.frameElement;
  if (!frame || frame.ownerDocument === doc) return { x: 0, y: 0, scale: 1 };
  const outer = offsetOf(frame.ownerDocument);
  const r = frame.getBoundingClientRect();
  const s = scaleOf(frame);
  return {
    x: outer.x + (r.left + frame.clientLeft * s) * outer.scale,
    y: outer.y + (r.top + frame.clientTop * s) * outer.scale,
    scale: outer.scale * s,
  };
}

// A box in the client coordinates of a document, in those of the overlay's viewport.
function toTop(doc: Document, r: Rect): DOMRect {
  const o = offsetOf(doc);
  return new DOMRect(o.x + r.x * o.scale, o.y + r.y * o.scale, r.width * o.scale, r.height * o.scale);
}

// A box in the client coordinates of the overlay's viewport, in those of a document.
function fromTop(doc: Document, r: Rect): Rect {
  const o = offsetOf(doc);
  return { x: (r.x - o.x) / o.scale, y: (r.y - o.y) / o.scale, width: r.width / o.scale, height: r.height / o.scale };
}

// An element's box in the client coordinates of the overlay's viewport.
const rectOf = (el: Element) => toTop(el.ownerDocument, el.getBoundingClientRect());

// The element under a point of a document's viewport, passing over the overlay and into same-origin iframes.
function elementIn(doc: Document, x: number, y: number): Element | null {
  const el = doc.elementsFromPoint(x, y).find((e) => e !== host && e !== doc.documentElement) ?? null;
  if (el?.tagName !== "IFRAME") return el;
  const frame = el as HTMLIFrameElement;
  let inner: Document | null = null;
  try {
    inner = frame.contentDocument;
  } catch {
    // A cross-origin iframe: the note is on the iframe.
  }
  if (!inner) return el;
  const p = fromTop(inner, toTop(doc, { x, y, width: 0, height: 0 }));
  return elementIn(inner, p.x, p.y) ?? el;
}

const elementAt = (x: number, y: number) => elementIn(document, x, y);

const pageOfDoc = (doc: Document) => (doc.defaultView ? pageOf(doc.defaultView) : "");

// The element of a marker's note, among the documents of the page: in the one whose page and viewport width are the
// note's state, else in one of its page.
function elementOf(marker: Marker): Element | null {
  const { state, target } = marker.note;
  const docs = documents();
  const doc =
    docs.find((d) => d.defaultView?.innerWidth === state.viewport.width && pageOfDoc(d) === state.page) ??
    docs.find((d) => pageOfDoc(d) === state.page);
  try {
    return doc?.querySelector(target.selector) ?? null;
  } catch {
    return null;
  }
}

// The elements of the <Options> on the page (options.tsx). The <Options> of one id switch together, as in a list
// whose every item is wrapped, so the first stands for them.
function optionElements(): Map<string, Element> {
  const found = new Map<string, Element>();
  for (const el of documents().flatMap((doc) => Array.from(doc.querySelectorAll(`[${optionsAttribute}]`)))) {
    const id = el.getAttribute(optionsAttribute) ?? "";
    if (!found.has(id)) found.set(id, el);
  }
  return found;
}

function optionSets(): OptionSet[] {
  return Array.from(optionElements(), ([id, el]) => {
    let variants: string[] = [];
    try {
      variants = JSON.parse(el.getAttribute("data-variants") ?? "[]") as string[];
    } catch {
      // Not an element of <Options>: it lists no variant.
    }
    return { id, variant: el.getAttribute("data-variant") ?? "", variants };
  });
}

function MarkerView({ marker }: { marker: Marker }) {
  const el = elementOf(marker);
  if (!el) return null;
  const r = rectOf(el);
  return (
    <div
      style={{
        position: "fixed",
        insetInlineStart: Math.max(0, r.right - 10),
        top: Math.max(0, r.top - 10),
        minWidth: 20,
        height: 20,
        padding: "0 4px",
        boxSizing: "border-box",
        borderRadius: 10,
        background: marker.color,
        color: "#fff",
        font: "bold 11px/20px system-ui, sans-serif",
        textAlign: "center",
        pointerEvents: "none",
      }}
    >
      {marker.label}
    </div>
  );
}

function Overlay() {
  const [picking, setPicking] = useState(false);
  const [markers, setMarkers] = useState<Marker[]>([]);
  const [hover, setHover] = useState<DOMRect | null>(null);
  const [drag, setDrag] = useState<{ x0: number; y0: number; x1: number; y1: number } | null>(null);
  const [, setTick] = useState(0);

  useEffect(() => {
    const channel = addons.getChannel();
    const rerender = () => {
      setTick((t) => t + 1);
    };
    const tellOptions = () => {
      channel.emit(events.options, optionSets());
      rerender();
    };
    // Another story, or new args or globals: the manager sends its notes' markers for it again.
    const onStory = () => {
      tellOptions();
      channel.emit(events.ready);
    };
    const onPick = (on: boolean) => {
      setPicking(on);
      setHover(null);
    };
    const onSwitch = ({ id, variant }: { id: string; variant: string }) => {
      for (const doc of documents())
        doc.defaultView?.dispatchEvent(new CustomEvent(switchEvent, { detail: { id, variant } }));
    };
    // A pick is a note on the variant shown, with the decider's reason when they gave one.
    const onPickVariant = ({ id, reason }: { id: string; reason: string }) => {
      const set = optionSets().find((s) => s.id === id);
      const el = optionElements().get(id);
      if (!set || !el) return;
      const { text, pick } = pickNote({ options: set.id, variant: set.variant, variants: set.variants }, reason);
      channel.emit(events.variantNote, { ...noteOf(text, el.firstElementChild ?? el, null), pick });
    };
    const onUpdate = () => {
      channel.emit(events.update);
    };
    channel.on(events.pick, onPick);
    channel.on(events.markers, setMarkers);
    channel.on(events.switch, onSwitch);
    channel.on(events.pickVariant, onPickVariant);
    // Only the dev server loads the overlay, so import.meta.hot is always there.
    import.meta.hot.on(updateEvent, onUpdate);
    window.addEventListener("scroll", rerender, true);
    window.addEventListener("resize", rerender);
    window.addEventListener(optionsEvent, tellOptions);
    window.addEventListener(storyEvent, onStory);
    channel.emit(events.ready);
    return () => {
      channel.off(events.pick, onPick);
      channel.off(events.markers, setMarkers);
      channel.off(events.switch, onSwitch);
      channel.off(events.pickVariant, onPickVariant);
      import.meta.hot.off(updateEvent, onUpdate);
      window.removeEventListener("scroll", rerender, true);
      window.removeEventListener("resize", rerender);
      window.removeEventListener(optionsEvent, tellOptions);
      window.removeEventListener(storyEvent, onStory);
    };
  }, []);

  useEffect(() => {
    if (!picking) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") addons.getChannel().emit(events.picked, null);
    };
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
    };
  }, [picking]);

  const onDown = (e: PointerEvent) => {
    setDrag({ x0: e.clientX, y0: e.clientY, x1: e.clientX, y1: e.clientY });
  };
  const onMove = (e: PointerEvent) => {
    if (drag) setDrag({ ...drag, x1: e.clientX, y1: e.clientY });
    else {
      const el = elementAt(e.clientX, e.clientY);
      setHover(el ? rectOf(el) : null);
    }
  };
  const onUp = (e: PointerEvent) => {
    if (!drag) return;
    const area =
      Math.abs(drag.x1 - drag.x0) > 4 || Math.abs(drag.y1 - drag.y0) > 4
        ? {
            x: Math.min(drag.x0, drag.x1),
            y: Math.min(drag.y0, drag.y1),
            width: Math.abs(drag.x1 - drag.x0),
            height: Math.abs(drag.y1 - drag.y0),
          }
        : null;
    setDrag(null);
    const at = area ? elementAt(area.x + area.width / 2, area.y + area.height / 2) : elementAt(e.clientX, e.clientY);
    if (!at) return;
    setPicking(false);
    setHover(null);
    // noteOf takes the area in the client coordinates of the element's document; the manager adds the text.
    addons.getChannel().emit(events.picked, noteOf("", at, area && fromTop(at.ownerDocument, area)));
  };

  if (!picking && markers.length === 0) return null;
  return (
    <>
      {picking && (
        <div
          style={{ position: "fixed", inset: 0, cursor: "crosshair", pointerEvents: "auto" }}
          onPointerDown={onDown}
          onPointerMove={onMove}
          onPointerUp={onUp}
        />
      )}
      {picking && hover && !drag && (
        <div
          style={{
            position: "fixed",
            insetInlineStart: hover.left,
            top: hover.top,
            width: hover.width,
            height: hover.height,
            outline: "2px solid #0b57d0",
            background: "rgba(11,87,208,.08)",
            pointerEvents: "none",
          }}
        />
      )}
      {drag && (
        <div
          style={{
            position: "fixed",
            insetInlineStart: Math.min(drag.x0, drag.x1),
            top: Math.min(drag.y0, drag.y1),
            width: Math.abs(drag.x1 - drag.x0),
            height: Math.abs(drag.y1 - drag.y0),
            outline: "2px dashed #0b57d0",
            pointerEvents: "none",
          }}
        />
      )}
      {markers.map((m) => (
        <MarkerView key={m.key} marker={m} />
      ))}
    </>
  );
}

// One overlay per window, in Storybook's preview. After an edit of the overlay, React Refresh imports this module
// again under another URL, and the new copy replaces the one before. While idle it draws nothing; web:shot still
// unmounts it (`myDevtoolsUnmount`), so that no marker of a note covers a shot.
const global = window as { myDevtoolsUnmount?: () => void };
global.myDevtoolsUnmount?.();
document.documentElement.appendChild(host);
const root = createRoot(host.attachShadow({ mode: "open" }));
root.render(<Overlay />);
global.myDevtoolsUnmount = () => {
  root.unmount();
  host.remove();
};
