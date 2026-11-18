// SPDX-License-Identifier: AGPL-3.0-only

// The confirmation before the app signs the reader out of this browser (ADR-0090 §13: a confirmation alone, no
// password): the question, what signing out removes from the browser and what stays, and Cancel beside Sign out. While
// the sign-out is on its way the confirmation spins and the dialog stays; when it did not finish, the dialog says so
// in place of what it explained, and its button tries again. Signed out, the app leaves for a page that loads anew.

import { Box, ConfirmDialog, useLocale } from "@my/design-system";
import { useState } from "react";
import { stringsIn } from "../locale";

export type SignOutDialogProps = {
  /** Cancel, Escape or a press outside: the reader stays signed in. */
  onCancel: () => void;
  /** Signs the reader out and leaves the app; rejects when the server did not end the session. */
  signOut: () => Promise<void>;
  /** Over the page, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
};

export function SignOutDialog({ onCancel, signOut, layer }: SignOutDialogProps) {
  const { t } = stringsIn(useLocale().locale);
  const [state, setState] = useState<"asking" | "leaving" | "failed">("asking");
  return (
    <ConfirmDialog
      title={t("settings.sign_out.title")}
      cancel={t("settings.sign_out.cancel")}
      confirm={t("settings.sign_out.confirm")}
      busy={state === "leaving"}
      onCancel={onCancel}
      onConfirm={() => {
        setState("leaving");
        signOut().catch(() => {
          setState("failed");
        });
      }}
      layer={layer}
    >
      {state === "failed" ? (
        <Box as="span" role="alert">
          {t("settings.sign_out.failed")}
        </Box>
      ) : (
        t("settings.sign_out.text")
      )}
    </ConfirmDialog>
  );
}
