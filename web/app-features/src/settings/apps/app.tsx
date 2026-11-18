// SPDX-License-Identifier: AGPL-3.0-only

// An app the reader registered, the screen under Apps at /settings/apps/{id} (ADR-0090 §1):
//
// - Credentials: the app's client ID, to copy; for a confidential app, Make a new secret, after a confirmation that
//   says the secret it has stops working, and the new one is then shown this once, in the screen's place, with Copy
//   and the plain statement that it is not shown again; and Issue token, which opens its screen (token.tsx).
// - App settings: a form saved as a whole, the fields of form.tsx, and a sentence that says the app's type and why it
//   stays. What the server refuses of a field shows under that field, and what came of saving right before the button.
// - Delete app, after a confirmation that says every token of it stops working, for everyone, for good; deleted, the
//   screen goes back to Apps.
//
// While the server's administrators have the app disabled, a summary at the screen's top says so and what it means.
// An address that names no app of the reader's says so in the sections' place, with the way to the list.

import {
  oAuthAppQuery,
  useOAuthApp,
  useOAuthAppRead,
  useOAuthAppWrites,
  type AppData,
  type OAuthApp,
} from "@my/app-data";
import {
  BackButton,
  Button,
  FormAlert,
  ModalPresence,
  SaveState,
  SettingsList,
  SettingsListRow,
  SettingsNote,
  SettingsScreen,
  SettingsSection,
  StatusSummary,
  SubmitBar,
  useLocale,
} from "@my/design-system";
import { useState } from "react";
import { stringsIn } from "../../locale";
import type { SettingsSubProps } from "../entries";
import { Asking } from "../security/parts";
import { AppAccessFields, AppIdentityFields, missingOf, readApp, refusedOf, type AppFaults } from "./form";
import { appsPaths, Kept, Once, Unread, type AppScreenProps } from "./parts";

/**
 * What the loader of a screen of the reader's app of `id` waits for: the app, from the cache when it has it; a read
 * that fails is the screen's to say.
 */
export async function loadOwnApp(data: AppData, id: string): Promise<void> {
  await data.queryClient.query({ ...oAuthAppQuery(data, id), staleTime: Infinity }).catch(() => undefined);
}

/** The name of the reader's app of `id`, the title of its page; the list's name until the app is read. */
export function useOwnAppTitle(id: string): string {
  const { t } = stringsIn(useLocale().locale);
  return useOAuthApp(id)?.name ?? t("settings.apps.own");
}

export function OwnApp({ id, onBack }: SettingsSubProps & AppScreenProps) {
  const { t } = stringsIn(useLocale().locale);
  const { status, retry } = useOAuthAppRead(id);
  const read = useOAuthApp(id);
  const writes = useOAuthAppWrites();
  const [asking, setAsking] = useState<"secret" | "delete">();
  /** The app as it was when the reader asked to delete it: the screen keeps it while it is being left. */
  const [kept, setKept] = useState<OAuthApp>();
  /** The secret the server just made, shown until the reader says they kept it. */
  const [secret, setSecret] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [faults, setFaults] = useState<AppFaults>({});
  const [result, setResult] = useState<{ saved: true } | { saved: false; said: string }>();
  /** New fields after each save, which start from the app as the server left it. */
  const [round, setRound] = useState(0);
  const app = read ?? kept;
  const back = <BackButton onPress={onBack}>{t("settings.entry.apps")}</BackButton>;

  if (app === undefined)
    return (
      <SettingsScreen
        sub
        title={t("settings.apps.own")}
        back={back}
        notice={<Unread status={status === "read" ? "loading" : status} retry={retry} />}
      />
    );
  if (secret !== undefined)
    return (
      <Once
        title={app.name}
        heading={t("settings.apps.credentials")}
        about={t("settings.apps.secret.once")}
        said={t("settings.apps.secret.rotated")}
        values={[{ what: "secret", label: t("settings.apps.secret"), value: secret }]}
        onDone={() => {
          setSecret(undefined);
        }}
      />
    );

  const submit = (form: HTMLFormElement) => {
    if (busy) return;
    const values = readApp(form);
    const missing = missingOf(values, t);
    setFaults(missing);
    setResult(undefined);
    if (Object.keys(missing).length > 0) return;
    setBusy(true);
    writes.update(app.id, values).then(
      () => {
        setBusy(false);
        setRound((n) => n + 1);
        setResult({ saved: true });
      },
      (error: unknown) => {
        setBusy(false);
        const { faults: refused, refused: placed } = refusedOf(error);
        setFaults(refused);
        setResult({ saved: false, said: placed.reason ?? t("settings.confirm.failed") });
      },
    );
  };

  return (
    <>
      <form
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          submit(event.currentTarget);
        }}
      >
        <SettingsScreen
          sub
          title={app.name}
          back={back}
          foot={
            <SubmitBar
              result={
                result === undefined ? undefined : result.saved ? (
                  <SaveState state="saved" />
                ) : (
                  <FormAlert>{result.said}</FormAlert>
                )
              }
            >
              <Button kind="primary" type="submit" busy={busy}>
                {t("settings.apps.edit.submit")}
              </Button>
            </SubmitBar>
          }
        >
          {app.disabled && (
            <StatusSummary
              icon="caution"
              title={t("settings.apps.disabled")}
              about={t("settings.apps.disabled.about")}
            />
          )}
          <SettingsSection heading={t("settings.apps.credentials")}>
            <Kept what="client_id" label={t("settings.apps.client_id")} value={app.client_id} />
            <SettingsList label={t("settings.apps.credentials")}>
              {app.client_type === "confidential" && (
                <SettingsListRow
                  title={t("settings.apps.secret")}
                  caption={t("settings.apps.secret.caption")}
                  end={
                    <Button
                      size="timeline"
                      onPress={() => {
                        setAsking("secret");
                      }}
                    >
                      {t("settings.apps.secret.rotate")}
                    </Button>
                  }
                />
              )}
              <SettingsListRow
                title={t("settings.apps.token")}
                caption={t("settings.apps.token.caption")}
                href={appsPaths.token(app.id)}
              />
            </SettingsList>
          </SettingsSection>
          <SettingsSection key={round} heading={t("settings.apps.edit")} about={t("settings.apps.edit.about")}>
            <AppIdentityFields app={app} faults={faults} />
            <AppAccessFields app={app} faults={faults} />
            <SettingsNote>{t("settings.apps.form.type.fixed", { type: app.client_type })}</SettingsNote>
          </SettingsSection>
          <SettingsSection heading={t("settings.apps.delete.heading")}>
            <SettingsList label={t("settings.apps.delete.heading")}>
              <SettingsListRow
                title={t("settings.apps.delete.row")}
                caption={t("settings.apps.delete.about")}
                end={
                  <Button
                    size="timeline"
                    onPress={() => {
                      setKept(app);
                      setAsking("delete");
                    }}
                  >
                    {t("settings.apps.delete")}
                  </Button>
                }
              />
            </SettingsList>
          </SettingsSection>
        </SettingsScreen>
      </form>
      <ModalPresence>
        {asking === "secret" && (
          <Asking
            key="secret"
            title={t("settings.apps.secret.rotate.title", { app: app.name })}
            confirm={t("settings.apps.secret.rotate")}
            run={() => writes.rotateSecret(app.id).then(setSecret)}
            onClose={() => {
              setAsking(undefined);
            }}
          >
            {t("settings.apps.secret.rotate.text")}
          </Asking>
        )}
        {asking === "delete" && (
          <Asking
            key="delete"
            title={t("settings.apps.delete.title", { app: app.name })}
            confirm={t("settings.apps.delete")}
            run={() => writes.remove(app.id).then(onBack)}
            onClose={() => {
              setAsking(undefined);
            }}
          >
            {t("settings.apps.delete.about")}
          </Asking>
        )}
      </ModalPresence>
    </>
  );
}
