// SPDX-License-Identifier: AGPL-3.0-only
// "N new posts" (web-client/README.md Y7, ADR-0110, ADR-0013): new items wait above a feed until the reader asks for them,
// because content that moves on its own breaks WCAG 2.2.2 and takes the reader's place away. A caller whose reader chose
// to have them inserted automatically uses the button for the count of what was inserted while the reader is away from
// the top, and for what is held back while the reader is in the feed; the count is announced the same way.
import type { ComponentPropsWithRef } from "react";
import { tv } from "tailwind-variants";
import { Button } from "./Button";
import { NewItemsIcon } from "./Icons";

const newItemsButton = tv({
  slots: {
    // Never display: none, even when empty: a live region must stay rendered to announce the next count.
    base: "flex justify-center",
    button:
      "rounded-full bg-accent-fill text-accent-fill-fg shadow-md data-hovered:bg-accent-fill-hover data-pressed:bg-accent-fill-hover",
  },
  variants: {
    sticky: {
      // The region has no height, so the posts below never move and no space is kept above the newest one: the button
      // stands just above the region's edge (in the page's top padding at the top of the page) and, once the reader
      // scrolls, floats over the feed's top. Only the button takes pointer input, so nothing under it is blocked.
      true: {
        base: "pointer-events-none sticky top-[calc(env(safe-area-inset-top)+2.75rem)] z-10 h-0 pointer-coarse:top-[calc(env(safe-area-inset-top)+3.75rem)] *:pointer-events-auto",
        button: "absolute bottom-0 pointer-coarse:min-h-11",
      },
    },
  },
});

export type NewItemsButtonProps = Omit<ComponentPropsWithRef<"div">, "children" | "role"> & {
  /** How many new items are waiting. At zero the button is not drawn; the status region stays in the page. */
  count: number;
  /** The button's text for this count, formatted by the caller with a plural message ("3 new posts"). */
  label: string;
  /** Shows the waiting items: the caller inserts them and moves the reader to the top of the feed. */
  onPress: () => void;
  /** Floats the button over the feed's top while the reader scrolls, in a region with no height of its own. */
  sticky?: boolean;
};

/**
 * A button that shows how many new items are waiting and inserts them only when pressed. It sits in a
 * polite `role="status"` region that is in the page from the first render, so a screen reader reads the
 * new count when it changes without the button taking focus.
 */
export function NewItemsButton({ count, label, onPress, sticky, className, ...props }: NewItemsButtonProps) {
  const slots = newItemsButton({ sticky });
  return (
    <div {...props} role="status" className={slots.base({ className })}>
      {count > 0 && (
        <Button variant="primary" size="sm" onPress={onPress} className={slots.button()}>
          <NewItemsIcon />
          {label}
        </Button>
      )}
    </div>
  );
}
