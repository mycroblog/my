// SPDX-License-Identifier: AGPL-3.0-only

// The password, the screen under Security at /settings/security/password (FR-002): a form saved as a whole that
// changes the password, or sets one for an account that has none, and under it the way to remove it. Both ask the
// reader to confirm it's them (confirmation.tsx): a change with the current password, a passkey or a linked account;
// a removal with a passkey or a linked account, never the password itself. Both end the account's other browsers and
// apps, which the screen says before, and says again once done; the lists follow by themselves (app-data).
//
// The password is removed only where another way to sign in remains, a passkey or a linked account: otherwise the
// row says why not, and has no button. Where the server limits sign-in to login providers (ADR-0106 §21–§22), no
// password can be set or changed, and the screen says so in the form's place; removing one stays.

import {
  refusedField,
  useHasPassword,
  useInstance,
  useLinkedIdentities,
  usePasskeys,
  usePasswordWrites,
  useSecurityRead,
  type Confirmation,
} from "@my/app-data";
import {
  BackButton,
  Button,
  FormAlert,
  SaveState,
  SettingsList,
  SettingsListRow,
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
import { Reading, securityPaths } from "./parts";

/** The least and the most characters of a password (the API's `new_password`). */
const passwordLength = { min: 8, max: 128 } as const;

export function SecurityPassword({ onBack, fragment, onFragmentRead, leave }: SettingsSubProps) {
  const { t } = stringsIn(useLocale().locale);
  const fieldId = useId();
  const { status, retry } = useSecurityRead();
  const hasPassword = useHasPassword() === true;
  const passkeys = usePasskeys() ?? [];
  const linked = useLinkedIdentities()?.items ?? [];
  const limited = useInstance()?.login_providers_only === true;
  const writes = usePasswordWrites();
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
  const removing = useConfirmation({
    operation: "password_removal",
    about: t("settings.security.password.remove.confirm"),
    returnTo: securityPaths.password,
    returned,
    leave,
    password: false,
  });
  const refused = (error: unknown) => {
    setResult({ done: false, said: saidOf(error, t) });
  };

  const submit = () => {
    if (typed.length < passwordLength.min || typed.length > passwordLength.max) {
      setFault(t("settings.security.password.new.invalid", passwordLength));
      return;
    }
    setFault(undefined);
    setResult(undefined);
    changing.ask((confirmed) => writes.change(typed, confirmed), {
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
        else refused(error);
      },
    });
  };
  const remove = () => {
    setResult(undefined);
    const run = (confirmed: Confirmation) =>
      // The dialog of a removal offers no password (`password: false`).
      confirmed.kind === "password"
        ? Promise.reject(new Error("settings: the password does not confirm its own removal"))
        : writes.remove(confirmed);
    removing.ask(run, {
      done: () => {
        setResult({ done: true, said: t("settings.security.password.removed") });
      },
      failed: refused,
    });
  };

  const said =
    result === undefined ? undefined : result.done ? (
      <SaveState state="saved">{result.said}</SaveState>
    ) : (
      <FormAlert>{result.said}</FormAlert>
    );
  const action = t(hasPassword ? "settings.security.password.change" : "settings.security.password.set");
  // Another way to sign in remains: a passkey, or a linked account.
  const removable = passkeys.length > 0 || linked.length > 0;
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
          foot={
            read && !limited ? (
              <SubmitBar result={said}>
                <Button kind="primary" type="submit" busy={changing.busy}>
                  {action}
                </Button>
              </SubmitBar>
            ) : undefined
          }
        >
          {!read ? (
            <Reading status={status} retry={retry} />
          ) : (
            <>
              {limited ? (
                <SettingsSection heading={action}>
                  <SettingsNote>{t("settings.security.providers_only")}</SettingsNote>
                  {said}
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
              )}
              {hasPassword && (
                <SettingsSection heading={t("settings.security.password.remove")}>
                  {removing.notice}
                  <SettingsList label={t("settings.security.password.remove")}>
                    <SettingsListRow
                      title={t("settings.security.password.remove.title")}
                      caption={t(
                        removable
                          ? "settings.security.password.remove.about"
                          : "settings.security.password.remove.last",
                      )}
                      end={
                        removable ? (
                          <Button size="timeline" busy={removing.busy} onPress={remove}>
                            {t("settings.security.password.remove.submit")}
                          </Button>
                        ) : undefined
                      }
                    />
                  </SettingsList>
                </SettingsSection>
              )}
            </>
          )}
        </SettingsScreen>
      </form>
      {changing.dialog}
      {removing.dialog}
    </>
  );
}
