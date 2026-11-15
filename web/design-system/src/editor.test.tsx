// SPDX-License-Identifier: AGPL-3.0-only

// The plain text of the editor (ADR-0092 §4): what $setPlainText writes, $plainText reads back unchanged, and the
// offsets of a selection are those of that plain text, in UTF-16 code units, also around a node that stands for text (a
// caller's custom emoji, read as its `:shortcode:`). Run headless: Lexical's editor without a DOM. And the editor is a
// text box to assistive technology, with its name, description and states, as it is first drawn.

import { describe, expect, test } from "bun:test";
import {
  $createParagraphNode,
  $createTextNode,
  $getRoot,
  createEditor,
  DecoratorNode,
  type LexicalEditor,
  type NodeKey,
} from "lexical";
import { renderToStaticMarkup } from "react-dom/server";
import { $plainText, $selectionOffsets, $selectOffsets, $setPlainText, PlainTextEditor } from "./editor";

/** A node that stands for text, as a caller's custom emoji does: shown as something else, read as `:shortcode:`. */
class ShortcodeNode extends DecoratorNode<null> {
  __shortcode: string;

  static override getType(): string {
    return "test-shortcode";
  }

  static override clone(node: ShortcodeNode): ShortcodeNode {
    return new ShortcodeNode(node.__shortcode, node.__key);
  }

  constructor(shortcode = "", key?: NodeKey) {
    super(key);
    this.__shortcode = shortcode;
  }

  override createDOM(): HTMLElement {
    throw new Error("headless: no DOM");
  }

  override updateDOM(): false {
    return false;
  }

  override isInline(): true {
    return true;
  }

  override getTextContent(): string {
    return `:${this.__shortcode}:`;
  }

  override decorate(): null {
    return null;
  }
}

function editor(): LexicalEditor {
  return createEditor({
    namespace: "test",
    nodes: [ShortcodeNode],
    onError: (error) => {
      throw error;
    },
  });
}

/** Runs `fn` in an update that commits before it returns. */
function write(e: LexicalEditor, fn: () => void): void {
  e.update(fn, { discrete: true });
}

/** Writes `text` in the editor with $setPlainText. */
function set(e: LexicalEditor, text: string): void {
  write(e, () => {
    $setPlainText(text);
  });
}

function read<T>(e: LexicalEditor, fn: () => T): T {
  return e.getEditorState().read(fn);
}

const texts: readonly [string, string][] = [
  ["nothing", ""],
  ["one line", "Walked along the river this morning."],
  ["lines", "first\nsecond\nthird"],
  ["an empty line between, and a newline at either end", "\nfirst\n\nsecond\n"],
  ["a newline alone", "\n"],
  ["a custom emoji's shortcode", "Tea time :teacup: with :wave:"],
  ["mentions", "@sam @sam@example.social hello"],
  ["hashtags in any script", "#山 #photography #日本語"],
  ["URLs", "see https://example.com/a/b?c=d&e=f#g and http://例え.jp/パス"],
  ["right-to-left text with left-to-right inside", "مرحبا بالعالم\nhello @sam #سلام"],
  ["astral characters and joined emoji", "😀 👩‍👩‍👧 🇯🇵 𠮷"],
];

describe("$setPlainText then $plainText gives the same text", () => {
  for (const [what, text] of texts)
    test(what, () => {
      const e = editor();
      set(e, text);
      expect(read(e, $plainText)).toBe(text);
    });
});

test("$setPlainText writes one paragraph, its lines joined by line breaks", () => {
  const e = editor();
  set(e, "a\nb\nc");
  read(e, () => {
    expect($getRoot().getChildrenSize()).toBe(1);
    expect($getRoot().getTextContent()).toBe("a\nb\nc");
  });
});

test("$setPlainText replaces what the editor holds", () => {
  const e = editor();
  set(e, "old\ntext");
  set(e, "new");
  expect(read(e, $plainText)).toBe("new");
});

test("$plainText reads paragraphs a line each", () => {
  const e = editor();
  write(e, () => {
    $getRoot()
      .clear()
      .append(
        $createParagraphNode().append($createTextNode("one")),
        $createParagraphNode(),
        $createParagraphNode().append($createTextNode("three")),
      );
  });
  expect(read(e, $plainText)).toBe("one\n\nthree");
});

test("$plainText reads a node that stands for text as the text it stands for", () => {
  const e = editor();
  write(e, () => {
    $getRoot()
      .clear()
      .append(
        $createParagraphNode().append(
          $createTextNode("Tea "),
          new ShortcodeNode("teacup"),
          $createTextNode(" now"),
          new ShortcodeNode("wave"),
        ),
      );
  });
  expect(read(e, $plainText)).toBe("Tea :teacup: now:wave:");
});

/** Selects `start`–`end` and reads the selection's offsets back. */
function roundTrip(e: LexicalEditor, start: number, end: number) {
  write(e, () => {
    $selectOffsets(start, end);
  });
  return read(e, $selectionOffsets);
}

describe("$selectOffsets then $selectionOffsets gives the same offsets", () => {
  for (const text of ["ab\ncd", "\n\nx\n", "مرحبا\nhi", "#山 @sam https://example.com"])
    test(JSON.stringify(text), () => {
      const e = editor();
      set(e, text);
      for (let start = 0; start <= text.length; start += 1)
        for (let end = start; end <= text.length; end += 1) expect(roundTrip(e, start, end)).toEqual({ start, end });
    });

  test("offsets count UTF-16 code units", () => {
    const e = editor();
    set(e, "😀a");
    expect(roundTrip(e, 2, 3)).toEqual({ start: 2, end: 3 });
    expect(roundTrip(e, 3, 3)).toEqual({ start: 3, end: 3 });
  });

  test("a caret alone has its start and end at one offset", () => {
    const e = editor();
    set(e, "abc");
    expect(roundTrip(e, 2, 2)).toEqual({ start: 2, end: 2 });
  });

  test("a selection made backwards reads from its start to its end", () => {
    const e = editor();
    set(e, "abcdef");
    expect(roundTrip(e, 5, 1)).toEqual({ start: 1, end: 5 });
  });

  test("the text's end, in an empty editor and after a last empty line", () => {
    const empty = editor();
    set(empty, "");
    expect(roundTrip(empty, 0, 0)).toEqual({ start: 0, end: 0 });
    const trailing = editor();
    set(trailing, "a\n");
    expect(roundTrip(trailing, 2, 2)).toEqual({ start: 2, end: 2 });
  });

  test("across paragraphs, the newline between them counts one", () => {
    const e = editor();
    write(e, () => {
      $getRoot()
        .clear()
        .append(
          $createParagraphNode().append($createTextNode("ab")),
          $createParagraphNode().append($createTextNode("cd")),
        );
    });
    for (const [start, end] of [
      [0, 5],
      [2, 3],
      [3, 3],
      [1, 4],
    ] as const)
      expect(roundTrip(e, start, end)).toEqual({ start, end });
  });

  test("before and after a node that stands for text, which counts as its whole text", () => {
    const e = editor();
    write(e, () => {
      $getRoot()
        .clear()
        .append(
          $createParagraphNode().append(
            $createTextNode("Tea "),
            new ShortcodeNode("teacup"),
            $createTextNode(" now "),
            new ShortcodeNode("wave"),
          ),
        );
    });
    // "Tea " 0–4, ":teacup:" 4–12, " now " 12–17, ":wave:" 17–23.
    expect(roundTrip(e, 4, 4)).toEqual({ start: 4, end: 4 });
    expect(roundTrip(e, 12, 12)).toEqual({ start: 12, end: 12 });
    expect(roundTrip(e, 4, 12)).toEqual({ start: 4, end: 12 });
    expect(roundTrip(e, 17, 23)).toEqual({ start: 17, end: 23 });
    expect(roundTrip(e, 23, 23)).toEqual({ start: 23, end: 23 });
  });

  test("an editor with no range selected has no offsets", () => {
    const e = editor();
    set(e, "abc");
    expect(read(e, $selectionOffsets)).toBeUndefined();
  });
});

/** The attributes of the text box that `props` draw, by name. */
function textbox(props: Partial<Parameters<typeof PlainTextEditor>[0]>): Record<string, string> {
  const html = renderToStaticMarkup(
    <PlainTextEditor
      namespace="test"
      value="a"
      onChange={() => undefined}
      label="Text"
      placeholder="Write here"
      look="composer"
      {...props}
    />,
  );
  const tag = /<div[^>]*\srole="textbox"[^>]*>/.exec(html)?.[0];
  if (tag === undefined) throw new Error(`no text box in ${html}`);
  return Object.fromEntries(Array.from(tag.matchAll(/\s([\w-]+)="([^"]*)"/g), (m) => [m[1] ?? "", m[2] ?? ""]));
}

describe("the text box", () => {
  test("has its name, its placeholder, its description and its states as given", () => {
    const box = textbox({ describedBy: "e1", invalid: true, required: true, multiline: true });
    expect(box).toMatchObject({
      role: "textbox",
      "aria-label": "Text",
      "aria-placeholder": "Write here",
      "aria-describedby": "e1",
      "aria-invalid": "true",
      "aria-required": "true",
      "aria-multiline": "true",
      contentEditable: "true",
      dir: "auto",
    });
  });

  test("says nothing of a state not set", () => {
    const box = textbox({ multiline: true });
    for (const name of ["aria-describedby", "aria-invalid", "aria-required", "aria-readonly"])
      expect(box[name] ?? "false").toBe("false");
  });

  test("of one line is not multiline", () => {
    expect(textbox({ multiline: false, look: "line", label: "Content warning" })).toMatchObject({
      "aria-label": "Content warning",
      "aria-multiline": "false",
    });
  });

  test("read only is not editable, and says so", () => {
    expect(textbox({ readOnly: true })).toMatchObject({ contentEditable: "false", "aria-readonly": "true" });
  });
});
