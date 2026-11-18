// SPDX-License-Identifier: AGPL-3.0-only

// Adding a passkey, the screen under Security at /settings/security/passkey (FR-004, ADR-0106 §26): the reader names
// it, confirms it's them (confirmation.tsx), and the browser's own dialog makes it; the screen then goes back to
// Security, whose list holds it. What stops it says what happened, right before the button: the browser's dialog closed
// or timed out, the device holds a passkey of the account already, the browser has no passkeys (said before anything
// is asked), or the server's refusal in its own words (the 20 an account may hold). Where the server limits sign-in to
// login providers (ADR-0106 §21), no passkey can be added, and the screen says so in the form's place.

import { refusedField, useInstance, usePasskeyWrites } from "@my/app-data";
import {
  BackButton,
  Button,
  FormAlert,
  SettingsNote,
  SettingsScreen,
  SettingsSection,
  SubmitBar,
  TextField,
  useLocale,
} from "@my/design-system";
import { PasskeyError, supportsPasskeys } from "@my/platform";
import { useId, useState } from "react";
import { stringsIn } from "../../locale";
import { saidOf, useConfirmation, useProviderReturn } from "../confirmation";
import type { SettingsSubProps } from "../entries";
import { securityPaths } from "./parts";

/** The most characters of a passkey's name (the API's `name`). */
const nameLength = 64;

export function SecurityPasskey({ onBack, fragment, onFragmentRead, leave }: SettingsSubProps) {
  const { t } = stringsIn(useLocale().locale);
  const fieldId = useId();
  const limited = useInstance()?.login_providers_only === true;
  const { add } = usePasskeyWrites();
  const returned = useProviderReturn({ fragment, onFragmentRead });
  const [typed, setTyped] = useState("");
  const [fault, setFault] = useState<string>();
  const [said, setSaid] = useState<string>();
  const confirmation = useConfirmation({
    operation: "passkey_registration",
    about: t("settings.security.passkeys.add.confirm"),
    returnTo: securityPaths.passkey,
    returned,
    leave,
  });

  const submit = () => {
    const name = typed.trim();
    if (name === "" || name.length > nameLength) {
      setFault(t("settings.security.passkeys.name.invalid", { max: nameLength }));
      return;
    }
    setFault(undefined);
    // Said before a proof is asked that would be for nothing.
    if (!supportsPasskeys()) {
      setSaid(t("settings.security.passkeys.add.failed", { reason: "unsupported" }));
      return;
    }
    setSaid(undefined);
    confirmation.ask((confirmed) => add(name, confirmed), {
      done: onBack,
      failed: (error) => {
        if (error instanceof PasskeyError) {
          // Ended by the app itself: nothing to tell.
          if (error.reason !== "aborted") setSaid(t("settings.security.passkeys.add.failed", { reason: error.reason }));
          return;
        }
        const field = refusedField(error);
        if (field?.member === "name") setFault(field.detail);
        else setSaid(saidOf(error, t));
      },
    });
  };

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
          title={t("settings.security.passkeys.add")}
          back={<BackButton onPress={onBack}>{t("settings.entry.security")}</BackButton>}
          foot={
            limited ? undefined : (
              <SubmitBar result={said === undefined ? undefined : <FormAlert>{said}</FormAlert>}>
                <Button kind="primary" type="submit" busy={confirmation.busy}>
                  {t("settings.security.passkeys.add")}
                </Button>
              </SubmitBar>
            )
          }
        >
          {limited ? (
            <SettingsSection heading={t("settings.security.passkeys.new")}>
              <SettingsNote>{t("settings.security.providers_only")}</SettingsNote>
            </SettingsSection>
          ) : (
            <SettingsSection
              heading={t("settings.security.passkeys.new")}
              about={t("settings.security.passkeys.add.about")}
            >
              {confirmation.notice}
              <TextField
                id={fieldId}
                name="name"
                label={t("settings.security.passkeys.name")}
                hint={t("settings.security.passkeys.name.hint")}
                error={fault}
                maxLength={nameLength}
                onChange={setTyped}
              />
            </SettingsSection>
          )}
        </SettingsScreen>
      </form>
      {confirmation.dialog}
    </>
  );
}
