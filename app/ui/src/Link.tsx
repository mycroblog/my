// SPDX-License-Identifier: AGPL-3.0-only
import { composeRenderProps, Link as RACLink, type LinkProps as RACLinkProps } from "react-aria-components";
import { tv, type VariantProps } from "tailwind-variants";

const link = tv({
  base: [
    "cursor-pointer rounded-sm text-accent underline-offset-3 data-hovered:text-accent-hover data-focus-visible:focus-ring",
    "data-disabled:cursor-not-allowed data-disabled:opacity-50 motion-safe:transition-colors",
  ],
  variants: {
    // Each variant sizes the link's icon, which sets no size of its own (web-client/16-look-states-and-wording.md G11).
    variant: {
      /** A link inside running text: always underlined, so color is not the only cue (WCAG 1.4.1). */
      inline: "underline data-hovered:decoration-2 [&>svg]:size-4 [&>svg]:shrink-0",
      /** A link on its own ("show more", a link to another site): a 44 px target on coarse pointers. */
      standalone:
        "inline-flex min-h-6 items-center font-medium no-underline data-hovered:underline pointer-coarse:min-h-11 [&>svg]:size-4 [&>svg]:shrink-0",
      /**
       * An item of the app's navigation: a 44 px target on coarse pointers and an icon at `size-6`, also
       * when the icon sits in an element of its own (the pill of the current item, with a badge).
       */
      navigation:
        "inline-flex min-h-6 items-center font-medium text-fg no-underline data-current:rounded-full data-current:bg-soft data-current:text-accent pointer-coarse:min-h-11 [&_svg]:size-6 [&_svg]:shrink-0",
    },
    /** The space between an icon and the text of the link (a step of `Space`). */
    gap: { 1: "gap-1", 2: "gap-2" },
    /** Wraps a long word anywhere (a domain, a handle). */
    wrap: { anywhere: "wrap-anywhere" },
    /** The text size when it differs from the text around the link. */
    size: { sm: "text-sm" },
  },
  defaultVariants: { variant: "inline" },
});

export type LinkProps = RACLinkProps &
  VariantProps<typeof link> & {
    /**
     * Leaves the link out of the tab order (`tabindex="-1"`), for a second link to the same place for the
     * pointer, such as an avatar next to the name that is the link for the keyboard. Hide it from screen
     * readers too, inside an element with `aria-hidden` (React Aria does not put `aria-hidden` on the
     * link), so that the place is reached once.
     */
    excludeFromTabOrder?: boolean;
  };

/**
 * A link (React Aria Link). With `href` it renders an `<a>`; client-side navigation comes from the
 * app's React Aria `RouterProvider`, so the kit knows no router. React Aria's useLink hands
 * `excludeFromTabOrder` to useFocusable, which sets the `tabindex`.
 */
export function Link({ variant, gap, wrap, size, className, ...props }: LinkProps) {
  return (
    <RACLink
      {...props}
      className={composeRenderProps(className, (className) => link({ variant, gap, wrap, size, className }))}
    />
  );
}
