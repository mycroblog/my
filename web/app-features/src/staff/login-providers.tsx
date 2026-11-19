// SPDX-License-Identifier: AGPL-3.0-only

// The external login providers (FR-010, ADR-0106) as a holder of `manage_login_providers` keeps them: at
// /admin/login-providers, each provider a door to its page, whether it is enabled, how many accounts are linked to it
// and whether it lets people sign up, with Add a login provider, under what limiting sign-in to the providers does now
// where it is on; at /admin/login-providers/new, a provider set up by its kind (OpenID Connect, GitHub, one Mastodon
// server, Discord), its name, where it is, the client the provider gave and who may use it (sign-up, linking, and the
// claims, Organizations or Discord servers one of which a user must have); at /admin/login-providers/{id}, the
// callback URL to register at the provider, the same settings as a form saved as a whole with its state and a new
// client secret, and Delete last.
//
// The client secret is given, never shown: a new one replaces it. Adding or enabling a provider while sign-in is
// limited to providers and none can be used starts the limit at once: the form asks first, with how many active users
// could no longer sign in and whether the reader is one of them. Disabling and deleting say how many linked accounts
// could no longer sign in with it; deleting asks for its name, since it cannot be undone.

import {
  loadAdminLoginProviders,
  loginProviderKinds,
  loginProviderLimits,
  loginProviderRefusalOf,
  refusalDetail,
  useAdminLoginProvider,
  useAdminLoginProviders,
  useAdminLoginProviderWrites,
  useLoginProvidersOnlyImpact,
  type AdminLoginProvider,
  type AppData,
  type CreateLoginProvider,
  type LoginProviderClaimCondition,
  type LoginProviderKind,
  type LoginProviderRefusal,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  CheckField,
  ChoiceList,
  ConfirmDialog,
  focusRefusedField,
  FormAlert,
  LinkButton,
  ModalPresence,
  SaveState,
  Select,
  SettingRow,
  SettingsEmpty,
  SettingsList,
  SettingsListRow,
  SettingsResult,
  SettingsScreen,
  SettingsSection,
  Stack,
  StatusSummary,
  SubmitBar,
  TextField,
} from "@my/design-system";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { useStaffStrings, type T } from "./parts";
import { CopyValue, LeaveDialog, ScreenNotice, type AdminSubProps } from "./settings/parts";

/** The address of the login providers. */
const adminLoginProvidersPath = "/admin/login-providers";
/** The address of a new provider's form. */
const newLoginProviderPath = `${adminLoginProvidersPath}/new`;
/** The address of the provider of `id`. */
const loginProviderPath = (id: string): string => `${adminLoginProvidersPath}/${id}`;

/** Reads the providers, for the routes' loaders. */
export const loadLoginProviders = (data: AppData): Promise<void> => loadAdminLoginProviders(data);

/** The words of a list a field holds: one a line, or separated by spaces or commas. */
const wordsOf = (text: string): string[] =>
  text
    .split(/[\s,]+/)
    .map((w) => w.trim())
    .filter((w) => w !== "");

/** The conditions on claims, one a line: `claim=value value`. */
const claimsOf = (text: string): LoginProviderClaimCondition[] =>
  text
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line !== "")
    .map((line) => {
      const at = line.indexOf("=");
      return at < 0
        ? { claim: line, values: [] }
        : { claim: line.slice(0, at).trim(), values: wordsOf(line.slice(at + 1)) };
    });

/** The conditions on claims as their field writes them. */
const claimsText = (claims: readonly LoginProviderClaimCondition[]): string =>
  claims.map((c) => `${c.claim}=${c.values.join(" ")}`).join("\n");

/** What a provider's form holds. */
type ProviderForm = {
  readonly kind: LoginProviderKind;
  readonly name: string;
  readonly issuer: string;
  readonly clientId: string;
  /** A new client secret; empty keeps the one it has. */
  readonly secret: string;
  readonly scopes: string;
  readonly signup: boolean;
  readonly link: boolean;
  /** The conditions of its kind, as their field writes them. */
  readonly conditions: string;
  readonly enabled: boolean;
};

/** The form of a provider as the server has it. */
const formOf = (p: AdminLoginProvider): ProviderForm => ({
  kind: p.kind,
  name: p.display_name,
  issuer: p.issuer ?? "",
  clientId: p.client_id,
  secret: "",
  scopes: p.scopes.join(" "),
  signup: p.allow_signup,
  link: p.allow_link,
  conditions:
    p.kind === "oidc" ? claimsText(p.claims) : p.kind === "github" ? p.organizations.join(" ") : p.guilds.join(" "),
  enabled: p.state === "enabled",
});

const blank: ProviderForm = {
  kind: "oidc",
  name: "",
  issuer: "",
  clientId: "",
  secret: "",
  scopes: "",
  signup: false,
  link: true,
  conditions: "",
  enabled: true,
};

/** The settings of the request that a form's kind takes. */
function settingsOf(f: ProviderForm) {
  const conditions =
    f.kind === "oidc"
      ? { claims: claimsOf(f.conditions) }
      : f.kind === "github"
        ? { organizations: wordsOf(f.conditions) }
        : f.kind === "discord"
          ? { guilds: wordsOf(f.conditions) }
          : {};
  return {
    display_name: f.name.trim(),
    ...(f.kind === "oidc" || f.kind === "mastodon" ? { issuer: f.issuer.trim() } : {}),
    ...(f.kind === "mastodon" ? {} : { scopes: wordsOf(f.scopes) }),
    allow_signup: f.signup,
    allow_link: f.link,
    ...conditions,
  };
}

/** The request that sets up the provider of a form. */
const createOf = (f: ProviderForm): CreateLoginProvider => ({
  kind: f.kind,
  ...settingsOf(f),
  ...(f.clientId.trim() === "" ? {} : { client_id: f.clientId.trim() }),
  ...(f.secret === "" ? {} : { client_secret: f.secret }),
});

/** The field each refusal of the server is about. */
type Field = "kind" | "name" | "issuer" | "clientId" | "secret" | "scopes" | "conditions";
const fieldOf: Readonly<Record<LoginProviderRefusal, Field>> = {
  kind: "kind",
  display_name: "name",
  issuer: "issuer",
  client_id: "clientId",
  client_secret: "secret",
  scopes: "scopes",
  conditions: "conditions",
};

/** Where a provider form's last save is. */
type Sent =
  | { readonly kind: "idle" | "sending" | "saved" }
  | { readonly kind: "refused"; readonly field?: Field | undefined; readonly reason?: string | undefined };

/** The kind of a provider in words. */
const kindWords = (t: T, kind: LoginProviderKind) => t("staff_settings.providers_admin.kind", { kind });

/** The providers, a door each, and what limiting sign-in to them does now where it is on. */
export function AdminLoginProvidersScreen(): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminLoginProviders();
  const { impact } = useLoginProvidersOnlyImpact(true);
  const title = t("staff.entry.login_providers");
  if (read.providers === undefined)
    return (
      <SettingsScreen
        icon="signIn"
        title={title}
        notice={
          <ScreenNotice
            status={read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{
              loading: t("staff_settings.providers_admin.loading"),
              failed: t("staff_settings.providers_admin.failed"),
            }}
          />
        }
      />
    );
  return (
    <SettingsScreen icon="signIn" title={title}>
      {impact?.setting_on === true && (
        <StatusSummary
          icon="caution"
          title={t("staff_settings.providers_admin.limit", {
            applies: impact.enabled_providers === 0 ? "no" : "yes",
          })}
          about={t("staff_settings.providers_admin.limit.about", {
            applies: impact.enabled_providers === 0 ? "no" : "yes",
            count: impact.users_without_provider,
            you: impact.includes_viewer ? "yes" : "no",
          })}
        />
      )}
      <SettingsSection
        heading={t("staff_settings.providers_admin.list")}
        count={String(read.providers.length)}
        action={
          <LinkButton size="timeline" href={newLoginProviderPath}>
            {t("staff_settings.providers_admin.add")}
          </LinkButton>
        }
      >
        {read.providers.length === 0 ? (
          <SettingsEmpty>{t("staff_settings.providers_admin.empty")}</SettingsEmpty>
        ) : (
          <SettingsList label={t("staff_settings.providers_admin.list")}>
            {read.providers.map((p) => (
              <SettingsListRow
                key={p.id}
                title={p.display_name}
                inactive={p.state !== "enabled"}
                standing={t("staff_settings.providers_admin.state", { state: p.state })}
                facts={[
                  kindWords(t, p.kind),
                  t("staff_settings.providers_admin.linked", { count: p.linked_users }),
                  t("staff_settings.providers_admin.signup", { allowed: p.allow_signup ? "yes" : "no" }),
                ]}
                href={loginProviderPath(p.id)}
              />
            ))}
          </SettingsList>
        )}
      </SettingsSection>
    </SettingsScreen>
  );
}

/** The fields of a provider, in their sections: its connection, then who may use it. */
function ProviderFields({
  id,
  form,
  fresh,
  sent,
  busy,
  onChange,
}: {
  id: string;
  form: ProviderForm;
  /** A provider being set up, whose kind is chosen here and whose secret is required. */
  fresh: boolean;
  sent: Sent;
  busy: boolean;
  onChange: (part: Partial<ProviderForm>) => void;
}) {
  const { t } = useStaffStrings();
  const error = (field: Field) =>
    sent.kind === "refused" && sent.field === field
      ? t("staff_settings.providers_admin.refused", { field })
      : undefined;
  const text = (
    field: Field,
    more: {
      secret?: boolean | undefined;
      lines?: number | undefined;
      hint?: string | undefined;
      max?: number | undefined;
    } = {},
  ) => (
    <TextField
      key={`${form.kind}-${field}`}
      id={`${id}-${field}`}
      name={field}
      label={t("staff_settings.providers_admin.field", { field, kind: form.kind })}
      hint={more.hint}
      type={more.secret === true ? "password" : "text"}
      autoComplete={more.secret === true ? "new-password" : "off"}
      autoCapitalize="none"
      defaultValue={field === "kind" ? form.name : form[field]}
      multiline={more.lines !== undefined}
      rows={more.lines}
      maxLength={more.max}
      error={error(field)}
      onChange={(value) => {
        onChange({ [field]: value });
      }}
    />
  );
  const conditionsHint = t("staff_settings.providers_admin.conditions.about", { kind: form.kind });
  return (
    <>
      {fresh && (
        <SettingsSection heading={t("staff_settings.providers_admin.section.kind")}>
          <ChoiceList
            id={`${id}-kind`}
            label={t("staff_settings.providers_admin.section.kind")}
            multiple={false}
            items={loginProviderKinds.map((kind) => ({
              key: kind,
              label: (
                <Stack gap="hair">
                  <Box as="span" weight="strong">
                    {kindWords(t, kind)}
                  </Box>
                  <Box as="span" text="caption" tone="muted" breaks="phrase">
                    {t("staff_settings.providers_admin.kind.about", { kind })}
                  </Box>
                </Stack>
              ),
            }))}
            chosen={[loginProviderKinds.indexOf(form.kind)]}
            onChange={([i]) => {
              const kind = i === undefined ? undefined : loginProviderKinds[i];
              if (kind !== undefined) onChange({ kind, conditions: "", scopes: "" });
            }}
            disabled={busy}
          />
        </SettingsSection>
      )}
      <SettingsSection
        heading={t("staff_settings.providers_admin.section.connection")}
        about={t("staff_settings.providers_admin.section.connection.about", { kind: form.kind })}
      >
        {text("name", { max: loginProviderLimits.displayName })}
        {(form.kind === "oidc" || form.kind === "mastodon") &&
          text("issuer", { hint: t("staff_settings.providers_admin.issuer.about", { kind: form.kind }) })}
        {text("clientId", {
          hint: form.kind === "mastodon" ? t("staff_settings.providers_admin.mastodon_client.about") : undefined,
        })}
        {text("secret", {
          secret: true,
          hint: t("staff_settings.providers_admin.secret.about", { fresh: fresh ? "yes" : "no" }),
        })}
        {form.kind !== "mastodon" &&
          text("scopes", { hint: t("staff_settings.providers_admin.scopes.about", { kind: form.kind }) })}
      </SettingsSection>
      <SettingsSection heading={t("staff_settings.providers_admin.section.who")}>
        <CheckField
          id={`${id}-signup`}
          name="allow_signup"
          value="true"
          label={t("staff_settings.providers_admin.allow_signup")}
          checked={form.signup}
          onChange={(signup) => {
            onChange({ signup });
          }}
          touch
          disabled={busy}
        />
        <CheckField
          id={`${id}-link`}
          name="allow_link"
          value="true"
          label={t("staff_settings.providers_admin.allow_link")}
          checked={form.link}
          onChange={(link) => {
            onChange({ link });
          }}
          touch
          disabled={busy}
        />
        {form.kind !== "mastodon" &&
          text("conditions", { lines: form.kind === "oidc" ? 3 : undefined, hint: conditionsHint })}
      </SettingsSection>
    </>
  );
}

/** The submit bar of a provider's form. */
function ProviderBar({ sent, busy, label, onSave }: { sent: Sent; busy: boolean; label: string; onSave: () => void }) {
  const { t } = useStaffStrings();
  return (
    <SubmitBar
      result={
        sent.kind === "saved" ? (
          <SaveState state="saved" />
        ) : sent.kind === "refused" ? (
          <FormAlert>
            {sent.reason ?? t(sent.field === undefined ? "staff_settings.save.failed" : "staff_settings.save.refused")}
          </FormAlert>
        ) : undefined
      }
    >
      <Button kind="primary" busy={busy} onPress={onSave}>
        {label}
      </Button>
    </SubmitBar>
  );
}

/** The state of a provider's form: what it holds, its last save, the focus on a refused field. */
function useProviderForm(start: ProviderForm, onUnsaved: AdminSubProps["onUnsaved"]) {
  const id = useId();
  const [base, setBase] = useState(start);
  const [form, setForm] = useState(start);
  const [sent, setSent] = useState<Sent>({ kind: "idle" });
  const focusNext = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (focusNext.current === undefined) return;
    focusRefusedField(focusNext.current);
    focusNext.current = undefined;
  });
  const busy = sent.kind === "sending";
  const unsaved = JSON.stringify(form) !== JSON.stringify(base);
  useEffect(() => {
    onUnsaved?.(unsaved && !busy);
  }, [onUnsaved, unsaved, busy]);
  useEffect(
    () => () => {
      onUnsaved?.(false);
    },
    [onUnsaved],
  );
  const refuseField = (field: Field) => {
    setSent({ kind: "refused", field });
    focusNext.current = `${id}-${field}`;
  };
  return {
    id,
    base,
    form,
    sent,
    busy,
    unsaved,
    setBase,
    setSent,
    change: (part: Partial<ProviderForm>) => {
      setForm((now) => ({ ...now, ...part }));
      setSent((was) => (was.kind === "saved" || was.kind === "refused" ? { kind: "idle" } : was));
    },
    /** Says what came of a refused write: the field its reason names, which takes the focus, or why. */
    refuse: (error: unknown) => {
      const reason = loginProviderRefusalOf(error);
      if (reason !== undefined) refuseField(fieldOf[reason]);
      else setSent({ kind: "refused", reason: refusalDetail(error) });
    },
    /** What the form finds wrong before sending: no name, and no client of a kind that needs one given. */
    check: (fresh: boolean): boolean => {
      if (form.name.trim() === "") refuseField("name");
      else if ((form.kind === "oidc" || form.kind === "mastodon") && form.issuer.trim() === "") refuseField("issuer");
      else if (fresh && form.kind !== "mastodon" && form.clientId.trim() === "") refuseField("clientId");
      else if (fresh && form.kind !== "mastodon" && form.secret === "") refuseField("secret");
      else return true;
      return false;
    },
  };
}

/** The question before what starts the limit of sign-in to providers at once, with who it keeps out. */
function LimitDialog({
  providerId,
  confirm,
  busy,
  onCancel,
  onConfirm,
}: {
  providerId?: string | undefined;
  confirm: string;
  busy: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const { t } = useStaffStrings();
  const { impact } = useLoginProvidersOnlyImpact(true, providerId);
  return (
    <ConfirmDialog
      title={t("staff_settings.providers_admin.starts.title")}
      cancel={t("staff_settings.ask.cancel")}
      confirm={confirm}
      busy={busy}
      onCancel={onCancel}
      onConfirm={onConfirm}
    >
      {t("staff_settings.providers_admin.starts.text", {
        count: impact?.users_without_provider ?? 0,
        known: impact === undefined ? "no" : "yes",
        you: impact?.includes_viewer === true ? "yes" : "no",
      })}
    </ConfirmDialog>
  );
}

/** The way back to the providers. */
function BackToProviders({ go }: { go: (href: string) => void }) {
  const { t } = useStaffStrings();
  return (
    <BackButton
      onPress={() => {
        go(adminLoginProvidersPath);
      }}
    >
      {t("staff.entry.login_providers")}
    </BackButton>
  );
}

/** A provider to set up: its kind, its connection, and who may use it. */
export function NewLoginProviderScreen({ go, onUnsaved, leaving }: AdminSubProps): ReactNode {
  const { t } = useStaffStrings();
  const writes = useAdminLoginProviderWrites();
  const f = useProviderForm(blank, onUnsaved);
  const { impact } = useLoginProvidersOnlyImpact(true);
  const [asking, setAsking] = useState(false);
  const send = () => {
    f.setSent({ kind: "sending" });
    writes.create(createOf(f.form)).then(
      (provider) => {
        setAsking(false);
        onUnsaved?.(false);
        go(loginProviderPath(provider.id));
      },
      (error: unknown) => {
        setAsking(false);
        f.refuse(error);
      },
    );
  };
  const save = () => {
    if (f.busy || !f.check(true)) return;
    if (impact?.starts_limit === true) setAsking(true);
    else send();
  };
  return (
    <>
      <SettingsScreen
        sub
        title={t("staff_settings.providers_admin.new")}
        back={<BackToProviders go={go} />}
        foot={<ProviderBar sent={f.sent} busy={f.busy} label={t("staff_settings.providers_admin.add")} onSave={save} />}
      >
        <ProviderFields id={f.id} form={f.form} fresh sent={f.sent} busy={f.busy} onChange={f.change} />
      </SettingsScreen>
      <ModalPresence>
        {asking ? (
          <LimitDialog
            key="limit"
            confirm={t("staff_settings.providers_admin.add")}
            busy={f.busy}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={send}
          />
        ) : (
          leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />
        )}
      </ModalPresence>
    </>
  );
}

/** What a provider's page asks before. */
type Ask = "limit" | "disable" | "delete";

/** A provider: its callback URL, its settings as a form with its state and a new secret, and Delete last. */
function ProviderPage({ provider, go, onUnsaved, leaving }: AdminSubProps & { provider: AdminLoginProvider }) {
  const { t } = useStaffStrings();
  const writes = useAdminLoginProviderWrites();
  const f = useProviderForm(formOf(provider), onUnsaved);
  const enabling = f.form.enabled && !f.base.enabled;
  const { impact } = useLoginProvidersOnlyImpact(enabling, provider.id);
  const [asking, setAsking] = useState<Ask | undefined>(undefined);
  const [deleting, setDeleting] = useState<"idle" | "sending" | "failed">("idle");
  // The fields are drawn again after a save, so that the secret's field, whose value is never kept, is empty again.
  const [round, setRound] = useState(0);
  const send = async () => {
    setAsking(undefined);
    f.setSent({ kind: "sending" });
    try {
      const { form, base } = f;
      const changed = JSON.stringify(settingsOf(form)) !== JSON.stringify(settingsOf(base));
      if (changed || form.clientId !== base.clientId)
        await writes.update(provider.id, {
          ...settingsOf(form),
          ...(form.clientId === base.clientId ? {} : { client_id: form.clientId.trim() }),
        });
      if (form.secret !== "") await writes.rotate(provider.id, form.secret);
      if (form.enabled !== base.enabled)
        await (form.enabled ? writes.enable(provider.id) : writes.disable(provider.id));
      f.setBase({ ...form, secret: "" });
      f.change({ secret: "" });
      f.setSent({ kind: "saved" });
      setRound((r) => r + 1);
    } catch (error) {
      f.refuse(error);
    }
  };
  const save = () => {
    if (f.busy || !f.check(false)) return;
    if (!f.unsaved) {
      go(adminLoginProvidersPath);
      return;
    }
    if (enabling && impact?.starts_limit === true) setAsking("limit");
    else if (!f.form.enabled && f.base.enabled && provider.linked_users > 0) setAsking("disable");
    else void send();
  };
  const remove = () => {
    setDeleting("sending");
    writes.remove(provider.id).then(
      () => {
        setAsking(undefined);
        onUnsaved?.(false);
        go(adminLoginProvidersPath);
      },
      () => {
        setAsking(undefined);
        setDeleting("failed");
      },
    );
  };
  return (
    <>
      <SettingsScreen
        sub
        title={provider.display_name}
        back={<BackToProviders go={go} />}
        foot={
          <ProviderBar
            sent={f.sent}
            busy={f.busy}
            label={t("staff_settings.save")}
            onSave={() => {
              save();
            }}
          />
        }
      >
        <SettingsSection
          heading={t("staff_settings.providers_admin.callback")}
          about={t("staff_settings.providers_admin.callback.about")}
        >
          <CopyValue
            label={t("staff_settings.providers_admin.callback")}
            value={provider.callback_url}
            copy={t("staff_settings.providers_admin.callback.copy")}
          />
        </SettingsSection>
        <SettingsSection heading={t("staff_settings.providers_admin.section.state")}>
          <SettingRow
            id={`${f.id}-state`}
            label={t("staff_settings.providers_admin.state.label")}
            about={t("staff_settings.providers_admin.linked", { count: provider.linked_users })}
            form
          >
            <Select
              name="state"
              options={(["enabled", "disabled"] as const).map((state) => ({
                value: state,
                label: t("staff_settings.providers_admin.state", { state }),
              }))}
              value={f.form.enabled ? "enabled" : "disabled"}
              onChange={(value) => {
                f.change({ enabled: value === "enabled" });
              }}
              isDisabled={f.busy}
            />
          </SettingRow>
        </SettingsSection>
        <ProviderFields
          key={round}
          id={f.id}
          form={f.form}
          fresh={false}
          sent={f.sent}
          busy={f.busy}
          onChange={f.change}
        />
        <SettingsSection
          heading={t("staff_settings.providers_admin.delete")}
          about={t("staff_settings.providers_admin.delete.about", { count: provider.linked_users })}
        >
          <Box>
            <Button
              size="timeline"
              touch
              isDisabled={f.busy}
              onPress={() => {
                setAsking("delete");
              }}
            >
              {t("staff_settings.providers_admin.delete.press")}
            </Button>
          </Box>
          <SettingsResult>
            {deleting === "failed" ? t("staff_settings.providers_admin.delete.failed") : undefined}
          </SettingsResult>
        </SettingsSection>
      </SettingsScreen>
      <ModalPresence>
        {asking === "limit" ? (
          <LimitDialog
            key="limit"
            providerId={provider.id}
            confirm={t("staff_settings.save")}
            busy={f.busy}
            onCancel={() => {
              setAsking(undefined);
            }}
            onConfirm={() => {
              void send();
            }}
          />
        ) : asking === "disable" ? (
          <ConfirmDialog
            key="disable"
            title={t("staff_settings.providers_admin.disable.title")}
            cancel={t("staff_settings.ask.cancel")}
            confirm={t("staff_settings.save")}
            busy={f.busy}
            onCancel={() => {
              setAsking(undefined);
            }}
            onConfirm={() => {
              void send();
            }}
          >
            {t("staff_settings.providers_admin.disable.text", { count: provider.linked_users })}
          </ConfirmDialog>
        ) : asking === "delete" ? (
          <ConfirmDialog
            key="delete"
            title={t("staff_settings.providers_admin.delete.title")}
            cancel={t("staff_settings.ask.cancel")}
            confirm={t("staff_settings.providers_admin.delete.press")}
            phrase={provider.display_name}
            busy={deleting === "sending"}
            onCancel={() => {
              setAsking(undefined);
            }}
            onConfirm={remove}
          >
            {t("staff_settings.providers_admin.delete.text", {
              name: provider.display_name,
              count: provider.linked_users,
            })}
          </ConfirmDialog>
        ) : (
          leaving !== undefined && <LeaveDialog key="leave" leaving={leaving} />
        )}
      </ModalPresence>
    </>
  );
}

/** The page of the provider of `id`, or that it loads, failed or is none. */
export function LoginProviderScreen({ id, ...sub }: AdminSubProps & { id: string }): ReactNode {
  const { t } = useStaffStrings();
  const read = useAdminLoginProvider(id);
  if (read.provider === undefined)
    return (
      <SettingsScreen
        sub
        title={t("staff.entry.login_providers")}
        back={<BackToProviders go={sub.go} />}
        notice={
          <ScreenNotice
            status={read.missing ? "missing" : read.status === "failed" ? "failed" : "loading"}
            retry={read.retry}
            words={{
              loading: t("staff_settings.providers_admin.loading"),
              failed: t("staff_settings.providers_admin.failed"),
              missing: t("staff_settings.providers_admin.missing"),
            }}
          />
        }
      />
    );
  return <ProviderPage key={read.provider.id} provider={read.provider} {...sub} />;
}

/** The title of the page of the provider of `id`: its name, once read. */
export function useLoginProviderTitle(id: string): string | undefined {
  return useAdminLoginProvider(id).provider?.display_name;
}
