// SPDX-License-Identifier: AGPL-3.0-only

// The notes of the decider (ADR-0168 §1) in Storybook's manager, beside the canvas and outside the story's viewport,
// so that they never cover the story (a phone's bottom navigation): a toolbar tool to pick what a note is on and to
// send the notes, and a Notes panel to write each note, read the agent's answer next to it, and switch between the
// variants of an <Options> and pick one. The preview's overlay (overlay.tsx) picks on the page and shows the notes'
// markers there; the two talk over Storybook's channel (channel.ts). The manager alone reads and writes the batches of
// the dev server; the preview passes on the dev server's word that one changed. Written with createElement, not JSX:
// Storybook builds the manager with the classic JSX transform.

import { createElement as h, useState, useSyncExternalStore, type CSSProperties } from "react";
import { addons, types, useStorybookState } from "storybook/manager-api";
import { events, type Marker, type OptionSet } from "./channel";
import { endpoint, type Batch, type Note, type NoteInput } from "./note";

const addonId = "my-devtools";
const toolId = `${addonId}/tool`;
const panelId = `${addonId}/panel`;
const pendingKey = "my-devtools:pending";
const clearedKey = "my-devtools:cleared";

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

interface State {
  picking: boolean;
  /** What was picked, waiting for its text. */
  draft: NoteInput | null;
  pending: NoteInput[];
  batches: Batch[];
  cleared: string;
  options: OptionSet[];
  markers: boolean;
  error: string;
}

let state: State = {
  picking: false,
  draft: null,
  pending: load(() => sessionStorage, pendingKey, []),
  batches: [],
  cleared: load(() => localStorage, clearedKey, ""),
  options: [],
  markers: true,
  error: "",
};
const listeners = new Set<() => void>();
const set = (next: Partial<State>) => {
  state = { ...state, ...next };
  if (next.pending) save(() => sessionStorage, pendingKey, next.pending);
  for (const l of listeners) l();
  showMarkers();
};
const useStore = () =>
  useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => state,
  );

const channel = () => addons.getChannel();

/** The story shown, as the preview names its page (story.ts). */
let page = "";

const sent = () => state.batches.filter((b) => b.page === page && b.sentAt > state.cleared).flatMap((b) => b.notes);

/** Tells the preview the markers of the notes of the story shown, unless they are hidden. */
function showMarkers() {
  const pending: Marker[] = state.pending
    .filter((n) => n.state.page === page)
    .map((note, i) => ({ key: `p${String(i)}`, label: String(i + 1), color: "#666", note }));
  const done: Marker[] = sent().map((note: Note, i) => ({
    key: note.id,
    label: String(pending.length + i + 1),
    color: note.reply ? "#137333" : "#0b57d0",
    note,
  }));
  channel().emit(events.markers, state.markers ? [...pending, ...done] : []);
}

function refresh() {
  fetch(`${endpoint}/batches`)
    .then(async (res) => (await res.json()) as Batch[])
    .then((batches) => {
      set({ batches });
    })
    .catch((err: unknown) => {
      set({ error: String(err) });
    });
}

const pick = (on: boolean) => {
  set({ picking: on });
  channel().emit(events.pick, on);
};

function send() {
  set({ error: "" });
  fetch(`${endpoint}/batches`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ page, notes: state.pending }),
  })
    .then((res) => {
      if (!res.ok) throw new Error(`the dev server answered ${String(res.status)}`);
      set({ pending: [] });
      refresh();
    })
    .catch((err: unknown) => {
      set({ error: String(err) });
    });
}

const font = "13px/1.4 system-ui, sans-serif";
const button: CSSProperties = { font, padding: "2px 8px", cursor: "pointer" };
const muted: CSSProperties = { color: "#666" };

/** The toolbar's tool: pick what a note is on, and send the notes written. */
function Tool() {
  const s = useStore();
  return h(
    "span",
    { style: { display: "inline-flex", gap: 4, alignItems: "center", margin: "0 6px" } },
    h(
      "button",
      {
        type: "button",
        style: button,
        "aria-pressed": s.picking,
        onClick: () => {
          pick(!s.picking);
        },
      },
      s.picking ? "Picking… (Esc)" : "Note",
    ),
    h(
      "button",
      { type: "button", style: button, disabled: s.pending.length === 0, onClick: send },
      `Send ${String(s.pending.length)}`,
    ),
  );
}

function Draft({ draft }: { draft: NoteInput }) {
  const [text, setText] = useState("");
  const add = () => {
    if (text.trim() === "") return;
    set({ draft: null, pending: [...state.pending, { ...draft, text: text.trim() }] });
  };
  return h(
    "div",
    { style: { marginBottom: 8 } },
    h("div", { style: muted }, `${draft.target.area ? "area" : "element"}: ${draft.source ?? draft.target.tag}`),
    h("textarea", {
      "aria-label": "Note",
      autoFocus: true,
      rows: 3,
      style: { font, width: "100%", boxSizing: "border-box" },
      value: text,
      onChange: (e: { target: { value: string } }) => {
        setText(e.target.value);
      },
      onKeyDown: (e: { key: string; metaKey: boolean; ctrlKey: boolean }) => {
        if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) add();
        if (e.key === "Escape") set({ draft: null });
      },
    }),
    h(
      "div",
      { style: { display: "flex", gap: 4, justifyContent: "flex-end" } },
      h(
        "button",
        {
          type: "button",
          style: button,
          onClick: () => {
            set({ draft: null });
          },
        },
        "Cancel",
      ),
      h("button", { type: "button", style: button, onClick: add }, "Add note"),
    ),
  );
}

function Options({ set: options }: { set: OptionSet }) {
  const [reason, setReason] = useState("");
  const pickIt = () => {
    channel().emit(events.pickVariant, { id: options.id, reason });
    setReason("");
  };
  return h(
    "div",
    {
      role: "group",
      "aria-label": `Options ${options.id}`,
      style: { marginTop: 8, paddingTop: 6, borderTop: "1px solid #ddd" },
    },
    h("div", { style: muted }, `Options ${options.id}`),
    h(
      "div",
      { style: { display: "flex", gap: 4, flexWrap: "wrap", marginTop: 4 } },
      ...options.variants.map((v) =>
        h(
          "button",
          {
            key: v,
            type: "button",
            style: { ...button, fontWeight: v === options.variant ? 700 : 400 },
            "aria-pressed": v === options.variant,
            onClick: () => {
              channel().emit(events.switch, { id: options.id, variant: v });
            },
          },
          v,
        ),
      ),
      h("button", { type: "button", style: button, onClick: pickIt }, `Pick ${options.variant}`),
    ),
    h("input", {
      "aria-label": `Reason for the pick of ${options.id}`,
      placeholder: "Reason (optional)",
      style: { font, width: "100%", boxSizing: "border-box", marginTop: 4 },
      value: reason,
      onChange: (e: { target: { value: string } }) => {
        setReason(e.target.value);
      },
      onKeyDown: (e: { key: string }) => {
        if (e.key === "Enter") pickIt();
      },
    }),
  );
}

/** The Notes panel: the note being written, the live options, the notes not sent and those sent with their answers. */
function Panel() {
  const s = useStore();
  const { storyId } = useStorybookState();
  page = `/iframe.html?id=${storyId}`;
  const done = sent();
  return h(
    "div",
    { style: { font, padding: 12 } },
    h(
      "div",
      { style: { display: "flex", gap: 4, flexWrap: "wrap", marginBottom: 8 } },
      h(Tool),
      h(
        "button",
        {
          type: "button",
          style: button,
          disabled: done.length === 0,
          onClick: () => {
            const now = new Date().toISOString();
            save(() => localStorage, clearedKey, now);
            set({ cleared: now });
          },
        },
        "Clear sent",
      ),
      h(
        "label",
        { style: { display: "inline-flex", gap: 4, alignItems: "center" } },
        h("input", {
          type: "checkbox",
          checked: s.markers,
          onChange: () => {
            set({ markers: !s.markers });
          },
        }),
        "Markers on the story",
      ),
    ),
    s.error && h("div", { style: { color: "#b3261e" } }, s.error),
    s.draft && h(Draft, { key: JSON.stringify(s.draft.target), draft: s.draft }),
    ...s.options.map((o) => h(Options, { key: o.id, set: o })),
    h(
      "ol",
      { style: { margin: "8px 0 0", padding: "0 0 0 20px" } },
      ...s.pending.map((n, i) =>
        h(
          "li",
          { key: `p${String(i)}`, style: { marginBottom: 6 } },
          h(
            "div",
            { style: muted },
            `${n.story ? `${n.story.title} › ${n.story.name}` : n.state.page} · ${n.source ?? n.target.tag} · not sent`,
          ),
          h("div", null, n.text),
          h(
            "button",
            {
              type: "button",
              style: button,
              onClick: () => {
                set({ pending: state.pending.filter((_, j) => j !== i) });
              },
            },
            "Remove",
          ),
        ),
      ),
      ...done.map((n) =>
        h(
          "li",
          { key: n.id, style: { marginBottom: 6 }, "data-note": n.id },
          h("div", { style: muted }, `${n.id} · ${n.source ?? n.target.tag}`),
          h("div", null, n.text),
          n.reply
            ? h(
                "div",
                { style: { marginTop: 4, padding: 4, background: "#eef3fd", borderRadius: 3 }, "data-reply": n.id },
                h("strong", null, n.reply.kind),
                `: ${n.reply.text}`,
                n.reply.options !== undefined && h("div", null, `Switch them under Options: ${n.reply.options}`),
              )
            : h("div", { style: muted }, "waiting for an answer"),
        ),
      ),
    ),
  );
}

addons.register(addonId, (api) => {
  const c = channel();
  c.on(events.picked, (note: NoteInput | null) => {
    set({ picking: false, draft: note });
    if (note !== null) {
      api.setSelectedPanel(panelId);
      api.togglePanel(true);
    }
  });
  c.on(events.variantNote, (note: NoteInput) => {
    set({ pending: [...state.pending.filter((n) => n.pick?.options !== note.pick?.options), note] });
  });
  c.on(events.options, (options: OptionSet[]) => {
    set({ options });
  });
  c.on(events.update, refresh);
  c.on(events.ready, () => {
    page = `/iframe.html?id=${api.getUrlState().storyId ?? ""}`;
    c.emit(events.pick, state.picking);
    showMarkers();
  });
  refresh();
  addons.add(toolId, { type: types.TOOL, title: "Notes", render: () => h(Tool) });
  addons.add(panelId, {
    type: types.PANEL,
    title: "Notes",
    render: ({ active }) => (active ? h(Panel) : null),
  });
});
