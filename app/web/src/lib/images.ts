// SPDX-License-Identifier: AGPL-3.0-only
// The renditions of a media file, as @my/ui's Picture chooses among them (ADR-0054 §1, §5). The types have the
// shape of Picture's ImageSources; lib does not import the kit (web-client/README.md A1–A3).
import type { components } from "@mycroblog/mykit";

type ImageCandidate = { url: string; width?: number | undefined };
type ImageSources = { still: readonly ImageCandidate[]; animated?: readonly ImageCandidate[] | undefined };

type MediaVariants = components["schemas"]["MediaVariants"];
type MediaVariant = components["schemas"]["MediaVariant"];

function candidate(variant: MediaVariant): ImageCandidate {
  return variant.width === undefined ? { url: variant.url } : { url: variant.url, width: variant.width };
}

/**
 * The renditions of an image (an avatar, a header, an attachment), smallest first, with its moving ones for an
 * animation; undefined for no image.
 */
export function imageSources(variants: MediaVariants | undefined): ImageSources | undefined {
  if (variants === undefined) return undefined;
  const still = [variants.small, variants.medium, variants.large].map(candidate);
  const animated = variants.animated;
  if (animated === undefined) return { still };
  return { still, animated: [animated.small, animated.medium, animated.large].map(candidate) };
}

/** An image at its original size, as the image viewer shows it: its renditions and, when known, its size. */
type OriginalImage = { sources: ImageSources; width?: number | undefined; height?: number | undefined };

/**
 * The original size of an image for the image viewer: the `large` rendition of an upload (still, and moving for an
 * animation), or `url` for a file without renditions (a remote file, through the media proxy) at the size its server
 * declared (`meta`); undefined without either.
 */
export function originalImage(
  variants: MediaVariants | undefined,
  url: string | undefined,
  meta: { width?: number | undefined; height?: number | undefined },
): OriginalImage | undefined {
  if (variants !== undefined) {
    const { large, animated } = variants;
    return {
      sources:
        animated === undefined
          ? { still: [{ url: large.url }] }
          : { still: [{ url: large.url }], animated: [{ url: animated.large.url }] },
      width: large.width,
      height: large.height,
    };
  }
  return url === undefined ? undefined : { sources: { still: [{ url }] }, width: meta.width, height: meta.height };
}
