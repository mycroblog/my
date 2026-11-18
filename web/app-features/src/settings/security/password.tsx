// SPDX-License-Identifier: AGPL-3.0-only

// The password, the screen under Security at /settings/security/password (FR-002): one form saved as a whole, the new
// password and, right after it, the button that changes it, or sets it for an account that has none. The reader
// confirms it's them first (confirmation.tsx): with the current password, a passkey or a linked account. It ends the
// account's other browsers and apps, which the screen says before, and says again once done; the lists follow by
// themselves (app-data). Removing the password is no part of this screen: it is a row of Security's Sign-in, so that
// nothing here stands between the field and its button.
//
// Where the server limits sign-in to login providers (ADR-0106 §21), no password can be set or changed, and the
// screen says so in the form's place.

import { refusedField, useHasPassword, useInstance, usePasswordWrites, useSecurityRead } from "@my/app-data";
import {
  BackButton,
  Button,
  FormAlert,
  SaveState,
  SettingsNote,
  SettingsScreen,
  SettingsSection,
  SubmitBar,
  TextField,
  useLocale,
} from "@my/design-system";
import { useId, useState } from "react";
import { stringsIn } from "../../locale";
import { saidOf, useConfirmation, useProviderReturn } from "../confirmation";
import type { SettingsSubProps } from "../entries";
import { securityPaths, useReadingNotice } from "./parts";

/** The least and the most characters of a password (the API's `new_password`). */
const passwordLength = { min: 8, max: 128 } as const;

export function SecurityPassword({ onBack, fragment, onFragmentRead, leave }: SettingsSubProps) {
  const { t } = stringsIn(useLocale().locale);
  const fieldId = useId();
  const { status, retry } = useSecurityRead();
  const notice = useReadingNotice(status, retry);
  const hasPassword = useHasPassword() === true;
  const limited = useInstance()?.login_providers_only === true;
  const { change } = usePasswordWrites();
  const returned = useProviderReturn({ fragment, onFragmentRead });
  const [typed, setTyped] = useState("");
  /** A new field after each password taken, empty again. */
  const [round, setRound] = useState(0);
  const [fault, setFault] = useState<string>();
  const [result, setResult] = useState<{ done: boolean; said: string }>();
  const changing = useConfirmation({
    operation: "password_change",
    about: t(hasPassword ? "settings.security.password.change.confirm" : "settings.security.password.set.confirm"),
    returnTo: securityPaths.password,
    returned,
    leave,
  });

  const submit = () => {
    if (typed.length < passwordLength.min || typed.length > passwordLength.max) {
      setFault(t("settings.security.password.new.invalid", passwordLength));
      return;
    }
    setFault(undefined);
    setResult(undefined);
    changing.ask((confirmed) => change(typed, confirmed), {
      done: () => {
        setTyped("");
        setRound((n) => n + 1);
        setResult({
          done: true,
          said: t(hasPassword ? "settings.security.password.changed" : "settings.security.password.set.done"),
        });
      },
      failed: (error) => {
        const field = refusedField(error);
        if (field?.member === "new_password") setFault(field.detail);
        else setResult({ done: false, said: saidOf(error, t) });
      },
    });
  };

  const action = t(hasPassword ? "settings.security.password.change" : "settings.security.password.set");
  const read = status === "read";
  return (
    <>
      <form
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          submit();
        }}
      >
        <SettingsScreen
          sub
          title={t("settings.security.password")}
          back={<BackButton onPress={onBack}>{t("settings.entry.security")}</BackButton>}
          notice={notice}
          foot={
            read && !limited ? (
              <SubmitBar
                result={
                  result === undefined ? undefined : result.done ? (
                    <SaveState state="saved">{result.said}</SaveState>
                  ) : (
                    <FormAlert>{result.said}</FormAlert>
                  )
                }
              >
                <Button kind="primary" type="submit" busy={changing.busy}>
                  {action}
                </Button>
              </SubmitBar>
            ) : undefined
          }
        >
          {read &&
            (limited ? (
              <SettingsSection heading={action}>
                <SettingsNote>{t("settings.security.providers_only")}</SettingsNote>
              </SettingsSection>
            ) : (
              <SettingsSection heading={action} about={t("settings.security.password.ends")}>
                {changing.notice}
                <TextField
                  key={round}
                  id={fieldId}
                  name="new_password"
                  type="password"
                  label={t("settings.security.password.new")}
                  hint={t("settings.security.password.new.hint", passwordLength)}
                  error={fault}
                  autoComplete="new-password"
                  onChange={setTyped}
                />
              </SettingsSection>
            ))}
        </SettingsScreen>
      </form>
      {changing.dialog}
    </>
  );
}
