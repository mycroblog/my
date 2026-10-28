// SPDX-License-Identifier: AGPL-3.0-only
// What the login providers screen shares (ADR-0106): the labels of the kinds, the fields of a provider's settings
// for its kind, and its callback URL with a copy button.
import { Button, Checkbox, Cluster, Fieldset, Hint, InlineError, Stack, Text, TextField } from "@my/ui";
import { useId, useRef, useState } from "react";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import { type LoginProviderKind, loginProviderLimits } from "../api/loginProviders";
import {
  type ClaimDraft,
  type LoginProviderDraft,
  type LoginProviderField,
  type LoginProviderFieldError,
  newClaimDraft,
  useCopyCallbackUrl,
} from "../hooks/useLoginProviders";

export const loginProviderKindLabels: Readonly<Record<LoginProviderKind, ClientMessageKey>> = {
  oidc: "client:admin.login_providers.kind.oidc",
  github: "client:admin.login_providers.kind.github",
  mastodon: "client:admin.login_providers.kind.mastodon",
  discord: "client:admin.login_providers.kind.discord",
};

type LoginProviderFieldsProps = {
  draft: LoginProviderDraft;
  change: (change: Partial<LoginProviderDraft>) => void;
  /** The refusal, shown on its field. */
  error: LoginProviderFieldError | undefined;
  /** Whether the form takes the client secret: only when the provider is set up. */
  withSecret: boolean;
  autoFocus?: boolean;
};

/**
 * The settings of a provider of the draft's kind: the display name, the issuer (`oidc`) or the server (`mastodon`),
 * the client ID, the client secret on setup, the extra scopes (not for `mastodon`), whether users may sign up and
 * link their accounts, and the conditions (claims for `oidc`, Organizations for `github`, servers for `discord`).
 */
export function LoginProviderFields({ draft, change, error, withSecret, autoFocus = false }: LoginProviderFieldsProps) {
  const t = useT();
  const fieldError = (field: LoginProviderField) =>
    error?.field === field ? { isInvalid: true, errorMessage: error.message } : {};
  // A new Mastodon provider registers its client at the server unless the administrator enters one.
  const registers = withSecret && draft.kind === "mastodon";
  return (
    <>
      <TextField
        label={t("client:admin.login_providers.display_name")}
        description={t(
          draft.kind === "oidc"
            ? "client:admin.login_providers.display_name.description_oidc"
            : "client:admin.login_providers.display_name.description",
        )}
        value={draft.displayName}
        onChange={(displayName) => {
          change({ displayName });
        }}
        isRequired
        maxLength={loginProviderLimits.displayName}
        autoComplete="off"
        autoFocus={autoFocus}
        {...fieldError("display_name")}
      />
      {draft.kind === "oidc" && (
        <TextField
          type="url"
          label={t("client:admin.login_providers.issuer")}
          description={t("client:admin.login_providers.issuer.description")}
          value={draft.issuer}
          onChange={(issuer) => {
            change({ issuer });
          }}
          isRequired
          maxLength={loginProviderLimits.issuer}
          autoComplete="off"
          spellCheck="false"
          {...fieldError("issuer")}
        />
      )}
      {draft.kind === "mastodon" && (
        <TextField
          label={t("client:admin.login_providers.server")}
          description={t("client:admin.login_providers.server.description")}
          value={draft.issuer}
          onChange={(issuer) => {
            change({ issuer });
          }}
          isRequired
          maxLength={loginProviderLimits.issuer}
          autoComplete="off"
          spellCheck="false"
          {...fieldError("issuer")}
        />
      )}
      <TextField
        label={t("client:admin.login_providers.client_id")}
        {...(registers && { description: t("client:admin.login_providers.client_id.description_mastodon") })}
        value={draft.clientId}
        onChange={(clientId) => {
          change({ clientId });
        }}
        isRequired={!registers}
        maxLength={loginProviderLimits.clientId}
        autoComplete="off"
        spellCheck="false"
        {...fieldError("client_id")}
      />
      {withSecret && (
        <SecretField
          value={draft.clientSecret}
          onChange={(clientSecret) => {
            change({ clientSecret });
          }}
          error={error}
          isRequired={!registers}
        />
      )}
      {draft.kind !== "mastodon" && (
        <TextField
          label={t("client:admin.login_providers.scopes")}
          description={t("client:admin.login_providers.scopes.description")}
          value={draft.scopes}
          onChange={(scopes) => {
            change({ scopes });
          }}
          autoComplete="off"
          spellCheck="false"
          {...fieldError("scopes")}
        />
      )}
      <Checkbox
        isSelected={draft.allowSignup}
        onChange={(allowSignup) => {
          change({ allowSignup });
        }}
        description={t("client:admin.login_providers.allow_signup.description")}
      >
        {t("client:admin.login_providers.allow_signup")}
      </Checkbox>
      <Checkbox
        isSelected={draft.allowLink}
        onChange={(allowLink) => {
          change({ allowLink });
        }}
        description={t("client:admin.login_providers.allow_link.description")}
      >
        {t("client:admin.login_providers.allow_link")}
      </Checkbox>
      {draft.kind === "oidc" && (
        <ClaimConditions
          claims={draft.claims}
          change={(claims) => {
            change({ claims });
          }}
          error={error}
        />
      )}
      {draft.kind === "github" && (
        <TextField
          label={t("client:admin.login_providers.organizations")}
          description={t("client:admin.login_providers.organizations.description")}
          value={draft.organizations}
          onChange={(organizations) => {
            change({ organizations });
          }}
          autoComplete="off"
          spellCheck="false"
          {...fieldError("conditions")}
        />
      )}
      {draft.kind === "discord" && (
        <TextField
          label={t("client:admin.login_providers.guilds")}
          description={t("client:admin.login_providers.guilds.description")}
          value={draft.guilds}
          onChange={(guilds) => {
            change({ guilds });
          }}
          autoComplete="off"
          spellCheck="false"
          inputMode="numeric"
          {...fieldError("conditions")}
        />
      )}
      {draft.kind === "mastodon" && <Hint>{t("client:admin.login_providers.mastodon.conditions")}</Hint>}
      {(error?.field === "form" || error?.field === "kind") && (
        <InlineError size="inherit" alert>
          {t("client:admin.login_providers.save_failed", { reason: error.message })}
        </InlineError>
      )}
    </>
  );
}

/** The client secret the provider issued: entered on setup and rotation only, and never shown again. */
export function SecretField({
  value,
  onChange,
  error,
  autoFocus = false,
  isRequired = true,
}: {
  value: string;
  onChange: (value: string) => void;
  error: LoginProviderFieldError | undefined;
  autoFocus?: boolean;
  /** False for a new Mastodon provider, which may register its client itself. */
  isRequired?: boolean;
}) {
  const t = useT();
  return (
    <TextField
      type="password"
      label={t("client:admin.login_providers.client_secret")}
      description={t("client:admin.login_providers.client_secret.description")}
      value={value}
      onChange={onChange}
      isRequired={isRequired}
      maxLength={loginProviderLimits.clientSecret}
      autoComplete="off"
      spellCheck="false"
      autoFocus={autoFocus}
      {...(error?.field === "client_secret" ? { isInvalid: true, errorMessage: error.message } : {})}
    />
  );
}

/**
 * The conditions of an `oidc` provider: a row per claim with its allowed values, in a group. A new row takes the
 * focus; removing one gives it to the button that adds a row.
 */
function ClaimConditions({
  claims,
  change,
  error,
}: {
  claims: readonly ClaimDraft[];
  change: (claims: readonly ClaimDraft[]) => void;
  error: LoginProviderFieldError | undefined;
}) {
  const t = useT();
  const errorId = useId();
  const addRef = useRef<HTMLDivElement>(null);
  const [added, setAdded] = useState(false);
  const isInvalid = error?.field === "conditions";
  const set = (index: number, row: Partial<ClaimDraft>) => {
    change(claims.map((current, i) => (i === index ? { ...current, ...row } : current)));
  };
  return (
    <Fieldset legend={t("client:admin.login_providers.claims")} {...(isInvalid && { "aria-describedby": errorId })}>
      <Hint>{t("client:admin.login_providers.claims.description")}</Hint>
      {claims.map((row, index) => {
        const number = String(index + 1);
        return (
          <Stack key={row.id} gap="2">
            <Text size="sm" weight="medium">
              {t("client:admin.login_providers.claims.row", { number })}
            </Text>
            <Cluster gap="2" align="end">
              <TextField
                grow
                shrink
                label={t("client:admin.login_providers.claims.claim", { number })}
                value={row.claim}
                onChange={(claim) => {
                  set(index, { claim });
                }}
                maxLength={loginProviderLimits.claimName}
                autoComplete="off"
                spellCheck="false"
                autoFocus={added && index === claims.length - 1}
              />
              <TextField
                grow
                shrink
                label={t("client:admin.login_providers.claims.values", { number })}
                value={row.values}
                onChange={(values) => {
                  set(index, { values });
                }}
                autoComplete="off"
                spellCheck="false"
              />
              <Button
                size="sm"
                onPress={() => {
                  change(claims.filter((_, i) => i !== index));
                  setAdded(false);
                  addRef.current?.querySelector<HTMLElement>("button")?.focus();
                }}
              >
                {t("client:admin.login_providers.claims.remove", { number })}
              </Button>
            </Cluster>
          </Stack>
        );
      })}
      <div ref={addRef}>
        <Button
          size="sm"
          isDisabled={claims.length >= loginProviderLimits.claims}
          onPress={() => {
            change([...claims, newClaimDraft()]);
            setAdded(true);
          }}
        >
          {t("client:admin.login_providers.claims.add")}
        </Button>
      </div>
      {isInvalid && (
        <InlineError id={errorId} alert>
          {error.message}
        </InlineError>
      )}
    </Fieldset>
  );
}

/** A provider's callback URL, to register at the provider, with a button that copies it. */
export function CallbackUrl({ url, name }: { url: string; name: string }) {
  const t = useT();
  const copy = useCopyCallbackUrl();
  const labelId = useId();
  return (
    <Stack gap="2">
      <Text as="p" size="sm" weight="medium" id={labelId}>
        {t("client:admin.login_providers.callback")}
      </Text>
      <Text as="p" size="sm" wrap="anywhere" mono selectAll aria-labelledby={labelId} dir="ltr">
        {url}
      </Text>
      <div>
        <Button
          size="sm"
          aria-label={t("client:admin.login_providers.callback.copy_of", { name })}
          onPress={() => {
            copy(url);
          }}
        >
          {t("client:admin.login_providers.callback.copy")}
        </Button>
      </div>
    </Stack>
  );
}
