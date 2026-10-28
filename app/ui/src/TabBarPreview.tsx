// SPDX-License-Identifier: AGPL-3.0-only
// A small picture of the phones' tab bar (AppNavigation below md), for the settings that arrange the navigation: the
// items it holds in their order, then the Menu button, drawn with the tab bar's own look (RailItem's `tab` form) so that
// the preview and the bar are the same thing (ADR-0110 P4). It shows the result of a change at once (P3) and does
// nothing itself: its items are plain text, not links or buttons, and no item is the current one. The caption names it
// (`aria-labelledby` too, since not every accessibility tree names a figure by its caption);
// the items are a list, so that assistive technology reads them in their order.
import { type ReactNode, useId } from "react";
import { tv } from "tailwind-variants";
import { MenuIcon } from "./Icons";
import { railItem } from "./RailItem";

const tabBarPreview = tv({
  slots: {
    figure: "flex w-full max-w-sm min-w-0 flex-col gap-2",
    // The bar's ground and border, as the tab bar draws them on the bottom edge, in a rounded frame.
    bar: "flex overflow-hidden rounded-lg border border-border bg-surface-raised",
    // Each item as wide as its label, the room left shared out evenly, as in the tab bar.
    item: "min-w-0 flex-auto",
    caption: "text-sm text-fg-muted",
  },
});

/** One item of the preview: its icon and its label, as the tab bar shows them. */
export type TabBarPreviewItem = {
  /** Identifies the item among the others (React's key). */
  id: string;
  /** The item's icon (one of the kit's icons); the preview sizes it. */
  icon: ReactNode;
  label: string;
};

export type TabBarPreviewProps = {
  /** The items of the tab bar, in their order: the same items the app's tab bar holds. */
  items: readonly TabBarPreviewItem[];
  /** The label of the Menu button, the bar's last item ("Menu"). */
  menuLabel: string;
  /** What the preview shows ("Tab bar on phones"), under it. */
  caption: ReactNode;
};

/** The phones' tab bar as the given items make it, with the Menu button last. */
export function TabBarPreview({ items, menuLabel, caption }: TabBarPreviewProps) {
  const slots = tabBarPreview();
  const captionId = useId();
  const look = railItem({ form: "tab" });
  const entries = [...items, { id: "\u0000menu", icon: <MenuIcon />, label: menuLabel }];
  return (
    <figure aria-labelledby={captionId} className={slots.figure()}>
      <ol className={slots.bar()}>
        {entries.map((item) => (
          <li key={item.id} className={slots.item()}>
            <span className={look.item({ className: "[&_svg]:size-6 [&_svg]:shrink-0" })}>
              <span className={look.pill()}>{item.icon}</span>
              <span className={look.label()}>{item.label}</span>
            </span>
          </li>
        ))}
      </ol>
      <figcaption id={captionId} className={slots.caption()}>
        {caption}
      </figcaption>
    </figure>
  );
}
