// SPDX-License-Identifier: AGPL-3.0-only

// The plain-text editor (ADR-0092 §3, §4): a Lexical editor for every input of the client that shows more than its
// characters while they are written, the composer's text and content warning first. Its value is plain text: its
// lines joined by "\n", and a node that stands for text (a custom emoji's image) read as the text it stands for. The
// caller brings its nodes and its plugins (what it marks, what it shows as an image); this holds the mechanism and
// the look: a text box with its name, its description and its states, each paragraph in its own direction, undo,
// paste as plain text (files pasted, and Ctrl+Enter or ⌘+Enter, the caller's), the input method's composition left to the browser, and the characters over a limit marked
// with the CSS Custom Highlight API, which adds no node, so that it never splits what the caller marks; and the
// suggestions for the word at the caret (ADR-0092 §3): the caret and the composition told to the caller, the list
// placed under the caret's line, the keys that move in it, and the ARIA of a text box that completes. It is a chunk
// of its own (`@my/design-system/editor`), which its callers load lazily: no screen pays for Lexical until it writes.

import { HistoryExtension } from "@lexical/history";
import { PlainTextExtension } from "@lexical/plain-text";
import { useLexicalComposerContext } from "@lexical/react/LexicalComposerContext";
import { ContentEditable } from "@lexical/react/LexicalContentEditable";
import { LexicalExtensionComposer } from "@lexical/react/LexicalExtensionComposer";
import { mergeRegister } from "@lexical/utils";
import {
  $createLineBreakNode,
  $createParagraphNode,
  $createRangeSelection,
  $createTextNode,
  $getRoot,
  $getSelection,
  $isElementNode,
  $isLineBreakNode,
  $isRangeSelection,
  $isTextNode,
  $setSelection,
  BLUR_COMMAND,
  COMMAND_PRIORITY_CRITICAL,
  COMMAND_PRIORITY_HIGH,
  defineExtension,
  FOCUS_COMMAND,
  INSERT_LINE_BREAK_COMMAND,
  INSERT_PARAGRAPH_COMMAND,
  KEY_ARROW_DOWN_COMMAND,
  KEY_ARROW_UP_COMMAND,
  KEY_ENTER_COMMAND,
  KEY_ESCAPE_COMMAND,
  KEY_TAB_COMMAND,
  LineBreakNode,
  PASTE_COMMAND,
  SKIP_DOM_SELECTION_TAG,
  type EditorThemeClasses,
  type Klass,
  type LexicalEditor,
  type LexicalNode,
  type PointType,
} from "lexical";
import {
  useEffect,
  useEffectEvent,
  useImperativeHandle,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
  type Ref,
  type RefObject,
} from "react";
import { tv } from "tailwind-variants/lite";
import { Box } from "./layout";

/** The text the editor holds: its paragraphs, a line each. */
export function $plainText(): string {
  return $getRoot()
    .getChildren()
    .map((p) => p.getTextContent())
    .join("\n");
}

/** Writes `text` in place of what the editor holds: one paragraph, a line break for each "\n". */
export function $setPlainText(text: string): void {
  const root = $getRoot();
  root.clear();
  const paragraph = $createParagraphNode();
  text.split("\n").forEach((line, i) => {
    if (i > 0) paragraph.append($createLineBreakNode());
    if (line !== "") paragraph.append($createTextNode(line));
  });
  root.append(paragraph);
}

/** Where `point` is in the plain text, in UTF-16 code units. */
export function $offsetOf(point: PointType): number {
  let at = 0;
  const paragraphs = $getRoot().getChildren();
  if (point.key === $getRoot().getKey())
    return paragraphs.slice(0, point.offset).reduce((n, p, i) => n + (i > 0 ? 1 : 0) + p.getTextContentSize(), 0);
  for (const [i, paragraph] of paragraphs.entries()) {
    if (i > 0) at += 1;
    if (paragraph.getKey() === point.key) {
      const before = $isElementNode(paragraph) ? paragraph.getChildren().slice(0, point.offset) : [];
      return at + before.reduce((n, k) => n + k.getTextContentSize(), 0);
    }
    for (const node of $isElementNode(paragraph) ? paragraph.getChildren() : []) {
      if (node.getKey() === point.key) return at + (point.type === "text" ? point.offset : 0);
      at += node.getTextContentSize();
    }
  }
  return at;
}

/** The point at `offset` of the plain text: in a text node where one holds it, else beside the node it falls on. */
function $pointAt(offset: number): { key: string; offset: number; type: "text" | "element" } {
  let at = 0;
  const paragraphs = $getRoot().getChildren();
  for (const [i, paragraph] of paragraphs.entries()) {
    if (i > 0) at += 1;
    if (!$isElementNode(paragraph)) continue;
    const nodes = paragraph.getChildren();
    for (const [j, node] of nodes.entries()) {
      const size = node.getTextContentSize();
      if ($isTextNode(node) && offset <= at + size) return { key: node.getKey(), offset: offset - at, type: "text" };
      if (offset <= at) return { key: paragraph.getKey(), offset: j, type: "element" };
      at += size;
      // Inside what a node stands for (a custom emoji's `:shortcode:`): after it.
      if (offset < at) return { key: paragraph.getKey(), offset: j + 1, type: "element" };
    }
    if (offset <= at || i === paragraphs.length - 1)
      return { key: paragraph.getKey(), offset: nodes.length, type: "element" };
  }
  return { key: $getRoot().getKey(), offset: paragraphs.length, type: "element" };
}

/** The selection's start and end in the plain text, or none where the editor has no range selected. */
export function $selectionOffsets(): { start: number; end: number } | undefined {
  const selection = $getSelection();
  if (!$isRangeSelection(selection)) return undefined;
  const anchor = $offsetOf(selection.anchor);
  const focus = $offsetOf(selection.focus);
  return { start: Math.min(anchor, focus), end: Math.max(anchor, focus) };
}

/** Selects from `start` to `end` of the plain text. */
export function $selectOffsets(start: number, end: number = start): void {
  const selection = $createRangeSelection();
  const anchor = $pointAt(start);
  const focus = $pointAt(end);
  selection.anchor.set(anchor.key, anchor.offset, anchor.type);
  selection.focus.set(focus.key, focus.offset, focus.type);
  $setSelection(selection);
}

/** The editor a caller's plugin is drawn inside of. */
export function useEditor(): LexicalEditor {
  return useLexicalComposerContext()[0];
}

/** What a caller does with the editor from outside it (the emoji picker's button writes at its selection). */
export type EditorHandle = {
  focus: () => void;
  /**
   * Writes what `write` returns over the selection the editor had last, or at its end where it had none, given the
   * text before and after it; the caret goes after it, and the editor takes the focus unless `focus` is false.
   */
  insert: (write: (before: string, after: string) => string, options?: { focus?: boolean }) => void;
  /**
   * Writes `text` over `start` to `end` of the plain text as typing would (one step of undo), the caret at `caret`
   * (after what was written where not given), and the editor takes the focus.
   */
  write: (start: number, end: number, text: string, caret?: number) => void;
};

/**
 * The suggestions for the word that ends at the caret (ADR-0092 §3), open: the list, and what the editor's keys do to
 * it. The editor keeps the focus and stays a text box; the active option is its `aria-activedescendant`.
 */
export type FieldSuggest = {
  /** Where the word ends, the caret: a multiline editor opens the list under its line (a line, under itself). */
  at: number;
  /** The list (`SuggestionList`), or the line that stands in its place. */
  list: ReactNode;
  /** The listbox's id while it lists options (`SuggestionList`'s `id`). */
  listId?: string | undefined;
  /** How many options it lists, and the index of the active one. */
  count: number;
  active: number;
  /** What a screen reader hears as the list opens or changes: how many there are, or the line in their place. */
  announce: string;
  /** The arrow keys: the option to make active, stopping at either end. */
  onActive: (index: number) => void;
  /** Enter or Tab on the active option, or a press on one. */
  onAccept: (index: number) => void;
  /** Escape: the list closes, and what holds the editor stays open. */
  onClose: () => void;
};

/** What an editor tells the suggestions: where its caret is. */
export type FieldCaret = {
  /** The caret's UTF-16 offset as it moves or the text changes; undefined when a range is selected or it left. */
  onCaret?: ((caret: number | undefined) => void) | undefined;
};

/** The text to the caller as it changes, and the caller's value into the editor where it is not what it holds. */
function ValuePlugin({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  const [editor] = useLexicalComposerContext();
  const held = useRef(value);
  const changed = useEffectEvent(onChange);
  useEffect(
    () =>
      editor.registerUpdateListener(({ editorState }) => {
        const text = editorState.read($plainText);
        if (text === held.current) return;
        held.current = text;
        changed(text);
      }),
    [editor],
  );
  useEffect(() => {
    if (value === held.current) return;
    held.current = value;
    const focused = editor.getRootElement()?.contains(editor.getRootElement()?.ownerDocument.activeElement ?? null);
    editor.update(() => {
      $setPlainText(value);
      // The caret goes to the end only where the reader writes, so that a value set from outside takes no focus.
      if (focused === true) $getRoot().selectEnd();
      else $setSelection(null);
    });
  }, [editor, value]);
  return null;
}

/**
 * The focus an editor takes once drawn, its caret at the end of what it holds (a reply's mentions, a draft), so that
 * what is typed follows it: the browser puts the caret at the start of a text box it focuses, and Lexical reads it from
 * there, so the caret is set again once the text box has the focus, where it holds something (in an empty one the
 * start is the end). The text box takes the focus at once, in this effect, which runs before those of what holds the
 * editor: a dialog around it (React Aria's) focuses itself as it mounts unless the focus is inside it already, and
 * the focus moves once. Drawn late (`late`), as its chunk comes, it takes the focus only while it is on the page or on
 * what holds the editor (the dialog that took it while the chunk came); a control the reader has gone to meanwhile (a
 * menu's trigger, which its menu gave the focus back to) keeps it.
 */
function FocusPlugin({ late }: { late: boolean }) {
  const [editor] = useLexicalComposerContext();
  useEffect(() => {
    const root = editor.getRootElement();
    const active = document.activeElement;
    if (root === null) return;
    if (late && !(active === null || active === document.body || active.contains(root))) return;
    if (!root.contains(document.activeElement)) root.focus({ preventScroll: true });
    editor.focus(
      () => {
        if (!root.contains(document.activeElement)) root.focus({ preventScroll: true });
        if (editor.getEditorState().read(() => $getRoot().getTextContentSize()) === 0) return;
        editor.update(() => {
          $getRoot().selectEnd();
        });
      },
      { defaultSelection: "rootEnd" },
    );
  }, [editor, late]);
  return null;
}

/** The editor's handle: its focus, and writing at the selection it had last. */
function HandlePlugin({ handle }: { handle: Ref<EditorHandle> }) {
  const [editor] = useLexicalComposerContext();
  const last = useRef<{ start: number; end: number } | undefined>(undefined);
  useEffect(
    () =>
      editor.registerUpdateListener(({ editorState }) => {
        const offsets = editorState.read($selectionOffsets);
        if (offsets !== undefined) last.current = offsets;
      }),
    [editor],
  );
  useImperativeHandle(
    handle,
    () => ({
      focus: () => {
        editor.focus();
      },
      insert: (write, { focus = true } = {}) => {
        editor.update(
          () => {
            const text = $plainText();
            const { start, end } = $selectionOffsets() ?? last.current ?? { start: text.length, end: text.length };
            const from = Math.min(start, text.length);
            const to = Math.min(end, text.length);
            $selectOffsets(from, to);
            const selection = $getSelection();
            if ($isRangeSelection(selection)) selection.insertRawText(write(text.slice(0, from), text.slice(to)));
          },
          // Without the focus, the page's selection stays where it is (in the picker).
          focus ? {} : { tag: SKIP_DOM_SELECTION_TAG },
        );
        if (focus) editor.focus();
      },
      write: (start, end, text, caret) => {
        // A press on a suggestion while an input method composes: the blur commits the composition first, so that the
        // input method does not write it again over what replaces it (ADR-0092 §3).
        if (editor.isComposing()) editor.getRootElement()?.blur();
        editor.update(() => {
          $selectOffsets(start, end);
          const selection = $getSelection();
          if ($isRangeSelection(selection)) selection.insertRawText(text);
          $selectOffsets(caret ?? start + text.length);
        });
        editor.focus();
      },
    }),
    [editor],
  );
  return null;
}

/**
 * Tells the suggestions where the caret is, also while an input method composes, and gives them the keys while they
 * are open: the arrows move the active option, Enter and Tab accept it, Escape closes the list alone. An input
 * method's keys are its own: Lexical dispatches no key command while it composes, nor for the Enter Safari sends with
 * the keyCode 229 after the composition ended, so that only a press picks an option then (ADR-0092 §3).
 */
function SuggestPlugin({ suggest, onCaret }: FieldCaret & { suggest: FieldSuggest | undefined }) {
  const editor = useEditor();
  const caret = useEffectEvent((at: number | undefined) => {
    onCaret?.(at);
  });
  const key = useEffectEvent((name: "down" | "up" | "accept" | "escape", event: KeyboardEvent | null): boolean => {
    if (suggest === undefined || event === null) return false;
    if (name === "escape") {
      // Only the list closes: what holds the editor leaves an Escape whose default was prevented.
      event.preventDefault();
      event.stopPropagation();
      suggest.onClose();
      return true;
    }
    // A key with Alt, Ctrl or Cmd, and Shift + Enter, stay the text's; with no options, every key is.
    const listed = suggest.listId !== undefined && suggest.count > 0;
    if (!listed || event.altKey || event.ctrlKey || event.metaKey || (name === "accept" && event.shiftKey))
      return false;
    // The key is the list's: what holds the editor leaves it too (a modal's focus containment moves the focus on a Tab
    // whose default was prevented).
    event.preventDefault();
    event.stopPropagation();
    if (name === "accept") suggest.onAccept(suggest.active);
    else suggest.onActive(Math.min(Math.max(suggest.active + (name === "down" ? 1 : -1), 0), suggest.count - 1));
    return true;
  });
  useEffect(() => {
    const focused = () => {
      const root = editor.getRootElement();
      return root !== null && root.contains(root.ownerDocument.activeElement);
    };
    return mergeRegister(
      editor.registerUpdateListener(({ editorState }) => {
        const offsets = focused() ? editorState.read($selectionOffsets) : undefined;
        caret(offsets !== undefined && offsets.start === offsets.end ? offsets.end : undefined);
      }),
      editor.registerCommand(
        BLUR_COMMAND,
        () => {
          caret(undefined);
          return false;
        },
        COMMAND_PRIORITY_CRITICAL,
      ),
      editor.registerCommand(
        FOCUS_COMMAND,
        () => {
          caret(editor.getEditorState().read($selectionOffsets)?.end);
          return false;
        },
        COMMAND_PRIORITY_CRITICAL,
      ),
      editor.registerCommand(KEY_ARROW_DOWN_COMMAND, (e) => key("down", e), COMMAND_PRIORITY_CRITICAL),
      editor.registerCommand(KEY_ARROW_UP_COMMAND, (e) => key("up", e), COMMAND_PRIORITY_CRITICAL),
      editor.registerCommand(KEY_ENTER_COMMAND, (e) => key("accept", e), COMMAND_PRIORITY_CRITICAL),
      editor.registerCommand(KEY_TAB_COMMAND, (e) => key("accept", e), COMMAND_PRIORITY_CRITICAL),
      editor.registerCommand(KEY_ESCAPE_COMMAND, (e) => key("escape", e), COMMAND_PRIORITY_CRITICAL),
    );
  }, [editor]);
  return null;
}

/**
 * A count that changes when the box `ref` is placed in may have moved what is measured in it: its width, the visual
 * viewport (a software keyboard), and the fonts that arrived (`font-display: swap` changes the glyphs' widths).
 */
function useRelayout(ref: RefObject<HTMLElement | null>): number {
  const [count, setCount] = useState(0);
  useEffect(() => {
    const node = ref.current?.offsetParent;
    if (!(node instanceof HTMLElement)) return;
    const bump = () => {
      setCount((n) => n + 1);
    };
    const observer = new ResizeObserver(bump);
    observer.observe(node);
    document.fonts.addEventListener("loadingdone", bump);
    window.visualViewport?.addEventListener("resize", bump);
    return () => {
      observer.disconnect();
      document.fonts.removeEventListener("loadingdone", bump);
      window.visualViewport?.removeEventListener("resize", bump);
    };
  }, [ref]);
  return count;
}

/** Where the list stands in the box it is placed in: under a line, or over it where the viewport has no room under it. */
type Place = { top: number } | { bottom: number };

/**
 * The place of the list `pop` in `box` for a line from `lineTop` to `lineBottom` (viewport coordinates): under the
 * line, `gap` apart; over it when it does not fit under it within the visual viewport (above the software keyboard)
 * and there is more room over it.
 */
function placeOf(box: DOMRect, lineTop: number, lineBottom: number, gap: number, pop: HTMLElement | null): Place {
  const view = window.visualViewport;
  const viewTop = view?.offsetTop ?? 0;
  const viewBottom = viewTop + (view?.height ?? window.innerHeight);
  const height = pop?.offsetHeight ?? 0;
  const below = viewBottom - lineBottom - gap;
  const above = lineTop - gap - viewTop;
  return below < height && above > below ? { bottom: box.bottom - lineTop + gap } : { top: lineBottom - box.top + gap };
}

const samePlace = (a: Place | undefined, b: Place) =>
  a !== undefined && ("top" in a ? "top" in b && a.top === b.top : "bottom" in b && a.bottom === b.bottom);

/** The box of the character before `at` of the plain text (the end of the word at the caret), in the viewport. */
function $rectBefore(editor: LexicalEditor, at: number): DOMRect | undefined {
  const point = $pointAt(at);
  const dom = editor.getElementByKey(point.key);
  if (dom === null) return undefined;
  const range = dom.ownerDocument.createRange();
  const text = point.type === "text" ? textIn(dom) : undefined;
  if (text !== undefined) {
    range.setStart(text, Math.max(0, Math.min(point.offset, text.length) - 1));
    range.setEnd(text, Math.min(point.offset, text.length));
  } else {
    const before = dom.childNodes[point.offset - 1] ?? dom;
    range.selectNode(before);
  }
  return Array.from(range.getClientRects()).at(-1) ?? range.getBoundingClientRect();
}

/**
 * The suggestions' list under the line of the word at the caret, across the width of the box it is placed in (the
 * editor's, or the band's of a line), or over that line where the viewport has no room under it; measured unseen first.
 */
function SuggestPopup({ suggest, line }: { suggest: FieldSuggest; line: boolean }) {
  const editor = useEditor();
  const pop = useRef<HTMLDivElement>(null);
  const [place, setPlace] = useState<Place>();
  useRelayout(pop);
  // Every render: the caret's line, and the list's height, may have changed.
  useLayoutEffect(() => {
    const anchor = pop.current?.offsetParent;
    const root = editor.getRootElement();
    if (!(anchor instanceof HTMLElement) || root === null) return;
    const rect = anchor.getBoundingClientRect();
    const style = getComputedStyle(root);
    const gap = parseFloat(style.fontSize) / 4;
    let next: Place;
    if (line) next = placeOf(rect, rect.top, rect.bottom, gap, pop.current);
    else {
      const mark = editor.getEditorState().read(() => $rectBefore(editor, suggest.at));
      if (mark === undefined) return;
      // The caret's line: its glyphs' box and half of the leading over and under it.
      const half = (parseFloat(style.lineHeight) - mark.height) / 2;
      next = placeOf(rect, mark.top - half, mark.bottom + half, gap, pop.current);
    }
    setPlace((now) => (samePlace(now, next) ? now : next));
  });
  return (
    <div
      ref={pop}
      className="absolute inset-x-0 z-20"
      style={place === undefined ? { top: 0, visibility: "hidden" } : place}
    >
      {suggest.list}
    </div>
  );
}

/**
 * Ctrl+Enter, or ⌘+Enter, sends what is written (`onSubmit`) and writes no line break; not while an input method
 * composes, where Enter commits the composition.
 */
function SubmitPlugin({ onSubmit }: { onSubmit: () => void }) {
  const [editor] = useLexicalComposerContext();
  const submit = useEffectEvent(onSubmit);
  useEffect(
    () =>
      editor.registerCommand(
        KEY_ENTER_COMMAND,
        (event) => {
          if (event === null || !(event.ctrlKey || event.metaKey) || event.isComposing || editor.isComposing())
            return false;
          event.preventDefault();
          submit();
          return true;
        },
        COMMAND_PRIORITY_CRITICAL,
      ),
    [editor],
  );
  return null;
}

/**
 * A paste of files (an image copied or a screenshot) goes to `onFiles` and writes nothing. A paste that has text too
 * (a document's selection, which some apps also put as a picture) writes its text, as any paste does.
 */
function FilesPlugin({ onFiles }: { onFiles: (files: File[]) => void }) {
  const [editor] = useLexicalComposerContext();
  const files = useEffectEvent(onFiles);
  useEffect(
    () =>
      editor.registerCommand(
        PASTE_COMMAND,
        (event) => {
          const data = "clipboardData" in event ? event.clipboardData : null;
          if (data === null || data.files.length === 0 || data.getData("text/plain") !== "") return false;
          event.preventDefault();
          files([...data.files]);
          return true;
        },
        COMMAND_PRIORITY_HIGH,
      ),
    [editor],
  );
  return null;
}

/** One line: Enter writes nothing, and a line break pasted in is taken out, as a text input does. */
function SingleLinePlugin() {
  const [editor] = useLexicalComposerContext();
  useEffect(
    () =>
      mergeRegister(
        editor.registerCommand(
          KEY_ENTER_COMMAND,
          (event) => {
            event?.preventDefault();
            return true;
          },
          COMMAND_PRIORITY_HIGH,
        ),
        editor.registerCommand(INSERT_LINE_BREAK_COMMAND, () => true, COMMAND_PRIORITY_HIGH),
        editor.registerCommand(INSERT_PARAGRAPH_COMMAND, () => true, COMMAND_PRIORITY_HIGH),
        editor.registerNodeTransform(LineBreakNode, (node) => {
          node.remove();
        }),
      ),
    [editor],
  );
  return null;
}

/** Whether the editor may be written in. */
function EditablePlugin({ editable }: { editable: boolean }) {
  const [editor] = useLexicalComposerContext();
  useEffect(() => {
    editor.setEditable(editable);
  }, [editor, editable]);
  return null;
}

/** The name the part over a limit is marked by, in every editor (`::highlight(editor-over)`). */
const overName = "editor-over";

/** The ranges of the DOM that hold the last `over` characters of the text (code points; a line between paragraphs is one). */
function $overRanges(editor: LexicalEditor, over: number): Range[] {
  const ranges: Range[] = [];
  let left = over;
  const paragraphs = $getRoot().getChildren();
  for (let i = paragraphs.length - 1; i >= 0 && left > 0; i--) {
    const paragraph = paragraphs[i];
    const nodes: LexicalNode[] = paragraph && $isElementNode(paragraph) ? paragraph.getChildren() : [];
    for (let j = nodes.length - 1; j >= 0 && left > 0; j--) {
      const node = nodes[j];
      if (node === undefined) continue;
      const chars = Array.from(node.getTextContent());
      const dom = editor.getElementByKey(node.getKey());
      if (dom !== null && !$isLineBreakNode(node)) {
        const range = dom.ownerDocument.createRange();
        const text = $isTextNode(node) ? textIn(dom) : undefined;
        if (text === undefined) range.selectNode(dom);
        else {
          const from = chars.slice(0, Math.max(0, chars.length - left)).join("").length;
          range.setStart(text, Math.min(from, text.length));
          range.setEnd(text, text.length);
        }
        ranges.push(range);
      }
      left -= chars.length;
    }
    left -= 1;
  }
  return ranges;
}

/** The first text of `node`'s DOM. */
function textIn(node: Node): Text | undefined {
  if (node.nodeType === Node.TEXT_NODE) return node as Text;
  for (const child of Array.from(node.childNodes)) {
    const text = textIn(child);
    if (text !== undefined) return text;
  }
  return undefined;
}

/** The highlights of the page, where the browser has them. */
function highlights(): HighlightRegistry | undefined {
  return typeof CSS === "undefined" || typeof Highlight === "undefined" ? undefined : CSS.highlights;
}

/** Marks the last `over` characters, after every update; the counter tells them where the browser cannot mark them. */
function OverPlugin({ over }: { over: number }) {
  const [editor] = useLexicalComposerContext();
  useEffect(() => {
    const registry = highlights();
    if (registry === undefined) return undefined;
    let ranges: Range[] = [];
    const clear = () => {
      const highlight = registry.get(overName);
      for (const range of ranges) highlight?.delete(range);
      ranges = [];
    };
    const paint = () => {
      clear();
      if (over <= 0) return;
      let highlight = registry.get(overName);
      if (highlight === undefined) {
        highlight = new Highlight();
        registry.set(overName, highlight);
      }
      ranges = editor.getEditorState().read(() => $overRanges(editor, over));
      for (const range of ranges) highlight.add(range);
    };
    paint();
    const stop = editor.registerUpdateListener(paint);
    return () => {
      stop();
      clear();
    };
  }, [editor, over]);
  return null;
}

const field = tv({
  slots: {
    // The text and its placeholder over each other; the placeholder lets a press through to the text.
    wrap: "[&>[aria-hidden]]:pointer-events-none",
    text: [
      "box-border border-0 p-0 font-sans text-text outline-hidden whitespace-pre-wrap [overflow-wrap:anywhere]",
      // The part over a limit: a deeper neutral and a line under it, apart from a mark in gray as in color.
      "[&_*::highlight(editor-over)]:bg-surface-raised-pressed [&::highlight(editor-over)]:bg-surface-raised-pressed",
      "[&_*::highlight(editor-over)]:underline [&::highlight(editor-over)]:underline",
      "[&_*::highlight(editor-over)]:decoration-text [&::highlight(editor-over)]:decoration-text",
    ],
    placeholder: "box-border p-0 font-sans text-muted-text select-none",
  },
  variants: {
    look: {
      // The composer's text: its size and leading, on the composer's surface; the suggestions open in its box.
      composer: {
        text: "text-[length:var(--composer-text-size)] leading-[var(--composer-text-leading)]",
        placeholder: "text-[length:var(--composer-text-size)] leading-[var(--composer-text-leading)]",
      },
      // A line on a band (the content warning): the body's size, as tall as a form's control; the suggestions open in
      // the band's box, which places them.
      line: {
        wrap: "min-h-control-form pe-control-timeline-inline",
        text: "text-body",
        placeholder: "text-body",
      },
    },
    // Three lines of room while nothing follows the text; with images or a poll under it, only its own lines.
    roomy: {
      true: { text: "min-h-composer-room" },
      false: {},
    },
    // A button at the end of the first line: the text keeps clear of it on every line. The room is the wrapper's, in
    // the page's direction as the button is placed: the text's own direction (dir="auto") would move it to the other
    // side where the text runs against the page.
    ended: {
      true: { wrap: "pe-icon-button-end" },
      false: {},
    },
  },
  compoundVariants: [
    {
      look: "composer",
      roomy: false,
      class: { text: "min-h-composer-line" },
    },
  ],
});

/**
 * The box of the text: the text and its placeholder over each other, in one cell as large as the larger. The
 * composer's is where the suggestions are placed (`SuggestPopup`); a line on a band takes the room left in the band and
 * stands in the middle of its height.
 */
function FieldBox({
  look,
  className,
  children,
}: {
  look: "composer" | "line";
  className: string;
  children: ReactNode;
}) {
  const own = look === "composer" ? "relative" : "min-w-0 flex-1 content-center";
  return <div className={`${className} grid grid-cols-[minmax(0,1fr)] [&>*]:[grid-area:1/1] ${own}`}>{children}</div>;
}

/**
 * The classes of the editor's nodes: a paragraph; and `token`, a part the caller's nodes mark (what the server will
 * read: a mention, a hashtag, a URL), whole on its line — it moves to the next line as one, and breaks inside only
 * where it is longer than a line — on the tint of the link it becomes, underlined in forced colors.
 */
const theme: EditorThemeClasses = {
  paragraph: "m-0",
  token: [
    "inline-block max-w-full rounded-[var(--composer-over-radius)] bg-mark [overflow-wrap:anywhere]",
    "forced-colors:bg-transparent forced-colors:underline",
  ].join(" "),
};

export type PlainTextEditorProps = {
  /** The editor's name among the editors of the page (Lexical's namespace). */
  namespace: string;
  /** The text it holds; set from outside, it replaces what the editor holds. */
  value: string;
  onChange: (value: string) => void;
  /** Its name, for assistive technology. */
  label: string;
  /** What it shows while it is empty, also told to assistive technology. */
  placeholder: string;
  /** The ID of what describes it (its error). */
  describedBy?: string | undefined;
  invalid?: boolean | undefined;
  required?: boolean | undefined;
  /** Several lines, or one: Enter writes nothing, and a line break pasted in is taken out. */
  multiline?: boolean | undefined;
  readOnly?: boolean | undefined;
  /** It takes the focus once drawn, its caret at the end. */
  autoFocus?: boolean | undefined;
  /**
   * It is drawn after what holds it (its chunk came later): with `autoFocus`, it takes the focus only from the page or
   * from what holds it, never from a control the reader has gone to since.
   */
  late?: boolean | undefined;
  /** How many characters at its end are over a limit, marked. */
  over?: number | undefined;
  /** The composer's text, or a line on a band (the content warning). */
  look: "composer" | "line";
  /** The composer's look: three lines of room while nothing follows it. */
  roomy?: boolean | undefined;
  /** A button sits at the end of its first line, which the text keeps clear of. */
  ended?: boolean | undefined;
  /** Ctrl+Enter or ⌘+Enter: what is written is sent. */
  onSubmit?: (() => void) | undefined;
  /** Files pasted in (an image copied): they are the caller's, and write nothing. */
  onFiles?: ((files: File[]) => void) | undefined;
  /** The caller's nodes (what it marks, what it shows as an image). */
  nodes?: readonly Klass<LexicalNode>[] | undefined;
  handle?: Ref<EditorHandle> | undefined;
  /** It completes what is typed in it (`aria-autocomplete`): the suggestions open in it as `suggest`. */
  completes?: boolean | undefined;
  /** The suggestions for the word that ends at the caret, while they are open. */
  suggest?: FieldSuggest | undefined;
  /** The caller's plugins, inside the editor's context. */
  children?: ReactNode;
} & FieldCaret;

/** A plain-text editor: a text box whose value is plain text, with the caller's nodes and plugins. */
export function PlainTextEditor({
  namespace,
  value,
  onChange,
  label,
  placeholder,
  describedBy,
  invalid = false,
  required = false,
  multiline = true,
  readOnly = false,
  autoFocus = false,
  late = false,
  over = 0,
  look,
  roomy = false,
  ended = false,
  onSubmit,
  onFiles,
  nodes = [],
  handle,
  completes = false,
  suggest,
  onCaret,
  children,
}: PlainTextEditorProps) {
  const listed = suggest !== undefined && suggest.listId !== undefined && suggest.count > 0;
  const s = field({ look, roomy: look === "composer" && roomy, ended });
  // The editor is built once, as it mounts, with what it holds then; the value set later goes in by ValuePlugin.
  const [extension] = useState(() =>
    defineExtension({
      name: `@my/design-system/editor/${namespace}`,
      namespace,
      theme,
      nodes: [...nodes],
      editable: !readOnly,
      dependencies: [PlainTextExtension, HistoryExtension],
      $initialEditorState: () => {
        $setPlainText(value);
      },
      onError: (error) => {
        reportError(error);
      },
    }),
  );
  return (
    <LexicalExtensionComposer extension={extension} contentEditable={null}>
      <FieldBox look={look} className={s.wrap()}>
        <ContentEditable
          className={s.text()}
          ariaLabel={label}
          ariaDescribedBy={describedBy}
          ariaInvalid={invalid ? true : undefined}
          ariaRequired={required ? true : undefined}
          ariaMultiline={multiline}
          ariaAutoComplete={completes ? "list" : undefined}
          ariaControls={listed ? suggest.listId : undefined}
          ariaActiveDescendant={listed ? `${suggest.listId ?? ""}-${String(suggest.active)}` : undefined}
          aria-placeholder={placeholder}
          placeholder={
            <Box as="p" lines={look === "line" ? 1 : undefined} className={s.placeholder()}>
              {placeholder}
            </Box>
          }
          dir="auto"
        />
        {suggest !== undefined && <SuggestPopup suggest={suggest} line={look === "line"} />}
        {completes && (
          <span className="sr-only" role="status">
            {suggest?.announce ?? ""}
          </span>
        )}
      </FieldBox>
      {(completes || onCaret !== undefined) && <SuggestPlugin suggest={suggest} onCaret={onCaret} />}
      <ValuePlugin value={value} onChange={onChange} />
      <EditablePlugin editable={!readOnly} />
      <OverPlugin over={over} />
      {onSubmit !== undefined && <SubmitPlugin onSubmit={onSubmit} />}
      {onFiles !== undefined && <FilesPlugin onFiles={onFiles} />}
      {!multiline && <SingleLinePlugin />}
      {autoFocus && <FocusPlugin late={late} />}
      {handle !== undefined && <HandlePlugin handle={handle} />}
      {children}
    </LexicalExtensionComposer>
  );
}
