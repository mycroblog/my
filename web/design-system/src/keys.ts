// SPDX-License-Identifier: AGPL-3.0-only

// The logic of the shortcuts (ADR-0025 §4), apart from React and the DOM: the chords of a keymap and how a key press
// matches one, the check of a keymap, and the declared actions with the one a press runs and the list the help shows.
// shortcuts.tsx connects it to the page. A chord is the key as KeyboardEvent.key names it ("n", "?", "ArrowDown"),
// after modifiers joined by "+": Mod (⌘ on macOS, Ctrl elsewhere), Ctrl, Meta, Alt and Shift. A letter matches in
// either case unless the chord has Shift; a printable key that is no letter ("?") matches whatever Shift made it.

/** An action's keys in the keymap, and whether they act inside a text field too. */
export type Binding = { keys: readonly string[]; inFields?: boolean };

/** Which keys run which action, by the action's ID (`post.reply`): the app's, in one place. */
export type Keymap = Readonly<Record<string, Binding>>;

/** Where an action acts: anywhere on the screen, or while focus is inside the element that declares it. */
export type Scope = "screen" | "focus";

type Chord = { key: string; mod: boolean; ctrl: boolean; meta: boolean; alt: boolean; shift: boolean };

const modifiers = ["Mod", "Ctrl", "Meta", "Alt", "Shift"] as const;

function parse(chord: string): Chord {
  // The key is what follows the last "+", unless the key is "+" itself.
  const cut = chord.endsWith("++") ? chord.length - 1 : chord.lastIndexOf("+") + 1;
  const key = chord.slice(cut);
  const mods = cut === 0 ? [] : chord.slice(0, cut - 1).split("+");
  if (key === "") throw new Error(`"${chord}" names no key`);
  for (const m of mods)
    if (!(modifiers as readonly string[]).includes(m))
      throw new Error(`"${chord}": ${m} is no modifier (${modifiers.join(", ")})`);
  return {
    key,
    mod: mods.includes("Mod"),
    ctrl: mods.includes("Ctrl"),
    meta: mods.includes("Meta"),
    alt: mods.includes("Alt"),
    shift: mods.includes("Shift"),
  };
}

const isLetter = (key: string) => key.length === 1 && key.toLowerCase() !== key.toUpperCase();

/** The chord as one string per platform, Mod resolved, so that two spellings of one chord compare equal. */
function canonical(c: Chord, mac: boolean): string {
  const ctrl = c.ctrl || (c.mod && !mac);
  const meta = c.meta || (c.mod && mac);
  const key = c.key.length === 1 ? c.key.toLowerCase() : c.key;
  return [ctrl && "ctrl", meta && "meta", c.alt && "alt", c.shift && "shift", key].filter(Boolean).join("+");
}

/** What a key press says: the key, its modifiers, and whether an input method is composing it. */
export type KeyPress = {
  key: string;
  ctrlKey: boolean;
  metaKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
  isComposing?: boolean;
  /** The key is held: this press repeats the one before. */
  repeat?: boolean;
  keyCode?: number;
};

/** An input method is composing the press: it is the IME's, never a shortcut's (229 is what some browsers send). */
const composing = (e: KeyPress) => e.isComposing === true || e.keyCode === 229;

function matches(c: Chord, e: KeyPress, mac: boolean): boolean {
  if (e.ctrlKey !== (c.ctrl || (c.mod && !mac))) return false;
  if (e.metaKey !== (c.meta || (c.mod && mac))) return false;
  if (e.altKey !== c.alt) return false;
  if (c.key.length === 1) {
    if (isLetter(c.key)) return e.key.toLowerCase() === c.key.toLowerCase() && e.shiftKey === c.shift;
    return e.key === c.key;
  }
  return e.key === c.key && e.shiftKey === c.shift;
}

// The chords a browser keeps for itself and never passes to a page (new window or tab, close, quit, switch tabs).
const reserved = [
  "Mod+N",
  "Mod+Shift+N",
  "Mod+T",
  "Mod+Shift+T",
  "Mod+W",
  "Mod+Shift+W",
  "Mod+Q",
  "Ctrl+Tab",
  "Ctrl+Shift+Tab",
  "Ctrl+PageUp",
  "Ctrl+PageDown",
  "Meta+H",
  "Meta+M",
  "Alt+F4",
].map(parse);

/**
 * The problems of a keymap: a chord that does not parse, and a chord the browser keeps (Ctrl or ⌘ with N, T or W),
 * which would never reach the page. Two actions on one chord are a conflict only within one scope, which the
 * declarations give (`Shortcuts.declare`).
 */
export function keymapProblems(keymap: Keymap): string[] {
  const problems: string[] = [];
  for (const [id, { keys }] of Object.entries(keymap)) {
    for (const chord of keys) {
      let c: Chord;
      try {
        c = parse(chord);
      } catch (e) {
        problems.push(`${id}: ${(e as Error).message}`);
        continue;
      }
      for (const mac of [true, false])
        if (reserved.some((r) => canonical(r, mac) === canonical(c, mac))) {
          problems.push(
            `${id}: the browser keeps ${spelled(chord, mac)} for itself and never passes it to the page; choose another key`,
          );
          break;
        }
    }
  }
  return problems;
}

/** Throws with every problem of `keymap`, or returns it. */
function checkKeymap(keymap: Keymap): Keymap {
  const problems = keymapProblems(keymap);
  if (problems.length > 0) throw new Error(`the keymap has problems:\n${problems.join("\n")}`);
  return keymap;
}

const names: Readonly<Record<string, string>> = {
  ArrowUp: "↑",
  ArrowDown: "↓",
  ArrowLeft: "←",
  ArrowRight: "→",
  Escape: "Esc",
  " ": "Space",
};

/** A chord as the reader's platform names its keys, one part per key: ["⌘", "Enter"] on macOS, ["Ctrl", "Enter"] elsewhere. */
export function display(chord: string, mac: boolean): string[] {
  const c = parse(chord);
  const parts: string[] = [];
  if (c.ctrl || (c.mod && !mac)) parts.push(mac ? "⌃" : "Ctrl");
  if (c.alt) parts.push(mac ? "⌥" : "Alt");
  if (c.shift) parts.push(mac ? "⇧" : "Shift");
  if (c.meta || (c.mod && mac)) parts.push(mac ? "⌘" : "Meta");
  parts.push(names[c.key] ?? c.key);
  return parts;
}

/** A chord in one string, as the reader's platform writes it: "⌘Enter" on macOS, "Ctrl+Enter" elsewhere. */
export const spelled = (chord: string, mac: boolean) => display(chord, mac).join(mac ? "" : "+");

/** The keys of an action as aria-keyshortcuts says them: "Control+Enter j", Mod resolved for the platform. */
export function ariaKeys(keys: readonly string[], mac: boolean): string {
  return keys
    .map((chord) => {
      const c = parse(chord);
      const parts: string[] = [];
      if (c.ctrl || (c.mod && !mac)) parts.push("Control");
      if (c.alt) parts.push("Alt");
      if (c.shift) parts.push("Shift");
      if (c.meta || (c.mod && mac)) parts.push("Meta");
      parts.push(c.key === " " ? "Space" : c.key);
      return parts.join("+");
    })
    .join(" ");
}

/** What an action learns of the press that runs it: whether the key is held (a move then jumps instead of gliding). */
export type Pressed = { repeat: boolean };

/** An action a part declared: its ID, its label (from the part's catalog), its scope, its element, and what it runs. */
export type Declared<E> = {
  id: string;
  label: string;
  scope: Scope;
  /** The element focus must be inside for a focus action; none for a screen action. */
  owner: () => E | null;
  /** Runs the action; `false` says it did nothing (the first post has no previous one), and the key goes on. */
  run: (press: Pressed) => unknown;
  /** The group of a focus action in the help: the ID's namespace (`post` of `post.reply`) unless given. */
  group?: string | undefined;
  /** The group's heading in the help ("When a post is focused"), from the declaring part's catalog. */
  groupLabel?: string | undefined;
};

/** An action and its keys, as the help lists it. */
export type Listed = { id: string; label: string; keys: readonly string[] };

/** A group of focus actions in the help: its heading, whether focus is in it now, and its actions. */
export type ListedGroup = { id: string; label: string; current: boolean; actions: Listed[] };

/** The group of a focus action: given, else its ID's namespace. */
const groupOf = (a: { id: string; group?: string | undefined }) => a.group ?? a.id.split(".")[0] ?? a.id;

/** Where a press happened: the focused element and its ancestors, from the inside out, and what it is inside. */
export type Place<E> = {
  /** The focused element and its ancestors, from the inside out, up to the layer that holds focus. */
  path: readonly E[];
  /** Focus is in a text field: only the actions the keymap allows there act. */
  inField: boolean;
  /** Focus is on a layer over the screen (a dialog, a menu): the screen's actions wait. */
  overlaid: boolean;
};

/** The declared actions of one keymap, and which of them acts on a press or shows in the help. */
export class Shortcuts<E> {
  readonly #keymap: Keymap;
  readonly #mac: boolean;
  readonly #declared: Declared<E>[] = [];

  constructor(keymap: Keymap, mac: boolean) {
    this.#keymap = checkKeymap(keymap);
    this.#mac = mac;
  }

  /** The keys are named as on macOS (⌘), else as elsewhere (Ctrl). */
  get mac(): boolean {
    return this.#mac;
  }

  /** The keys of an action, as aria-keyshortcuts says them; none for an action the keymap gives no key. */
  aria(id: string): string | undefined {
    const keys = this.#keymap[id]?.keys ?? [];
    return keys.length === 0 ? undefined : ariaKeys(keys, this.#mac);
  }

  /**
   * Declares an action until the returned function takes it back. Throws when another action of the same scope (on
   * the same element, for a focus action) has one of its keys: a conflict.
   */
  declare(action: Declared<E>): () => void {
    const owner = action.owner();
    const mine = new Set((this.#keymap[action.id]?.keys ?? []).map((k) => canonical(parse(k), this.#mac)));
    for (const other of this.#declared) {
      if (other.id === action.id || other.scope !== action.scope) continue;
      if (action.scope === "focus" && other.owner() !== owner) continue;
      const shared = (this.#keymap[other.id]?.keys ?? []).find((k) => mine.has(canonical(parse(k), this.#mac)));
      if (shared !== undefined)
        throw new Error(
          `${action.id} and ${other.id} share ${shared} in the ${action.scope} scope${action.scope === "focus" ? " of one element" : ""}: give one of them another key in the keymap`,
        );
    }
    this.#declared.push(action);
    return () => {
      const i = this.#declared.indexOf(action);
      if (i >= 0) this.#declared.splice(i, 1);
    };
  }

  #acts(action: Declared<E>, e: KeyPress, inField: boolean): boolean {
    const binding = this.#keymap[action.id];
    if (binding === undefined || (inField && binding.inFields !== true)) return false;
    return binding.keys.some((k) => matches(parse(k), e, this.#mac));
  }

  /** The latest declaration of each ID among `actions`. */
  #latest(actions: Declared<E>[]): Declared<E>[] {
    const byId = new Map<string, Declared<E>>();
    for (const a of actions) byId.set(a.id, a);
    return [...byId.values()];
  }

  /**
   * Runs the action of a press, if any, and returns whether it took the key: false for no action, or for one that
   * returned `false` (it did nothing), so that the key goes on to the page.
   */
  press(e: KeyPress, place: Place<E>): boolean {
    const action = this.find(e, place);
    return action !== undefined && action.run({ repeat: e.repeat === true }) !== false;
  }

  /**
   * The action a press runs: one of the focused element's, else of the nearest ancestor that declares one, else the
   * screen's; none while an input method composes, in a text field (but for keys allowed there), or for a free key.
   */
  find(e: KeyPress, place: Place<E>): Declared<E> | undefined {
    if (composing(e)) return undefined;
    for (const el of place.path) {
      const own = this.#declared.filter((a) => a.scope === "focus" && a.owner() === el);
      const hit = this.#latest(own).find((a) => this.#acts(a, e, place.inField));
      if (hit !== undefined) return hit;
    }
    if (place.overlaid) return undefined;
    return this.#latest(this.#declared.filter((a) => a.scope === "screen")).find((a) =>
      this.#acts(a, e, place.inField),
    );
  }

  /**
   * What the help lists with focus at `path`: the screen's actions, then a group per kind of thing that takes focus
   * (`post`, `timeline`) of the focus actions declared on the screen, each action once with its keys in the keymap's
   * order. The groups focus is in now come first, the nearest first, marked current; the others follow in the
   * keymap's order. An action the keymap gives no key, and a group left with none, are left out.
   */
  list(path: readonly E[]): { screen: Listed[]; groups: ListedGroup[] } {
    const order = Object.keys(this.#keymap);
    const rank = (id: string) => order.indexOf(id);
    const listed = (actions: Declared<E>[]) =>
      this.#latest(actions)
        .filter((a) => (this.#keymap[a.id]?.keys.length ?? 0) > 0)
        .sort((a, b) => rank(a.id) - rank(b.id))
        .map((a) => ({ id: a.id, label: a.label, keys: this.#keymap[a.id]?.keys ?? [] }));
    const focus = this.#declared.filter((a) => a.scope === "focus");
    const current: string[] = [];
    for (const el of path)
      for (const a of focus) if (a.owner() === el && !current.includes(groupOf(a))) current.push(groupOf(a));
    const groups = [...new Set(focus.map(groupOf))].flatMap((id): ListedGroup[] => {
      const members = focus.filter((a) => groupOf(a) === id);
      const actions = listed(members);
      if (actions.length === 0) return [];
      const label = members.find((a) => a.groupLabel !== undefined)?.groupLabel ?? id;
      return [{ id, label, current: current.includes(id), actions }];
    });
    const first = (g: ListedGroup) => Math.min(...g.actions.map((a) => rank(a.id)));
    const place = (g: ListedGroup) => (g.current ? current.indexOf(g.id) - current.length : first(g));
    groups.sort((a, b) => place(a) - place(b));
    return { screen: listed(this.#declared.filter((a) => a.scope === "screen")), groups };
  }
}

/** What an element is, as far as a shortcut cares: whether typing goes into it. */
export type FieldLike = {
  tagName: string;
  type?: string;
  isContentEditable?: boolean;
  getAttribute(name: string): string | null;
};

// The input types that take no typing: a key press on one is free for a shortcut.
const untyped = new Set([
  "button",
  "checkbox",
  "radio",
  "submit",
  "reset",
  "range",
  "color",
  "file",
  "image",
  "hidden",
]);

/** Typing goes into the element: a text input, a text area, a select, an editable element or a textbox. */
export function isTextField(el: FieldLike): boolean {
  const tag = el.tagName.toLowerCase();
  if (tag === "input") return !untyped.has((el.type ?? "text").toLowerCase());
  if (tag === "textarea" || tag === "select" || el.isContentEditable === true) return true;
  const role = el.getAttribute("role");
  return role === "textbox" || role === "searchbox" || role === "combobox";
}
