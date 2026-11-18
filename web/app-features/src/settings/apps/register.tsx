// SPDX-License-Identifier: AGPL-3.0-only

// Registering an app, the screen under Apps at /settings/apps/new (ADR-0090 §1): a form saved as a whole, the fields
// of form.tsx (the name and the website, the redirect URIs and the scopes) and the type. Registered, the screen says
// so, then shows the app's client ID and, for a confidential app, its secret, which the server gives this once: the
// secret stands under the plain statement of that, each has its Copy, and the button at the screen's end goes back to
// Apps, whose list holds the app.
//
// What the server refuses of a field shows under that field, and anything else right before the button, in the
// server's words (25 apps already, too many registered within the hour). A server whose administrators turned
// registration off answers so only to a registration: from then on the screen says it in the form's place, as the
// section of Apps does.

import {
  oAuthAppRegistrationOff,
  useOAuthAppRegistrationOff,
  useOAuthAppWrites,
  type OAuthAppCreated,
} from "@my/app-data";
import {
  BackButton,
  Button,
  FormAlert,
  SettingsNote,
  SettingsScreen,
  SettingsSection,
  SubmitBar,
  useLocale,
} from "@my/design-system";
import { useState } from "react";
import { stringsIn } from "../../locale";
import type { SettingsSubProps } from "../entries";
import {
  AppAccessFields,
  AppIdentityFields,
  AppTypeChoice,
  missingOf,
  readApp,
  refusedOf,
  type AppFaults,
  type AppType,
} from "./form";
import { Once } from "./parts";

export function AppRegister({ onBack }: SettingsSubProps) {
  const { t } = stringsIn(useLocale().locale);
  const { create } = useOAuthAppWrites();
  const off = useOAuthAppRegistrationOff();
  const [type, setType] = useState<AppType>("confidential");
  const [busy, setBusy] = useState(false);
  const [faults, setFaults] = useState<AppFaults>({});
  const [said, setSaid] = useState<string>();
  /** The app the server just registered, with its secret: shown until the reader leaves. */
  const [made, setMade] = useState<OAuthAppCreated>();

  if (made !== undefined) {
    const secret = made.client_secret;
    return (
      <Once
        title={made.app.name}
        said={t("settings.apps.registered", { name: made.app.name })}
        about={t(secret === undefined ? "settings.apps.secret.none" : "settings.apps.registered.about")}
        values={[
          { what: "client_id", label: t("settings.apps.client_id"), value: made.app.client_id },
          ...(secret === undefined
            ? []
            : [{ what: "secret" as const, label: t("settings.apps.secret"), value: secret }]),
        ]}
        onDone={onBack}
      />
    );
  }

  const submit = (form: HTMLFormElement) => {
    if (busy) return;
    const values = readApp(form);
    const missing = missingOf(values, t);
    setFaults(missing);
    setSaid(undefined);
    if (Object.keys(missing).length > 0) return;
    setBusy(true);
    const { website, ...needed } = values;
    const request = { ...needed, client_type: type, ...(website === "" ? {} : { website }) };
    create(request).then(setMade, (error: unknown) => {
      setBusy(false);
      // The screen says it in the form's place, from what the data keeps of the refusal.
      if (oAuthAppRegistrationOff(error)) return;
      const { faults: refused, refused: placed } = refusedOf(error);
      setFaults(refused);
      setSaid(placed.reason ?? t("settings.confirm.failed"));
    });
  };

  return (
    <form
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        submit(event.currentTarget);
      }}
    >
      <SettingsScreen
        sub
        title={t("settings.apps.register")}
        back={<BackButton onPress={onBack}>{t("settings.entry.apps")}</BackButton>}
        foot={
          off ? undefined : (
            <SubmitBar result={said === undefined ? undefined : <FormAlert>{said}</FormAlert>}>
              <Button kind="primary" type="submit" busy={busy}>
                {t("settings.apps.register")}
              </Button>
            </SubmitBar>
          )
        }
      >
        {off && (
          <SettingsSection heading={t("settings.apps.own")}>
            <SettingsNote>{t("settings.apps.own.off")}</SettingsNote>
          </SettingsSection>
        )}
        {!off && (
          <SettingsSection heading={t("settings.apps.form.identity")}>
            <AppIdentityFields faults={faults} />
          </SettingsSection>
        )}
        {!off && (
          <SettingsSection heading={t("settings.apps.form.authorization")}>
            <AppAccessFields faults={faults} />
          </SettingsSection>
        )}
        {!off && (
          <SettingsSection heading={t("settings.apps.form.type")} about={t("settings.apps.form.type.about")}>
            <AppTypeChoice value={type} onChange={setType} />
          </SettingsSection>
        )}
      </SettingsScreen>
    </form>
  );
}
