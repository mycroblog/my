// SPDX-License-Identifier: AGPL-3.0-only
import { useId } from "react";
import { tv } from "tailwind-variants";
import type { BrandLogo } from "./BrandImage";
import { Link } from "./Link";
import { Wordmark } from "./Wordmark";

const serverNameLink = tv({
  slots: {
    base: "",
    link: "max-w-full font-extrabold text-fg wrap-anywhere",
    // The link around a logo is as wide as the logo, and its focus ring follows the logo's box.
    logoLink: "flex max-w-full rounded-sm",
  },
  variants: {
    size: {
      base: {},
      // The name as the title of a column (the sidebar, the third column), lined up with the column's direction.
      lg: { base: "text-lg align-match-parent" },
    },
  },
  defaultVariants: { size: "base" },
});

export type ServerNameLinkProps = {
  /** The server's name: the administrator's text, in any language (web-client/README.md Y10). */
  name: string;
  /** Describes the link ("The server's top page"). */
  description: string;
  /** Whether the user is on the page the link leads to (`aria-current="page"`). */
  current?: boolean;
  /** `lg` where the name heads a column (the sidebar, the third column); `base` in running content (the menu sheet). */
  size?: "base" | "lg";
  /** The server's own logo, which shows in the link in place of the name (the name is its alt text). */
  logo?: BrandLogo | undefined;
};

/**
 * The server's name as a link to the root address, in a paragraph of its own that takes the name's direction
 * (`dir="auto"`). It is named by the name it shows and described by `description`. With the server's own `logo`, the
 * link shows the logo, an image named by the server's name, instead of the text.
 */
export function ServerNameLink({ name, description, current = false, size, logo }: ServerNameLinkProps) {
  const slots = serverNameLink({ size });
  const descriptionId = useId();
  return (
    <p dir="auto" className={slots.base()}>
      <Link
        href="/"
        {...(current && { "aria-current": "page" as const })}
        aria-describedby={descriptionId}
        variant="standalone"
        className={logo === undefined ? slots.link() : slots.logoLink()}
      >
        {logo === undefined ? name : <Wordmark name={name} decorative={false} size="sm" logo={logo} />}
      </Link>
      {/* A description of a hidden element is still read (accname), and the element stays out of the tree. */}
      <span id={descriptionId} hidden>
        {description}
      </span>
    </p>
  );
}
