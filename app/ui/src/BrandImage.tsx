// SPDX-License-Identifier: AGPL-3.0-only
// The server's own icon and logo (ADR-0116): SVGs that its administrators upload, which the server sanitizes and
// serves under /brand/. They are shown only as images (`<img>`), never inline, so that nothing in them runs in the
// page; `Wordmark` shows a logo, `BrandIcon` an icon.
import { tv, type VariantProps } from "tailwind-variants";

/** The server's own logo: the URL of its SVG, of the SVG for dark grounds when it has one, and the size of its viewBox. */
export type BrandLogo = {
  src: string;
  /** The logo for dark grounds; `src` is used there without it. */
  darkSrc?: string | undefined;
  width: number;
  height: number;
};

const brandIcon = tv({
  base: "block shrink-0 object-contain",
  variants: {
    size: {
      /** Fills its box, such as the mark's tile of a calling card. */
      fill: "size-full",
      /** Beside the server's name in a panel. */
      md: "size-10 rounded-lg",
    },
  },
  defaultVariants: { size: "fill" },
});

export type BrandIconProps = VariantProps<typeof brandIcon> & {
  /** The URL of the icon's SVG. */
  src: string;
};

/**
 * The server's own square icon. It is decorative: where it shows, the server's name is beside it or names the page.
 */
export function BrandIcon({ src, size }: BrandIconProps) {
  return <img src={src} alt="" decoding="async" className={brandIcon({ size })} />;
}
