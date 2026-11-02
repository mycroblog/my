// SPDX-License-Identifier: AGPL-3.0-only
// An account in a list or beside a text: the avatar, the display name and the handle, as a link to the profile or as plain
// text, with a slot at the end for an action or a badge.
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";
import { Avatar } from "./Avatar";
import type { ImageSources } from "./Picture";
import { Link } from "./Link";

const accountItem = tv({
  slots: {
    // The size container of a `below` row, around the row: an element's container query never answers to itself.
    container: "@container min-w-0",
    root: "flex min-w-0 flex-col gap-3",
    body: "grid min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-x-3",
    avatar: "",
    name: "font-extrabold align-match-parent",
    // Left to right, and lined up with the name at the row's start in either direction (G9).
    handle: "text-sm font-normal text-fg-muted align-match-parent",
    details: "flex flex-col text-sm text-fg-muted",
    // Under a linked account, outside the link: lined up with the name, past a spacer as wide as the avatar (size-10)
    // and the same gap.
    linkDetails: "flex min-w-0 gap-3 text-sm text-fg-muted",
    linkDetailsSpacer: "w-10 shrink-0",
    linkDetailsBody: "flex min-w-0 flex-1 flex-col gap-1",
    trailing: "flex shrink-0 flex-wrap items-center gap-2",
  },
  variants: {
    layout: {
      /** Fills the width it is in. */
      row: { body: "w-full text-fg data-hovered:text-fg" },
      /** A row of a list that is a whole target: padded, with a hover background. */
      padded: {
        body: "w-full rounded-md px-4 py-3 text-fg data-hovered:bg-surface-sunken data-hovered:text-fg",
        linkDetails: "-mt-2 px-4 pb-3",
      },
      /** Only as wide as its text, for an account named in running content. */
      inline: { body: "inline-grid max-w-full" },
    },
    link: {
      true: { name: "wrap-anywhere", handle: "wrap-anywhere" },
      false: { name: "truncate", handle: "truncate" },
    },
    trailing: {
      beside: { root: "flex-row items-center" },
      // Below the account until the container is wide enough for both on one line, where the slot leaves the account
      // at least half of the row and its own items wrap.
      below: {
        root: "@lg:flex-row @lg:items-center",
        trailing: "@lg:max-w-1/2 @lg:justify-end",
      },
      none: {},
    },
  },
  compoundVariants: [
    // The account's padding is on its body, inside the row: the row adds no gap, and the slot below it the same padding.
    { layout: "padded", trailing: "below", class: { root: "gap-0", trailing: "px-4 pb-3 @lg:ps-0 @lg:pb-0" } },
  ],
  defaultVariants: { layout: "row", link: false, trailing: "none" },
});

/** The rows the avatar spans, by the lines under the name: only the filled ones, so that no empty row adds height. */
const avatarRows = ["row-span-1", "row-span-2", "row-span-3"] as const;

export type AccountItemProps = {
  /** The display name as plain text: the avatar's fallback letter and, without `nameContent`, the name shown. */
  name: string;
  /** The name as rich content (custom emoji), shown instead of `name`. */
  nameContent?: ReactNode;
  /** The handle (`@user@example.com`), always left to right. Leave it out when `name` already is the handle. */
  handle?: string | undefined;
  /** The renditions of the avatar image. */
  avatar?: ImageSources | undefined;
  /** The avatar image URL, for an account that has no renditions. */
  avatarSrc?: string | undefined;
  /** What picks the tint of the avatar's letter, such as the account's ID (see `Avatar`). */
  avatarSeed?: string | undefined;
  /** Makes the account a link to this address, named "name handle". Without it the item is plain text. */
  href?: string | undefined;
  /** `row` fills the width, `padded` is a whole-row target with a hover background, `inline` is as wide as its text. */
  layout?: "row" | "padded" | "inline";
  /**
   * More lines under the handle. With `href` they stand under the link, outside it, so that the link's name stays the
   * account's name (a line of the profile text, a mark of the relationship).
   */
  details?: ReactNode;
  /** An action or a badge at the end. */
  trailing?: ReactNode;
  /** Where `trailing` goes: always `beside` the account (default), or `below` it until the container is wide. */
  trailingPlacement?: "beside" | "below";
  /** Draws the plain-text form (without `href`) as a paragraph, for a row whose account is a statement of its own. */
  paragraph?: boolean;
};

/**
 * The account row: avatar, name and handle. With `href` the whole name is one link; the space between the name and the
 * handle keeps them apart in the link's accessible name. The names are the user's own text, in any language
 * (`dir="auto"` for the name, `ltr` for the handle).
 */
export function AccountItem({
  name,
  nameContent,
  handle,
  avatar,
  avatarSrc,
  avatarSeed,
  href,
  layout,
  details,
  trailing,
  trailingPlacement = "beside",
  paragraph = false,
}: AccountItemProps) {
  const isLink = href !== undefined;
  const hasTrailing = trailing !== undefined && trailing !== null && trailing !== false;
  const showDetails = !isLink && details !== undefined;
  const slots = accountItem({ layout, link: isLink, trailing: hasTrailing ? trailingPlacement : "none" });
  const content = (
    <>
      <Avatar
        sources={avatar}
        src={avatarSrc}
        name={name}
        seed={avatarSeed}
        decorative
        className={slots.avatar({ className: avatarRows[(handle === undefined ? 0 : 1) + (showDetails ? 1 : 0)] })}
      />
      <span dir="auto" className={slots.name()}>
        {nameContent ?? name}
      </span>
      {handle !== undefined && (
        <>
          {/* The space keeps the name and the handle apart in the link's name. */}{" "}
          <span dir="ltr" className={slots.handle()}>
            {handle}
          </span>
        </>
      )}
      {showDetails && <span className={slots.details()}>{details}</span>}
    </>
  );
  const link = isLink && (
    <Link href={href} variant="standalone" className={slots.body()}>
      <>{content}</>
    </Link>
  );
  const body = isLink ? (
    details === undefined ? (
      link
    ) : (
      <div className="flex min-w-0 flex-col">
        {link}
        <div className={slots.linkDetails()}>
          <span aria-hidden="true" className={slots.linkDetailsSpacer()} />
          <div className={slots.linkDetailsBody()}>{details}</div>
        </div>
      </div>
    )
  ) : paragraph ? (
    <p className={slots.body()}>{content}</p>
  ) : (
    <div className={slots.body()}>{content}</div>
  );
  if (!hasTrailing) return body;
  const row = (
    <div className={slots.root()}>
      <div className="min-w-0 flex-1">{body}</div>
      <div className={slots.trailing()}>{trailing}</div>
    </div>
  );
  return trailingPlacement === "below" ? <div className={slots.container()}>{row}</div> : row;
}
