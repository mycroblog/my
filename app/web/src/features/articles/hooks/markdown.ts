// SPDX-License-Identifier: AGPL-3.0-only
// What the editor's buttons and the dropped images do to the Markdown of the body (ADR-0160 §4): each takes the text
// and the selection of the text area and returns the new text with the selection to set. The server renders the body;
// these only write the Markdown it reads.

/** The text of the body and the selection to set in the text area after a change. */
export type TextEdit = { value: string; selectionStart: number; selectionEnd: number };

const strong = "**";

/**
 * Makes the selection bold (`**text**`), or plain again when it is bold already. With nothing selected it writes the
 * marks with the caret between them, so that what is typed next is bold.
 */
export function toggleBold(value: string, start: number, end: number): TextEdit {
  const before = value.slice(0, start);
  const after = value.slice(end);
  if (before.endsWith(strong) && after.startsWith(strong)) {
    return {
      value: before.slice(0, -strong.length) + value.slice(start, end) + after.slice(strong.length),
      selectionStart: start - strong.length,
      selectionEnd: end - strong.length,
    };
  }
  return {
    value: `${before}${strong}${value.slice(start, end)}${strong}${after}`,
    selectionStart: start + strong.length,
    selectionEnd: end + strong.length,
  };
}

/** The address a new link starts with, selected so that typing or pasting replaces it. */
const linkPlaceholder = "https://";

/**
 * Makes the selection the text of a link (`[text](https://)`) and selects the address to type. With nothing selected
 * the caret goes between the brackets, for the link's text.
 */
export function insertLink(value: string, start: number, end: number): TextEdit {
  const label = value.slice(start, end);
  const head = `${value.slice(0, start)}[${label}](`;
  const next = `${head}${linkPlaceholder})${value.slice(end)}`;
  if (label === "") return { value: next, selectionStart: start + 1, selectionEnd: start + 1 };
  return { value: next, selectionStart: head.length, selectionEnd: head.length + linkPlaceholder.length };
}

/** The Markdown of an image of the author's own upload `id`: the server shows it once it is processed. */
function imageMarkdown(id: string): string {
  return `![](media:${id})`;
}

/**
 * Puts the images of the uploads `ids` in place of the selection, each a paragraph of its own (a blank line before
 * and after it, unless the text starts or ends there), and puts the caret after them.
 */
export function insertImages(value: string, start: number, end: number, ids: readonly string[]): TextEdit {
  const head = value.slice(0, start);
  const tail = value.slice(end);
  const lead = head === "" || head.endsWith("\n\n") ? "" : head.endsWith("\n") ? "\n" : "\n\n";
  const trail = tail === "" || tail.startsWith("\n\n") ? "" : tail.startsWith("\n") ? "\n" : "\n\n";
  const images = ids.map(imageMarkdown).join("\n\n");
  const caret = head.length + lead.length + images.length + trail.length;
  return { value: `${head}${lead}${images}${trail}${tail}`, selectionStart: caret, selectionEnd: caret };
}

/** The most hashtags an article takes (ArticleTagsInput). */
export const maxTags = 10;

/**
 * The hashtags typed in the hashtags field: separated by spaces or commas, with or without `#`, each once (compared
 * as the server compares them, under NFKC and lowercase), in the order typed.
 */
export function parseTags(input: string): string[] {
  const seen = new Set<string>();
  const tags: string[] = [];
  for (const word of input.split(/[\s,，、]+/u)) {
    const tag = word.replace(/^[#＃]+/u, "");
    if (tag === "") continue;
    const key = tag.normalize("NFKC").toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    tags.push(tag);
  }
  return tags;
}

/** The hashtags field's text of saved hashtags. */
export function tagsText(tags: readonly string[]): string {
  return tags.map((tag) => `#${tag.replace(/^#/u, "")}`).join(" ");
}
