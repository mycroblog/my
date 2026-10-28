// SPDX-License-Identifier: AGPL-3.0-only
import { type ComponentPropsWithRef, type ReactNode, useState } from "react";
import { ToggleButton } from "react-aria-components";
import { tv } from "tailwind-variants";
import { Blurhash } from "./Blurhash";
import { HideMediaIcon } from "./Icons";
import { ExternalLink } from "./ExternalLink";
import { VisuallyHidden } from "./VisuallyHidden";

const linkCard = tv({
  slots: {
    // Positioned, so that the link's stretched hit area covers the whole card.
    base: "relative flex w-full flex-col overflow-hidden rounded-lg border border-border bg-surface-raised",
    media: "relative aspect-video w-full overflow-hidden border-b border-border bg-surface-sunken",
    // Above the blur, which is positioned: the image covers it as it loads.
    image: "relative size-full object-cover",
    blur: "absolute inset-0",
    text: "flex min-w-0 flex-col gap-0.5 px-3 py-2",
    site: "min-w-0 truncate text-xs font-bold text-fg-muted",
    // The link's ::after covers the card, so that the whole card is one link without nesting the toggle in it.
    link: [
      "min-w-0 font-extrabold wrap-anywhere text-fg no-underline after:absolute after:inset-0 after:rounded-lg",
      "data-hovered:text-fg data-hovered:underline data-focus-visible:underline",
    ],
    description: "line-clamp-2 min-w-0 text-sm wrap-anywhere text-fg-muted",
    byline: "min-w-0 text-xs wrap-anywhere text-fg-muted",
    // Above the link's hit area, so that it stays its own control.
    toggle:
      "absolute z-10 cursor-default rounded-full bg-surface-raised font-extrabold text-fg select-none data-focus-visible:focus-ring data-hovered:bg-surface-sunken motion-safe:transition-colors",
  },
  variants: {
    revealed: {
      false: {
        toggle:
          "inset-0 m-auto size-fit min-h-10 max-w-[calc(100%-1rem)] px-4 py-2 text-sm shadow-md pointer-coarse:min-h-11",
      },
      true: {
        toggle: "inset-s-1.5 top-1.5 grid size-8 place-items-center shadow-sm pointer-coarse:size-11 [&>svg]:size-5",
      },
    },
  },
});

/** The image of a link card. */
export type LinkCardImage = {
  /** The image URL (the media proxy's small rendition). */
  src: string;
  /** The size of the image in pixels, so that the browser reserves its room. */
  width: number;
  height: number;
  /** The blurhash of the image, drawn while it is concealed or loading. */
  blurhash?: string | undefined;
  /** The page's description of the image, or undefined when it has none: the image is then decorative. */
  alt?: string | undefined;
};

export type LinkCardProps = Omit<ComponentPropsWithRef<"div">, "children" | "title"> & {
  /** The page the card links to. It opens in a new tab, without a referrer. */
  href: string;
  /** The title of the page. */
  title: string;
  /** The description of the page; nothing is shown when it is empty. */
  description?: string | undefined;
  /** The name of the site (its host when the page names none). */
  siteName: string;
  /** The language of the page, for the title, description and site name. */
  lang?: string | undefined;
  /** The image of the page. */
  image?: LinkCardImage | undefined;
  /** One line under the description: who wrote the page and when. */
  byline?: ReactNode;
  /** Whether the image is sensitive: it is concealed until the reader shows it. */
  sensitive?: boolean;
  /** The name of the toggle button that shows a sensitive image ("Show sensitive media"); only used when `sensitive`. */
  revealLabel: string;
  /** Whether a sensitive image is shown at first. */
  defaultRevealed?: boolean;
};

/**
 * The card of a link in a post: the image, site name, title, description and byline of the linked page. The whole
 * card is one link to the page (the title's link stretches over the card), an ExternalLink that opens in a new tab
 * with `rel="noopener noreferrer nofollow"`. The page's texts are user content, shown with `dir="auto"` and the
 * page's language (web-client/README.md Y10). A sensitive image is concealed behind a toggle button above the link's area, as in
 * MediaGrid: while concealed, the image is not loaded and its blur stands in for it.
 */
export function LinkCard({
  href,
  title,
  description,
  siteName,
  lang,
  image,
  byline,
  sensitive = false,
  revealLabel,
  defaultRevealed = false,
  className,
  ...props
}: LinkCardProps) {
  const [shown, setShown] = useState(defaultRevealed);
  const [loaded, setLoaded] = useState(false);
  const revealed = !sensitive || shown;
  const slots = linkCard({ revealed });
  return (
    <div {...props} className={slots.base({ className })}>
      {image !== undefined && (
        <div className={slots.media()}>
          {image.blurhash !== undefined && !(revealed && loaded) && (
            <Blurhash hash={image.blurhash} className={slots.blur()} />
          )}
          {revealed && (
            <img
              src={image.src}
              width={image.width}
              height={image.height}
              alt={image.alt ?? ""}
              {...(image.alt !== undefined && { dir: "auto", lang })}
              loading="lazy"
              decoding="async"
              onLoad={() => {
                setLoaded(true);
              }}
              className={slots.image()}
            />
          )}
          {sensitive && (
            <ToggleButton isSelected={revealed} onChange={setShown} className={slots.toggle()}>
              {revealed ? (
                <>
                  <HideMediaIcon />
                  <VisuallyHidden>{revealLabel}</VisuallyHidden>
                </>
              ) : (
                revealLabel
              )}
            </ToggleButton>
          )}
        </div>
      )}
      <div className={slots.text()}>
        <span dir="auto" lang={lang} className={slots.site()}>
          {siteName}
        </span>
        <ExternalLink href={href} rel="nofollow" icon={false} className={slots.link()}>
          <span dir="auto" lang={lang}>
            {title}
          </span>
        </ExternalLink>
        {description !== undefined && description !== "" && (
          <p dir="auto" lang={lang} className={slots.description()}>
            {description}
          </p>
        )}
        {byline !== undefined && byline !== null && <p className={slots.byline()}>{byline}</p>}
      </div>
    </div>
  );
}
