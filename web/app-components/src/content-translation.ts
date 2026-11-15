// SPDX-License-Identifier: AGPL-3.0-only

// A post's text translated by a translator that takes plain text (the browser's, ADR-0148 §12), its structure kept:
// the same fragments the server sends its providers (§8, internal/app/translation/fragment.go). The text the server
// rendered and sanitized is read as data (html.ts) and split at its blocks; in each run of inline content, whatever
// must not be translated (a mention, a hashtag, a link, code, a `:shortcode:`, a URL or a handle in plain text, and
// text that looks like a marker) becomes a numbered marker, `⟦1⟧`, and decoration (bold, a span) a pair of them,
// `⟦2⟧…⟦/2⟧`, around the words it covers. A translation must bring each marker back exactly once, in order of
// nesting; one that does not is translated again piece by piece, the text between the markers alone, with the
// markers put back here. The translator's output is only ever text: it goes back into the tree as text nodes and is
// written out escaped, so markup in it shows as characters and is never parsed (§12). What comes back is HTML of the
// same elements and attributes as the server's, which PostContent draws as it draws any post.

import { blockTags, parseContent, type ContentElement, type ContentNode } from "./html";

/** Translates one fragment of plain text. */
export type TranslateText = (text: string) => Promise<string>;

const markerPattern = /⟦\/?[1-9][0-9]*⟧/gu;

/** What a marker stands for: nodes kept as they are, or the opening or the closing of an element around words. */
type Held = { kind: "item"; nodes: ContentNode[] } | { kind: "open"; element: ContentElement } | { kind: "close" };

/** One fragment: its text with markers, and what each marker stands for. */
type Fragment = { text: string; held: Map<string, Held> };

// A URL or a handle in plain text, which a remote post or a content warning may hold without a link around it.
const plainHeld = /https?:\/\/[^\s]+|@[\w.-]+(?:@[\w.-]+\.[\w-]+)?|:[a-zA-Z0-9_]+:/gu;

/** Builds a fragment, numbering its markers from 1. */
class Masker {
  private out = "";
  private last = 0;
  readonly held = new Map<string, Held>();

  item(nodes: ContentNode[]) {
    this.last += 1;
    const marker = `⟦${String(this.last)}⟧`;
    this.held.set(marker, { kind: "item", nodes });
    this.out += marker;
  }

  /** Writes the opening marker of `element`'s pair, and returns what writes the closing one. */
  open(element: ContentElement): () => void {
    this.last += 1;
    const n = String(this.last);
    this.held.set(`⟦${n}⟧`, { kind: "open", element });
    this.held.set(`⟦/${n}⟧`, { kind: "close" });
    this.out += `⟦${n}⟧`;
    return () => {
      this.out += `⟦/${n}⟧`;
    };
  }

  /** Writes text, with what must not be translated in it held. */
  text(text: string) {
    const all = [...text.matchAll(markerPattern), ...text.matchAll(plainHeld)].toSorted((a, b) => a.index - b.index);
    let at = 0;
    for (const m of all) {
      if (m.index < at) continue;
      this.out += text.slice(at, m.index);
      this.item([m[0]]);
      at = m.index + m[0].length;
    }
    this.out += text.slice(at);
  }

  finish(): Fragment {
    return { text: this.out, held: this.held };
  }
}

/** Whether an element is decoration around words to translate, rather than one item kept as it is. */
function hasWords(node: ContentNode): boolean {
  if (typeof node === "string") return node.trim() !== "";
  if (["a", "code", "ruby", "br"].includes(node.tag) || blockTags.has(node.tag)) return false;
  return node.children.some(hasWords);
}

function inline(m: Masker, node: ContentNode) {
  if (typeof node === "string") {
    m.text(node);
    return;
  }
  if (!hasWords(node)) {
    m.item([node]);
    return;
  }
  const close = m.open(node);
  for (const child of node.children) inline(m, child);
  close();
}

/** Whether a fragment has words to translate: letters outside its markers. */
const sendable = (text: string) => /\p{L}/u.test(text.replace(markerPattern, ""));

/**
 * The nodes of a fragment's translation, each marker back as what it stands for; undefined when a marker is missing,
 * repeated, unknown, or a pair does not nest.
 */
function restore(translated: string, held: Map<string, Held>): ContentNode[] | undefined {
  const root: ContentNode[] = [];
  const stack: { marker: string; children: ContentNode[] }[] = [];
  const into = () => stack.at(-1)?.children ?? root;
  const seen = new Set<string>();
  let at = 0;
  for (const m of translated.matchAll(markerPattern)) {
    const marker = m[0];
    const what = held.get(marker);
    if (what === undefined || seen.has(marker)) return undefined;
    seen.add(marker);
    if (m.index > at) into().push(translated.slice(at, m.index));
    at = m.index + marker.length;
    if (what.kind === "item") into().push(...what.nodes);
    else if (what.kind === "open") {
      const element: ContentElement = { ...what.element, children: [] };
      into().push(element);
      stack.push({ marker: marker.replace("⟦", "⟦/"), children: element.children });
    } else if (stack.pop()?.marker !== marker) return undefined;
  }
  if (at < translated.length) into().push(translated.slice(at));
  return seen.size === held.size && stack.length === 0 ? root : undefined;
}

/** A fragment translated: whole, else piece by piece between its markers, which go back as they were. */
async function translateFragment(fragment: Fragment, translate: TranslateText): Promise<ContentNode[]> {
  if (!sendable(fragment.text)) return restore(fragment.text, fragment.held) ?? [];
  const whole = restore(await translate(fragment.text), fragment.held);
  if (whole !== undefined) return whole;
  let rebuilt = "";
  let at = 0;
  const piece = async (text: string) => {
    const core = text.trim();
    if (!sendable(core)) return text;
    const start = text.indexOf(core);
    // A translation with marker-like text in it is text: its brackets are kept apart from the markers.
    const done = (await translate(core)).replace(/[⟦⟧]/gu, (c) => (c === "⟦" ? "[" : "]"));
    return text.slice(0, start) + done + text.slice(start + core.length);
  };
  for (const m of fragment.text.matchAll(markerPattern)) {
    rebuilt += (await piece(fragment.text.slice(at, m.index))) + m[0];
    at = m.index + m[0].length;
  }
  rebuilt += await piece(fragment.text.slice(at));
  const pieces = restore(rebuilt, fragment.held);
  if (pieces === undefined) throw new Error("app-components: the translation lost the post's structure");
  return pieces;
}

/** The translated children of a block, or of the top level: each run of inline nodes one fragment. */
async function translateNodes(nodes: readonly ContentNode[], translate: TranslateText): Promise<ContentNode[]> {
  const out: ContentNode[] = [];
  let run: ContentNode[] = [];
  const flush = async () => {
    if (run.length === 0) return;
    const m = new Masker();
    for (const node of run) inline(m, node);
    run = [];
    out.push(...(await translateFragment(m.finish(), translate)));
  };
  for (const node of nodes) {
    if (typeof node === "string" || !blockTags.has(node.tag)) {
      run.push(node);
      continue;
    }
    await flush();
    out.push(node.tag === "pre" ? node : { ...node, children: await translateNodes(node.children, translate) });
  }
  await flush();
  return out;
}

const escapeText = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
const escapeAttr = (s: string) => escapeText(s).replace(/"/g, "&quot;");

/** The HTML of nodes, as the server writes it: text escaped, the attributes the parser kept. */
function html(nodes: readonly ContentNode[]): string {
  return nodes
    .map((node) => {
      if (typeof node === "string") return escapeText(node);
      if (node.tag === "br") return "<br>";
      const attrs = Object.entries(node.attrs)
        .map(([name, value]) => (value === "" ? ` ${name}` : ` ${name}="${escapeAttr(value)}"`))
        .join("");
      const inside = html(node.children);
      // The parser drops the first line break after <pre>, so one more goes there.
      const lead = node.tag === "pre" && inside.startsWith("\n") ? "\n" : "";
      return `<${node.tag}${attrs}>${lead}${inside}</${node.tag}>`;
    })
    .join("");
}

/**
 * The text of a post (its `content`, as the server rendered and sanitized it) translated by `translate`, fragment by
 * fragment, as HTML of the same structure; rejects when `translate` does, or when a fragment's structure cannot be
 * kept.
 */
export async function translateContent(content: string, translate: TranslateText): Promise<string> {
  return html(await translateNodes(parseContent(content), translate));
}

/** Plain text of a post (its content warning, a poll's option, an image's alt text) translated by `translate`. */
export async function translatePlain(text: string, translate: TranslateText): Promise<string> {
  const m = new Masker();
  m.text(text);
  const nodes = await translateFragment(m.finish(), translate);
  return nodes.map((n) => (typeof n === "string" ? n : "")).join("");
}
