// SPDX-License-Identifier: AGPL-3.0-only
// The one place where a link of the kit opens in a new tab (web-client/README.md Y12): every link that leaves the app
// (another site, the server's own pages outside the app, the links of server-rendered HTML in PostContent) gets
// target="_blank", rel="noopener noreferrer" and, for assistive technology, the app's "opens in a new tab" text.
import { createContext, type ReactNode, use } from "react";
import { composeRenderProps } from "react-aria-components";
import { tv } from "tailwind-variants";
import { ExternalLinkIcon } from "./Icons";
import { Link, type LinkProps } from "./Link";
import { VisuallyHidden } from "./VisuallyHidden";

/** The app-wide settings of the links that leave the app, which an app renders once at its root. */
export type ExternalLinkSettings = {
  /**
   * The text that assistive technology reads after the name of every link that opens in a new tab, such as
   * "(opens in a new tab)"; nothing is read when it is undefined.
   */
  newTabLabel: string | undefined;
  /**
   * Whether a link of server-rendered HTML (PostContent) leads to one of the app's own pages, which the app opens
   * in the same tab. By default only a link to the page's own origin does.
   */
  isAppLink: (url: URL) => boolean;
};

export const ExternalLinkContext = createContext<ExternalLinkSettings>({
  newTabLabel: undefined,
  isAppLink: (url) => url.origin === globalThis.location.origin,
});

/** The rel of a link that opens in a new tab: `noopener noreferrer`, with the tokens of `rel` (`nofollow`, `ugc`) kept. */
export function externalRel(rel?: string): string {
  const tokens = new Set(["noopener", "noreferrer"]);
  for (const token of (rel ?? "").split(/\s+/)) {
    if (token !== "") tokens.add(token.toLowerCase());
  }
  return [...tokens].join(" ");
}

/** The gap between the name and the icon, when the link has the icon. */
const externalLink = tv({ variants: { icon: { true: "gap-1", false: "" } } });

export type ExternalLinkProps = Omit<LinkProps, "target" | "children" | "href"> & {
  href: string;
  /** The visible name of the link; the "opens in a new tab" text and the icon follow it. */
  children: ReactNode;
  /** Whether the link ends with the external-link icon (on by default; off for a link that is a whole card). */
  icon?: boolean;
};

/**
 * A link that leaves the app and opens in a new tab: `target="_blank"`, `rel="noopener noreferrer"` plus the
 * tokens of `rel`, the external-link icon (which mirrors in right-to-left text), and the "opens in a new tab"
 * text of `ExternalLinkContext` for assistive technology: after the children, or after `aria-label` when the
 * link has one. React Aria's router binding does not take a link with a `target`, so the browser opens it.
 */
export function ExternalLink({
  rel,
  icon = true,
  children,
  className,
  "aria-label": ariaLabel,
  ...props
}: ExternalLinkProps) {
  const { newTabLabel } = use(ExternalLinkContext);
  const label = ariaLabel === undefined || newTabLabel === undefined ? ariaLabel : `${ariaLabel} ${newTabLabel}`;
  // Apart from the name by a space, which a browser keeps: the hidden text is a block (`sr-only` is absolute).
  const hiddenText = newTabLabel === undefined || ariaLabel !== undefined ? undefined : ` ${newTabLabel}`;
  return (
    <Link
      {...props}
      {...(label !== undefined && { "aria-label": label })}
      // eslint-disable-next-line restrict/syntax -- the one link that opens in a new tab (web-client/README.md Y12)
      target="_blank"
      rel={externalRel(rel)}
      className={composeRenderProps(className, (className) => externalLink({ icon, className }))}
    >
      {children}
      {hiddenText !== undefined && <VisuallyHidden>{hiddenText}</VisuallyHidden>}
      {icon && <ExternalLinkIcon />}
    </Link>
  );
}
