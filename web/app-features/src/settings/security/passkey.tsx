// SPDX-License-Identifier: AGPL-3.0-only

// Adding a passkey, the screen under Security at /settings/security/passkey (FR-004, ADR-0106 §26): the reader names
// it, confirms it's them (confirmation.tsx), and the browser's own dialog makes it; the screen then goes back to
// Security, whose list holds it. What stops it says what happened, right before the button: the browser's dialog closed
// or timed out, the device holds a passkey of the account already, the browser has no passkeys (said before anything
// is asked), or the server's refusal in its own words (the 20 an account may hold). Where the server limits sign-in to
// login providers (ADR-0106 §21), no passkey can be added, and the screen says so in the form's place. The form comes
// once what Security shows is read, the ways the account confirms with among it; until then the screen says that it
// loads, or that it could not be read.

import { refusedMember, useInstance, usePasskeyWrites, useSecurityRead } from "@my/app-data";
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
import { useReadingNotice } from "../parts";
import { securityPaths, useFieldFault, type SecuritySubProps } from "./parts";

/** The most characters of a passkey's name (the API's `name`). */
const nameLength = 64;

export function SecurityPasskey({ onBack, fragment, onFragmentRead, leave }: SecuritySubProps) {
  const { t } = stringsIn(useLocale().locale);
  const fieldId = useId();
  // The ways the reader confirms with are the account's own (its password, its passkeys, its linked accounts): no
  // form before they are read, or the confirmation would say the account has none.
  const { status, retry } = useSecurityRead();
  const notice = useReadingNotice(status, retry);
  const read = status === "read";
  const limited = useInstance()?.login_providers_only === true;
  const { add } = usePasskeyWrites();
  const returned = useProviderReturn({ fragment, onFragmentRead });
  const [typed, setTyped] = useState("");
  const fault = useFieldFault(fieldId);
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
      fault.say(t("settings.security.passkeys.name.invalid", { max: nameLength }));
      return;
    }
    fault.clear();
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
        // What the server refused of the name stands under its field, which takes the focus once the dialog left.
        const refused = refusedMember(error);
        if (refused?.member === "name") fault.say(refused.said, false);
        else setSaid(saidOf(error, t));
      },
      left: fault.focus,
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
          notice={notice}
          foot={
            read && !limited ? (
              <SubmitBar result={said === undefined ? undefined : <FormAlert>{said}</FormAlert>}>
                <Button kind="primary" type="submit" busy={confirmation.busy}>
                  {t("settings.security.passkeys.add")}
                </Button>
              </SubmitBar>
            ) : undefined
          }
        >
          {read &&
            (limited ? (
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
                  error={fault.words}
                  maxLength={nameLength}
                  onChange={setTyped}
                />
              </SettingsSection>
            ))}
        </SettingsScreen>
      </form>
      {confirmation.dialog}
    </>
  );
}
