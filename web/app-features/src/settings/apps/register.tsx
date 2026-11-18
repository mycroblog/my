// SPDX-License-Identifier: AGPL-3.0-only

// Registering an app, the screen under Apps at /settings/apps/new (ADR-0090 §1): a form saved as a whole, the fields
// of form.tsx (the name and the website, the redirect URIs and the scopes) and the type. Registered, the screen says
// so, then shows the app's client ID and, for a confidential app, its secret, which the server gives this once: the
// secret stands under the plain statement of that, each has its Copy, and the button at the screen's end goes back to
// Apps, whose list holds the app.
//
// A field left empty that an app needs, and what the server refuses of a field, show under that field, and the first
// such field takes the focus; anything else the server refuses shows right before the button, in the server's words
// (25 apps already, too many registered within the hour), and the focus stays there. Where the server's administrators turned
// registration off, which the list of the reader's apps says, the screen says so in the form's place from the first
// read, as the section of Apps does, and offers no form; until that list is read it says that it loads, or that it
// could not be read. Registration turned off while the form was open is told by the refusal, and the screen then says
// the same.

import {
  oAuthAppRegistrationOff,
  oAuthAppsQuery,
  useOAuthAppRegistrationOff,
  useOAuthAppsRead,
  useOAuthAppWrites,
  type AppData,
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
import { useRef, useState } from "react";
import { stringsIn } from "../../locale";
import type { SettingsSubProps } from "../entries";
import { useFaultFocus } from "../parts";
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
import { Once, Unread } from "./parts";

/**
 * What the loader of the screen waits for: the list of the reader's apps, which says whether the server lets users
 * register one, from the cache when it has it; a read that fails is the screen's to say.
 */
export async function loadAppRegister(data: AppData): Promise<void> {
  await data.queryClient.query({ ...oAuthAppsQuery(data), staleTime: Infinity }).catch(() => undefined);
}

export function AppRegister({ onBack }: SettingsSubProps) {
  const { t } = stringsIn(useLocale().locale);
  const { create } = useOAuthAppWrites();
  const { status, retry } = useOAuthAppsRead();
  const off = useOAuthAppRegistrationOff();
  const [type, setType] = useState<AppType>("confidential");
  const [busy, setBusy] = useState(false);
  const [faults, setFaults] = useState<AppFaults>({});
  const [said, setSaid] = useState<string>();
  /** The app the server just registered, with its secret: shown until the reader leaves. */
  const [made, setMade] = useState<OAuthAppCreated>();
  const formRef = useRef<HTMLFormElement>(null);
  const focusFault = useFaultFocus(formRef);

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
    if (Object.keys(missing).length > 0) {
      // Nothing is sent: the first field left empty takes the focus, with what it needs under it.
      focusFault();
      return;
    }
    setBusy(true);
    const { website, ...needed } = values;
    const request = { ...needed, client_type: type, ...(website === "" ? {} : { website }) };
    create(request).then(setMade, (error: unknown) => {
      setBusy(false);
      // Turned off since the list was read: the data corrects the list, and the screen says it in the form's place.
      if (oAuthAppRegistrationOff(error)) return;
      const { faults: refused, refused: placed } = refusedOf(error);
      setFaults(refused);
      setSaid(placed.reason ?? t("settings.confirm.failed"));
      // The field the server refused takes the focus; a refusal that names none stays before the button.
      focusFault();
    });
  };

  const back = <BackButton onPress={onBack}>{t("settings.entry.apps")}</BackButton>;
  // Whether the server lets users register apps is not known before the list is read: no form until then.
  if (status !== "read")
    return (
      <SettingsScreen
        sub
        title={t("settings.apps.register")}
        back={back}
        notice={<Unread status={status} retry={retry} />}
      />
    );

  return (
    <form
      ref={formRef}
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        submit(event.currentTarget);
      }}
    >
      <SettingsScreen
        sub
        title={t("settings.apps.register")}
        back={back}
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
