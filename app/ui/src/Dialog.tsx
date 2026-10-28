// SPDX-License-Identifier: AGPL-3.0-only
// Modal dialogs: Dialog (centered) and Sheet (from the bottom on narrow screens). Both are React Aria's
// ModalOverlay + Modal + Dialog, which trap focus while open, close on Esc, hide the rest of the page
// from assistive technology, and return focus to the trigger on close (WAI-ARIA APG, dialog pattern).
// Every dialog also closes by pointer alone: a dialog has a × button, an alert dialog a cancel button,
// and a press on the scrim closes either unless the dialog holds input that the press would lose.
import type { ReactNode, Ref } from "react";
import { Button, Dialog as AriaDialog, DialogTrigger, Heading, Modal, ModalOverlay } from "react-aria-components";
import { tv } from "tailwind-variants";
import { CloseIcon } from "./Icons";

export { DialogTrigger };
export type { DialogTriggerProps } from "react-aria-components";

/**
 * The overlay is its own size container, so the panel answers to the space it can use (web-client/README.md D2)
 * wherever it is portalled. Its height follows the visual viewport, which shrinks under an on-screen keyboard.
 */
const modal = tv({
  slots: {
    overlay: [
      "fixed inset-x-0 top-0 z-50 flex h-full max-h-(--visual-viewport-height) justify-center bg-overlay @container",
      "motion-safe:transition-opacity motion-safe:duration-normal data-entering:opacity-0 data-exiting:opacity-0",
      "motion-safe:data-exiting:ease-exit",
    ],
    panel:
      "flex w-full flex-col bg-surface-raised text-fg shadow-lg outline-none motion-safe:transition motion-safe:duration-normal",
    // The panel is a flex column with a capped height, so the dialog and then its body shrink and scroll.
    dialog: "flex min-h-0 flex-col outline-none",
    header: "flex items-start gap-2 px-4 pt-4 pb-2",
    title: "flex-1 text-xl font-extrabold tracking-tight text-pretty",
    close: [
      "-me-2 -mt-1 grid size-9 shrink-0 place-items-center rounded-full text-fg-muted outline-none",
      "data-hovered:bg-surface-sunken data-hovered:text-fg data-focus-visible:focus-ring pointer-coarse:size-11 [&>svg]:size-5",
    ],
    // React Aria cannot set overscroll-behavior on iOS WebKit under the shell's CSP, so each overlay's
    // scrollable surface sets it, and its scroll never chains to the page (web-client/09-app-shell.md §9).
    body: "min-h-0 overflow-y-auto overscroll-contain px-4 pb-4",
  },
  variants: {
    placement: {
      center: {
        overlay: "items-center p-4",
        panel:
          "max-h-[min(85svh,100%)] rounded-xl data-entering:scale-95 data-entering:opacity-0 data-exiting:scale-95 data-exiting:opacity-0",
      },
      sheet: {
        overlay: "items-end @lg:items-center @lg:p-4",
        // The sheet sits on the bottom edge on phones, so the home indicator and rounded corners need room.
        panel: [
          "max-h-[min(90svh,calc(100%-2rem))] rounded-t-2xl pb-[env(safe-area-inset-bottom)] @lg:max-h-[min(85svh,100%)] @lg:max-w-lg @lg:rounded-xl @lg:pb-0",
          "data-entering:translate-y-full data-exiting:translate-y-full @lg:data-entering:translate-y-4 @lg:data-entering:opacity-0",
          "@lg:data-exiting:translate-y-4 @lg:data-exiting:opacity-0",
        ],
      },
    },
    // Only the centered dialog has sizes; a sheet spans narrow screens and is max-w-lg elsewhere.
    size: { sm: {}, md: {}, lg: {} },
  },
  compoundVariants: [
    { placement: "center", size: "sm", class: { panel: "max-w-sm" } },
    { placement: "center", size: "md", class: { panel: "max-w-md" } },
    { placement: "center", size: "lg", class: { panel: "max-w-2xl" } },
  ],
  defaultVariants: { placement: "center", size: "md" },
});

type ModalDialogProps = {
  /** The heading of the dialog; it also names the dialog (aria-labelledby). */
  title: ReactNode;
  /** The content. A function receives `close` for actions that close the dialog. */
  children: ReactNode | ((renderProps: { close: () => void }) => ReactNode);
  /** Controlled open state, for a dialog opened without a DialogTrigger (from a route, for example). */
  isOpen?: boolean;
  defaultOpen?: boolean;
  onOpenChange?: (isOpen: boolean) => void;
  /**
   * Close when the user presses outside the panel (a click or tap on the scrim). On by default, so that every
   * dialog closes by pointer alone; turn it off for a dialog whose input a stray tap would lose (a form).
   */
  isDismissable?: boolean;
  /** Ignore Esc, for a dialog that must be answered. */
  isKeyboardDismissDisabled?: boolean;
  /** Classes merged onto the panel with tailwind-merge. */
  className?: string;
  ref?: Ref<HTMLElement>;
  /** `alertdialog` for confirmations that interrupt the user (WAI-ARIA APG, alert dialog pattern). */
  role?: "dialog" | "alertdialog";
  /**
   * The accessible name of the × button in the header, so that the dialog closes by click or tap alone.
   * It is required: only an `alertdialog`, whose content offers a cancel button, passes `null` to leave it out.
   */
  closeLabel: string | null;
};

function ModalDialog({
  title,
  children,
  closeLabel,
  role = "dialog",
  className,
  ref,
  placement,
  size,
  isDismissable = true,
  ...overlayProps
}: ModalDialogProps & { placement: "center" | "sheet"; size?: "sm" | "md" | "lg" }) {
  const styles = modal({ placement, size });
  return (
    <ModalOverlay {...overlayProps} isDismissable={isDismissable} className={styles.overlay()}>
      <Modal className={styles.panel({ class: className })}>
        <AriaDialog ref={ref} role={role} className={styles.dialog()}>
          {({ close }) => (
            <>
              <div className={styles.header()}>
                <Heading slot="title" className={styles.title()}>
                  {title}
                </Heading>
                {closeLabel !== null && (
                  <Button slot="close" aria-label={closeLabel} className={styles.close()}>
                    <CloseIcon />
                  </Button>
                )}
              </div>
              <div className={styles.body()}>{typeof children === "function" ? children({ close }) : children}</div>
            </>
          )}
        </AriaDialog>
      </Modal>
    </ModalOverlay>
  );
}

export type DialogProps = ModalDialogProps & {
  /** The maximum inline size of the panel. */
  size?: "sm" | "md" | "lg";
};

/**
 * A modal dialog centered over a scrim. Put it inside a `DialogTrigger` next to its trigger button,
 * or control it with `isOpen` / `onOpenChange`.
 */
export function Dialog(props: DialogProps) {
  return <ModalDialog {...props} placement="center" />;
}

export type SheetProps = ModalDialogProps;

/**
 * A modal sheet: it rises from the bottom edge where the space is narrower than 32rem (phones) and is a
 * centered dialog elsewhere. It pads for the safe area and caps its height in svh units.
 */
export function Sheet(props: SheetProps) {
  return <ModalDialog {...props} placement="sheet" />;
}
