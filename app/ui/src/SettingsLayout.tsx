// SPDX-License-Identifier: AGPL-3.0-only
// The body of the settings: where the column is wide enough (`@2xl`, 42rem), a
// sticky menu of the sections beside one pane; narrower, the page of the settings stacks its heading, the menu and
// what else it holds, and a section's page shows the section alone. It asks its own container, not the viewport
// (web-client/README.md D2).
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";
import { Card } from "./Card";

const settingsLayout = tv({
  slots: {
    root: "@container",
    base: "grid grid-cols-1 items-start gap-6 @2xl:grid-cols-[14rem_minmax(0,1fr)]",
    menu: "@2xl:sticky @2xl:top-4",
    pane: "min-w-0",
  },
  variants: {
    // Narrower, a section's page hides the menu; the page of the settings keeps both, the menu first.
    view: {
      menu: {},
      pane: { menu: "@max-2xl:hidden" },
    },
  },
});

export type SettingsLayoutProps = {
  /** The menu's items (`RailItem`s in a list); the layout brings the card and the navigation landmark. */
  menu: ReactNode;
  /** The name of the menu's navigation landmark. */
  menuLabel: string;
  /** Narrow: `menu` (the page of the settings) stacks the menu and the pane; `pane` (a section) shows the section only. */
  view: "menu" | "pane";
  /** The page's heading, before the menu and the pane (the page of the settings; a section brings its own). */
  heading?: ReactNode;
  /** The section. */
  children: ReactNode;
};

/** A sticky menu card beside the pane card; narrow, the two stacked on the page of the settings, the section alone on a section's page. */
export function SettingsLayout({ menu, menuLabel, view, heading, children }: SettingsLayoutProps) {
  const slots = settingsLayout({ view });
  return (
    <div className={slots.root()}>
      {heading}
      <div className={slots.base()}>
        <nav aria-label={menuLabel} className={slots.menu()}>
          <Card padding="3">{menu}</Card>
        </nav>
        <div className={slots.pane()}>
          <Card padding="8">{children}</Card>
        </div>
      </div>
    </div>
  );
}
