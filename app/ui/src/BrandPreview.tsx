// SPDX-License-Identifier: AGPL-3.0-only
// How a server's own icon or logo will look where it is used (ADR-0116 §3), before an administrator saves it: in a
// browser tab, on a phone's home screen, as a maskable app icon, in the header of an email on a light and on a dark
// ground, and as a link preview. Each preview is an <img> of a drawn PNG or of the sanitized SVG (never an inline SVG),
// decorative: the caption under it says what it shows.
import type { ReactNode } from "react";
import { tv, type VariantProps } from "tailwind-variants";

const brandPreview = tv({
  slots: {
    figure: "flex min-w-0 flex-col items-center gap-2",
    frame: "flex items-center justify-center overflow-hidden",
    image: "block",
    caption: "text-center text-sm text-fg-muted",
  },
  variants: {
    place: {
      /** The image itself, as large as a heading's mark, on a sunken ground. */
      image: {
        frame: "h-24 max-w-full rounded-lg bg-surface-sunken p-3",
        image: "h-full w-auto max-w-full object-contain",
      },
      /** A favicon in a tab: the PNG at half its pixels, beside a stand-in of a tab's title. */
      tab: { frame: "h-10 gap-2 rounded-t-lg border border-b-0 border-border bg-surface-raised px-3", image: "size-4" },
      /** The iOS home screen: the opaque PNG in a rounded square. */
      homeScreen: { frame: "size-16 rounded-2xl shadow-card", image: "size-full" },
      /** The maskable app icon in a circle, the tightest mask Android applies. */
      maskable: { frame: "size-16 rounded-full shadow-card", image: "size-full" },
      /** An email's header on a light ground: the PNG at half its pixels (48 px high). */
      mail: {
        frame: "h-18 w-full max-w-md rounded-lg border border-border bg-surface-raised px-4 scheme-light",
        image: "h-12 w-auto max-w-full object-contain",
      },
      /** An email's header on a dark ground. */
      darkMail: {
        frame: "h-18 w-full max-w-md rounded-lg border border-border bg-surface-raised px-4 scheme-dark",
        image: "h-12 w-auto max-w-full object-contain",
      },
      /** A link preview: the opaque 1200 × 630 PNG, as wide as the room. */
      linkPreview: { frame: "w-full max-w-md rounded-lg border border-border", image: "aspect-1200/630 w-full" },
    },
  },
});

export type BrandPreviewProps = Required<VariantProps<typeof brandPreview>> & {
  /** The URL of the PNG or SVG to show (a blob: URL before saving). */
  src: string;
  /** What the preview shows ("Browser tab"). */
  caption: ReactNode;
};

/** One preview of a brand image, with its caption. */
export function BrandPreview({ place, src, caption }: BrandPreviewProps) {
  const slots = brandPreview({ place });
  return (
    <figure className={slots.figure()}>
      <span className={slots.frame()}>
        <img src={src} alt="" decoding="async" className={slots.image()} />
        {/* A tab's title, as a gray bar: the tab is recognized by its shape. */}
        {place === "tab" && <span aria-hidden="true" className="h-2 w-16 rounded-full bg-border" />}
      </span>
      <figcaption className={slots.caption()}>{caption}</figcaption>
    </figure>
  );
}

const brandPreviewList = tv({ base: "flex flex-wrap items-end gap-6" });

/** The previews of one image, side by side, wrapping on a narrow screen. */
export function BrandPreviewList({ children }: { children: ReactNode }) {
  return <div className={brandPreviewList()}>{children}</div>;
}
