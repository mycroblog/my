// SPDX-License-Identifier: AGPL-3.0-only
import type { ComponentPropsWithRef } from "react";

export type BannerImageProps = Omit<ComponentPropsWithRef<"img">, "className" | "style" | "alt"> & {
  /** The administrator's alt text; empty makes the image decorative. */
  alt: string;
};

/** An image at the width of the page, cropped to at most 20rem tall: the picture that represents a server. */
export function BannerImage({ alt, ...props }: BannerImageProps) {
  return (
    <img
      decoding="async"
      {...props}
      alt={alt}
      className="h-auto max-h-80 w-full rounded-xl bg-surface-sunken object-cover"
    />
  );
}
