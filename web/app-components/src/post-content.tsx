// SPDX-License-Identifier: AGPL-3.0-only

// The one renderer of what people write (ADR-0025 §4): the body of every post, remote and local alike, since the
// server renders both as the same HTML (ADR-0019 §2): a local post from its plain text (internal/app/content, text.go),
// a remote one through its sanitizer. It draws that HTML as React elements (html.ts) with the parts of
// @my/design-system: paragraphs and line breaks, links, mentions and hashtags, the shortened text of a URL, and each
// `:shortcode:` of the post's custom emoji as its image (ADR-0051 §4). Every block reads in its own direction
// (`dir="auto"`), so that a post keeps its punctuation at the right end whatever the page's direction, and a list
// puts its indent on the side its markers stand, where a list in the page's direction would leave them hanging out.
// A hashtag of this server (its page on the serving domain that ServerDomain gives, or on the page's own host, which
// serves the app and holds before the instance is read) opens the app's hashtag screen in place; every other link
// opens in a new tab. An article's text (ADR-0160 §4) is drawn the same way with the article's allowlist, as the blocks
// of ArticleText: its headings, rules and images, and its tables each in an ArticleTable.

import { ArticleTable, ContentBody, ContentLink, CustomEmoji, UrlPart, type ContentLinkKind } from "@my/design-system";
import { createContext, createElement, Fragment, use, type ReactNode } from "react";
import { articleBlockTags, blockTags, parseContent, type ContentElement, type ContentNode } from "./html";
import { hashtagPath } from "./paths";

/**
 * The serving domain of this server (the instance's `domain`), which the app places around its screens: a hashtag
 * link to its page there opens the app's hashtag screen, as one to its page on the page's own host does.
 */
export const ServerDomain = createContext<string | undefined>(undefined);

/**
 * What drawing a text needs besides its nodes: its custom emoji by shortcode, and the hosts whose hashtag pages are
 * the app's (the server's domain, and the page's own host).
 */
type Drawing = { readonly emojis: ReadonlyMap<string, ContentEmoji>; readonly hosts: readonly (string | undefined)[] };

/** The host of the page the app runs on; undefined outside a browser. */
const pageHost = (): string | undefined => (typeof location === "undefined" ? undefined : location.host);

/** A custom emoji of the text, as the API gives it (CustomEmoji). */
export type ContentEmoji = { shortcode: string; url: string; static_url: string };

export type PostContentProps = {
  /** The text as the server rendered and sanitized it: a status's `content`, local or remote. */
  html: string;
  /** The custom emoji of the text (the status's `emojis`): only their shortcodes draw as images. */
  emojis?: readonly ContentEmoji[] | undefined;
  /** The language of the text, when it is known (a BCP 47 tag). */
  lang?: string | undefined;
  /**
   * The text is an article's (ADR-0160 §4): read with the article's allowlist (headings, rules, images, tables), and
   * drawn as the blocks that ArticleText of @my/design-system holds, with no body of a post around them (`lang` is
   * then the holder's).
   */
  article?: boolean | undefined;
};

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

/** Plain text people wrote outside a post's body (a display name, a content warning), with its custom emoji. */
export function EmojiText({ text, emojis = [] }: { text: string; emojis?: readonly ContentEmoji[] | undefined }) {
  return emojiText(text, new Map(emojis.map((emoji) => [emoji.shortcode, emoji])));
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

const tableParts: ReadonlySet<string> = new Set(["table", "thead", "tbody", "tr"]);

const classes = (element: ContentElement) => new Set(element.attrs.class?.split(/\s+/));

/** What a link is, by the classes the server gives it: Mastodon's `mention hashtag` for a hashtag. */
function linkKind(element: ContentElement): ContentLinkKind {
  const of = classes(element);
  return of.has("hashtag") ? "hashtag" : of.has("mention") ? "mention" : "link";
}

/** One part as React elements. Code shows what its author typed, colons included. */
function draw(node: ContentNode, key: number, drawing: Drawing, code: boolean): ReactNode {
  if (typeof node === "string")
    return <Fragment key={key}>{code ? codeText(node) : emojiText(node, drawing.emojis)}</Fragment>;
  const inCode = code || node.tag === "code" || node.tag === "pre";
  // A table and its sections and rows hold no text: the line breaks between their parts are left out.
  const parts = tableParts.has(node.tag)
    ? node.children.filter((child) => typeof child !== "string" || child.trim() !== "")
    : node.children;
  const children = parts.map((child, i) => draw(child, i, drawing, inCode));
  const { lang, href, translate } = node.attrs;
  switch (node.tag) {
    case "br":
      return <br key={key} />;
    case "a": {
      const kind = linkKind(node);
      const inApp =
        kind === "hashtag"
          ? drawing.hosts.map((host) => hashtagPath(href ?? "", host)).find((path) => path !== undefined)
          : undefined;
      return (
        <ContentLink
          key={key}
          href={inApp ?? href ?? ""}
          kind={kind}
          lang={lang}
          translate={translate}
          inApp={inApp !== undefined}
        >
          {children}
        </ContentLink>
      );
    }
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
          dir="auto"
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
    case "img": {
      const { src, alt, width, height, loading } = node.attrs;
      return (
        <img
          key={key}
          src={src}
          alt={alt ?? ""}
          width={width === undefined ? undefined : Number(width)}
          height={height === undefined ? undefined : Number(height)}
          loading={loading ?? "lazy"}
        />
      );
    }
    case "table":
      return (
        <ArticleTable key={key} lang={lang}>
          {children}
        </ArticleTable>
      );
    case "th":
    case "td":
      return createElement(node.tag, { key, lang, align: node.attrs.align }, ...children);
    default:
      return createElement(
        node.tag,
        { key, lang, dir: articleBlockTags.has(node.tag) && node.tag !== "hr" ? "auto" : undefined },
        ...children,
      );
  }
}

/** The text's blocks: a run of text or inline elements outside a block is a paragraph of its own. */
function blocksOf(nodes: ContentNode[], blocks: ReadonlySet<string>): ContentNode[] {
  const out: ContentNode[] = [];
  let run: ContentNode[] = [];
  const flush = () => {
    if (run.some((node) => typeof node !== "string" || node.trim() !== ""))
      out.push({ tag: "p", attrs: {}, children: run });
    run = [];
  };
  for (const node of nodes) {
    if (typeof node !== "string" && blocks.has(node.tag)) {
      flush();
      out.push(node);
    } else run.push(node);
  }
  flush();
  return out;
}

const escapes: Readonly<Record<string, string>> = { "&": "&amp;", "<": "&lt;", ">": "&gt;" };

/**
 * The HTML an excerpt of a status draws (a quote, a notice, the post a reply answers): its text, or an article's title,
 * which a list gives in place of its text (ADR-0160 §8).
 */
export function excerptHtml(status: { content: string; article?: { title: string } | undefined }): string {
  if (status.article === undefined) return status.content;
  return `<p>${status.article.title.replace(/[&<>]/g, (c) => escapes[c] ?? c)}</p>`;
}

/**
 * The body of a post, from the HTML the server rendered and sanitized, with its custom emoji; or, for an article, the
 * blocks of its text.
 */
export function PostContent({ html, emojis = [], lang, article = false }: PostContentProps) {
  const drawing: Drawing = {
    emojis: new Map(emojis.map((emoji) => [emoji.shortcode, emoji])),
    hosts: [use(ServerDomain), pageHost()],
  };
  const blocks = blocksOf(parseContent(html, article ? "article" : "post"), article ? articleBlockTags : blockTags).map(
    (node, i) => draw(node, i, drawing, false),
  );
  return article ? blocks : <ContentBody lang={lang}>{blocks}</ContentBody>;
}
