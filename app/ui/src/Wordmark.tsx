// SPDX-License-Identifier: AGPL-3.0-only
import type { ComponentPropsWithRef } from "react";
import { tv, type VariantProps } from "tailwind-variants";
import type { BrandLogo } from "./BrandImage";
import darkBackground from "./logos/Wordmark-DarkBg.svg";
import monochrome from "./logos/Wordmark-Monochrome.svg";
import primaryColor from "./logos/Wordmark-Primary-Color.svg";

const wordmark = tv({
  slots: {
    base: "inline-block shrink-0 align-middle leading-none",
    image: "h-full w-auto",
    dark: "hidden dark:block",
    light: "block dark:hidden",
  },
  variants: {
    // The artwork is 1250 x 700, so the height sets the width.
    size: {
      sm: { base: "h-8" },
      md: { base: "h-12" },
      lg: { base: "h-16" },
    },
    // A server's own logo is 1 to 8 times as wide as it is high: in a narrow space it shrinks to the room there,
    // keeping its proportions, against the start.
    own: {
      true: {
        base: "max-w-full min-w-0 shrink",
        image: "max-w-full object-contain object-left rtl:object-right",
      },
    },
  },
  defaultVariants: { size: "md" },
});

export type WordmarkProps = Omit<ComponentPropsWithRef<"span">, "children"> &
  VariantProps<typeof wordmark> & {
    /**
     * The accessible name of a standalone wordmark: the product name ("my"), or the server's name for its own `logo`.
     * Ignored when `decorative`.
     */
    name: string;
    /**
     * Whether the wordmark is decorative. Pass `true` when the server name or the product name is shown as text
     * next to it. Pass `false` for a standalone logo, which is then an image named `name`.
     */
    decorative: boolean;
    /**
     * The artwork. `adaptive` (default) is the coral wordmark on a light ground and the white one on a dark ground,
     * following the theme. `monochrome` is the single charcoal ink, for a ground that is neither.
     */
    tone?: "adaptive" | "monochrome";
    /**
     * The server's own logo, shown in place of the built-in artwork (its dark variant on a dark ground). It includes
     * the server's name, so `name` is the server's name and no text goes beside it.
     */
    logo?: BrandLogo | undefined;
  };

/**
 * The hand-lettered "my" with its three-stroke spark, drawn as given in `assets/Logos` (never redrawn or recolored:
 * the inks are fixed in the files, so the variant that matches the ground is picked here), or the server's own logo
 * in its place. A logo is an SVG shown as an image, never inline.
 */
export function Wordmark({ name, decorative, size, tone = "adaptive", logo, className, ...props }: WordmarkProps) {
  const { base, image, dark, light } = wordmark({ size, own: logo !== undefined });
  const alt = decorative ? "" : name;
  if (logo !== undefined) {
    const sized = { width: logo.width, height: logo.height, decoding: "async" as const };
    if (logo.darkSrc === undefined) {
      return (
        <span {...props} className={base({ className })}>
          <img src={logo.src} alt={alt} {...sized} className={image()} />
        </span>
      );
    }
    // Two pictures, one shown per ground: the box is the one image, named once whichever shows.
    return (
      <span {...props} {...(!decorative && { role: "img", "aria-label": name })} className={base({ className })}>
        <img src={logo.src} alt="" {...sized} className={image({ className: light() })} />
        <img src={logo.darkSrc} alt="" {...sized} className={image({ className: dark() })} />
      </span>
    );
  }
  return (
    <span {...props} className={base({ className })}>
      {tone === "monochrome" ? (
        <img src={monochrome} alt={alt} className={image()} />
      ) : (
        <>
          <img src={primaryColor} alt={alt} className={image({ className: light() })} />
          <img src={darkBackground} alt={alt} className={image({ className: dark() })} />
        </>
      )}
    </span>
  );
}
