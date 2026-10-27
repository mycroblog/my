// SPDX-License-Identifier: AGPL-3.0-only
import { type ComponentPropsWithRef, createContext, use, useState } from "react";

/** One rendition of an image: its URL and, when known, its width in pixels. */
export type ImageCandidate = { url: string; width?: number | undefined };

/**
 * The renditions of an image to choose from: the still ones, and for an animation the moving ones (ADR-0054 §3).
 * The browser picks one by the width it shows the image at (`srcset` and `sizes`).
 */
export type ImageSources = {
  still: readonly ImageCandidate[];
  animated?: readonly ImageCandidate[] | undefined;
};

/**
 * Whether animations play by themselves: the user's "play animations automatically" setting, which the app provides
 * at its root. Off, an animation shows its still rendition until it is hovered or tapped. On, it still stays still
 * while the system asks for reduced motion (`prefers-reduced-motion: reduce`).
 */
export const AutoplayContext = createContext(true);

/**
 * Whether the animations of a part of the page play now although they do not play by themselves: true inside a post
 * or a profile header that is hovered, focused or tapped (`useAnimationScope`). Like autoplay, it never overrides a
 * reduced-motion preference.
 */
export const AnimationsPlayContext = createContext(false);

/** Whether animations may move here: they play by themselves, or the scope they are in plays them. */
export function useAnimationsPlay(): boolean {
  const autoplay = use(AutoplayContext);
  const scope = use(AnimationsPlayContext);
  return autoplay || scope;
}

/** The `src`, `srcset` and `sizes` of a set of renditions: a `srcset` only when every one has a known width. */
function sourceAttributes(candidates: readonly ImageCandidate[], sizes: string | undefined): { src: string; srcSet?: string; sizes?: string } {
  const unique = candidates.filter((candidate, i) => candidates.findIndex((other) => other.url === candidate.url) === i);
  const src = unique[0]?.url ?? "";
  if (unique.length < 2 || sizes === undefined || unique.some((candidate) => candidate.width === undefined)) return { src };
  return { src, srcSet: unique.map((candidate) => `${candidate.url} ${String(candidate.width)}w`).join(", "), sizes };
}

export type PictureProps = Omit<ComponentPropsWithRef<"img">, "src" | "srcSet" | "sizes"> & {
  sources: ImageSources;
  /** The width the image is shown at, as the `sizes` attribute; without it, the first rendition is loaded. */
  sizes?: string | undefined;
};

/**
 * An image in the rendition that fits: a plain `<img>` for a still image, and for an animation a `<picture>` whose
 * moving `<source>` applies only without a reduced-motion preference. When animations do not play by themselves
 * (`AutoplayContext`), the moving `<source>` is there only while the pointer is over the image or the scope it is in
 * plays (`AnimationsPlayContext`); otherwise the still rendition shows.
 */
export function Picture({ sources, sizes, onPointerEnter, onPointerLeave, ...props }: PictureProps) {
  const plays = useAnimationsPlay();
  const [hovered, setHovered] = useState(false);
  const still = sourceAttributes(sources.still, sizes);
  const animated = sources.animated;
  if (animated === undefined || animated.length === 0) return <img {...props} {...still} onPointerEnter={onPointerEnter} onPointerLeave={onPointerLeave} />;
  const moving = sourceAttributes(animated, sizes);
  // The <img> stays in place whether the <source> is there or not: the browser picks the rendition again when a
  // <source> comes or goes, and the pointer stays over the same element.
  return (
    <picture>
      {(plays || hovered) && (
        <source media="(prefers-reduced-motion: no-preference)" srcSet={moving.srcSet ?? moving.src} {...(moving.sizes === undefined ? {} : { sizes: moving.sizes })} />
      )}
      <img
        {...props}
        {...still}
        onPointerEnter={(event) => {
          setHovered(true);
          onPointerEnter?.(event);
        }}
        onPointerLeave={(event) => {
          setHovered(false);
          onPointerLeave?.(event);
        }}
      />
    </picture>
  );
}
