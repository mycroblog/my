// SPDX-License-Identifier: AGPL-3.0-only

// The overlay of the dev server (ADR-0168 §1): the decider writes notes on elements or areas, sends them as one
// batch, reads the agent's answer next to each note, and switches between the variants of an <Options> and picks
// one (options.tsx). It also shows the page at a device viewport (sizes.ts), as Storybook's viewport toolbar does: the
// page itself, whole, in one iframe of that width and height over it, rotated or not and scaled down to fit the
// window, whose elements take the notes and whose <Options> switch. The plugin of
// vite.ts loads this module on the dev server only. It is a developer tool: plain, in a shadow root of its own, and
// it sets nothing on the page.

import { useEffect, useState, type CSSProperties, type PointerEvent } from "react";
import { createRoot } from "react-dom/client";
import { noteOf } from "./capture";
import {
  endpoint,
  optionsAttribute,
  optionsEvent,
  switchEvent,
  pickNote,
  updateEvent,
  type Batch,
  type Note,
  type NoteInput,
  type Rect,
} from "./note";
import { sizes, viewportOf, type Size } from "./sizes";

const pendingKey = "my-devtools:pending";
const clearedKey = "my-devtools:cleared";
const sizeKey = "my-devtools:size";

/** The device viewport the decider chose, rotated from its natural orientation or not. */
interface Chosen {
  size: Size;
  rotated: boolean;
}

// The margin around the device iframe, inside which it shrinks to fit the window.
const margin = 16;

function load<T>(storage: () => Storage, key: string, fallback: T): T {
  try {
    const v = storage().getItem(key);
    return v === null ? fallback : (JSON.parse(v) as T);
  } catch {
    return fallback;
  }
}

function save(storage: () => Storage, key: string, value: unknown): void {
  try {
    storage().setItem(key, JSON.stringify(value));
  } catch {
    // Storage is off: the notes last until the page reloads.
  }
}

const font = "13px/1.4 system-ui, sans-serif";
const box: CSSProperties = {
  font,
  color: "#111",
  background: "#fff",
  border: "1px solid #888",
  borderRadius: 4,
  boxShadow: "0 2px 8px rgba(0,0,0,.25)",
  pointerEvents: "auto",
};
const button: CSSProperties = { font, padding: "2px 8px", cursor: "pointer" };

interface Draft {
  element: Element;
  area: Rect | null;
  text: string;
}

const host = document.createElement("div");
host.setAttribute("data-my-devtools", "");

// The iframe that shows the page at a device size, while a size is chosen. It is in the overlay's shadow root.
let device: HTMLIFrameElement | null = null;

const deviceDocument = () => {
  try {
    return device?.contentDocument ?? null;
  } catch {
    return null;
  }
};

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

// The documents the decider sees: the page at a device size while one is chosen, the page itself otherwise.
const documents = () => tree(deviceDocument() ?? document);

// How much an iframe is scaled on screen: the device iframe shrinks to fit the window.
const scaleOf = (frame: Element) => {
  const width = (frame as HTMLElement).offsetWidth;
  return width > 0 ? frame.getBoundingClientRect().width / width : 1;
};

interface Offset {
  x: number;
  y: number;
  scale: number;
}

// Where a document's viewport starts in the top viewport, and its scale: the content box of each iframe it is in.
function offsetOf(doc: Document): Offset {
  const frame = doc.defaultView?.frameElement;
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

// A box in the client coordinates of a document, in those of the top viewport.
function toTop(doc: Document, r: Rect): DOMRect {
  const o = offsetOf(doc);
  return new DOMRect(o.x + r.x * o.scale, o.y + r.y * o.scale, r.width * o.scale, r.height * o.scale);
}

// A box in the client coordinates of the top viewport, in those of a document.
function fromTop(doc: Document, r: Rect): Rect {
  const o = offsetOf(doc);
  return { x: (r.x - o.x) / o.scale, y: (r.y - o.y) / o.scale, width: r.width / o.scale, height: r.height / o.scale };
}

// An element's box in the client coordinates of the top viewport.
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

// The element under a point of the top viewport: in the page at a device size while one is chosen.
function elementAt(x: number, y: number): Element | null {
  const inner = deviceDocument();
  if (!device || !inner) return elementIn(document, x, y);
  const r = device.getBoundingClientRect();
  if (x < r.left || x >= r.right || y < r.top || y >= r.bottom) return null;
  const p = fromTop(inner, { x, y, width: 0, height: 0 });
  return elementIn(inner, p.x, p.y);
}

const pageOf = (doc: Document) => {
  const { location } = doc;
  return location.pathname + location.search + location.hash;
};

// The element of a note again, among the documents the decider sees: in the one whose page and viewport width are
// the note's state, else in one of its page.
function elementOf(note: NoteInput): Element | null {
  const docs = documents();
  const doc =
    docs.find((d) => d.defaultView?.innerWidth === note.state.viewport.width && pageOf(d) === note.state.page) ??
    docs.find((d) => pageOf(d) === note.state.page);
  try {
    return doc?.querySelector(note.target.selector) ?? null;
  } catch {
    return null;
  }
}

const page = () => pageOf(document);

interface OptionSet {
  id: string;
  variant: string;
  variants: string[];
  /** The element the variant renders, which a pick is a note on. */
  element: Element;
}

// The <Options> the decider sees (options.tsx), read from their elements. The <Options> of one id switch together,
// as in a list whose every item is wrapped, so the first stands for them.
function optionSets(): OptionSet[] {
  const seen = new Set<string>();
  const all = documents().flatMap((doc) => Array.from(doc.querySelectorAll(`[${optionsAttribute}]`)));
  const first = all.filter((el) => {
    const id = el.getAttribute(optionsAttribute) ?? "";
    if (seen.has(id)) return false;
    seen.add(id);
    return true;
  });
  return first.map((el) => {
    let variants: string[] = [];
    try {
      variants = JSON.parse(el.getAttribute("data-variants") ?? "[]") as string[];
    } catch {
      // Not an element of <Options>: it lists no variant.
    }
    return {
      id: el.getAttribute(optionsAttribute) ?? "",
      variant: el.getAttribute("data-variant") ?? "",
      variants,
      element: el.firstElementChild ?? el,
    };
  });
}

// Each window runs its own <Options>, so the switch goes to every one of them: the page's, and the page's at a
// device size, which then agree when the decider goes back.
const switchTo = (id: string, variant: string) => {
  const inner = deviceDocument();
  for (const doc of [...tree(document), ...(inner ? tree(inner) : [])]) {
    doc.defaultView?.dispatchEvent(new CustomEvent(switchEvent, { detail: { id, variant } }));
  }
};

function Marker({ label, note, color }: { label: string; note: NoteInput; color: string }) {
  const el = elementOf(note);
  if (!el) return null;
  const r = rectOf(el);
  return (
    <div
      style={{
        position: "fixed",
        left: Math.max(0, r.right - 10),
        top: Math.max(0, r.top - 10),
        minWidth: 20,
        height: 20,
        padding: "0 4px",
        boxSizing: "border-box",
        borderRadius: 10,
        background: color,
        color: "#fff",
        font: "bold 11px/20px system-ui, sans-serif",
        textAlign: "center",
        pointerEvents: "none",
      }}
    >
      {label}
    </div>
  );
}

function ReplyLine({ note }: { note: Note }) {
  if (!note.reply) return <div style={{ color: "#666" }}>waiting for an answer</div>;
  return (
    <div style={{ marginTop: 4, padding: 4, background: "#eef3fd", borderRadius: 3 }} data-reply={note.id}>
      <strong>{note.reply.kind}</strong>: {note.reply.text}
      {note.reply.options !== undefined && <div>Switch them under Options: {note.reply.options}</div>}
    </div>
  );
}

function Overlay() {
  const [picking, setPicking] = useState(false);
  const [hover, setHover] = useState<DOMRect | null>(null);
  const [drag, setDrag] = useState<{ x0: number; y0: number; x1: number; y1: number } | null>(null);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [pending, setPending] = useState<NoteInput[]>(() => load(() => sessionStorage, pendingKey, []));
  const [batches, setBatches] = useState<Batch[]>([]);
  const [cleared, setCleared] = useState<string>(() => load(() => localStorage, clearedKey, ""));
  const [open, setOpen] = useState(true);
  const [error, setError] = useState("");
  const [reasons, setReasons] = useState<Record<string, string>>({});
  const [chosen, setChosen] = useState<Chosen | null>(() => {
    const v = load<Partial<Chosen> | null>(() => localStorage, sizeKey, null);
    return v?.size !== undefined && v.size in sizes ? { size: v.size, rotated: v.rotated === true } : null;
  });
  const [, setTick] = useState(0);
  const rerender = () => {
    setTick((t) => t + 1);
  };

  const choose = (c: Chosen | null) => {
    setChosen(c);
    save(() => localStorage, sizeKey, c);
  };

  const refresh = () => {
    fetch(`${endpoint}/batches`)
      .then(async (res) => (await res.json()) as Batch[])
      .then(setBatches)
      .catch((err: unknown) => {
        setError(String(err));
      });
  };

  useEffect(() => {
    refresh();
    // Only the dev server loads the overlay, so import.meta.hot is always there.
    import.meta.hot.on(updateEvent, refresh);
    window.addEventListener("scroll", rerender, true);
    window.addEventListener("resize", rerender);
    window.addEventListener(optionsEvent, rerender);
    return () => {
      import.meta.hot.off(updateEvent, refresh);
      window.removeEventListener("scroll", rerender, true);
      window.removeEventListener("resize", rerender);
      window.removeEventListener(optionsEvent, rerender);
    };
  }, []);

  useEffect(() => {
    if (!picking) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setPicking(false);
    };
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
    };
  }, [picking]);

  const keep = (notes: NoteInput[]) => {
    setPending(notes);
    save(() => sessionStorage, pendingKey, notes);
  };

  const add = () => {
    if (!draft || draft.text.trim() === "") return;
    // noteOf takes the area in the client coordinates of the element's document.
    const area = draft.area && fromTop(draft.element.ownerDocument, draft.area);
    keep([...pending, noteOf(draft.text.trim(), draft.element, area)]);
    setDraft(null);
  };

  // A pick is a note on the variant, sent with the other notes, with the decider's reason when they wrote one.
  const pick = (set: OptionSet) => {
    const { text, pick } = pickNote(
      { options: set.id, variant: set.variant, variants: set.variants },
      reasons[set.id] ?? "",
    );
    keep([...pending.filter((n) => n.pick?.options !== set.id), { ...noteOf(text, set.element, null), pick }]);
    setReasons({ ...reasons, [set.id]: "" });
  };
  const sets = optionSets();

  const send = () => {
    setError("");
    fetch(`${endpoint}/batches`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ page: page(), notes: pending }),
    })
      .then((res) => {
        if (!res.ok) throw new Error(`the dev server answered ${res.status}`);
        keep([]);
        refresh();
      })
      .catch((err: unknown) => {
        setError(String(err));
      });
  };

  const sent = batches.filter((b) => b.page === page() && b.sentAt > cleared).flatMap((b) => b.notes);

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
    setDraft({ element: at, area, text: "" });
  };

  const draftRect = draft ? (draft.area ?? rectOf(draft.element)) : null;

  // The device viewport, scaled down to fit the window whole.
  const viewport = chosen && viewportOf(chosen.size, chosen.rotated);
  const scale = viewport
    ? Math.min(
        1,
        (window.innerWidth - 2 * margin) / viewport.width,
        (window.innerHeight - 2 * margin) / viewport.height,
      )
    : 1;

  return (
    <>
      {chosen && viewport && (
        <div
          style={{
            position: "fixed",
            inset: 0,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            background: "#ddd",
            pointerEvents: "auto",
          }}
        >
          <div style={{ width: viewport.width * scale, height: viewport.height * scale, outline: "1px solid #888" }}>
            <iframe
              ref={(el) => {
                device = el;
              }}
              title={`The page at ${chosen.size} ${viewport.width}×${viewport.height}`}
              src={location.href}
              style={{
                display: "block",
                width: viewport.width,
                height: viewport.height,
                border: 0,
                background: "#fff",
                transform: `scale(${scale})`,
                transformOrigin: "0 0",
              }}
              onLoad={(e) => {
                e.currentTarget.contentWindow?.addEventListener("scroll", rerender, true);
                rerender();
              }}
            />
          </div>
        </div>
      )}
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
            left: hover.left,
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
            left: Math.min(drag.x0, drag.x1),
            top: Math.min(drag.y0, drag.y1),
            width: Math.abs(drag.x1 - drag.x0),
            height: Math.abs(drag.y1 - drag.y0),
            outline: "2px dashed #0b57d0",
            pointerEvents: "none",
          }}
        />
      )}
      {pending.map((n, i) => (
        <Marker key={`p${i}`} label={`${i + 1}`} note={n} color="#666" />
      ))}
      {sent.map((n, i) => (
        <Marker key={n.id} label={`${pending.length + i + 1}`} note={n} color={n.reply ? "#137333" : "#0b57d0"} />
      ))}
      {draft && draftRect && (
        <div
          style={{
            ...box,
            position: "fixed",
            left: Math.min(Math.max(8, draftRect.x), window.innerWidth - 300),
            top: Math.min(draftRect.y + draftRect.height + 8, window.innerHeight - 140),
            width: 280,
            padding: 8,
          }}
        >
          <div style={{ marginBottom: 4, color: "#444" }}>
            {draft.area ? "area" : "element"}:{" "}
            {draft.element.closest("[data-source]")?.getAttribute("data-source") ?? draft.element.tagName.toLowerCase()}
          </div>
          <textarea
            aria-label="Note"
            autoFocus
            rows={3}
            style={{ font, width: "100%", boxSizing: "border-box" }}
            value={draft.text}
            onChange={(e) => {
              setDraft({ ...draft, text: e.target.value });
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) add();
              if (e.key === "Escape") setDraft(null);
            }}
          />
          <div style={{ display: "flex", gap: 4, justifyContent: "flex-end", marginTop: 4 }}>
            <button
              type="button"
              style={button}
              onClick={() => {
                setDraft(null);
              }}
            >
              Cancel
            </button>
            <button type="button" style={button} onClick={add}>
              Add note
            </button>
          </div>
        </div>
      )}
      <div
        role="region"
        aria-label="Feedback"
        style={{
          ...box,
          position: "fixed",
          right: 12,
          bottom: 12,
          width: open ? 320 : "auto",
          maxHeight: "60vh",
          overflow: "auto",
          padding: 8,
        }}
      >
        <div style={{ display: "flex", gap: 4, flexWrap: "wrap" }}>
          <button
            type="button"
            style={button}
            aria-pressed={picking}
            onClick={() => {
              setPicking(!picking);
              setOpen(true);
            }}
          >
            {picking ? "Picking… (Esc)" : "Note"}
          </button>
          <button type="button" style={button} disabled={pending.length === 0} onClick={send}>
            Send {pending.length}
          </button>
          {open && (
            <button
              type="button"
              style={button}
              disabled={sent.length === 0}
              onClick={() => {
                const now = new Date().toISOString();
                setCleared(now);
                save(() => localStorage, clearedKey, now);
              }}
            >
              Clear sent
            </button>
          )}
          <button
            type="button"
            style={button}
            onClick={() => {
              setOpen(!open);
            }}
          >
            {open ? "Hide" : "Show"}
          </button>
        </div>
        <div role="group" aria-label="Viewport" style={{ display: "flex", gap: 4, flexWrap: "wrap", marginTop: 4 }}>
          {[null, ...(Object.keys(sizes) as Size[])].map((s) => {
            const on = s === (chosen?.size ?? null);
            const v = s && viewportOf(s, chosen?.rotated ?? false);
            return (
              <button
                key={s ?? "page"}
                type="button"
                style={{ ...button, fontWeight: on ? 700 : 400 }}
                aria-pressed={on}
                onClick={() => {
                  choose(s && { size: s, rotated: chosen?.rotated ?? false });
                }}
              >
                {s && v ? `${s} ${v.width}×${v.height}` : "Page"}
              </button>
            );
          })}
          <button
            type="button"
            style={button}
            disabled={!chosen}
            aria-pressed={chosen?.rotated ?? false}
            onClick={() => {
              if (chosen) choose({ ...chosen, rotated: !chosen.rotated });
            }}
          >
            Rotate
          </button>
        </div>
        {error && <div style={{ color: "#b3261e", marginTop: 4 }}>{error}</div>}
        {open &&
          sets.map((set) => (
            <div
              key={set.id}
              role="group"
              aria-label={`Options ${set.id}`}
              style={{ marginTop: 8, paddingTop: 6, borderTop: "1px solid #ddd" }}
            >
              <div style={{ color: "#666" }}>Options {set.id}</div>
              <div style={{ display: "flex", gap: 4, flexWrap: "wrap", marginTop: 4 }}>
                {set.variants.map((v) => (
                  <button
                    key={v}
                    type="button"
                    style={{ ...button, fontWeight: v === set.variant ? 700 : 400 }}
                    aria-pressed={v === set.variant}
                    onClick={() => {
                      switchTo(set.id, v);
                    }}
                  >
                    {v}
                  </button>
                ))}
                <button
                  type="button"
                  style={button}
                  onClick={() => {
                    pick(set);
                  }}
                >
                  Pick {set.variant}
                </button>
              </div>
              <input
                aria-label={`Reason for the pick of ${set.id}`}
                placeholder="Reason (optional)"
                style={{ font, width: "100%", boxSizing: "border-box", marginTop: 4 }}
                value={reasons[set.id] ?? ""}
                onChange={(e) => {
                  setReasons({ ...reasons, [set.id]: e.target.value });
                }}
                onKeyDown={(e) => {
                  if (e.key === "Enter") pick(set);
                }}
              />
            </div>
          ))}
        {open && (
          <ol style={{ margin: "8px 0 0", padding: "0 0 0 20px" }}>
            {pending.map((n, i) => (
              <li key={`p${i}`} style={{ marginBottom: 6 }}>
                <div style={{ color: "#666" }}>{n.source ?? n.target.tag} · not sent</div>
                <div>{n.text}</div>
                <button
                  type="button"
                  style={button}
                  onClick={() => {
                    keep(pending.filter((_, j) => j !== i));
                  }}
                >
                  Remove
                </button>
              </li>
            ))}
            {sent.map((n) => (
              <li key={n.id} style={{ marginBottom: 6 }} data-note={n.id}>
                <div style={{ color: "#666" }}>
                  {n.id} · {n.source ?? n.target.tag}
                </div>
                <div>{n.text}</div>
                <ReplyLine note={n} />
              </li>
            ))}
          </ol>
        )}
      </div>
    </>
  );
}

// One overlay per page, in the top window: the one of a page in a frame is its parent's. After an edit of the
// overlay, React Refresh imports this module again under another URL, and the new copy replaces the one before.
if (window.top === window) {
  const global = window as { myDevtoolsUnmount?: () => void };
  global.myDevtoolsUnmount?.();
  document.documentElement.appendChild(host);
  const root = createRoot(host.attachShadow({ mode: "open" }));
  root.render(<Overlay />);
  global.myDevtoolsUnmount = () => {
    root.unmount();
    host.remove();
  };
}
