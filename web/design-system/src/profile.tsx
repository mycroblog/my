// SPDX-License-Identifier: AGPL-3.0-only

// The head of a profile: the banner, 3:1, on the raised surface when there is none and from edge to edge on a narrow
// column; the avatar over its lower edge, half on it, ringed by the page's surface, with the reader's actions at the
// other end of its row, as far from the banner's edge as from the avatar; the name with its marks over the handle and
// its badges; then the rest, a profile's gap apart (the bio, the fields, when it joined and its counts, lines of the
// account's state, the featured hashtags). Its parts: a badge, the fields, the line of counts, and a list of hashtags.

import type { ReactNode } from "react";
import { Link } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { useReader } from "./locale";

const head = tv({
  slots: {
    root: "@container grid grid-cols-[minmax(0,1fr)] gap-screen-profile-gap",
    banner: [
      "-mx-shell-edge -mt-shell-gap aspect-banner overflow-hidden bg-surface-raised",
      "@reply:mx-0 @reply:mt-0 @reply:rounded-overlay",
    ],
    image: "block size-full object-cover",
    // The row's start undoes the profile's gap, so that the avatar's half reaches onto the banner.
    row: "-mt-[calc(var(--avatar-preview)/2+var(--screen-profile-gap))] flex items-start gap-screen-head-room",
    avatar:
      "ms-screen-avatar-inset inline-flex rounded-full shadow-[0_0_0_var(--screen-avatar-ring)_var(--color-surface)]",
    actions: "ms-auto mt-[calc(var(--avatar-preview)/2+var(--screen-head-room))] flex items-center gap-screen-tab-gap",
    names: "grid gap-[calc(var(--screen-name-gap)/3)]",
    name: "m-0 flex flex-wrap items-center gap-screen-name-gap text-title",
    handle: "m-0 flex flex-wrap items-center gap-screen-name-gap text-caption text-muted-text",
    handleText: "",
  },
});

export type ProfileHeadProps = {
  /** The id of the name, which names the head. */
  nameId: string;
  /** The banner's image; the raised surface without one. */
  banner?: string | undefined;
  /** The avatar, at the preview's size. */
  avatar: ReactNode;
  /** What the reader may do, at the end of the avatar's row. */
  actions?: ReactNode;
  /** The display name. */
  name: ReactNode;
  /** What follows the name on its line (a locked account's lock, a bot's badge). */
  marks?: ReactNode;
  /** The handle, as `@user@domain`. */
  handle: string;
  /** What follows the handle on its line (follows you, the role badges). */
  badges?: ReactNode;
  /** The rest of the head, one part under another. */
  children?: ReactNode;
};

export function ProfileHead({
  nameId,
  banner,
  avatar,
  actions,
  name,
  marks,
  handle,
  badges,
  children,
}: ProfileHeadProps) {
  const s = head();
  const { direction } = useReader(undefined);
  return (
    <section className={s.root()} aria-labelledby={nameId} dir={direction}>
      <div className={s.banner()}>{banner !== undefined && <img className={s.image()} src={banner} alt="" />}</div>
      <div className={s.row()}>
        <span className={s.avatar()}>{avatar}</span>
        {actions !== undefined && <div className={s.actions()}>{actions}</div>}
      </div>
      <div className={s.names()}>
        <h2 className={s.name()}>
          <bdi id={nameId}>{name}</bdi>
          {marks}
        </h2>
        <p className={s.handle()}>
          <bdi dir="ltr">{handle}</bdi>
          {badges}
        </p>
      </div>
      {children}
    </section>
  );
}

const badge = tv({
  base: "rounded-control bg-surface-raised px-screen-name-gap text-caption leading-[1.5] font-label text-text",
});

/** A small label on the raised surface (follows you, a role, a bot), as the author wrote it, isolated. */
export function Badge({ children }: { children: ReactNode }) {
  return (
    <bdi dir="auto" className={badge()}>
      {children}
    </bdi>
  );
}

const fields = tv({
  slots: {
    root: "m-0 grid gap-screen-field-gap",
    field: "grid",
    name: "text-caption text-muted-text",
    value: "m-0",
  },
});

/** A profile's fields: each name, muted and small, over its value. */
export function FieldList({ items }: { items: readonly { name: ReactNode; value: ReactNode }[] }) {
  const s = fields();
  return (
    <dl className={s.root()}>
      {items.map((item, i) => (
        <div key={i} className={s.field()}>
          <dt className={s.name()}>{item.name}</dt>
          <dd className={s.value()}>{item.value}</dd>
        </div>
      ))}
    </dl>
  );
}

const meta = tv({
  slots: {
    root: "m-0 flex flex-wrap gap-x-screen-meta-gap gap-y-screen-tab-gap text-caption text-muted-text",
    count: "rounded-control text-muted-text no-underline outline-hidden data-focus-visible:shadow-focus",
    value: "text-body font-strong text-text",
  },
});

/** A line of a profile's facts: when it joined (text) and its counts (`MetaCount`). */
export function MetaLine({ children }: { children: ReactNode }) {
  return <p className={meta().root()}>{children}</p>;
}

/** A count of a profile's line: its label, then its value in the text's color; a link when it opens a list. */
export function MetaCount({ label, value, href }: { label: string; value: string; href?: string | undefined }) {
  const s = meta();
  const body = (
    <>
      {label} <strong className={s.value()}>{value}</strong>
    </>
  );
  return href === undefined ? (
    <span>{body}</span>
  ) : (
    <Link href={href} className={s.count()}>
      {body}
    </Link>
  );
}

const tags = tv({
  slots: {
    root: "grid gap-screen-name-gap",
    heading: "m-0 text-caption font-normal text-muted-text",
    list: "m-0 flex list-none flex-wrap gap-screen-name-gap p-0",
    tag: [
      "inline-flex items-baseline gap-screen-name-gap rounded-control bg-surface-raised px-chip-inline text-body",
      "leading-[var(--chip-height)]",
      "text-text no-underline outline-hidden data-hovered:bg-surface-raised-hover data-focus-visible:shadow-focus",
    ],
    count: "text-caption text-muted-text",
  },
});

/** The hashtags a profile features, under a heading: each a link on the raised surface, its count muted. */
export function TagList({
  heading,
  items,
}: {
  heading: string;
  items: readonly { name: string; href: string; count: string }[];
}) {
  const s = tags();
  return (
    <div className={s.root()}>
      <h3 className={s.heading()}>{heading}</h3>
      <ul className={s.list()}>
        {items.map((item) => (
          <li key={item.name}>
            <Link href={item.href} className={s.tag()}>
              <bdi>#{item.name}</bdi> <span className={s.count()}>{item.count}</span>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
