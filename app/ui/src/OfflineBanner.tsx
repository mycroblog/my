// SPDX-License-Identifier: AGPL-3.0-only
// The app's one offline bar (web-client/16-look-states-and-wording.md G17). Its message is a polite live region (role="status") that is in the
// page from the first render, empty while online, so that going offline is announced: a live region announces only what
// changes after it is in the page (web-client/README.md Y7). An action of the bar (a link to the server's status page)
// follows the message outside the live region, so that what is announced is the message alone. The bar sticks to the
// top edge and pads itself for the top safe-area inset, so the app renders it before, and outside of, any element that
// already pads for that inset. On a short screen (the short: variant, such as 320 × 225 at 400% zoom) it does not stick
// but scrolls away with the page: the tab bar and the floating new-post button already take almost half of the
// screen's height, and what stays in place must not take more (WCAG 1.4.10; the layout of web-client/09-app-shell.md §9).
// The bar and the shell after it share the screen's height: the bar's height (0 while online) is the
// `--offline-bar-block-size` of the document, which the shell takes off its heights that are the screen's, and by which
// its sticky columns start under the bar, so that a short page does not scroll by the bar's height.
import { type ComponentPropsWithRef, type ReactNode, useRef } from "react";
import { mergeRefs } from "react-aria";
import { tv } from "tailwind-variants";
import { useBlockSizeProperty } from "./blockSizeProperty";
import { WarningIcon } from "./Icons";

const offlineBanner = tv({
  slots: {
    region: "sticky top-0 z-30 short:static",
    message: "flex items-center justify-center gap-2",
    icon: "shrink-0 text-warning [&>svg]:size-4.5",
    // The accent of a link has too little contrast on the warning tint, so a link takes the bar's text color and is
    // underlined, so that color is not its only cue (WCAG 1.4.1).
    action: "[&_a]:text-fg [&_a]:underline [&_a[data-hovered]]:text-fg",
  },
  variants: {
    // The bar's look only while it shows, so that the empty region takes no space.
    isOffline: {
      true: {
        region: [
          "flex flex-wrap items-center justify-center gap-x-3 gap-y-1 bg-warning-bg px-[max(1rem,env(safe-area-inset-left),env(safe-area-inset-right))] pt-[calc(0.5rem+env(safe-area-inset-top))] pb-2",
          "text-center text-sm font-bold text-fg",
        ],
      },
    },
  },
});

export type OfflineBannerProps = Omit<ComponentPropsWithRef<"div">, "children" | "role"> & {
  /** Whether the app is offline; the bar shows only then. */
  isOffline: boolean;
  /** The message ("You're offline. Posts you send are kept and sent when you're back online."). */
  children: ReactNode;
  /** What the user can do about it (a link), after the message and outside its live region; shown only while offline. */
  action?: ReactNode;
};

/**
 * Render it once, at the top of the app and before the shell (`AppShell`, `AdminShell`), for the whole session; switch
 * `isOffline` as the connection changes.
 */
export function OfflineBanner({ isOffline, children, action, className, ref, ...props }: OfflineBannerProps) {
  const slots = offlineBanner({ isOffline });
  const regionRef = useRef<HTMLDivElement>(null);
  useBlockSizeProperty(regionRef, "--offline-bar-block-size");
  return (
    <div {...props} ref={mergeRefs(regionRef, ref)} className={slots.region({ className })}>
      <div role="status">
        {isOffline && (
          <p className={slots.message()}>
            <span className={slots.icon()}>
              <WarningIcon filled />
            </span>
            <span>{children}</span>
          </p>
        )}
      </div>
      {isOffline && action !== undefined && <span className={slots.action()}>{action}</span>}
    </div>
  );
}
