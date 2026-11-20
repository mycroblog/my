// SPDX-License-Identifier: AGPL-3.0-only

// What the screens of the server's administration built here share: the notice in a screen's sections' place while
// what it shows loads, when it could not be read (with Try again, a finger's target), or when its address names
// nothing (with the way to its list); the question before leaving a form with changes not saved; what a screen under
// an entry is given by its route; and a value to pass on, with the button that copies it.

import {
  Box,
  Button,
  ConfirmDialog,
  CopyBlock,
  FeedNotice,
  Icon,
  Row,
  Select,
  SettingRow,
  TextLink,
} from "@my/design-system";
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

/**
 * A value to pass on (a callback URL, a link, a secret shown once), named `label`, with the button that copies it and
 * what came of copying; `primary` where copying it is what the screen is for (a link to hand over, a secret); `code`,
 * a block of lines kept as they are (a stack, a payload).
 */
export function CopyValue({
  label,
  value,
  copy,
  code = false,
  primary = false,
}: {
  label: string;
  value: string;
  copy: string;
  code?: boolean | undefined;
  primary?: boolean | undefined;
}): ReactNode {
  const { t } = useStaffStrings();
  const [copied, setCopied] = useState<boolean>();
  return (
    <CopyBlock
      label={label}
      {...(code ? { code: value } : { values: [value] })}
      action={
        <Button
          {...(primary ? { kind: "primary" as const } : {})}
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

/**
 * A setting that is on or off, in a form saved as a whole: its row, with the one select of On and Off that every such
 * setting of the administration takes (a switch saves at once, which no form here does), and what it holds under it.
 */
export function OnOffRow({
  id,
  label,
  about,
  on,
  onChange,
  disabled = false,
  error,
  values,
}: {
  id: string;
  label: string;
  about?: string | undefined;
  on: boolean;
  onChange: (on: boolean) => void;
  disabled?: boolean | undefined;
  error?: string | undefined;
  /** What stands close under the row as its own (a mark of what cannot be undone, what the setting holds). */
  values?: ReactNode;
}): ReactNode {
  const { t } = useStaffStrings();
  return (
    <SettingRow id={id} label={label} about={about} form error={error} values={values}>
      <Select
        name={id}
        options={["true", "false"].map((v) => ({ value: v, label: t("staff_settings.bool", { value: v }) }))}
        value={on ? "true" : "false"}
        onChange={(v) => {
          onChange(v === "true");
        }}
        isDisabled={disabled}
      />
    </SettingRow>
  );
}

/**
 * The mark of a choice or an action that cannot be undone, in the text's color with the glyph of caution: the one look
 * every such option and act of the administration has, beside the question that confirms it.
 */
export function Irreversible({ children }: { children: string }): ReactNode {
  return (
    <Row align="start" gap="entry-caption">
      <Row as="span" inline line rigid align="center">
        <Icon name="caution" place="caption" />
      </Row>
      <Box as="span" text="caption" weight="strong" breaks="phrase">
        {children}
      </Box>
    </Row>
  );
}
