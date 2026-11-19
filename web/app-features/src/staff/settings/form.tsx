// SPDX-License-Identifier: AGPL-3.0-only

// A page of the instance's settings (topics.ts) as a form saved as a whole: its sections and the submit bar, Save
// sending only what changed, the settings all together or none (PATCH /admin/settings), the texts each by its key and
// the pictures uploaded first. A setting is drawn by what it holds; its name and, where its effect is not plain, a line
// about it; a choice whose options each change much says what each does under it, before anything is saved.
//
// What saving would end or take away is asked first, with the server's own count: switching federation to the
// allowlist (the follows it ends for good, with how many accounts and which servers); limiting sign-in to the login
// providers (who could no longer sign in, the reader among them or not); and a maximum age of the server's posts, set
// or made shorter (old posts are deleted). On a one-person server the sections of signing up say that nobody else signs
// up, in place of their settings. The mode of a decision hook goes from off to enforce only through observe, so enforce
// is not offered while it is off.
//
// What is wrong stands under what it is about, which takes the focus: a number out of its bounds before sending, and
// the key the server names, in the words of its rule; the submit bar then says only that nothing was saved. Leaving
// with changes not saved asks first (`leaving`).

import { languageName } from "@my/app-components";
import {
  refusalDetail,
  refusedField,
  useAdminSettingsWrites,
  useFederationScopeImpact,
  useLoginProvidersOnlyImpact,
  type AdminSetting,
  type AdminSettingKey,
  type InstanceTextKey,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  CheckField,
  ChoiceList,
  ConfirmDialog,
  FieldError,
  focusRefusedField,
  FormAlert,
  ModalPresence,
  NumberField,
  PictureField,
  SaveState,
  Select,
  SettingRow,
  SettingsScreen,
  SettingsSection,
  Stack,
  SubmitBar,
  TextField,
} from "@my/design-system";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { useStaffStrings, type T } from "../parts";
import { Irreversible, LeaveDialog, OnOffRow, type AdminSubProps } from "./parts";
import { ProvidersField, type ProvidersValue } from "./providers";
import { factorOf, keysOf, textsOf, type SettingField, type SettingTopic } from "./topics";
import {
  choiceAbouts,
  choiceNames,
  optionWords,
  sectionAbouts,
  sectionHeadings,
  settingAbouts,
  settingNames,
  textLabels,
  wordOf,
} from "./words";

/** The words of an option of a select, by the family of settings that share them. */
const families: Readonly<Partial<Record<AdminSettingKey, string>>> = {
  local_timeline_visibility: "audience",
  federated_timeline_visibility: "audience",
  domain_actions_visibility: "audience",
  domain_actions_rationale_visibility: "audience",
  invite_creators: "creators",
  event_creators: "creators",
  article_authoring: "authoring",
  remote_media_policy: "remote_media",
  search_visibilities: "search_visibilities",
  hook_inbound_status_mode: "hook_mode",
  hook_inbound_follow_mode: "hook_mode",
  hook_signup_check_mode: "hook_mode",
  hook_media_check_mode: "hook_mode",
  hook_local_status_mode: "hook_mode",
  hook_inbound_actor_mode: "hook_mode",
  hook_local_profile_mode: "hook_mode",
  hook_inbound_status_on_failure: "hook_failure",
  hook_inbound_follow_on_failure: "hook_failure",
  hook_signup_check_on_failure: "hook_failure",
  hook_media_check_on_failure: "hook_failure",
  hook_inbound_actor_on_failure: "hook_failure",
};

/** The settings whose change cannot be undone, marked under their row; and the options of a choice that cannot. */
const irreversibleKeys: ReadonlySet<string> = new Set(["login_providers_only", "auto_delete_max_age_days"]);
const irreversibleChoices: Readonly<Record<string, string>> = { federation_scope: "allowlist" };

/** The languages the app is written in, which a server's default may be. */
const appLanguages = ["en", "ja", "de", "es", "fr", "pt-BR", "zh-CN", "zh-TW"];

/** An option's value as the catalog's select takes it: `public,local` is `public_local`. */
const optionOf = (value: string) => value.replace(/[^a-z0-9_]/g, "_");

/** The images of the general page, uploaded as Save is pressed. */
type Images = Partial<Record<"thumbnail" | "preview_image", { readonly file: File; readonly url: string }>>;

/** What the form holds: the settings by key, as the server writes them; the texts by key; the pictures picked. */
type Form = {
  readonly values: Readonly<Record<string, string>>;
  readonly texts: Readonly<Partial<Record<InstanceTextKey, string>>>;
  readonly images: Images;
};

/** Where the last save is: refused, with what is wrong and where, or why in the submit bar's words. */
type Sent =
  | { readonly kind: "idle" | "sending" | "saved" }
  | {
      readonly kind: "refused";
      readonly faults?: Readonly<Record<string, string>> | undefined;
      readonly reason?: string | undefined;
    };

/** What Save asks before saving. */
type Ask = "scope" | "providers_only" | "auto_delete_max";

/** The rule a value of `setting` follows, in the form's words: the server says only that it is not valid. */
function ruleOf(t: T, setting: AdminSetting): string {
  const factor = factorOf(setting.key);
  return t("staff_settings.rule", {
    kind: setting.kind,
    min: Math.ceil((setting.minimum ?? 0) / factor),
    max: Math.floor((setting.maximum ?? 0) / factor),
  });
}

/** The form's settings and texts as the server has them. */
function formOf(
  topic: SettingTopic,
  settings: ReadonlyMap<AdminSettingKey, AdminSetting>,
  texts: Readonly<Partial<Record<InstanceTextKey, string>>>,
): Form {
  return {
    values: Object.fromEntries(keysOf(topic).map((k) => [k, settings.get(k)?.value ?? ""])),
    texts: Object.fromEntries(textsOf(topic).map((k) => [k, texts[k] ?? ""])),
    images: {},
  };
}

export type TopicFormProps = Partial<AdminSubProps> & {
  readonly topic: SettingTopic;
  readonly title: string;
  /** The way back, its words and where it goes, for a page under an entry; an entry's own screen has none. */
  readonly back?: { readonly label: string; readonly href: string } | undefined;
  readonly settings: ReadonlyMap<AdminSettingKey, AdminSetting>;
  readonly texts: Readonly<Partial<Record<InstanceTextKey, string>>>;
  /** Sections that only state something, over the form's own (how translation is used this month). */
  readonly before?: ReactNode;
};

/** A topic's settings as a form saved as a whole. */
export function TopicForm({
  topic,
  title,
  back,
  settings,
  texts,
  before,
  go,
  onUnsaved,
  leaving,
}: TopicFormProps): ReactNode {
  const { t } = useStaffStrings();
  const id = useId();
  const writes = useAdminSettingsWrites();
  const [base, setBase] = useState<Form>(() => formOf(topic, settings, texts));
  const [form, setForm] = useState<Form>(base);
  const [sent, setSent] = useState<Sent>({ kind: "idle" });
  const [asking, setAsking] = useState<Ask | undefined>(undefined);
  // The picture of an image uploaded here, shown under its new value until a read of the settings carries its image
  // (the server leaves it out while the file is processed).
  const [shownImages, setShown] = useState<Readonly<Record<string, { readonly id: string; readonly url: string }>>>({});
  const busy = sent.kind === "sending";
  const focusNext = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (focusNext.current === undefined) return;
    focusRefusedField(focusNext.current);
    focusNext.current = undefined;
  });

  const changedKeys = Object.keys(form.values).filter((k) => form.values[k] !== base.values[k]);
  const changedTexts = (Object.keys(form.texts) as InstanceTextKey[]).filter((k) => form.texts[k] !== base.texts[k]);
  const unsaved = changedKeys.length > 0 || changedTexts.length > 0 || Object.keys(form.images).length > 0;
  useEffect(() => {
    onUnsaved?.(unsaved && !busy);
  }, [onUnsaved, unsaved, busy]);
  useEffect(
    () => () => {
      onUnsaved?.(false);
    },
    [onUnsaved],
  );

  // The impacts are asked for once the reader chose what has one, so that the question has its count.
  const scopeImpact = useFederationScopeImpact(
    form.values["federation_scope"] === "allowlist" && base.values["federation_scope"] !== "allowlist",
  );
  const providersImpact = useLoginProvidersOnlyImpact(
    form.values["login_providers_only"] === "true" && base.values["login_providers_only"] !== "true",
  );

  /** A change of the form: what the last save said was about the form as it was. */
  const change = (part: Partial<Form>) => {
    setForm((now) => ({ ...now, ...part }));
    setSent((was) => (was.kind === "saved" || was.kind === "refused" ? { kind: "idle" } : was));
  };
  const setValue = (key: string, value: string) => {
    change({ values: { ...form.values, [key]: value } });
  };

  /** The id of what takes the focus for a key or a text. */
  const controlOf = (name: string) => `${id}-${name}`;
  /** The id of the setting row of a key, whose control is `<row>-control`. */
  const rowOf = (key: string) => `${id}-row-${key}`;
  /** Where a fault of `name` takes the focus. */
  const focusOf = (name: string): string => {
    const field = topic.sections.flatMap((s) => s.fields).find((f) => fieldNames(f).includes(name));
    if (field === undefined) return controlOf(name);
    if (field.kind === "setting") return field.as === "choices" ? controlOf(name) : `${rowOf(name)}-control`;
    if (field.kind === "checks") return controlOf(field.keys[0] ?? name);
    if (field.kind === "image") return name === field.alt ? controlOf(name) : controlOf(field.key);
    if (field.kind === "providers") return `${controlOf("providers")}-control`;
    return controlOf(name);
  };

  /** Marks `faults` (by key or text) and puts the focus on the first in the form's order. */
  const refuse = (faults: Readonly<Record<string, string>>) => {
    setSent({ kind: "refused", faults });
    const order = topic.sections.flatMap((s) => s.fields.flatMap(fieldNames));
    const first = order.find((n) => faults[n] !== undefined);
    if (first !== undefined) focusNext.current = focusOf(first);
  };

  /** What the form finds wrong before sending: a number left empty. */
  const ownFaults = (): Record<string, string> =>
    Object.fromEntries(
      topic.sections
        .flatMap((s) => s.fields)
        .flatMap((f) => (f.kind === "setting" ? [f] : []))
        .flatMap((f) => {
          const setting = settings.get(f.key);
          return setting?.kind === "int" && form.values[f.key] === "" ? [[f.key, ruleOf(t, setting)]] : [];
        }),
    );

  const send = async () => {
    setAsking(undefined);
    setSent({ kind: "sending" });
    // Each part that succeeds is kept as done, so that a save tried again sends only what did not go: an image
    // uploaded becomes its setting's value (its picture shown meanwhile), a text recorded becomes the text saved.
    const uploaded: Record<string, string> = {};
    let failing: string | undefined;
    try {
      for (const [key, image] of Object.entries(form.images)) {
        failing = key;
        const id = await writes.upload(image.file);
        uploaded[key] = id;
        setShown((now) => ({ ...now, [key]: { id, url: image.url } }));
        setForm((now) => ({
          ...now,
          values: { ...now.values, [key]: id },
          images: Object.fromEntries(Object.entries(now.images).filter(([k]) => k !== key)),
        }));
      }
      for (const key of changedTexts) {
        failing = key;
        const body = form.texts[key] ?? "";
        await writes.setText(key, body);
        setBase((now) => ({ ...now, texts: { ...now.texts, [key]: body } }));
      }
      failing = undefined;
      const values = { ...Object.fromEntries(changedKeys.map((k) => [k, form.values[k] ?? ""])), ...uploaded };
      if (Object.keys(values).length > 0) await writes.save(values);
      setBase((now) => ({ ...now, values: { ...now.values, ...values } }));
      setSent({ kind: "saved" });
    } catch (error) {
      const named = refusedField(error);
      const key = named?.member.startsWith("values.") === true ? named.member.slice("values.".length) : undefined;
      const setting = key === undefined ? undefined : settings.get(key as AdminSettingKey);
      if (key !== undefined && setting !== undefined) refuse({ [key]: ruleOf(t, setting) });
      else if (failing !== undefined && named !== undefined) refuse({ [failing]: named.detail });
      else setSent({ kind: "refused", reason: refusalDetail(error) });
    }
  };

  const save = () => {
    if (busy) return;
    const faults = ownFaults();
    if (Object.keys(faults).length > 0) {
      refuse(faults);
      return;
    }
    if (!unsaved) {
      if (back !== undefined) go?.(back.href);
      return;
    }
    const v = form.values;
    const was = base.values;
    if (v["federation_scope"] === "allowlist" && was["federation_scope"] !== "allowlist") setAsking("scope");
    else if (v["login_providers_only"] === "true" && was["login_providers_only"] !== "true")
      setAsking("providers_only");
    else if (
      v["auto_delete_max_age_days"] !== undefined &&
      Number(v["auto_delete_max_age_days"]) > 0 &&
      (Number(was["auto_delete_max_age_days"]) === 0 ||
        Number(v["auto_delete_max_age_days"]) < Number(was["auto_delete_max_age_days"]))
    )
      setAsking("auto_delete_max");
    else void send();
  };

  const faults = sent.kind === "refused" ? (sent.faults ?? {}) : {};
  const single = base.values["single_user"] === "true";

  /** A field of a section, drawn by what it holds. */
  const field = (f: SettingField, alone?: string): ReactNode => {
    if (f.kind === "text") {
      const key = f.key;
      return (
        <TextField
          key={key}
          id={controlOf(key)}
          name={key}
          label={t(wordOf(textLabels, key))}
          row
          hint={key === "name" ? t("staff_settings.text.name.about") : undefined}
          defaultValue={form.texts[key] ?? ""}
          multiline={f.lines > 1}
          rows={f.lines > 1 ? f.lines : undefined}
          error={faults[key]}
          onChange={(value) => {
            change({ texts: { ...form.texts, [key]: value } });
          }}
        />
      );
    }
    if (f.kind === "image") {
      const setting = settings.get(f.key);
      const picked = form.images[f.key];
      const value = form.values[f.key] ?? "";
      const kept = shownImages[f.key];
      const shown =
        picked?.url ??
        (value === ""
          ? undefined
          : setting?.value === value && setting.image !== undefined
            ? setting.image.url
            : kept?.id === value
              ? kept.url
              : setting?.image?.url);
      return (
        <Stack key={f.key} gap="screen-row-gap">
          <PictureField
            id={controlOf(f.key)}
            label={t(wordOf(settingNames, f.key))}
            hint={t(wordOf(settingAbouts, f.key))}
            shape="banner"
            src={shown}
            error={faults[f.key]}
            choose={t("staff_settings.image.choose", { set: shown === undefined ? "no" : "yes" })}
            onFile={(file) => {
              change({ images: { ...form.images, [f.key]: { file, url: URL.createObjectURL(file) } } });
            }}
            remove={
              shown === undefined
                ? undefined
                : {
                    label: t("staff_settings.image.remove"),
                    onPress: () => {
                      const rest: Images = Object.fromEntries(Object.entries(form.images).filter(([k]) => k !== f.key));
                      change({ images: rest, values: { ...form.values, [f.key]: "" } });
                    },
                  }
            }
            isDisabled={busy}
          />
          <TextField
            id={controlOf(f.alt)}
            name={f.alt}
            label={t(wordOf(textLabels, f.alt))}
            row
            defaultValue={form.texts[f.alt] ?? ""}
            multiline
            rows={2}
            error={faults[f.alt]}
            onChange={(value) => {
              change({ texts: { ...form.texts, [f.alt]: value } });
            }}
          />
        </Stack>
      );
    }
    if (f.kind === "checks")
      return (
        <Stack key={f.id}>
          {f.keys.map((key, i) => (
            <CheckField
              key={key}
              id={controlOf(key)}
              name={key}
              value="true"
              label={t(wordOf(settingNames, key))}
              checked={form.values[key] === "true"}
              onChange={(on) => {
                setValue(key, on ? "true" : "false");
              }}
              error={i === 0 ? f.keys.map((k) => faults[k]).find((e) => e !== undefined) : undefined}
              touch
              disabled={busy}
            />
          ))}
        </Stack>
      );
    if (f.kind === "providers")
      return (
        <ProvidersField
          key="providers"
          id={controlOf("providers")}
          value={{
            list: form.values["translation_providers"] ?? "",
            visibilities: form.values["translation_provider_visibilities"] ?? "",
          }}
          error={faults["translation_providers"] ?? faults["translation_provider_visibilities"]}
          disabled={busy}
          onChange={(next: ProvidersValue) => {
            change({
              values: {
                ...form.values,
                translation_providers: next.list,
                translation_provider_visibilities: next.visibilities,
              },
            });
          }}
        />
      );
    return settingField(f, alone);
  };

  /** A setting drawn by its kind: a list of its options, a select, a number, a text. */
  const settingField = (f: SettingField & { kind: "setting" }, alone: string | undefined): ReactNode => {
    const setting = settings.get(f.key);
    if (setting === undefined) return null;
    const value = form.values[f.key] ?? "";
    const label = t(wordOf(settingNames, f.key));
    const about = f.about ? t(wordOf(settingAbouts, f.key)) : undefined;
    if (f.as === "choices") {
      const options = setting.choices ?? [];
      const fault = faults[f.key];
      return (
        <Stack key={f.key} gap="hair">
          <ChoiceList
            id={controlOf(f.key)}
            label={label}
            multiple={false}
            describedBy={faults[f.key] === undefined ? undefined : `${controlOf(f.key)}-error`}
            items={options.map((option) => ({
              key: option,
              label: (
                <Stack gap="hair">
                  <Box as="span" weight="strong">
                    {t(wordOf(choiceNames, f.key), { value: optionOf(option) })}
                  </Box>
                  <Box as="span" text="caption" tone="muted" breaks="phrase">
                    {t(wordOf(choiceAbouts, f.key), { value: optionOf(option) })}
                  </Box>
                  {irreversibleChoices[f.key] === option && (
                    <Irreversible>{t("staff_settings.irreversible", { what: f.key })}</Irreversible>
                  )}
                </Stack>
              ),
            }))}
            chosen={[options.indexOf(value)].filter((i) => i >= 0)}
            onChange={([i]) => {
              const next = i === undefined ? undefined : options[i];
              if (next !== undefined) setValue(f.key, next);
            }}
            disabled={busy}
          />
          {fault !== undefined && <FieldError id={`${controlOf(f.key)}-error`}>{fault}</FieldError>}
        </Stack>
      );
    }
    // What cannot be undone is marked under its row, in the text's color, beside the question Save asks.
    const mark = irreversibleKeys.has(f.key) ? (
      <Irreversible>{t("staff_settings.irreversible", { what: f.key })}</Irreversible>
    ) : undefined;
    const row = (control: ReactNode, wide = false) => (
      <SettingRow
        key={f.key}
        id={rowOf(f.key)}
        label={label}
        about={about}
        form
        error={faults[f.key]}
        wide={wide}
        values={mark}
      >
        {control}
      </SettingRow>
    );
    switch (setting.kind) {
      case "bool":
        return (
          <OnOffRow
            key={f.key}
            id={rowOf(f.key)}
            label={label}
            about={about}
            on={value === "true"}
            onChange={(on) => {
              setValue(f.key, on ? "true" : "false");
            }}
            disabled={busy}
            error={faults[f.key]}
            values={mark}
          />
        );
      case "enum": {
        const family = families[f.key] ?? "audience";
        // The mode of a hook goes from off to enforce only through observe (the server refuses it otherwise).
        const offered = (setting.choices ?? []).filter(
          (c) => !(family === "hook_mode" && c === "enforce" && base.values[f.key] === "off"),
        );
        return row(
          <Select
            name={f.key}
            options={offered.map((v) => ({
              value: v,
              label: t(wordOf(optionWords, family), { value: optionOf(v) }),
            }))}
            value={value}
            onChange={(v) => {
              setValue(f.key, v);
            }}
            isDisabled={busy}
          />,
        );
      }
      case "int": {
        const factor = factorOf(f.key);
        return row(
          <NumberField
            name={f.key}
            value={value === "" ? null : Math.round(Number(value) / factor)}
            min={Math.ceil((setting.minimum ?? 0) / factor)}
            max={Math.floor((setting.maximum ?? Number.MAX_SAFE_INTEGER) / factor)}
            unit={f.unit === undefined ? undefined : t("staff_settings.unit", { unit: f.unit })}
            onChange={(n) => {
              setValue(f.key, n === null ? "" : String(n * factor));
            }}
            isDisabled={busy}
          />,
        );
      }
      case "locale": {
        const known = appLanguages.includes(value) || value === "" ? appLanguages : [value, ...appLanguages];
        return row(
          <Select
            name={f.key}
            options={known.map((tag) => ({ value: tag, label: languageName(tag, tag) ?? tag }))}
            value={value}
            onChange={(v) => {
              setValue(f.key, v);
            }}
            isDisabled={busy}
          />,
        );
      }
      default:
        return (
          <TextField
            key={f.key}
            id={controlOf(f.key)}
            name={f.key}
            label={label}
            labelHidden={alone === label}
            row
            hint={about}
            type={setting.kind === "email" ? "email" : "text"}
            defaultValue={value}
            error={faults[f.key]}
            autoCapitalize="none"
            onChange={(v) => {
              setValue(f.key, v);
            }}
          />
        );
    }
  };

  // Of a one-person server, the sections of signing up say that nobody else signs up, in place of their settings.
  const hiddenOnSingle = new Set(["mode", "invites"]);

  return (
    <>
      <SettingsScreen
        sub
        icon={back === undefined ? topic.icon : undefined}
        title={title}
        back={
          back === undefined ? undefined : (
            <BackButton
              onPress={() => {
                go?.(back.href);
              }}
            >
              {back.label}
            </BackButton>
          )
        }
        foot={
          <SubmitBar
            result={
              sent.kind === "saved" ? (
                <SaveState state="saved" />
              ) : sent.kind === "refused" ? (
                <FormAlert>
                  {sent.reason ??
                    t(sent.faults === undefined ? "staff_settings.save.failed" : "staff_settings.save.refused")}
                </FormAlert>
              ) : undefined
            }
          >
            <Button kind="primary" busy={busy} onPress={save}>
              {t("staff_settings.save")}
            </Button>
          </SubmitBar>
        }
      >
        {before}
        {topic.sections.map((s) => (
          <SettingsSection
            key={s.id}
            heading={t(wordOf(sectionHeadings, s.id))}
            about={s.about ? t(wordOf(sectionAbouts, s.id)) : undefined}
          >
            {single && hiddenOnSingle.has(s.id) && (
              <Box as="p" breaks="phrase">
                {t("staff_settings.single_user.hidden")}
              </Box>
            )}
            {s.fields
              // On a one-person server, a section of signing up keeps only what turns that off.
              .filter(
                (f) => !(single && hiddenOnSingle.has(s.id) && !(f.kind === "setting" && f.key === "single_user")),
              )
              .map((f) => field(f, s.fields.length === 1 ? t(wordOf(sectionHeadings, s.id)) : undefined))}
          </SettingsSection>
        ))}
      </SettingsScreen>
      <ModalPresence>
        {asking === "scope" ? (
          <ConfirmDialog
            key="scope"
            title={t("staff_settings.ask.scope.title")}
            cancel={t("staff_settings.ask.cancel")}
            confirm={t("staff_settings.ask.scope.confirm")}
            onCancel={() => {
              setAsking(undefined);
            }}
            onConfirm={() => {
              void send();
            }}
          >
            {scopeImpact === undefined
              ? t("staff_settings.ask.scope.unknown")
              : t("staff_settings.ask.scope.text", {
                  follows: scopeImpact.follows,
                  accounts: scopeImpact.local_accounts,
                  servers: scopeImpact.domains,
                  top: scopeImpact.top_domains
                    .slice(0, 3)
                    .map((d) => d.domain)
                    .join(", "),
                })}
          </ConfirmDialog>
        ) : asking === "providers_only" ? (
          <ConfirmDialog
            key="providers_only"
            title={t("staff_settings.ask.providers_only.title")}
            cancel={t("staff_settings.ask.cancel")}
            confirm={t("staff_settings.ask.providers_only.confirm")}
            onCancel={() => {
              setAsking(undefined);
            }}
            onConfirm={() => {
              void send();
            }}
          >
            {providersImpact.impact === undefined
              ? t("staff_settings.ask.providers_only.unknown")
              : t("staff_settings.ask.providers_only.text", {
                  count: providersImpact.impact.users_without_provider,
                  you: providersImpact.impact.includes_viewer ? "yes" : "no",
                  providers: providersImpact.impact.enabled_providers === 0 ? "none" : "some",
                })}
          </ConfirmDialog>
        ) : asking === "auto_delete_max" ? (
          <ConfirmDialog
            key="auto_delete_max"
            title={t("staff_settings.ask.auto_delete_max.title")}
            cancel={t("staff_settings.ask.cancel")}
            confirm={t("staff_settings.ask.auto_delete_max.confirm")}
            onCancel={() => {
              setAsking(undefined);
            }}
            onConfirm={() => {
              void send();
            }}
          >
            {t("staff_settings.ask.auto_delete_max.text", {
              days: Number(form.values["auto_delete_max_age_days"] ?? 0),
              grace: Number(form.values["auto_delete_grace_days"] ?? 0),
            })}
          </ConfirmDialog>
        ) : (
          leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />
        )}
      </ModalPresence>
    </>
  );
}

/** The keys and texts a field holds, in its order. */
function fieldNames(f: SettingField): readonly string[] {
  switch (f.kind) {
    case "setting":
      return [f.key];
    case "text":
      return [f.key];
    case "image":
      return [f.key, f.alt];
    case "checks":
      return f.keys;
    case "providers":
      return ["translation_providers", "translation_provider_visibilities"];
  }
}
