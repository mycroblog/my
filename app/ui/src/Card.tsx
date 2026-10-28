// SPDX-License-Identifier: AGPL-3.0-only
// The raised surface that side panels, settings panes and detail sections sit on.
import { createElement } from "react";
import { tv } from "tailwind-variants";
import type { PolymorphicProps } from "./layout";

/** The elements a card may render. */
export type CardElement = "div" | "section" | "article" | "aside" | "li";

const card = tv({
  // A raised face of the soft surfaces: lifted by its shadow, not outlined. Forced-colors mode drops the shadow, so the
  // border comes back there.
  base: "rounded-xl bg-surface-raised shadow-soft-raised forced-colors:border forced-colors:border-[CanvasText]",
  variants: {
    padding: { "0": "p-0", "3": "p-3", "4": "p-4", "6": "p-6", "8": "p-8" },
  },
});

type CardOwnProps = {
  /**
   * The inner space, a spacing step: `4` for a side panel or a compact block, `6` for a panel of a page, `8` for a
   * settings pane; `0` when rows inside bring their own padding (a list of rows divided by `border`).
   */
  padding?: "0" | "3" | "4" | "6" | "8";
};

export type CardProps<E extends CardElement = "div"> = PolymorphicProps<E, CardOwnProps>;

/**
 * A raised surface: `surface-raised`, `rounded-xl` and `shadow-soft-raised` (a 1px border in forced-colors mode). Lay it
 * on the page, never on another card.
 */
export function Card<E extends CardElement = "div">({ as, padding = "6", ...rest }: CardProps<E>) {
  return createElement(as ?? "div", { ...rest, className: card({ padding }) });
}
