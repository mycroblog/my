// SPDX-License-Identifier: AGPL-3.0-only

// The card of a link in an entry (a post's first link, ADR-0062 §8): one link to the page on the raised surface, as
// something embedded is, never outweighing the post. `large` puts the page's image above its text, cropped to at most
// 16:9 and a height; `small` puts it beside the text as a square at the start. The text is the site's name, muted,
// over the title, strong, over the description, muted, each in at most two lines (the site's in one), in the page's
// language and their own direction; the page's author, when its server confirms them, ends it, read "by" its name. It
// opens the page in a new tab, and says so. Without an image it is its text alone. A sensitive post's card image is
// covered as sensitive media is (media.tsx): the line above the card, outside the link, and the image's blurhash in its
// place, nothing of it loaded until shown, a press on it showing it rather than opening the page. The
// image has no border and blends into the surface; it is described by the page's own description of it, or is
// decorative. The link is named by what it shows, in its order: the site, the title, the description, the author.

import { useId, useState, type ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { Blurhash } from "./blurhash";
import { stringsIn, useReader } from "./locale";
import { SensitiveLine, sensitiveLabel } from "./media";

export type LinkCardProps = {
  /** The page. */
  href: string;
  title: string;
  /** Empty for none. */
  description: string;
  /** The site's name, or the host of the page. */
  site: string;
  /** The page's image, through the media proxy, with its size, its own description and its blurhash. */
  image?:
    { src: string; width: number; height: number; alt?: string | undefined; blurhash?: string | undefined } | undefined;
  /** The image above the text, or a square beside it (the reader's preference). */
  size?: "large" | "small" | undefined;
  /** The post is sensitive: the image is covered, its blurhash in its place, until the reader shows it. */
  sensitive?: boolean | undefined;
  /**
   * The page's author, as its server confirms them: what shows (an inline avatar and the name), and the name as text,
   * which a screen reader reads after "by".
   */
  author?: { shown: ReactNode; name: string } | undefined;
  /** The page's language, when it says it. */
  lang?: string | undefined;
};

const card = tv({
  slots: {
    root: [
      "box-border flex overflow-hidden rounded-control bg-surface-raised text-text no-underline outline-hidden",
      "data-[hovered]:bg-surface-raised-hover hover:bg-surface-raised-hover focus-visible:shadow-focus",
    ],
    figure: "relative m-0 shrink-0 overflow-hidden bg-surface-raised-pressed",
    image: "block size-full object-cover",
    covered: "grid grid-cols-[minmax(0,1fr)] gap-media",
    text: "grid min-w-0 flex-1 content-start gap-[0.125rem] p-embed",
    site: "truncate text-caption text-muted-text",
    title: "line-clamp-2 text-body font-label",
    description: "line-clamp-2 text-caption text-muted-text",
    author: [
      "mt-embed-gap flex min-w-0 items-center gap-entry-caption text-caption text-muted-text",
      "[&>bdi]:min-w-0 [&>bdi]:truncate",
    ],
  },
  variants: {
    size: {
      large: {
        root: "flex-col",
        figure: "max-h-media-card-max w-full",
      },
      small: {
        root: "flex-row items-stretch",
        figure: "aspect-square w-media-card-thumb self-start",
        text: "py-embed-gap",
      },
    },
    covered: { true: { image: "cursor-pointer" } },
  },
});

export function LinkCard({
  href,
  title,
  description,
  site,
  image,
  size = "large",
  sensitive = false,
  author,
  lang,
}: LinkCardProps) {
  const { lang: reader, direction } = useReader(undefined);
  const { t } = stringsIn(reader);
  const [shown, setShown] = useState(false);
  const id = useId();
  const coverable = sensitive && image !== undefined;
  const covered = coverable && !shown;
  const s = card({ size, covered });
  // The large image is never taller than 16:9: a taller one is cropped to it, a wider one keeps its shape.
  const ratio = image === undefined ? undefined : Math.max(image.width / image.height, 16 / 9);
  const link = (
    <a id={id} className={s.root()} href={href} target="_blank" rel="noopener noreferrer nofollow" dir={direction}>
      {image !== undefined && (
        <span
          className={s.figure()}
          style={size === "large" && ratio !== undefined ? { aspectRatio: String(ratio) } : undefined}
          // A press on the covered image shows it, as the line above does, instead of opening the page.
          onClick={
            covered
              ? (e) => {
                  e.preventDefault();
                  setShown(true);
                }
              : undefined
          }
        >
          {covered ? (
            // Nothing of a covered image loads: its blurhash stands in for it.
            <Blurhash hash={image.blurhash} className={s.image()} />
          ) : (
            <img className={s.image()} src={image.src} alt={image.alt ?? ""} />
          )}
        </span>
      )}
      <span className={s.text()} lang={lang}>
        <bdi className={s.site()} dir="auto">
          {site}
        </bdi>
        <span className={s.title()} dir="auto">
          {title}
        </span>
        {description !== "" && (
          <span className={s.description()} dir="auto">
            {description}
          </span>
        )}
        {author !== undefined && (
          <>
            <span className={s.author()} aria-hidden="true">
              {author.shown}
            </span>
            <span className="sr-only">{t("card.author", { name: author.name })}</span>
          </>
        )}
      </span>
      <span className="sr-only"> {t("media.new_tab")}</span>
    </a>
  );
  if (!coverable) return link;
  return (
    <div className={s.covered()} dir={direction}>
      <SensitiveLine
        label={sensitiveLabel([{ kind: "image" }], t)}
        shown={shown}
        controls={id}
        onToggle={() => {
          setShown(!shown);
        }}
      />
      {link}
    </div>
  );
}
