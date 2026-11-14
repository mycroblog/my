// SPDX-License-Identifier: AGPL-3.0-only

// The look of what people write, such as a post's body (ADR-0025 §4): the parts that @my/app-components' one content
// renderer draws it with. The body styles the plain elements it holds (paragraphs, quotes, lists, code), so that the
// renderer writes them without a class; a link, a part of a shortened URL and a custom emoji are parts of their own.
// Long words and URLs wrap anywhere, so that no body scrolls sideways on a phone; code wraps where the renderer marks
// a break first (after a slash or a question mark, before a pipe).

import type { ReactNode } from "react";
import { tv } from "tailwind-variants/lite";

// A quote is marked by a bar at its start and muted text, never by a surface, which means code or a quoted post; its
// blocks sit a paragraph's gap apart, a nested quote as close as the lines around it. Code sits on the raised surface,
// inline as a chip; a code block wraps and never scrolls, a wrapped line's continuation indented.
const body = tv({
  base: [
    "grid grid-cols-[minmax(0,1fr)] gap-content-paragraph text-body text-text wrap-anywhere",
    "[&_blockquote]:m-0 [&_ol]:m-0 [&_p]:m-0 [&_pre]:m-0 [&_ul]:m-0",
    "[&_blockquote]:border-s-content-quote [&_blockquote]:border-s-quote-bar",
    "[&_blockquote]:[border-inline-start-style:solid] [&_blockquote]:ps-content-quote [&_blockquote]:text-muted-text",
    "[&_blockquote>*+*]:mt-content-paragraph [&_blockquote>blockquote]:mt-0 [&_blockquote>blockquote+*]:mt-0",
    "[&_ol]:list-decimal [&_ol]:ps-content-indent [&_ul]:list-disc [&_ul]:ps-content-indent",
    "[&_code]:rounded-control [&_code]:bg-surface-raised [&_code]:px-content-code-chip",
    "[&_pre]:rounded-control [&_pre]:bg-surface-raised [&_pre]:px-content-code-inline [&_pre]:py-content-code-block",
    "[&_pre]:whitespace-pre-wrap [&_pre]:[text-indent:var(--content-code-hang)_hanging_each-line]",
    "[&_pre_code]:bg-transparent [&_pre_code]:p-0",
  ],
});

export type ContentBodyProps = {
  /** The blocks of the text: paragraphs, quotes, lists and code. */
  children: ReactNode;
  /** The language of the text, when it is known (a BCP 47 tag). */
  lang?: string | undefined;
};

/** The body of a text people wrote: its blocks one under another, a paragraph's gap apart. */
export function ContentBody({ children, lang }: ContentBodyProps) {
  return (
    <div className={body()} lang={lang}>
      {children}
    </div>
  );
}

/** What a link in a text is: a link to a page, a mention of an account, or a hashtag. */
export type ContentLinkKind = "link" | "mention" | "hashtag";

// A link takes the accent and is underlined as the design system's text links are, so that it tells from the text
// around it without its color (WCAG 1.4.1); its target reaches the minimum around its text without moving the line
// (WCAG 2.5.8); a mention or a hashtag breaks only around it, never inside.
const link = tv({
  base: "text-accent-text underline underline-offset-[0.15em] py-[calc((var(--target-minimum)-1em)/2)]",
  variants: { kind: { link: "", mention: "whitespace-nowrap", hashtag: "whitespace-nowrap" } },
});

export type ContentLinkProps = {
  href: string;
  kind: ContentLinkKind;
  children: ReactNode;
  /** The language of the link's text, when it differs from the text around it. */
  lang?: string | undefined;
  /** `no` when the link's text is a name or a handle that no translation may change. */
  translate?: "no" | undefined;
};

/** A link in a text, which opens in a new tab and tells the page it leads to nothing of where it came from. */
export function ContentLink({ href, kind, children, lang, translate }: ContentLinkProps) {
  return (
    <a
      className={link({ kind })}
      href={href}
      rel="nofollow noopener noreferrer"
      target="_blank"
      lang={lang}
      translate={translate}
    >
      {children}
    </a>
  );
}

// The server shortens the text of a URL as Mastodon does: the scheme and what follows its first characters are
// hidden, and the shown part ends in an ellipsis when the rest is cut. A hidden part takes no room but stays in the
// text, so that copying the link copies the whole URL.
const urlPart = tv({
  variants: {
    part: {
      hidden: "absolute inline-block size-0 overflow-hidden text-[0px] leading-[0]",
      cut: "after:content-['…']",
    },
  },
});

export type UrlPartProps = {
  /** `hidden` for a part that is not shown, `cut` for the shown part that the rest follows. */
  part: "hidden" | "cut";
  children: ReactNode;
};

/** A part of a shortened URL in a link's text. */
export function UrlPart({ part, children }: UrlPartProps) {
  return <span className={urlPart({ part })}>{children}</span>;
}

export type CustomEmojiProps = {
  /** The shortcode, without the colons around it. */
  shortcode: string;
  /** The image, animated when the emoji is. */
  url: string;
  /** A still image of the emoji; the same as `url` when it has none. */
  staticUrl: string;
};

/**
 * A custom emoji (ADR-0051 §4): its image at the size of the text around it, read and copied as `:shortcode:`. It
 * moves only when the reader allows motion, and shows its still image otherwise.
 */
export function CustomEmoji({ shortcode, url, staticUrl }: CustomEmojiProps) {
  const code = `:${shortcode}:`;
  return (
    <picture>
      {url !== staticUrl && <source media="(prefers-reduced-motion: no-preference)" srcSet={url} />}
      <img
        className="mx-[0.05em] my-[calc((1em-var(--content-emoji))/2)] inline size-content-emoji align-middle"
        src={staticUrl}
        alt={code}
        title={code}
        draggable={false}
      />
    </picture>
  );
}
