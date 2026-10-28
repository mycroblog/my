// SPDX-License-Identifier: AGPL-3.0-only
// The confirmation that stands before an action that cannot be undone (a ban, suspending a domain, deleting
// a post): the WAI-ARIA APG alert dialog pattern on the kit's Dialog. The title asks the question, the
// description says what happens (role="alertdialog" names it with aria-labelledby and describes it with
// aria-describedby), and the buttons are verbs (web-client/16-look-states-and-wording.md G22).
//
// Focus follows the APG dialog pattern: on open it moves to the first safe control, which is the first field
// of `children` (a reason, a duration) or else the cancel button, never the destructive one; Esc closes the
// dialog like cancel; on close React Aria returns focus to the trigger.
import type { ReactNode } from "react";
import { FocusScope } from "react-aria";
import { Form, Text } from "react-aria-components";
import { tv } from "tailwind-variants";
import { Button } from "./Button";
import { Dialog } from "./Dialog";
import { DialogActions } from "./DialogActions";

const confirmDialog = tv({
  slots: {
    form: "flex flex-col gap-4",
    description: "text-base text-fg",
    fields: "flex flex-col gap-4",
    error: "text-sm text-danger",
  },
});

export type ConfirmDialogProps = {
  /** The question ("Suspend example.com?"); it names the dialog. */
  title: ReactNode;
  /** What the action does and whether it can be undone; it describes the dialog. */
  description: ReactNode;
  /** Fields the action needs, such as a `ReasonField` or a `DurationSelect`. The first one gets focus on open. */
  children?: ReactNode;
  /** The verb of the action ("Suspend"). */
  confirmLabel: ReactNode;
  /** The way out ("Cancel"). */
  cancelLabel: ReactNode;
  /** `danger` for actions that remove or block something; `primary` for the others. */
  tone?: "danger" | "primary";
  /**
   * Runs the action once the fields are valid (the browser's constraint validation of `children`).
   * The dialog stays open: call `close` when the action has succeeded.
   */
  onConfirm: (close: () => void) => void;
  /** While the action runs: the confirm button waits, and cancel and Esc still close the dialog. */
  isPending?: boolean;
  /** Why the action failed, shown in the dialog above the buttons (role="alert") so that it can be tried again. */
  errorMessage?: ReactNode;
  /** Controlled open state, for a dialog opened without a DialogTrigger. */
  isOpen?: boolean;
  defaultOpen?: boolean;
  onOpenChange?: (isOpen: boolean) => void;
};

/**
 * A confirmation dialog. Put it inside a `DialogTrigger` next to its trigger button, or control it with
 * `isOpen` / `onOpenChange`. It has no × button: cancel, Esc and a press outside close it, except that a dialog
 * with fields ignores a press outside, which would lose what was typed.
 */
export function ConfirmDialog({
  title,
  description,
  children,
  confirmLabel,
  cancelLabel,
  tone = "danger",
  onConfirm,
  isPending = false,
  errorMessage,
  ...overlayProps
}: ConfirmDialogProps) {
  const styles = confirmDialog();
  return (
    <Dialog
      {...overlayProps}
      role="alertdialog"
      closeLabel={null}
      size="sm"
      title={title}
      isDismissable={children === undefined}
    >
      {({ close }) => (
        <Form
          className={styles.form()}
          onSubmit={(event) => {
            event.preventDefault();
            if (!isPending) onConfirm(close);
          }}
        >
          {/* The description slot of React Aria's Dialog, which an alertdialog references with aria-describedby. */}
          <Text slot="description" elementType="p" className={styles.description()}>
            {description}
          </Text>
          {children !== undefined && (
            // Focuses the first field on open; the dialog's own focus scope still contains Tab (APG dialog pattern).
            <FocusScope autoFocus>
              <div className={styles.fields()}>{children}</div>
            </FocusScope>
          )}
          {errorMessage !== undefined && (
            <p role="alert" className={styles.error()}>
              {errorMessage}
            </p>
          )}
          <DialogActions>
            <Button onPress={close} autoFocus={children === undefined}>
              {cancelLabel}
            </Button>
            <Button type="submit" variant={tone} isPending={isPending}>
              {confirmLabel}
            </Button>
          </DialogActions>
        </Form>
      )}
    </Dialog>
  );
}
