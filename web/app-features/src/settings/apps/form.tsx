// SPDX-License-Identifier: AGPL-3.0-only

// The fields of an app the reader registers (ADR-0090 §1), which the form that registers one and the form that
// changes one share: its name and its website, which the consent page shows; its redirect URIs, one on a line; and
// its scopes, the most it may ask for, by their names with spaces between. The fields keep what the reader types, and
// the form reads them as it is sent (`readApp`): a field left empty that an app needs says so under itself before
// anything is sent (`missingOf`), and what the server refuses of a field shows there too (`refusedOf`); the first
// field at fault then takes the focus (`useFaultFocus` of ../parts.tsx, on the form). The type, confidential or
// public, is chosen once, as the app is registered, between the two with what each means.

import type { OAuthApp } from "@my/app-data";
import { ChoiceList, TextField, useLocale } from "@my/design-system";
import { useId } from "react";
import { fieldError, placeRefusal, type Refused } from "../../field-errors";
import { stringsIn } from "../../locale";
import { scopesOf } from "./scopes";

type T = ReturnType<typeof stringsIn>["t"];

/** The most characters of an app's name, and the most redirect URIs it has (the API's `CreateOAuthAppRequest`). */
const appLimits = { name: 100, redirectUris: 10 } as const;

/** The fields of the form, by the member of the request each answers to. */
export type AppField = "name" | "website" | "redirect_uris" | "scope";
const members = { name: "name", website: "website", redirect_uris: "redirect_uris", scope: "scope" } as const;

/** What is wrong with the fields of the form, each under its own; none for most. */
export type AppFaults = Partial<Record<AppField, string>>;

/** The type of an app: one that keeps a secret, or one that cannot. */
export type AppType = OAuthApp["client_type"];

/** What the fields hold as the form is sent. */
export type AppValues = {
  name: string;
  /** Empty for none. */
  website: string;
  redirect_uris: string[];
  /** The scopes, a space between two. */
  scope: string;
};

/** The fields of `form` as the reader left them: trimmed, a redirect URI per line, one space between two scopes. */
export function readApp(form: HTMLFormElement): AppValues {
  const fields = new FormData(form);
  const text = (name: string) => {
    const value = fields.get(name);
    return typeof value === "string" ? value.trim() : "";
  };
  return {
    name: text("name"),
    website: text("website"),
    redirect_uris: text("redirect_uris")
      .split(/\r?\n/)
      .map((line) => line.trim())
      .filter((line) => line !== ""),
    scope: scopesOf(text("scope")).join(" "),
  };
}

/** What an app needs that `values` leave empty, each said under its field; none when all are there. */
export function missingOf(values: AppValues, t: T): AppFaults {
  return {
    ...(values.name === "" ? { name: t("settings.apps.form.missing", { field: "name" }) } : {}),
    ...(values.redirect_uris.length === 0
      ? { redirect_uris: t("settings.apps.form.missing", { field: "redirect_uris" }) }
      : {}),
    ...(values.scope === "" ? { scope: t("settings.apps.form.missing", { field: "scope" }) } : {}),
  };
}

/** A write the server refused, placed on the form: what is wrong under the field it names, and the form's summary. */
export function refusedOf(error: unknown): { faults: AppFaults; refused: Refused<AppField> } {
  const refused = placeRefusal<AppField>(error, members);
  const faults: AppFaults = {};
  for (const field of Object.values(members)) {
    const text = fieldError(refused, field);
    if (text !== undefined) faults[field] = text;
  }
  return { faults, refused };
}

type FieldsProps = {
  /** The app the form changes, whose settings the fields start from; none for a new app. */
  app?: OAuthApp | undefined;
  faults: AppFaults;
};

/** The name of the app and its website. */
export function AppIdentityFields({ app, faults }: FieldsProps) {
  const { t } = stringsIn(useLocale().locale);
  const id = useId();
  return (
    <>
      <TextField
        id={`${id}-name`}
        name="name"
        label={t("settings.apps.form.name")}
        hint={t("settings.apps.form.name.hint", { max: appLimits.name })}
        error={faults.name}
        defaultValue={app?.name}
        maxLength={appLimits.name}
        autoComplete="off"
      />
      <TextField
        id={`${id}-website`}
        name="website"
        label={t("settings.apps.form.website")}
        hint={t("settings.apps.form.website.hint")}
        error={faults.website}
        defaultValue={app?.website}
        autoComplete="off"
        autoCapitalize="none"
      />
    </>
  );
}

/** The redirect URIs of the app, one on a line, and the scopes it may ask for. */
export function AppAccessFields({ app, faults }: FieldsProps) {
  const { t } = stringsIn(useLocale().locale);
  const id = useId();
  return (
    <>
      <TextField
        id={`${id}-redirect`}
        name="redirect_uris"
        label={t("settings.apps.form.redirect_uris")}
        hint={t("settings.apps.form.redirect_uris.hint", { max: appLimits.redirectUris })}
        error={faults.redirect_uris}
        defaultValue={app?.redirect_uris.join("\n")}
        autoCapitalize="none"
        multiline
        // One a line: a second and a third stay in view.
        rows={3}
      />
      <TextField
        id={`${id}-scope`}
        name="scope"
        label={t("settings.apps.form.scope")}
        hint={t("settings.apps.form.scope.hint")}
        error={faults.scope}
        defaultValue={app?.scope}
        autoComplete="off"
        autoCapitalize="none"
      />
    </>
  );
}

/** The types of an app, in the order the form offers them. */
const appTypes = ["confidential", "public"] as const satisfies readonly AppType[];

/** The type of a new app, chosen among the two, each with what it means. */
export function AppTypeChoice({ value, onChange }: { value: AppType; onChange: (type: AppType) => void }) {
  const { t } = stringsIn(useLocale().locale);
  return (
    <ChoiceList
      label={t("settings.apps.form.type")}
      multiple={false}
      items={appTypes.map((type) => ({ key: type, label: t("settings.apps.form.type.choice", { type }) }))}
      chosen={[appTypes.indexOf(value)]}
      onChange={([chosen]) => {
        onChange(appTypes[chosen ?? 0] ?? "confidential");
      }}
    />
  );
}
