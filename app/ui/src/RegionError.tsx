// SPDX-License-Identifier: AGPL-3.0-only
// The error state of one region of data (web-client/16-look-states-and-wording.md G16): what happened, the reason and "Try again", in place of
// the region's content only, so that a page keeps its h1 and a list keeps the items it has already loaded.
// A failure the user only opened the page to see is found by reading, so it is not a live region; only the result of
// the user's own action interrupts as an alert: a retry from its button that failed again, or what `alert` marks
// (WAI-ARIA APG, Alert: https://www.w3.org/WAI/ARIA/apg/patterns/alert/). The button stays outside the alert, whose
// text alone is inserted anew for each failure, because a live region announces what is added to it, and focus stays
// on the button through a retry that fails.
import { type ComponentPropsWithRef, type ReactNode, useLayoutEffect, useRef, useState } from "react";
import { tv } from "tailwind-variants";
import { Button } from "./Button";
import { WarningIcon } from "./Icons";
import type { HeadingLevel } from "./EmptyState";

const regionError = tv({
  slots: {
    base: "flex items-start gap-3 rounded-lg bg-danger-bg px-4 py-3 text-fg",
    icon: "mt-px shrink-0 text-danger [&>svg]:size-5.5",
    body: "flex min-w-0 flex-col items-start gap-1.5",
    message: "flex min-w-0 flex-col items-start gap-1.5",
    heading: "text-base font-extrabold text-fg",
    reason: "text-sm wrap-anywhere text-fg",
    action: "mt-1 *:bg-surface-raised",
  },
});

type RetryProps =
  | {
      /** The label of the retry button ("Try again"). */
      retryLabel: string;
      /** Requests the region's data again. */
      onRetry: () => void;
      /**
       * The retry is in progress: the button waits and keeps its focus, and when it ends with the error still here,
       * the error is announced as an alert. The caller keeps the error mounted meanwhile.
       */
      isRetrying?: boolean;
    }
  | { retryLabel?: never; onRetry?: never; isRetrying?: never };

export type RegionErrorProps = Omit<ComponentPropsWithRef<"div">, "children" | "title" | "role"> &
  RetryProps & {
    /** What happened, in one short sentence ("Couldn't load the posts"). */
    heading: ReactNode;
    /**
     * The heading's level in the page outline, one below the heading of the region it replaces. Without it the
     * message is a paragraph, as for the failure of the next page of a list.
     */
    headingLevel?: HeadingLevel;
    /** Why, from the error (`errorMessage(error)`), or what the user can do about it. */
    reason?: ReactNode;
    /**
     * Makes the error an assertive live region (`role="alert"`), except during a retry; only for the result of the
     * user's own action. A retry from the button that failed again needs no `alert`.
     */
    alert?: boolean;
  };

/**
 * The nearest heading above `element` that takes focus from script (`focusable`; every screen's `h1` does), within its
 * dialog if it is in one, or else that dialog.
 */
function focusTargetAbove(element: HTMLElement): HTMLElement | null {
  const dialog = element.closest<HTMLElement>('[role="dialog"], [role="alertdialog"]');
  const headings = (dialog ?? document).querySelectorAll<HTMLElement>(
    ["h1", "h2", "h3", "h4", "h5", "h6"].map((heading) => `${heading}[tabindex="-1"]`).join(", "),
  );
  const above = Array.from(headings).filter(
    (heading) => heading.compareDocumentPosition(element) & Node.DOCUMENT_POSITION_FOLLOWING,
  );
  return above.at(-1) ?? dialog;
}

/** What a region shows instead of its content when loading it failed. */
export function RegionError({
  heading,
  headingLevel,
  reason,
  retryLabel,
  onRetry,
  isRetrying = false,
  alert = false,
  className,
  ...props
}: RegionErrorProps) {
  const slots = regionError();
  const [retried, setRetried] = useState(false);
  const [failedAgain, setFailedAgain] = useState(false);
  const [wasRetrying, setWasRetrying] = useState(isRetrying);
  // Adjusting state while rendering, on the end of a retry (react.dev: "Adjusting some state when a prop changes").
  // It waits for the end, not the press: the caller's `isRetrying` turns true only after the press has rendered.
  if (wasRetrying !== isRetrying) {
    setWasRetrying(isRetrying);
    setFailedAgain(retried && !isRetrying);
  }
  const isAlert = failedAgain || (alert && !isRetrying);

  const actionRef = useRef<HTMLDivElement>(null);
  // The content that loaded takes this error's place and the button goes with it. Focus goes to where the next Tab
  // reaches that content, without scrolling away from it. A layout effect's cleanup runs while the button is in the
  // page and still has focus.
  useLayoutEffect(() => {
    const action = actionRef.current;
    if (!retried || action === null) return;
    return () => {
      if (action.contains(document.activeElement)) focusTargetAbove(action)?.focus({ preventScroll: true });
    };
  }, [retried]);

  const Heading = headingLevel === undefined ? "p" : (`h${String(headingLevel)}` as `h${HeadingLevel}`);
  return (
    <div {...props} className={slots.base({ className })}>
      <span className={slots.icon()}>
        <WarningIcon filled />
      </span>
      <div className={slots.body()}>
        <div key={String(isAlert)} role={isAlert ? "alert" : undefined} className={slots.message()}>
          <Heading className={slots.heading()}>{heading}</Heading>
          {reason !== undefined && <p className={slots.reason()}>{reason}</p>}
        </div>
        {onRetry !== undefined && (
          <div ref={actionRef} className={slots.action()}>
            <Button
              size="sm"
              onPress={() => {
                setRetried(true);
                onRetry();
              }}
              isPending={isRetrying}
            >
              {retryLabel}
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
