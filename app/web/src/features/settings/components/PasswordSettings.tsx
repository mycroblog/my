// SPDX-License-Identifier: AGPL-3.0-only
// The password page below its heading (FR-002, FR-004). A user with a password gets the form that changes it with the
// current one (what a change does: the other browsers and apps are signed out, the passkeys and two-factor
// authentication stay) and the section that removes it after a passkey confirms it in an alert dialog (ConfirmDialog),
// which a user without a passkey is told to add first. A user who removed the password gets the form that sets one
// after a passkey confirms it. What a change, a setting or a removal did stays on the page in one StatusMessage
// (role="status") at its top, which stays in place when the page turns from one kind into the other. A wrong current password is that field's error and any other failure an alert next to the form or
// in the dialog (web-client/README.md E2, web-client/16-look-states-and-wording.md G16).
import { Box, Button, ConfirmDialog, InlineError, Section, Stack, StatusMessage, TextField } from "@my/ui";
import { useState } from "react";
import { useT } from "../../../lib/i18n";
import { passwordMaxLength, passwordMinLength } from "../api/password";
import {
  type PasswordChangeForm,
  type PasswordRemoval,
  usePasswordChangeForm,
  usePasswordRemoval,
} from "../hooks/usePasswordChange";
import { ExternalConfirmationChoice } from "./ExternalConfirmationChoice";

export function PasswordSettings() {
  const t = useT();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const form = usePasswordChangeForm(setStatus);
  const removal = usePasswordRemoval(setStatus);
  return (
    <Stack container gap="8">
      <StatusMessage>{status}</StatusMessage>
      {form.hasPassword ? (
        <>
          <Section title={t("client:settings.password.change.title")}>
            <p>{t("client:settings.password.description")}</p>
            <ChangeForm form={form} />
          </Section>
          <Section title={t("client:settings.password.remove.title")}>
            <RemovePassword removal={removal} />
          </Section>
        </>
      ) : (
        <Section title={t("client:settings.password.set.title")}>
          <p>{t("client:settings.password.set.description")}</p>
          {form.external.isPreferred ? (
            <>
              <ExternalConfirmationChoice confirmation={form.external} />
              {form.external.confirmed !== undefined && <ChangeForm form={form} />}
            </>
          ) : form.isPasskeySupported ? (
            <ChangeForm form={form} />
          ) : (
            <p>{t("client:settings.password.passkeys_unsupported")}</p>
          )}
        </Section>
      )}
    </Stack>
  );
}

/** The form that changes the password after the current one, or sets one after a passkey for a user without one. */
function ChangeForm({ form }: { form: PasswordChangeForm }) {
  const t = useT();
  const lengths = { min: passwordMinLength, max: passwordMaxLength };
  return (
    <Stack as="form" onSubmit={form.submit} align="start" gap="4">
      {form.hasPassword && (
        <Box width="sm">
          <TextField
            label={t("client:settings.password.current")}
            type="password"
            autoComplete="current-password"
            value={form.currentPassword}
            onChange={form.changeCurrentPassword}
            isRequired
            isInvalid={form.currentPasswordError !== undefined}
            errorMessage={form.currentPasswordError ?? t("client:settings.password.current.required")}
          />
        </Box>
      )}
      <Box width="sm">
        <TextField
          label={t("client:settings.password.new")}
          description={t("client:settings.password.new.description", lengths)}
          type="password"
          autoComplete="new-password"
          value={form.newPassword}
          onChange={form.changeNewPassword}
          isRequired
          minLength={passwordMinLength}
          maxLength={passwordMaxLength}
          errorMessage={t("client:settings.password.new.invalid", lengths)}
        />
      </Box>
      {form.error !== undefined && (
        <InlineError alert size="inherit">
          {form.error}
        </InlineError>
      )}
      <Button type="submit" variant="primary" isPending={form.isPending}>
        {t(form.hasPassword ? "client:settings.password.submit" : "client:settings.password.set.submit")}
      </Button>
    </Stack>
  );
}

/** Removing the password: what it does, and its button and dialog; or why it cannot be removed here. */
function RemovePassword({ removal }: { removal: PasswordRemoval }) {
  const t = useT();
  if (removal.external.isPreferred) {
    // A user without passkeys signs in again at a linked provider first.
    return (
      <Stack align="start" gap="4">
        <p>{t("client:settings.password.remove.description")}</p>
        <ExternalConfirmationChoice confirmation={removal.external} />
        {removal.external.confirmed !== undefined && <RemovePasswordDialog removal={removal} />}
      </Stack>
    );
  }
  if (!removal.hasPasskey) return <p>{t("client:settings.password.remove.no_passkey")}</p>;
  if (!removal.isSupported) return <p>{t("client:settings.password.passkeys_unsupported")}</p>;
  return (
    <Stack align="start" gap="4">
      <p>{t("client:settings.password.remove.description")}</p>
      <RemovePasswordDialog removal={removal} />
    </Stack>
  );
}

/** The button that removes the password, and its alert dialog. */
function RemovePasswordDialog({ removal }: { removal: PasswordRemoval }) {
  const t = useT();
  return (
    <>
      <Button variant="danger" onPress={removal.open}>
        {t("client:settings.password.remove.open")}
      </Button>
      <ConfirmDialog
        isOpen={removal.isOpen}
        onOpenChange={(isOpen) => {
          if (!isOpen) removal.cancel();
        }}
        title={t("client:settings.password.remove.confirm.title")}
        description={t("client:settings.password.remove.confirm.description")}
        confirmLabel={t("client:settings.password.remove.confirm")}
        cancelLabel={t("client:settings.password.remove.cancel")}
        tone="danger"
        onConfirm={() => {
          removal.confirm();
        }}
        isPending={removal.isPending}
        {...(removal.error === undefined ? {} : { errorMessage: removal.error })}
      />
    </>
  );
}
