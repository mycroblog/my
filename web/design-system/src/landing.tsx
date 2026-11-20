// SPDX-License-Identifier: AGPL-3.0-only

// The parts of a server's front page for a visitor (the landing page), besides those it shares with the pages of its
// own outside the app (form-page.tsx): its frame, with the server's banner, a section of what it tells under a heading, the list of
// what a member can do there, and the low-key line of the places a visitor may go to without an account.
//
// The frame is one column on a phone. `column` keeps it one column of the app's reading width at every width: the
// introduction, the account's part, what it tells, the footer. `aside` sets the account's part beside the rest from
// `tablet`, on the raised surface, staying in view while the rest scrolls; on a phone it comes right after the
// introduction. Like the frame of the form pages, it carries the page's surface onto <html>.

import type { ReactNode } from "react";
import { Link } from "react-aria-components";
import { tv } from "tailwind-variants/lite";
import { Icon, type IconName } from "./icons";
import { Row, Stack } from "./layout";

const frame = tv({
  slots: {
    root: [
      "box-border flex min-h-dvh justify-center bg-surface px-shell-edge pt-form-page-top pb-form-page-bottom font-sans",
      "text-body text-text [body:has(&)]:m-0 [html:has(&)]:bg-surface tablet:pt-form-page-top-wide",
    ],
    body: "grid w-full grid-cols-[minmax(0,1fr)] content-start gap-form-page-gap",
    intro: "grid grid-cols-[minmax(0,1fr)] content-start gap-form-page-intro-gap",
    banner: "aspect-banner w-full rounded-overlay object-cover",
    join: "grid grid-cols-[minmax(0,1fr)] content-start gap-form-page-form-gap",
    main: "grid grid-cols-[minmax(0,1fr)] content-start gap-form-page-gap",
    footer: "",
  },
  variants: {
    layout: {
      column: { body: "max-w-[var(--shell-column)]" },
      aside: {
        body: [
          "max-w-[calc(var(--shell-column)+var(--shell-gap-wide)+var(--form-page-column))]",
          "tablet:grid-cols-[minmax(0,1fr)_minmax(0,var(--shell-aside))] tablet:gap-x-shell-gap-wide",
          "desktop:grid-cols-[minmax(0,1fr)_minmax(0,var(--form-page-column))]",
        ],
        intro: "tablet:col-start-1 tablet:row-start-1",
        join: [
          "tablet:sticky tablet:top-form-page-top-wide tablet:col-start-2 tablet:row-span-3 tablet:row-start-1",
          "tablet:self-start tablet:rounded-overlay tablet:bg-surface-raised tablet:p-shell-gap",
        ],
        main: "tablet:col-start-1 tablet:row-start-2",
        footer: "tablet:col-start-1 tablet:row-start-3",
      },
    },
  },
});

/** The frame of the landing page: the introduction, the account's part, what the page tells, and the footer. */
export function LandingFrame({
  layout,
  banner,
  intro,
  join,
  footer,
  label,
  children,
}: {
  layout: "column" | "aside";
  /** The server's banner (its thumbnail), at the top of the introduction; it says nothing the name does not. */
  banner?: string | undefined;
  /** The server's introduction: its banner, its mark and name, its summary, its counts. */
  intro: ReactNode;
  /** The account's part: joining, or signing in. */
  join: ReactNode;
  footer?: ReactNode;
  /** The name of the account's part for assistive technology. */
  label: string;
  children: ReactNode;
}) {
  const s = frame({ layout });
  return (
    <div className={s.root()}>
      <div className={s.body()}>
        <header className={s.intro()}>
          {banner !== undefined && banner !== "" && <img className={s.banner()} src={banner} alt="" />}
          {intro}
        </header>
        <section className={s.join()} aria-label={label}>
          {join}
        </section>
        <main className={s.main()}>{children}</main>
        {footer !== undefined && <div className={s.footer()}>{footer}</div>}
      </div>
    </div>
  );
}

const parts = tv({
  slots: {
    heading: "text-title",
    glyph: "text-muted-text",
    places: "text-caption text-muted-text",
    // A place reads as the words around it; its target reaches the minimum around its text without moving the line
    // (WCAG 2.5.8).
    place: "rounded-control py-target-em text-muted-text no-underline outline-hidden data-focus-visible:shadow-focus",
  },
});

/** A part of what the page tells, under its heading. */
export function LandingSection({ id, heading, children }: { id: string; heading: string; children: ReactNode }) {
  return (
    <Stack as="section" gap="form-page-head-gap" aria-labelledby={id}>
      <h2 id={id} className={parts().heading()} dir="auto">
        {heading}
      </h2>
      {children}
    </Stack>
  );
}

/** What a member can do on the server, each a line after its glyph. */
export function LandingFeatures({ items }: { items: readonly { key: string; icon: IconName; text: string }[] }) {
  const s = parts();
  return (
    <Stack as="ul" plain gap="form-page-list-gap">
      {items.map((item) => (
        <Row as="li" key={item.key} align="start" gap="form-page-mark-gap">
          <Row as="span" inline rigid className={s.glyph()}>
            <Icon name={item.icon} place="action" />
          </Row>
          <span dir="auto">{item.text}</span>
        </Row>
      ))}
    </Stack>
  );
}

/** The places a visitor may go to without an account, low-key: `heading`, and the places in a line under it. */
export function LandingPlaces({
  heading,
  items,
}: {
  heading: string;
  items: readonly { href: string; icon: IconName; label: string }[];
}) {
  const s = parts();
  if (items.length === 0) return null;
  return (
    <Stack as="nav" gap="hair" aria-label={heading} className={s.places()}>
      <span>{heading}</span>
      {/* Wrapped rows stand far enough apart that the targets of two rows never overlap. */}
      <Row wrap align="baseline" gapX="form-page-gap" gapY="form-page-footer-rows">
        {items.map((item) => (
          <Row
            key={item.href}
            inline
            align="center"
            gap="entry-caption"
            as={<Link href={item.href} className={s.place()} />}
          >
            <Icon name={item.icon} place="caption" />
            {item.label}
          </Row>
        ))}
      </Row>
    </Stack>
  );
}
