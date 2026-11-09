// SPDX-License-Identifier: AGPL-3.0-only
// The icon-only buttons (web-client/16-look-states-and-wording.md G12): a button whose children show no words takes its
// name once, as `label`, and gives it both to its accessible name (aria-label) and to a tooltip of the same words, which
// opens on hover after a delay and at once on keyboard focus (WAI-ARIA APG, tooltip pattern). The tooltip describes
// the button; it never replaces its name. Lint (restrict/icons) rejects an icon-only Button or ToggleButton outside a
// TooltipTrigger, so a feature builds one from these.
import type { ReactNode } from "react";
import { composeRenderProps } from "react-aria-components";
import { Button, ButtonLink, type ButtonLinkProps, type ButtonProps } from "./Button";
import { ToggleButton, type ToggleButtonProps } from "./ToggleButton";
import { Tooltip, type TooltipProps, TooltipTrigger } from "./Tooltip";

type IconProps = {
  /** The button's name: its aria-label and the words of its tooltip. */
  label: string;
  /** The icon, and at most a count beside it: nothing that names the button. */
  children: ReactNode;
  /** Where the tooltip opens; above the button by default. */
  tooltipPlacement?: TooltipProps["placement"];
};

export type IconButtonProps = Omit<ButtonProps, "aria-label" | "aria-labelledby" | "children"> & IconProps;

/** A `Button` that shows only an icon (`ghost` and `icon` by default), named by `label` and its tooltip. */
export function IconButton({
  label,
  children,
  tooltipPlacement,
  variant = "ghost",
  size = "icon",
  ...props
}: IconButtonProps) {
  return (
    <TooltipTrigger>
      <Button {...props} variant={variant} size={size} aria-label={label}>
        {children}
      </Button>
      <Tooltip {...(tooltipPlacement !== undefined && { placement: tooltipPlacement })}>{label}</Tooltip>
    </TooltipTrigger>
  );
}

export type IconButtonLinkProps = Omit<ButtonLinkProps, "aria-label" | "aria-labelledby" | "children"> & IconProps;

/**
 * A link that shows only an icon (and a count), as `IconButton` does: a place to go rather than an action, such as the
 * replies of a post (its page). Named by `label` and its tooltip; `subtle` and `toolbar` by default, for a post's action bar.
 */
export function IconButtonLink({
  label,
  children,
  tooltipPlacement,
  variant = "subtle",
  size = "toolbar",
  ...props
}: IconButtonLinkProps) {
  return (
    <TooltipTrigger>
      <ButtonLink {...props} variant={variant} size={size} aria-label={label}>
        {children}
      </ButtonLink>
      <Tooltip {...(tooltipPlacement !== undefined && { placement: tooltipPlacement })}>{label}</Tooltip>
    </TooltipTrigger>
  );
}

export type IconToggleButtonProps = Omit<ToggleButtonProps, "aria-label" | "aria-labelledby" | "children"> &
  IconProps & {
    /**
     * The accessible name while the button shows a count: `label` with the count as it is shown ("Favourite, 3"), so
     * that the name contains the visible text (WCAG 2.5.3). The tooltip keeps `label`. Without it, the name is `label`.
     */
    countedLabel?: string | undefined;
    /**
     * Left out of a container narrower than `@sm` (a narrow `PostCard`), for an action that a menu beside it also
     * holds, so that a row of actions fits: `display: none`, so that it takes neither room nor the focus. It needs an
     * `@container` around it, as `PostCard` is; without one it is always left out.
     */
    collapsible?: boolean;
  };

/**
 * A `ToggleButton` that shows only an icon (and a count), named by `label` (with the count it shows, `countedLabel`)
 * and its tooltip. The name stays the same in both states ("Favourite"); the icon's shape and `aria-pressed` tell the state.
 */
export function IconToggleButton({
  label,
  countedLabel,
  children,
  tooltipPlacement,
  collapsible = false,
  className,
  ...props
}: IconToggleButtonProps) {
  return (
    <TooltipTrigger>
      <ToggleButton
        {...props}
        aria-label={countedLabel ?? label}
        className={composeRenderProps(className, (className) =>
          collapsible ? ["hidden @sm:inline-flex", className].filter(Boolean).join(" ") : (className ?? ""),
        )}
      >
        {children}
      </ToggleButton>
      <Tooltip {...(tooltipPlacement !== undefined && { placement: tooltipPlacement })}>{label}</Tooltip>
    </TooltipTrigger>
  );
}
