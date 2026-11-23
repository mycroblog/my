// SPDX-License-Identifier: AGPL-3.0-only

// The composer's text and content warning in the plain-text editor (ADR-0092 §3, §4): what the server will read in
// them is marked as it is typed — a mention, a hashtag and a URL as a token that never breaks inside its line, a custom
// emoji's `:shortcode:` shown as its image — while the value stays the plain text the server counts and reads: the
// image is read, counted and copied as its `:shortcode:`. The marks follow the tokenizer (text-tokens.ts) and are drawn
// again after each change, never while an input method composes: the browser keeps the text it composes, and the
// marks come at its end. The content warning shows its custom emoji alone (ADR-0092 §5). This module is a chunk of
// its own, with Lexical, loaded with the composer (composer.tsx).

import { useCustomEmojis, type PickerCustomEmoji } from "@my/app-data";
import { CustomEmoji } from "@my/design-system";
import {
  $selectionOffsets,
  $selectOffsets,
  PlainTextEditor,
  useEditor,
  type EditorHandle,
  type FieldCaret,
  type FieldSuggest,
} from "@my/design-system/editor";
import {
  $applyNodeReplacement,
  $createLineBreakNode,
  $createTextNode,
  $getRoot,
  $isElementNode,
  $isRootNode,
  addClassNamesToElement,
  DecoratorNode,
  HISTORY_MERGE_TAG,
  ParagraphNode,
  TextNode,
  type EditorConfig,
  type ElementNode,
  type Klass,
  type LexicalNode,
  type NodeKey,
  type SerializedLexicalNode,
  type Spread,
} from "lexical";
import { useEffect, useEffectEvent, useMemo, type ReactNode, type Ref } from "react";
import { textTokens, type TextToken } from "./text-tokens";

/** Gives a token's element the editor's `token` class: whole on its line, on the tint of the link it becomes. */
function token(dom: HTMLElement, config: EditorConfig): HTMLElement {
  const name: unknown = config.theme["token"];
  if (typeof name === "string") addClassNamesToElement(dom, name);
  return dom;
}

/** A mention as it is typed: `@user`, `@user@domain`. */
class MentionNode extends TextNode {
  override $config() {
    return this.config("my-mention", { extends: TextNode });
  }

  override createDOM(config: EditorConfig): HTMLElement {
    return token(super.createDOM(config), config);
  }
}

/** A hashtag as it is typed. */
class HashtagNode extends TextNode {
  override $config() {
    return this.config("my-hashtag", { extends: TextNode });
  }

  override createDOM(config: EditorConfig): HTMLElement {
    return token(super.createDOM(config), config);
  }
}

/** An http(s) URL as it is typed. */
class UrlNode extends TextNode {
  override $config() {
    return this.config("my-url", { extends: TextNode });
  }

  override createDOM(config: EditorConfig): HTMLElement {
    return token(super.createDOM(config), config);
  }
}

type SerializedCustomEmojiNode = Spread<{ shortcode: string; url: string; staticUrl: string }, SerializedLexicalNode>;

/** A custom emoji of the server, shown as its image while it is written, and read and copied as its `:shortcode:`. */
class CustomEmojiNode extends DecoratorNode<ReactNode> {
  __shortcode: string;
  __url: string;
  __staticUrl: string;

  static override getType(): string {
    return "my-custom-emoji";
  }

  static override clone(node: CustomEmojiNode): CustomEmojiNode {
    return new CustomEmojiNode(node.__shortcode, node.__url, node.__staticUrl, node.__key);
  }

  static override importJSON(json: SerializedCustomEmojiNode): CustomEmojiNode {
    return new CustomEmojiNode(json.shortcode, json.url, json.staticUrl);
  }

  constructor(shortcode: string, url: string, staticUrl: string, key?: NodeKey) {
    super(key);
    this.__shortcode = shortcode;
    this.__url = url;
    this.__staticUrl = staticUrl;
  }

  override exportJSON(): SerializedCustomEmojiNode {
    return { ...super.exportJSON(), shortcode: this.__shortcode, url: this.__url, staticUrl: this.__staticUrl };
  }

  override createDOM(): HTMLElement {
    return document.createElement("span");
  }

  override updateDOM(): false {
    return false;
  }

  override isInline(): true {
    return true;
  }

  /**
   * The caret steps over it as over a character, never selecting it as a node: a node selected has no caret, and
   * what is typed then would land at the start of the text.
   */
  override isKeyboardSelectable(): false {
    return false;
  }

  override getTextContent(): string {
    return `:${this.__shortcode}:`;
  }

  override decorate(): ReactNode {
    return <CustomEmoji shortcode={this.__shortcode} url={this.__url} staticUrl={this.__staticUrl} />;
  }
}

/** What an editor marks: the kinds of token, and the custom emoji of the server by shortcode. */
type Marks = {
  readonly kinds: readonly TextToken["kind"][];
  readonly custom: ReadonlyMap<string, PickerCustomEmoji>;
};

/** A part of a paragraph: plain text (line breaks in it), a token, or a custom emoji. */
type Segment = { kind: "plain" | "mention" | "hashtag" | "url" | "emoji"; text: string };

/** The segments, the plain ones next to each other joined and the empty ones left out. */
function joined(segments: readonly Segment[]): Segment[] {
  const out: Segment[] = [];
  for (const s of segments) {
    const last = out.at(-1);
    if (s.kind === "plain" && s.text === "") continue;
    if (s.kind === "plain" && last?.kind === "plain") out[out.length - 1] = { kind: "plain", text: last.text + s.text };
    else out.push(s);
  }
  return out;
}

/** The segments a paragraph holds now. */
function $segmentsOf(nodes: readonly LexicalNode[]): Segment[] {
  return joined(
    nodes.map((node) => {
      const text = node.getTextContent();
      if (node instanceof MentionNode) return { kind: "mention", text };
      if (node instanceof HashtagNode) return { kind: "hashtag", text };
      if (node instanceof UrlNode) return { kind: "url", text };
      if (node instanceof CustomEmojiNode) return { kind: "emoji", text };
      return { kind: "plain", text };
    }),
  );
}

/** The segments `text` should be in, by its tokens. */
function wanted(text: string, marks: Marks): Segment[] {
  const out: Segment[] = [];
  let at = 0;
  for (const t of textTokens(text)) {
    if (!marks.kinds.includes(t.kind)) continue;
    out.push({ kind: "plain", text: text.slice(at, t.start) });
    const kind = t.kind === "shortcode" ? (marks.custom.has(t.text.slice(1, -1)) ? "emoji" : "plain") : t.kind;
    out.push({ kind, text: t.text });
    at = t.end;
  }
  out.push({ kind: "plain", text: text.slice(at) });
  return joined(out);
}

/** The nodes of a segment. */
function $nodesOf(segment: Segment, marks: Marks): LexicalNode[] {
  switch (segment.kind) {
    case "mention":
      return [$applyNodeReplacement(new MentionNode(segment.text))];
    case "hashtag":
      return [$applyNodeReplacement(new HashtagNode(segment.text))];
    case "url":
      return [$applyNodeReplacement(new UrlNode(segment.text))];
    case "emoji": {
      const emoji = marks.custom.get(segment.text.slice(1, -1));
      if (emoji !== undefined)
        return [$applyNodeReplacement(new CustomEmojiNode(emoji.shortcode, emoji.url, emoji.static_url))];
      return [$createTextNode(segment.text)];
    }
    case "plain":
      return segment.text
        .split("\n")
        .flatMap((line, i) => [
          ...(i > 0 ? [$createLineBreakNode()] : []),
          ...(line === "" ? [] : [$createTextNode(line)]),
        ]);
  }
}

const same = (a: readonly Segment[], b: readonly Segment[]) => JSON.stringify(a) === JSON.stringify(b);

/** Draws a paragraph's marks again where its tokens changed, the selection kept where it was in the text. */
function $mark(paragraph: ElementNode, marks: Marks): void {
  const nodes = paragraph.getChildren();
  const want = wanted(nodes.map((n) => n.getTextContent()).join(""), marks);
  if (same($segmentsOf(nodes), want)) return;
  const selection = $selectionOffsets();
  paragraph.clear();
  paragraph.append(...want.flatMap((s) => $nodesOf(s, marks)));
  if (selection !== undefined) $selectOffsets(selection.start, selection.end);
}

/** The nodes an editor of `kinds` holds. */
const nodesFor = (kinds: readonly TextToken["kind"][]): Klass<LexicalNode>[] => [
  ...(kinds.includes("mention") ? [MentionNode] : []),
  ...(kinds.includes("hashtag") ? [HashtagNode] : []),
  ...(kinds.includes("url") ? [UrlNode] : []),
  ...(kinds.includes("shortcode") ? [CustomEmojiNode] : []),
];

/** Marks the editor's text after each change but while an input method composes, and again when the emoji change. */
function MarksPlugin({ marks }: { marks: Marks }) {
  const editor = useEditor();
  const mark = useEffectEvent((paragraph: ElementNode) => {
    $mark(paragraph, marks);
  });
  const { kinds } = marks;
  useEffect(() => {
    const run = (node: LexicalNode) => {
      if (editor.isComposing()) return;
      const paragraph = $isElementNode(node) ? node : node.getParent();
      if (paragraph !== null && !$isRootNode(paragraph)) mark(paragraph);
    };
    const klasses: Klass<LexicalNode>[] = [TextNode, ParagraphNode, ...nodesFor(kinds)];
    const stops = klasses.map((klass) => editor.registerNodeTransform(klass, run));
    return () => {
      for (const stop of stops) stop();
    };
  }, [editor, kinds]);
  useEffect(() => {
    editor.update(
      () => {
        for (const paragraph of $getRoot().getChildren())
          if ($isElementNode(paragraph) && !editor.isComposing()) $mark(paragraph, marks);
      },
      // Drawn again by itself, not by the writer: no step of its own to undo.
      { tag: HISTORY_MERGE_TAG },
    );
  }, [editor, marks]);
  return null;
}

/** The marks of `kinds`, with the custom emoji the server has. */
function useMarks(kinds: readonly TextToken["kind"][]): Marks {
  const emojis = useCustomEmojis();
  return useMemo(() => ({ kinds, custom: new Map(emojis.map((e) => [e.shortcode, e])) }), [emojis, kinds]);
}

const textKinds: readonly TextToken["kind"][] = ["mention", "hashtag", "url", "shortcode"];
const textNodes = nodesFor(textKinds);
const warningKinds: readonly TextToken["kind"][] = ["shortcode"];
const warningNodes = nodesFor(warningKinds);

export type PostTextProps = {
  value: string;
  onChange: (value: string) => void;
  label: string;
  placeholder: string;
  /** How many characters at its end are over the limit, marked. */
  over: number;
  /** The ID of what is wrong with it. */
  describedBy?: string | undefined;
  invalid: boolean;
  readOnly: boolean;
  roomy: boolean;
  autoFocus: boolean;
  /**
   * Its chunk had not come when the composer opened: drawn later, it takes the focus only where the reader has not
   * moved it since.
   */
  late?: boolean | undefined;
  /** The emoji button sits at the end of its first line. */
  ended: boolean;
  /** The emoji button and the suggestions write at its selection. */
  handle: Ref<EditorHandle>;
  /** The suggestions for the word at its caret, while they are open (suggest.tsx). */
  suggest: FieldSuggest | undefined;
  /** Ctrl+Enter or ⌘+Enter: what is written is sent, as its form's action does. */
  onSubmit?: (() => void) | undefined;
  /** Files pasted in (an image copied), to attach. */
  onFiles?: ((files: File[]) => void) | undefined;
} & FieldCaret;

/**
 * The text of a post, its mentions, hashtags and URLs marked and its custom emoji shown as their images; it completes
 * accounts, hashtags and emoji.
 */
export function PostText(props: PostTextProps) {
  const marks = useMarks(textKinds);
  return (
    <PlainTextEditor namespace="composer-text" look="composer" nodes={textNodes} completes {...props}>
      <MarksPlugin marks={marks} />
    </PlainTextEditor>
  );
}

export type WarningTextProps = {
  value: string;
  onChange: (value: string) => void;
  label: string;
  placeholder: string;
  readOnly: boolean;
  /** The ID of what is wrong with it. */
  describedBy?: string | undefined;
  invalid?: boolean | undefined;
  /** It takes the focus once drawn, its caret at the end (the writer added it). */
  autoFocus: boolean;
  /** The suggestions write at its selection. */
  handle: Ref<EditorHandle>;
  /** The suggestions for the emoji at its caret, while they are open (suggest.tsx). */
  suggest: FieldSuggest | undefined;
  /** Ctrl+Enter or ⌘+Enter: what is written is sent, as its form's action does. */
  onSubmit?: (() => void) | undefined;
} & FieldCaret;

/** The content warning: one line, its custom emoji shown as their images; it completes emoji. */
export function WarningText(props: WarningTextProps) {
  const marks = useMarks(warningKinds);
  return (
    <PlainTextEditor
      namespace="composer-warning"
      look="line"
      multiline={false}
      nodes={warningNodes}
      completes
      {...props}
    >
      <MarksPlugin marks={marks} />
    </PlainTextEditor>
  );
}
