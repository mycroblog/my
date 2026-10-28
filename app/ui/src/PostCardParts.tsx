// SPDX-License-Identifier: AGPL-3.0-only
// The parts that fill the slots of PostCard and QuoteCard the same way everywhere: the links on a name, a time and an
// avatar, the line of a boost, and the link to the post on its own server.
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";
import { ExternalLink } from "./ExternalLink";
import { BoostIcon } from "./Icons";
import { Link } from "./Link";
import { PostCardAuthor, type PostCardAuthorProps } from "./PostCard";
import { Text } from "./Text";

const parts = tv({
  slots: {
    // In the color of the text, underlined on hover: a name is no running text.
    name: "text-fg no-underline data-hovered:text-fg data-hovered:underline",
    // A block as wide as its text (the link is no stripe across the row), so that the name and the handle inside
    // are cut to the room of the header's row.
    author: "block w-fit max-w-full min-w-0",
    // Muted like the other details of the header (G6) and underlined only on hover and focus, so that a feed is
    // no column of underlines.
    time: "text-fg-muted no-underline data-hovered:text-fg-muted data-hovered:underline data-focus-visible:underline",
    avatar: "block rounded-full",
    origin: "ms-auto min-w-0 text-sm font-normal text-fg-muted data-hovered:text-fg",
  },
  variants: {
    // A target of at least 24 px (D4), which a time in a header of its own row needs.
    dense: { false: { time: "inline-flex min-h-6 items-center" } },
  },
});

export type PostCardNameLinkProps = {
  /** The profile. */
  href: string;
  /** The name, usually an `EmojiText`; the user's text, isolated from the sentence around it (`<bdi>`). */
  children: ReactNode;
};

/**
 * A link to a profile on a name inside a sentence ("Bob boosted", "Bob followed you"): in the color of the text,
 * underlined on hover. The name is the user's text, so it is isolated from the sentence (`<bdi>`, Y10).
 */
export function PostCardNameLink({ href, children }: PostCardNameLinkProps) {
  return (
    <Link href={href} className={parts().name()}>
      <bdi>{children}</bdi>
    </Link>
  );
}

export type PostCardAuthorLinkProps = PostCardAuthorProps & {
  /** The profile. */
  href: string;
};

/** The author slot: a link to the profile around a PostCardAuthor, named by the display name and the handle in full. */
export function PostCardAuthorLink({ href, name, handle }: PostCardAuthorLinkProps) {
  const slots = parts();
  return (
    <Link href={href} className={`${slots.name()} ${slots.author()}`}>
      <PostCardAuthor name={name} {...(handle !== undefined && { handle })} />
    </Link>
  );
}

export type PostCardTimeLinkProps = {
  /** The page the time leads to: the post, or its revisions. */
  href: string;
  /** A `<time>`, or the mark of an edited post. */
  children: ReactNode;
  /** Without the 24 px minimum height: for a time inside a row that is tall enough already (QuoteCard). */
  dense?: boolean;
};

/** A link on a time, or on the mark of an edited post (FR-109): muted, underlined only on hover and focus. */
export function PostCardTimeLink({ href, children, dense = false }: PostCardTimeLinkProps) {
  return (
    <Link href={href} className={parts({ dense }).time()}>
      {children}
    </Link>
  );
}

export type PostCardAvatarLinkProps = {
  /** The profile, the same as the name's. */
  href: string;
  /** A decorative `<Avatar decorative />`. */
  children: ReactNode;
};

/**
 * The avatar as a second link to the profile for the pointer: out of the tab order and hidden from screen readers,
 * which reach the profile by the name (one tab stop per profile). React Aria's Link does not take `aria-hidden`, so
 * an element around it hides it.
 */
export function PostCardAvatarLink({ href, children }: PostCardAvatarLinkProps) {
  return (
    <div aria-hidden="true">
      <Link href={href} excludeFromTabOrder className={parts().avatar()}>
        {children}
      </Link>
    </div>
  );
}

/** The context line of a boost: the boost icon and the sentence ("Bob boosted"), which has a PostCardNameLink in it. */
export function PostCardBoostContext({ children }: { children: ReactNode }) {
  return (
    <>
      <BoostIcon />
      <span>{children}</span>
    </>
  );
}

export type PostCardOriginProps = {
  /** The post's page on its own server. */
  href: string;
  /** The server's host, which is what the link shows. */
  host: string;
  /** The accessible name, which says where the link leads and contains the host (WCAG 2.5.3, label in name). */
  label: string;
};

/** The link from a post of another server to the post's own page, at the end of the actions row. */
export function PostCardOrigin({ href, host, label }: PostCardOriginProps) {
  return (
    <ExternalLink href={href} variant="standalone" aria-label={label} className={parts().origin()}>
      <Text dir="ltr" wrap="truncate" shrink>
        {host}
      </Text>
    </ExternalLink>
  );
}
