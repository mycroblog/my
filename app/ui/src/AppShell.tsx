// SPDX-License-Identifier: AGPL-3.0-only
// The layout around every page of the app (ADR-0026, web-client/09-app-shell.md §9). Mobile first (D1): one column with the
// navigation as a bottom tab bar; from md a side rail and the page; from xl a third column. Heights use svh (D5) and
// every edge that touches the screen adds its safe-area inset. The offline bar before the shell (OfflineBanner) shares
// the screen with it: the shell's heights are the screen's under the bar and what sticks to the top starts under it
// (the *-under-bar utilities of theme.css), so that a short page does not scroll by the bar's height. It answers to the viewport, not to a container: the
// shell is the one place where viewport breakpoints belong (D2), since what it lays out is the screen itself (the tab
// bar is fixed to the screen's bottom edge, the rail and the third column are as tall as the screen). Its stories set
// the story's viewport instead of the frame's width (storyViewports.ts).
import { type ReactNode, type Ref, useId } from "react";
import { tv } from "tailwind-variants";
import { Card } from "./Card";
import { Link } from "./Link";

const appShell = tv({
  slots: {
    base: [
      "min-h-svh-under-bar px-[max(env(safe-area-inset-left),env(safe-area-inset-right))] pt-[env(safe-area-inset-top)]",
    ],
    skipLink:
      "sr-only z-20 rounded-md bg-surface-raised px-3 py-2 shadow-md focus:not-sr-only focus:fixed focus:inset-s-2 focus:top-2",
    // The end padding keeps the last item of a list clear of what floats over it (web-client/16-look-states-and-wording.md G4): below md the tab
    // bar (min-h-16 and its border; min-h-12 on short screens) and, while the floating button shows (data-compose-fab),
    // the button (3rem) above it; from md the button in the corner; from xl nothing. Each gap is 1rem (0.5rem on short
    // screens). The app's stylesheet clears focused elements of the same (scroll-padding-block-end).
    main: "mx-auto flex w-full min-w-0 flex-col gap-4 px-4 pt-6 md:px-8 md:pb-6",
    // The top bar's footer (the server's information, its documents and the software), in the page's column.
    footer: "mx-auto w-full min-w-0 px-4 pb-[max(1.5rem,env(safe-area-inset-bottom))] md:px-8",
    aside: "sticky top-under-bar flex h-svh-under-bar flex-col gap-6 overflow-y-auto border-s border-border p-6",
    floating: [
      "fixed inset-e-[max(1rem,env(safe-area-inset-right))] bottom-[calc(5rem+env(safe-area-inset-bottom))] z-10",
      "short:bottom-[calc(3.5rem+env(safe-area-inset-bottom))] md:bottom-[max(1.5rem,env(safe-area-inset-bottom))]",
    ],
  },
  variants: {
    // `side`: the navigation is the tab bar, then the rail beside the page. `top`: the signed-out top bar above it.
    // The top bar's pages have no tab bar to clear and nothing floats over them; `main` grows, so that a short page
    // still has its footer at the bottom edge.
    navigationPlacement: {
      side: {
        base: "md:grid md:grid-cols-[auto_minmax(0,1fr)]",
        main: [
          "pb-[calc(5rem+env(safe-area-inset-bottom))] short:pb-[calc(4rem+env(safe-area-inset-bottom))]",
          "data-compose-fab:pb-[calc(9rem+env(safe-area-inset-bottom))] data-compose-fab:short:pb-[calc(7rem+env(safe-area-inset-bottom))]",
          "data-compose-fab:md:pb-[calc(5.5rem+env(safe-area-inset-bottom))] data-compose-fab:xl:pb-6",
        ],
      },
      top: { base: "flex flex-col", main: "flex-1 pb-6" },
    },
    withAside: { true: {} },
    // The column's width: `narrow` for reading, `wide` for a page with something beside its text (the signed-out landing).
    width: { narrow: { main: "max-w-2xl", footer: "max-w-2xl" }, wide: { main: "max-w-5xl", footer: "max-w-5xl" } },
  },
  compoundVariants: [
    { navigationPlacement: "side", withAside: true, class: { base: "xl:grid-cols-[auto_minmax(0,1fr)_24rem]" } },
  ],
  defaultVariants: { navigationPlacement: "side", width: "narrow" },
});

export type AppShellProps = {
  /** The main navigation (`AppNav`): the bottom tab bar below md, the rail and the sidebar from md. */
  navigation: ReactNode;
  /** `side` (default) for the tab bar and the rail; `top` when the navigation is the signed-out top bar (`AppNav` `placement="top"`), above the page. */
  navigationPlacement?: "side" | "top";
  /** The width of the page's column: `narrow` (default) for reading, `wide` for a page that lays things side by side. */
  width?: "narrow" | "wide";
  /** The page, in the `main` landmark. */
  children: ReactNode;
  /** The skip link's text ("Skip to main content"), the first focusable element of the page. */
  skipLinkLabel: string;
  /** The id of `main`, the skip link's target. */
  mainId?: string;
  /** The `main` element, for the code that moves focus to the page's heading after a navigation. */
  mainRef?: Ref<HTMLElement>;
  /**
   * The third column, sticky and as tall as the screen; pass it only on screens that show it (xl). The app gives it
   * the new-post form and the server's information (`AsidePanel`). Only the `side` placement has a third column: the
   * top bar's pages are one column, and the shell leaves it out there.
   */
  aside?: ReactNode;
  /**
   * The `top` placement's footer, the `contentinfo` landmark after `main` in the page's column: the links to the
   * server's information and documents. The `side` placement has none: its menu and third column carry them.
   */
  footer?: ReactNode;
  /**
   * The slot that floats over the page's end corner: above the tab bar below md, in the corner from md. The app puts
   * the new-post button there on screens without the third column. The slot may hold nothing visible.
   */
  floating?: ReactNode;
  /** Whether the floating slot shows a button now, so that `main` keeps the end of the page clear of it (G4). */
  floatingVisible?: boolean;
};

/**
 * The app's frame: the skip link, the navigation, `main`, the third column (`side`) or the footer (`top`), and the
 * floating slot. Render the
 * connection bar (`OfflineBanner`) before it, since it pads for the top safe-area inset itself; the shell takes the
 * screen's height under the bar.
 */
export function AppShell({
  navigation,
  children,
  skipLinkLabel,
  mainId = "main",
  mainRef,
  aside,
  floating,
  floatingVisible = false,
  navigationPlacement = "side",
  width = "narrow",
  footer,
}: AppShellProps) {
  const side = navigationPlacement === "side";
  const withAside = side && aside !== undefined;
  const slots = appShell({ navigationPlacement, width, withAside });
  return (
    <div className={slots.base()}>
      {/* The first focusable element of the page; visible only while focused. */}
      <Link href={`#${mainId}`} variant="standalone" className={slots.skipLink()}>
        {skipLinkLabel}
      </Link>
      {navigation}
      <main
        ref={mainRef}
        id={mainId}
        tabIndex={-1}
        data-compose-fab={floatingVisible ? "" : undefined}
        className={slots.main()}
      >
        {children}
      </main>
      {!side && footer !== undefined && <footer className={slots.footer()}>{footer}</footer>}
      {withAside && <div className={slots.aside()}>{aside}</div>}
      {floating !== undefined && <div className={slots.floating()}>{floating}</div>}
    </div>
  );
}

const shellDock = tv({
  base: "sticky z-10",
  variants: {
    // Below md the dock sits 1rem above the tab bar (as the floating button does); from md 1rem above the bottom edge.
    // On short screens (a phone in landscape, 400% zoom) it does not stick: it ends the page and scrolls with it, as
    // the offline bar does, so that what is fixed stays within half the screen's height (WCAG 1.4.10). While the
    // floating new-post button shows, the dock leaves its end corner to the button (4rem), up to xl, where nothing floats.
    clearsTabBar: {
      true: [
        "bottom-[calc(5rem+env(safe-area-inset-bottom))] short:static md:bottom-[max(1rem,env(safe-area-inset-bottom))]",
        "with-compose-fab:me-16 xl:with-compose-fab:me-0",
      ],
      false: "bottom-[max(1rem,env(safe-area-inset-bottom))]",
    },
  },
  defaultVariants: { clearsTabBar: true },
});

export type ShellDockProps = {
  /** The bar that stays in view: the call bar (`CallBar`). */
  children: ReactNode;
  /** Whether the page has the bottom tab bar below md (`AppShell`); false in the admin frame (`AdminShell`). */
  clearsTabBar?: boolean;
};

/**
 * A bar docked at the bottom of the page's column, which stays in view while the page scrolls: render it as the last
 * child of the page (inside `main`), so that it comes after the page's content in the focus order and the end of the
 * page is never under it. It stays clear of the tab bar and the floating new-post button (web-client/09-app-shell.md §9).
 */
export function ShellDock({ children, clearsTabBar = true }: ShellDockProps) {
  return <div className={shellDock({ clearsTabBar })}>{children}</div>;
}

const asidePanel = tv({
  slots: {
    base: "flex flex-col gap-2",
    heading: "text-sm font-bold text-fg-muted",
  },
});

export type AsidePanelProps = {
  /**
   * The panel's heading, an `h2` that names the `aside` landmark ("About this server"): text, or text with a `Badge`
   * of a count beside it ("Announcements", 2 unread), whose label then is part of the name.
   */
  heading: ReactNode;
  children: ReactNode;
};

/** A section of the third column: an `aside` landmark named by its small, muted heading, its content one above another. */
export function AsidePanel({ heading, children }: AsidePanelProps) {
  const slots = asidePanel();
  const headingId = useId();
  return (
    <Card as="aside" padding="4" aria-labelledby={headingId}>
      <div className={slots.base()}>
        <h2 id={headingId} className={slots.heading()}>
          {heading}
        </h2>
        {children}
      </div>
    </Card>
  );
}
