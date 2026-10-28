// SPDX-License-Identifier: AGPL-3.0-only
// The frame of the admin and moderation mode: a
// mode of its own, apart from the app, with its own side menu and never the app's rail. From md a 15rem menu beside the
// page; below md a small top bar with the menu button, which opens the same menu in a sheet. Like AppShell it answers to
// the viewport, not to a container (web-client/README.md D2): the menu is as tall as the screen and the top bar is fixed to
// the screen's top edge.
import { type ReactNode, type Ref, useId } from "react";
import { tv } from "tailwind-variants";
import { DialogTrigger, Sheet } from "./Dialog";
import { IconButton } from "./IconButton";
import { BackIcon, MenuIcon } from "./Icons";
import { Link } from "./Link";

const adminShell = tv({
  slots: {
    base: "min-h-svh px-[max(env(safe-area-inset-left),env(safe-area-inset-right))] pt-[env(safe-area-inset-top)] md:grid md:grid-cols-[15rem_minmax(0,1fr)]",
    skipLink:
      "sr-only z-20 rounded-md bg-surface-raised px-3 py-2 shadow-md focus:not-sr-only focus:fixed focus:inset-s-2 focus:top-2",
    // The menu beside the page (from md): sticky, as tall as the screen, on the raised surface.
    side: "hidden border-e border-border bg-surface-raised md:sticky md:top-0 md:block md:h-svh md:overflow-y-auto",
    // Below md: the bar with the menu button and the name of the mode.
    topBar:
      "sticky top-0 z-10 flex min-h-14 items-center gap-2 border-b border-border bg-surface-raised px-3 md:hidden",
    topTitle: "min-w-0 truncate text-base font-extrabold",
    main: "mx-auto flex w-full min-w-0 max-w-6xl flex-col gap-6 px-4 pt-6 pb-[calc(2rem+env(safe-area-inset-bottom))] md:px-8",
    menu: "flex flex-col gap-1 p-3 pb-[calc(1.5rem+env(safe-area-inset-bottom))]",
    brand: "flex flex-col gap-1 px-2 pt-1 pb-4",
    modeLabel: "text-xs font-extrabold tracking-widest text-fg-muted uppercase",
    list: "flex flex-col gap-4",
    footer: "mt-4 border-t border-border pt-4",
  },
});

export type AdminShellProps = {
  /** The wordmark and the server's name (`BrandMark`), at the top of the menu. */
  brand: ReactNode;
  /** The name of the mode ("Admin and moderation"): above the menu's items, in the top bar, and the name of the menu's landmark. */
  modeLabel: string;
  /** The menu's items: `AdminNavSection`s. */
  navigation: ReactNode;
  /** Under the items: the link back to the app (`AdminNavBack`). */
  footer: ReactNode;
  /** The menu button's name ("Admin menu"), which is also the title of the sheet it opens. */
  menuButtonLabel: string;
  /** The sheet's close button's label. */
  closeLabel: string;
  /** Whether the menu's sheet is open (below md). */
  menuOpen: boolean;
  onMenuOpenChange: (isOpen: boolean) => void;
  /** The page, in the `main` landmark. */
  children: ReactNode;
  /** The skip link's text ("Skip to main content"), the first focusable element of the page. */
  skipLinkLabel: string;
  /** The id of `main`, the skip link's target. */
  mainId?: string;
  /** The `main` element, for the code that moves focus to the page's heading after a navigation. */
  mainRef?: Ref<HTMLElement>;
};

/**
 * The frame of the admin and moderation mode: the skip link, the menu (beside the page from md, in a sheet below it)
 * and `main`. It brings the navigation landmark; the same menu is in the sidebar and in the sheet, and only one of them
 * is on screen at a width. Render the connection bar before it, since it pads for the top safe-area inset itself.
 */
export function AdminShell({
  brand,
  modeLabel,
  navigation,
  footer,
  menuButtonLabel,
  closeLabel,
  menuOpen,
  onMenuOpenChange,
  children,
  skipLinkLabel,
  mainId = "main",
  mainRef,
}: AdminShellProps) {
  const slots = adminShell();
  const menu = (
    <nav aria-label={modeLabel} className={slots.menu()}>
      <div className={slots.brand()}>
        {brand}
        <p className={slots.modeLabel()}>{modeLabel}</p>
      </div>
      <div className={slots.list()}>{navigation}</div>
      <div className={slots.footer()}>{footer}</div>
    </nav>
  );
  return (
    <div className={slots.base()}>
      <Link href={`#${mainId}`} variant="standalone" className={slots.skipLink()}>
        {skipLinkLabel}
      </Link>
      <div className={slots.side()}>{menu}</div>
      <div className="min-w-0">
        <header className={slots.topBar()}>
          <DialogTrigger isOpen={menuOpen} onOpenChange={onMenuOpenChange}>
            <IconButton label={menuButtonLabel}>
              <MenuIcon />
            </IconButton>
            <Sheet title={menuButtonLabel} closeLabel={closeLabel}>
              {menu}
            </Sheet>
          </DialogTrigger>
          <p className={slots.topTitle()}>{modeLabel}</p>
        </header>
        <main ref={mainRef} id={mainId} tabIndex={-1} className={slots.main()}>
          {children}
        </main>
      </div>
    </div>
  );
}

const adminNavBack = tv({
  base: "min-h-11 gap-2 rounded-lg px-3 text-sm font-bold text-fg data-hovered:bg-surface-sunken [&>svg]:size-5",
});

const adminNavSection = tv({
  slots: {
    base: "flex flex-col gap-1",
    heading: "px-3 pb-1 text-xs font-extrabold text-fg-muted",
    list: "flex flex-col gap-1",
  },
});

export type AdminNavSectionProps = {
  /** The group's name ("Moderation"). */
  heading: string;
  /** The group's items: `RailItem`s with `form="section"` in list items. */
  children: ReactNode;
};

/** A named group of the menu's items. */
export function AdminNavSection({ heading, children }: AdminNavSectionProps) {
  const slots = adminNavSection();
  const id = useId();
  return (
    <section aria-labelledby={id} className={slots.base()}>
      <h2 id={id} className={slots.heading()}>
        {heading}
      </h2>
      <ul className={slots.list()}>{children}</ul>
    </section>
  );
}

export type AdminNavBackProps = {
  href: string;
  /** The link's words ("Back to the app"). */
  label: string;
};

/** The way out of the mode, under the menu's items. */
export function AdminNavBack({ href, label }: AdminNavBackProps) {
  return (
    <Link href={href} variant="navigation" className={adminNavBack()}>
      <BackIcon />
      {label}
    </Link>
  );
}
