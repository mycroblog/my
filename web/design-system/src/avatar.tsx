// SPDX-License-Identifier: AGPL-3.0-only

// The avatar: an account's image in a circle, with no edge, on a light backing in both schemes, so that a transparent
// image sits on the avatar's own surface and a dark logo stays seen in dark. An image may blend into a surface of its own tone; no line is drawn around it.
// Without an image, or when the image fails to load, the first letter of the display name stands in. Its accessible
// name is the display name; where the name sits next to it (the post header, a reply context) it is decorative and
// hidden, so the name is read once. React Aria has no avatar or image primitive, so this is plain markup.

import { useState } from "react";
import { tv } from "tailwind-variants/lite";
import { Box, Row } from "./layout";

/**
 * Where an avatar sits, which gives its size: inline beside a name, at the start of a list's row on one line (a
 * suggestion), at the head of a post, or in a profile preview.
 */
export type AvatarSize = "inline" | "row" | "post" | "preview";

export type AvatarProps = {
  /** The account's display name: the accessible name, and the initial without an image. */
  name: string;
  /** The image; without one, or when it fails to load, the initial stands in. */
  src?: string | undefined;
  size: AvatarSize;
  /** The name sits next to the avatar, which is then hidden from assistive technology. */
  decorative?: boolean;
};

const avatar = tv({
  slots: {
    // On a raised surface (a section of a settings screen), it takes the page's surface, so that its circle shows.
    base: "rounded-full bg-surface-raised [[data-surface-raised]_&]:bg-surface",
    // An image sits on the light avatar-backing in both schemes, so that a dark logo on transparency stays seen.
    image: "object-cover bg-avatar-backing",
    // The initial's own surface, over the backing: the place for a color of the account's own, which would tell two
    // accounts with the same initial apart.
    fallback: "bg-surface-raised leading-none font-label text-text [[data-surface-raised]_&]:bg-surface",
    // A decorative avatar's initial is drawn, not written: inside a link named by its own words (a row of faces), a
    // written initial would be visible text missing from that name.
    drawn: "before:content-[attr(data-initial)]",
  },
  variants: {
    size: {
      inline: {
        base: "size-avatar-inline",
        fallback: "text-[length:calc(var(--avatar-inline)*var(--avatar-initial-small))]",
      },
      row: { base: "size-avatar-row", fallback: "text-[length:calc(var(--avatar-row)*var(--avatar-initial-small))]" },
      post: { base: "size-avatar-post", fallback: "text-[length:calc(var(--avatar-post)*var(--avatar-initial))]" },
      preview: {
        base: "size-avatar-preview",
        fallback: "text-[length:calc(var(--avatar-preview)*var(--avatar-initial))]",
      },
    },
  },
});

const segmenter = new Intl.Segmenter(undefined, { granularity: "grapheme" });

/** The first letter or digit of a name, as one grapheme, so that a ja name gives its first kanji or kana. */
function initialOf(name: string): string {
  for (const { segment } of segmenter.segment(name)) if (/[\p{L}\p{N}]/u.test(segment)) return segment.toUpperCase();
  return "";
}

export function Avatar({ name, src, size, decorative = false }: AvatarProps) {
  // The source that failed, so that a new source is tried again.
  const [failed, setFailed] = useState<string>();
  const s = avatar({ size });
  const shown = src !== undefined && src !== "" && src !== failed;
  return (
    <Row
      as="span"
      inline
      rigid
      clip
      baseline="middle"
      className={s.base()}
      {...(decorative ? { "aria-hidden": true } : { role: "img", "aria-label": name })}
    >
      {shown ? (
        <Box
          full="both"
          className={s.image()}
          as={
            <img
              src={src}
              alt=""
              onError={() => {
                setFailed(src);
              }}
            />
          }
        />
      ) : decorative ? (
        <Row
          as="span"
          full="both"
          align="center"
          justify="center"
          className={`${s.fallback()} ${s.drawn()}`}
          data-initial={initialOf(name)}
        />
      ) : (
        <Row as="span" full="both" align="center" justify="center" className={s.fallback()}>
          {initialOf(name)}
        </Row>
      )}
    </Row>
  );
}
