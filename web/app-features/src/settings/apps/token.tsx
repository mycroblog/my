// SPDX-License-Identifier: AGPL-3.0-only

// Issuing a token, the screen under an app the reader registered, at /settings/apps/{id}/token (ADR-0090 §2, §17): an
// access token and a refresh token of the app for the reader's own account, with no consent page and no redirect. The
// form holds the scopes, by their names, which start as all of the app's that the account may put in a token: the
// reader takes out what the token should not carry, and an emptied field asks for the ones it started with, never for
// nothing: the server reads a request without scopes as the app's whole scope, and refuses it where that holds a
// permission scope the account cannot have. Issued, the screen says so, with the token's scopes and how long it
// works, then shows the two tokens, which the server gives this once: each stands under its name and the plain
// statement that it is not shown again, with its Copy, and the button at the screen's end goes back to the app.
//
// A permission scope (`admin:<permission>`) is only ever carried by a token issued here, and only for a permission
// the reader's account has: the screen says so, and the field starts without the ones the account cannot have, which
// the account's permissions say: the form comes once the app and the account are both read. What the server refuses
// of the scopes shows under the field, which takes the focus; an app the administrators disabled gets no token, which
// the screen says in the form's place.

import { useOAuthApp, useOAuthAppTokenRead, useOAuthAppWrites, usePermissions, type OAuthAppToken } from "@my/app-data";
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
import { useId, useRef, useState } from "react";
import { stringsIn } from "../../locale";
import type { SettingsSubProps } from "../entries";
import { refusedOf, useFaultFocus } from "./form";
import { Once, Unread, type AppScreenProps } from "./parts";
import { scopesOf } from "./scopes";

/** The permission a permission scope names (`admin:manage_invites`); undefined for any other scope. */
function permissionOf(scope: string): string | undefined {
  const name = /^admin:(.+)$/.exec(scope)?.[1];
  // The broad administration scopes are no permission's, and no app has them.
  return name === undefined || name === "read" || name === "write" ? undefined : name;
}

export function AppToken({ id, onBack }: SettingsSubProps & AppScreenProps) {
  const { t } = stringsIn(useLocale().locale);
  const fieldId = useId();
  const { status, retry } = useOAuthAppTokenRead(id);
  const app = useOAuthApp(id);
  const permissions: readonly string[] = usePermissions();
  const { issueToken } = useOAuthAppWrites();
  const [busy, setBusy] = useState(false);
  const [fault, setFault] = useState<string>();
  const [said, setSaid] = useState<string>();
  /** The tokens the server just issued, shown until the reader leaves. */
  const [made, setMade] = useState<OAuthAppToken>();
  const formRef = useRef<HTMLFormElement>(null);
  const focusFault = useFaultFocus(formRef);

  if (made !== undefined)
    return (
      <Once
        title={t("settings.apps.token")}
        said={t("settings.apps.token.issued")}
        about={t("settings.apps.token.facts", { scope: made.scope, minutes: Math.round(made.expires_in / 60) })}
        values={[
          { what: "access_token", label: t("settings.apps.token.access"), value: made.access_token },
          { what: "refresh_token", label: t("settings.apps.token.refresh"), value: made.refresh_token },
        ]}
        onDone={onBack}
      />
    );

  const back = <BackButton onPress={onBack}>{app?.name ?? t("settings.entry.apps")}</BackButton>;
  // No field before the account is read too: it starts from the account's permissions, and takes its start once.
  if (app === undefined || status !== "read")
    return (
      <SettingsScreen
        sub
        title={t("settings.apps.token")}
        back={back}
        notice={<Unread status={status === "read" ? "loading" : status} retry={retry} />}
      />
    );

  // The scopes the reader may put in a token: the app's, without the permission scopes their account cannot have.
  const offered = scopesOf(app.scope)
    .filter((scope) => {
      const permission = permissionOf(scope);
      return permission === undefined || permissions.includes(permission);
    })
    .join(" ");

  const submit = (form: HTMLFormElement) => {
    if (busy) return;
    const typed = new FormData(form).get("scope");
    const scope = scopesOf(typeof typed === "string" ? typed : "").join(" ");
    setFault(undefined);
    setSaid(undefined);
    setBusy(true);
    // An emptied field asks for what it started with: sent without scopes, the request would ask for the app's whole
    // scope, the permission scopes the account cannot have among it. Only where the field started empty too (the app
    // has no scope the account may have) is none sent, and the server says why not.
    const asked = scope === "" ? offered : scope;
    issueToken(app.id, asked === "" ? undefined : asked).then(setMade, (error: unknown) => {
      setBusy(false);
      const { faults, refused } = refusedOf(error);
      setFault(faults.scope);
      setSaid(refused.reason ?? t("settings.confirm.failed"));
      // The scopes the server refused take the focus; a refusal of anything else stays before the button.
      focusFault();
    });
  };

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
        title={t("settings.apps.token")}
        back={back}
        foot={
          app.disabled ? undefined : (
            <SubmitBar result={said === undefined ? undefined : <FormAlert>{said}</FormAlert>}>
              <Button kind="primary" type="submit" busy={busy}>
                {t("settings.apps.token")}
              </Button>
            </SubmitBar>
          )
        }
      >
        {app.disabled ? (
          <SettingsSection heading={t("settings.apps.token.heading")}>
            <SettingsNote>{t("settings.apps.token.disabled")}</SettingsNote>
          </SettingsSection>
        ) : (
          <SettingsSection heading={t("settings.apps.token.heading")} about={t("settings.apps.token.about")}>
            <TextField
              id={fieldId}
              name="scope"
              label={t("settings.apps.form.scope")}
              hint={t("settings.apps.token.scope.hint")}
              error={fault}
              defaultValue={offered}
              autoComplete="off"
              autoCapitalize="none"
            />
            <SettingsNote>{t("settings.apps.token.admin")}</SettingsNote>
          </SettingsSection>
        )}
      </SettingsScreen>
    </form>
  );
}
