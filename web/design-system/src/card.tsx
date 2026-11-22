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
import { Box, Column, Row, Stack } from "./layout";
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
  /** A sensitive post's image is shown at first; covered by default. */
  defaultShown?: boolean | undefined;
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
      "box-border rounded-control bg-surface-raised text-text no-underline outline-hidden",
      "data-[hovered]:bg-surface-raised-hover hover:bg-surface-raised-hover focus-visible:shadow-focus",
      "active:translate-y-press active:bg-surface-raised-pressed",
    ],
    figure: "bg-surface-raised-pressed",
    image: "object-cover",
    text: "p-embed",
    site: "text-caption text-muted-text",
    title: "text-body font-label",
    description: "text-caption text-muted-text",
    author: "text-caption text-muted-text",
  },
  variants: {
    size: {
      large: {},
      small: {
        figure: "aspect-square w-media-card-thumb",
        text: "py-embed-gap",
      },
    },
    covered: { true: { image: "cursor-pointer" } },
  },
});

/** The line of the page's author: an inline avatar and the name, which ends in an ellipsis where the line is narrower. */
function AuthorLine({ className, children }: { className: string; children: ReactNode }) {
  return (
    <Row
      as="span"
      align="center"
      gap="entry-caption"
      spaceAbove="embed-gap"
      className={`${className} [&>bdi]:truncate`}
      aria-hidden="true"
    >
      {children}
    </Row>
  );
}

export function LinkCard({
  href,
  title,
  description,
  site,
  image,
  size = "large",
  sensitive = false,
  defaultShown = false,
  author,
  lang,
}: LinkCardProps) {
  const { lang: reader, direction } = useReader(undefined);
  const { t } = stringsIn(reader);
  const [shown, setShown] = useState(defaultShown);
  const id = useId();
  const coverable = sensitive && image !== undefined;
  const covered = coverable && !shown;
  const s = card({ size, covered });
  // The large image is never taller than 16:9: a taller one is cropped to it, a wider one keeps its shape.
  const ratio = image === undefined ? undefined : Math.max(image.width / image.height, 16 / 9);
  // The image above the text, or beside it.
  const Layout = size === "large" ? Column : Row;
  const link = (
    <Layout
      as={<a id={id} href={href} target="_blank" rel="noopener noreferrer nofollow" dir={direction} />}
      clip
      className={s.root()}
    >
      {image !== undefined && (
        <Box
          as="span"
          anchor
          clip
          rigid
          full={size === "large"}
          max={size === "large" ? "media-card" : undefined}
          self={size === "small" ? "start" : undefined}
          ratio={size === "large" ? ratio : undefined}
          className={s.figure()}
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
            <Box as={<Blurhash hash={image.blurhash} />} display="block" full="both" className={s.image()} />
          ) : (
            <Box as={<img src={image.src} alt={image.alt ?? ""} />} display="block" full="both" className={s.image()} />
          )}
        </Box>
      )}
      <Stack as="span" grow pack="start" gap="hair" className={s.text()} lang={lang}>
        <Box as="bdi" lines={1} className={s.site()} dir="auto">
          {site}
        </Box>
        <Box as="span" lines={2} className={s.title()} dir="auto">
          {title}
        </Box>
        {description !== "" && (
          <Box as="span" lines={2} className={s.description()} dir="auto">
            {description}
          </Box>
        )}
        {author !== undefined && (
          <>
            <AuthorLine className={s.author()}>{author.shown}</AuthorLine>
            <span className="sr-only">{t("card.author", { name: author.name })}</span>
          </>
        )}
      </Stack>
      <span className="sr-only"> {t("media.new_tab")}</span>
    </Layout>
  );
  if (!coverable) return link;
  return (
    <Stack gap="media" dir={direction}>
      <SensitiveLine
        label={sensitiveLabel([{ kind: "image" }], t)}
        shown={shown}
        controls={id}
        onToggle={() => {
          setShown(!shown);
        }}
      />
      {link}
    </Stack>
  );
}
