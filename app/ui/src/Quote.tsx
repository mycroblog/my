// SPDX-License-Identifier: AGPL-3.0-only
// A quoted piece of someone else's text: the sunken well and the clamped excerpt of a post. A notification quotes
// the post it is about, a moderation notice quotes the moderators' words, and the form quotes the post being replied to.
import type { ReactNode } from "react";
import { Link } from "./Link";
import { Text } from "./Text";

export type QuoteBlockProps = {
  /** Lines of text (`Text as="p"`): what the quote says. */
  children: ReactNode;
};

/** A block of lines on a sunken well, one line above another. */
export function QuoteBlock({ children }: QuoteBlockProps) {
  return (
    <div className="flex min-w-0 flex-col gap-1 rounded-md bg-surface-sunken px-3 py-2 text-fg-muted">{children}</div>
  );
}

export type PostExcerptProps = {
  /** The post's page in the app. */
  href: string;
  /** The excerpt: a `PostExcerptText`, or a sentence for a post that has no text to quote. */
  children: ReactNode;
};

/** The excerpt of a post as a quote that links to the post, in the color of the text and underlined on hover. */
export function PostExcerpt({ href, children }: PostExcerptProps) {
  return (
    <Link
      href={href}
      className="block rounded-md bg-surface-sunken px-3 py-2 wrap-anywhere text-fg-muted no-underline data-hovered:text-fg data-hovered:underline"
    >
      {children}
    </Link>
  );
}

export type PostExcerptTextProps = {
  /** The post's language (BCP 47), for hyphenation and the right glyphs. */
  lang?: string | undefined;
  /** The start of the post's text, plain. */
  children: ReactNode;
};

/** The start of a post's text: its own direction, line breaks kept, cut after three lines. */
export function PostExcerptText({ lang, children }: PostExcerptTextProps) {
  return (
    <Text dir="auto" {...(lang !== undefined && { lang })} lineClamp={3} whitespace="preLine">
      {children}
    </Text>
  );
}
