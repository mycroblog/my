// SPDX-License-Identifier: AGPL-3.0-only

// The one renderer of what people write (ADR-0025 §4): the body of every post, remote and local alike, since the
// server renders both as the same HTML (ADR-0019 §2): a local post from its plain text (internal/app/content, text.go),
// a remote one through its sanitizer. It draws that HTML as React elements (html.ts) with the parts of
// @my/design-system: paragraphs and line breaks, links, mentions and hashtags, the shortened text of a URL, and each
// `:shortcode:` of the post's custom emoji as its image (ADR-0051 §4). Each block reads in its own direction
// (`dir="auto"`), so that a post keeps its punctuation at the right end whatever the page's direction.

import { ContentBody, ContentLink, CustomEmoji, UrlPart, type ContentLinkKind } from "@my/design-system";
import { createElement, Fragment, type ReactNode } from "react";
import { blockTags, parseContent, type ContentElement, type ContentNode } from "./html";

/** A custom emoji of the text, as the API gives it (CustomEmoji). */
export type ContentEmoji = { shortcode: string; url: string; static_url: string };

export type PostContentProps = {
  /** The text as the server rendered and sanitized it: a status's `content`, local or remote. */
  html: string;
  /** The custom emoji of the text (the status's `emojis`): only their shortcodes draw as images. */
  emojis?: readonly ContentEmoji[] | undefined;
  /** The language of the text, when it is known (a BCP 47 tag). */
  lang?: string | undefined;
};

// The blocks that read in their own direction.
const directed: ReadonlySet<string> = new Set(["p", "pre", "blockquote", "li"]);

const shortcode = /:([a-zA-Z0-9_]+):/g;

/** Text with each `:shortcode:` of one of `emojis` as its image. */
function emojiText(text: string, emojis: ReadonlyMap<string, ContentEmoji>): ReactNode {
  if (emojis.size === 0) return text;
  const out: ReactNode[] = [];
  let at = 0;
  for (const match of text.matchAll(shortcode)) {
    const emoji = emojis.get(match[1] ?? "");
    if (emoji === undefined) continue;
    if (match.index > at) out.push(text.slice(at, match.index));
    out.push(
      <CustomEmoji key={match.index} shortcode={emoji.shortcode} url={emoji.url} staticUrl={emoji.static_url} />,
    );
    at = match.index + match[0].length;
  }
  if (at === 0) return text;
  if (at < text.length) out.push(text.slice(at));
  return out;
}

// The places where code may break before it breaks inside a word: after a slash (a run of them, as in "https://") or
// a question mark, and before a pipe.
const codeBreak = /(?<=[/?])(?=[^/])|(?<=.)(?=\|)/g;

/** Code as its author typed it, with a break opportunity (<wbr>) at each of its places, which copying leaves out. */
function codeText(text: string): ReactNode {
  const parts = text.split(codeBreak);
  if (parts.length === 1) return text;
  return parts.flatMap((part, i) => (i === 0 ? [part] : [<wbr key={i} />, part]));
}

const classes = (element: ContentElement) => new Set(element.attrs.class?.split(/\s+/));

/** What a link is, by the classes the server gives it: Mastodon's `mention hashtag` for a hashtag. */
function linkKind(element: ContentElement): ContentLinkKind {
  const of = classes(element);
  return of.has("hashtag") ? "hashtag" : of.has("mention") ? "mention" : "link";
}

/** One part as React elements. Code shows what its author typed, colons included. */
function draw(node: ContentNode, key: number, emojis: ReadonlyMap<string, ContentEmoji>, code: boolean): ReactNode {
  if (typeof node === "string") return <Fragment key={key}>{code ? codeText(node) : emojiText(node, emojis)}</Fragment>;
  const inCode = code || node.tag === "code" || node.tag === "pre";
  const children = node.children.map((child, i) => draw(child, i, emojis, inCode));
  const { lang, href, translate } = node.attrs;
  switch (node.tag) {
    case "br":
      return <br key={key} />;
    case "a":
      return (
        <ContentLink key={key} href={href ?? ""} kind={linkKind(node)} lang={lang} translate={translate}>
          {children}
        </ContentLink>
      );
    case "span": {
      const of = classes(node);
      const part = of.has("invisible") ? "hidden" : of.has("ellipsis") ? "cut" : undefined;
      if (part !== undefined)
        return (
          <UrlPart key={key} part={part}>
            {children}
          </UrlPart>
        );
      return (
        <span key={key} lang={lang} translate={translate}>
          {children}
        </span>
      );
    }
    case "ol":
      return (
        <ol
          key={key}
          lang={lang}
          start={node.attrs.start === undefined ? undefined : Number(node.attrs.start)}
          reversed={node.attrs.reversed === undefined ? undefined : true}
        >
          {children}
        </ol>
      );
    case "li":
      return (
        <li
          key={key}
          lang={lang}
          dir="auto"
          value={node.attrs.value === undefined ? undefined : Number(node.attrs.value)}
        >
          {children}
        </li>
      );
    default:
      return createElement(node.tag, { key, lang, dir: directed.has(node.tag) ? "auto" : undefined }, ...children);
  }
}

/** The text's blocks: a run of text or inline elements outside a block is a paragraph of its own. */
function blocksOf(nodes: ContentNode[]): ContentNode[] {
  const out: ContentNode[] = [];
  let run: ContentNode[] = [];
  const flush = () => {
    if (run.some((node) => typeof node !== "string" || node.trim() !== ""))
      out.push({ tag: "p", attrs: {}, children: run });
    run = [];
  };
  for (const node of nodes) {
    if (typeof node !== "string" && blockTags.has(node.tag)) {
      flush();
      out.push(node);
    } else run.push(node);
  }
  flush();
  return out;
}

/** The body of a post, from the HTML the server rendered and sanitized, with its custom emoji. */
export function PostContent({ html, emojis = [], lang }: PostContentProps) {
  const byCode = new Map(emojis.map((emoji) => [emoji.shortcode, emoji]));
  return (
    <ContentBody lang={lang}>{blocksOf(parseContent(html)).map((node, i) => draw(node, i, byCode, false))}</ContentBody>
  );
}
