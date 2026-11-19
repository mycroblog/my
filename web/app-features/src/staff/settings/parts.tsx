// SPDX-License-Identifier: AGPL-3.0-only

// What the screens of the server's administration built here share: the notice in a screen's sections' place while
// what it shows loads, when it could not be read (with Try again, a finger's target), or when its address names
// nothing (with the way to its list); the question before leaving a form with changes not saved; what a screen under
// an entry is given by its route; and a value to pass on, with the button that copies it.

import { Button, ConfirmDialog, CopyBlock, FeedNotice, TextLink } from "@my/design-system";
import { copyText } from "@my/platform";
import { useState, type ReactNode } from "react";
import { useStaffStrings } from "../parts";

/** What a screen under an entry of the administration is given by its route. */
export type AdminSubProps = {
  /** Goes to `href`, an address of the app, in place of the screen (back to the entry, to what a write made). */
  readonly go: (href: string) => void;
  /** Hears whether the form holds changes that are not saved, as that comes and goes; false before it leaves. */
  readonly onUnsaved?: ((unsaved: boolean) => void) | undefined;
  /** The reader is leaving with changes not saved, and is held back: the screen asks whether to leave or stay. */
  readonly leaving?: { readonly onStay: () => void; readonly onLeave: () => void } | undefined;
};

/**
 * In a screen's sections' place: that what it shows loads (`loading`), could not be read (`failed`, with Try again),
 * or that its address names nothing (`missing`, with the way to the list at `all`).
 */
export function ScreenNotice({
  status,
  retry,
  words,
  all,
}: {
  status: "loading" | "failed" | "missing";
  retry: () => void;
  words: { readonly loading: string; readonly failed: string; readonly missing?: string | undefined };
  all?: { readonly label: string; readonly href: string } | undefined;
}): ReactNode {
  const { t } = useStaffStrings();
  if (status === "missing")
    return (
      <FeedNotice alert lead action={all === undefined ? undefined : <TextLink href={all.href}>{all.label}</TextLink>}>
        {words.missing ?? words.failed}
      </FeedNotice>
    );
  if (status === "loading") return <FeedNotice lead>{words.loading}</FeedNotice>;
  return (
    <FeedNotice
      alert
      lead
      action={
        <Button size="timeline" touch onPress={retry}>
          {t("staff.retry")}
        </Button>
      }
    >
      {words.failed}
    </FeedNotice>
  );
}

/** The question before leaving a form whose changes are not saved: leave, losing them, or stay. */
export function LeaveDialog({ leaving }: { leaving: { readonly onStay: () => void; readonly onLeave: () => void } }) {
  const { t } = useStaffStrings();
  return (
    <ConfirmDialog
      title={t("staff_settings.leave.title")}
      cancel={t("staff_settings.leave.cancel")}
      confirm={t("staff_settings.leave.confirm")}
      onCancel={leaving.onStay}
      onConfirm={leaving.onLeave}
    >
      {t("staff_settings.leave.text")}
    </ConfirmDialog>
  );
}

/** A value to pass on (a callback URL), named `label`, with the button that copies it and what came of copying. */
export function CopyValue({ label, value, copy }: { label: string; value: string; copy: string }): ReactNode {
  const { t } = useStaffStrings();
  const [copied, setCopied] = useState<boolean>();
  return (
    <CopyBlock
      label={label}
      values={[value]}
      action={
        <Button
          size="timeline"
          onPress={() => {
            void copyText(value).then(setCopied);
          }}
        >
          {copy}
        </Button>
      }
      result={copied === undefined ? undefined : t(copied ? "staff_settings.copied" : "staff_settings.copy_failed")}
    />
  );
}
