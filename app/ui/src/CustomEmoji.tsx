// SPDX-License-Identifier: AGPL-3.0-only
// Custom emoji in user content (ADR-0051 §4). A text has an emoji as `:shortcode:`; the API lists the
// emoji of each text with the URLs of its image. The image goes in as a DOM element, never as HTML
// (ADR-0019 §2): its alternative text is the `:shortcode:` it replaces, so a screen reader reads what the
// author wrote. An animated emoji moves only when the user allows motion: the still image is the `<img>`,
// and the animated one a `<source>` for `(prefers-reduced-motion: no-preference)` (ADR-0054 §3).
import { tv } from "tailwind-variants";
import { useAnimationsPlay } from "./Picture";

/**
 * A custom emoji that a text may use: its shortcode without colons, its image, and a still image of it. The
 * names are those of the client API's `CustomEmoji`, so that its `emojis` pass as they are.
 */
export type CustomEmojiImage = {
  shortcode: string;
  url: string;
  static_url: string;
};

/** One part of a text: a run of plain text, or an emoji in place of its `:shortcode:`. `start` is where it begins in the text. */
export type CustomEmojiSegment = { start: number; text: string } | { start: number; emoji: CustomEmojiImage };

/** A shortcode between colons: what may be an emoji. Only shortcodes of the given emoji become images. */
const shortcodePattern = /:([^\s:]+):/gu;

/**
 * Splits `text` into runs of text and the emoji of `emojis` that it names as `:shortcode:`. A shortcode
 * that is not among `emojis` stays text, and its closing colon may open the next one (`:a::b:` and
 * `:x:b:` both find `:b:`).
 */
export function splitCustomEmoji(text: string, emojis: readonly CustomEmojiImage[]): CustomEmojiSegment[] {
  if (emojis.length === 0 || !text.includes(":")) return [{ start: 0, text }];
  const byShortcode = new Map(emojis.map((emoji) => [emoji.shortcode, emoji]));
  const out: CustomEmojiSegment[] = [];
  const pattern = new RegExp(shortcodePattern);
  let last = 0;
  for (let match = pattern.exec(text); match !== null; match = pattern.exec(text)) {
    const emoji = byShortcode.get(match[1] ?? "");
    const end = match.index + match[0].length;
    if (emoji === undefined) {
      // The closing colon may open the next shortcode.
      pattern.lastIndex = end - 1;
      continue;
    }
    if (match.index > last) out.push({ start: last, text: text.slice(last, match.index) });
    out.push({ start: match.index, emoji });
    last = end;
  }
  if (last < text.length || out.length === 0) out.push({ start: last, text: text.slice(last) });
  return out;
}

// As high as the text around it, on its baseline's middle; Tailwind's base makes images blocks, and an
// emoji is a word of the line.
const customEmoji = tv({ base: "inline-block h-[1.25em] w-auto max-w-none align-middle" });

/** The name an emoji is read and shown by: the `:shortcode:` it stands for. */
function labelOf(emoji: CustomEmojiImage): string {
  return `:${emoji.shortcode}:`;
}

/** Whether an emoji has an animated image apart from its still one, and it may move. */
function animates(emoji: CustomEmojiImage, allowMotion: boolean): boolean {
  return allowMotion && emoji.url !== emoji.static_url;
}

export type CustomEmojiProps = {
  emoji: CustomEmojiImage;
  /**
   * Whether an animated emoji may move when the user allows motion; false shows the still image only. The default
   * is the app's `AutoplayContext`, or the post or profile it is in while that plays (`AnimationsPlayContext`).
   */
  allowMotion?: boolean | undefined;
};

/** One custom emoji: its image, read as its `:shortcode:`. */
export function CustomEmoji({ emoji, allowMotion: allowed }: CustomEmojiProps) {
  const plays = useAnimationsPlay();
  const allowMotion = allowed ?? plays;
  const label = labelOf(emoji);
  const image = (
    <img
      src={emoji.static_url}
      alt={label}
      title={label}
      draggable={false}
      decoding="async"
      className={customEmoji()}
    />
  );
  if (!animates(emoji, allowMotion)) return image;
  return (
    <picture>
      <source media="(prefers-reduced-motion: no-preference)" srcSet={emoji.url} />
      {image}
    </picture>
  );
}

/** No custom emoji: the default of the components, one array so that it is the same on every render. */
export const noCustomEmojis: readonly CustomEmojiImage[] = [];

export type EmojiTextProps = {
  /** Plain text, such as a display name or a content warning. */
  text: string;
  /** The custom emoji the text may use; a `:shortcode:` of any other stays text. */
  emojis?: readonly CustomEmojiImage[] | undefined;
  allowMotion?: boolean | undefined;
};

/** Plain text with its custom emoji drawn as images. It renders no element of its own: put it where the text goes. */
export function EmojiText({ text, emojis = noCustomEmojis, allowMotion }: EmojiTextProps) {
  return (
    <>
      {splitCustomEmoji(text, emojis).map((segment) =>
        "emoji" in segment ? (
          <CustomEmoji key={segment.start} emoji={segment.emoji} allowMotion={allowMotion} />
        ) : (
          segment.text
        ),
      )}
    </>
  );
}

/** Builds the DOM of one emoji, as CustomEmoji renders it. */
function emojiElement(document: Document, emoji: CustomEmojiImage, allowMotion: boolean): HTMLElement {
  const image = document.createElement("img");
  image.src = emoji.static_url;
  image.alt = labelOf(emoji);
  image.title = labelOf(emoji);
  image.draggable = false;
  image.decoding = "async";
  image.className = customEmoji();
  if (!animates(emoji, allowMotion)) return image;
  const picture = document.createElement("picture");
  const source = document.createElement("source");
  source.media = "(prefers-reduced-motion: no-preference)";
  source.srcset = emoji.url;
  picture.append(source, image);
  return picture;
}

/**
 * Draws the custom emoji of `emojis` in the text nodes under `root`, which hold HTML that React does not
 * render (the sanitized body of PostContent), and returns the function that puts the `:shortcode:` text
 * back. The elements are made with the DOM API, never from HTML.
 */
export function insertCustomEmojis(root: Element, emojis: readonly CustomEmojiImage[], allowMotion = true): () => void {
  const document = root.ownerDocument;
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  const texts: Text[] = [];
  for (let node = walker.nextNode(); node !== null; node = walker.nextNode()) {
    // Code shows what the author typed, colons included.
    if (node instanceof Text && node.data.includes(":") && node.parentElement?.closest("code, pre") == null)
      texts.push(node);
  }
  const inserted: HTMLElement[] = [];
  for (const node of texts) {
    const segments = splitCustomEmoji(node.data, emojis);
    if (segments.every((segment) => "text" in segment)) continue;
    const parts = segments.map((segment) => {
      if ("text" in segment) return document.createTextNode(segment.text);
      const element = emojiElement(document, segment.emoji, allowMotion);
      inserted.push(element);
      return element;
    });
    node.replaceWith(...parts);
  }
  return () => {
    for (const element of inserted) {
      const label = element instanceof HTMLImageElement ? element.alt : (element.querySelector("img")?.alt ?? "");
      element.replaceWith(document.createTextNode(label));
    }
    root.normalize();
  };
}
