// SPDX-License-Identifier: AGPL-3.0-only
// The external login providers of the admin screens (ADR-0106, manage_login_providers): the list, setting one up,
// changing, enabling, disabling and deleting it, and giving it a new client secret. The form edits the settings as
// text (scopes, Organizations and Discord servers as lists in one field, the claims of the conditions one per row) and sends only
// what the kind takes. What each change did is announced and kept in a StatusMessage; a refused setting stays next
// to its field.
import { useAnnouncer, useToast } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  createLoginProviderMutation,
  deleteLoginProviderMutation,
  type LoginProvider,
  type LoginProviderChange,
  type LoginProviderClaimCondition,
  type LoginProviderKind,
  loginProviderProblem,
  loginProvidersQueryOptions,
  type NewLoginProvider,
  refreshLoginProviders,
  rotateLoginProviderSecretMutation,
  setLoginProviderEnabledMutation,
  updateLoginProviderMutation,
} from "../api/loginProviders";

/** A provider for a sentence: its display name, isolated from the text around it (U+2068 … U+2069). */
export function loginProviderName(provider: Pick<LoginProvider, "display_name">): string {
  return `⁨${provider.display_name}⁩`;
}

/** The login providers that are not deleted, newest first. */
export function useLoginProviders(): LoginProvider[] {
  const api = useApi();
  return useSuspenseQuery(loginProvidersQueryOptions(api)).data.items;
}

/** The fields of a provider that the server may refuse, and the rest of the form. */
export type LoginProviderField = "kind" | "display_name" | "issuer" | "client_id" | "client_secret" | "scopes" | "conditions" | "form";

/** Why the settings of a provider were refused: the field and the localized reason. */
export type LoginProviderFieldError = { field: LoginProviderField; message: string };

/** Which field a failed change of a provider is about. */
function fieldError(error: unknown): LoginProviderFieldError {
  const problem = loginProviderProblem(error);
  return { field: problem?.reason ?? "form", message: errorMessage(error) };
}

/** A condition of an `oidc` provider as the form edits it: the claim and its values separated by commas, keyed by `id`. */
export type ClaimDraft = { id: string; claim: string; values: string };

let lastClaimId = 0;

/** A new, empty condition row with an ID of its own (the key of its row). */
export function newClaimDraft(claim = "", values = ""): ClaimDraft {
  lastClaimId += 1;
  return { id: `claim-${String(lastClaimId)}`, claim, values };
}

/** The settings of a provider as the form edits them. The secret is only entered on setup. */
export type LoginProviderDraft = {
  kind: LoginProviderKind;
  displayName: string;
  issuer: string;
  clientId: string;
  clientSecret: string;
  /** The extra scopes, separated by spaces. */
  scopes: string;
  allowSignup: boolean;
  allowLink: boolean;
  claims: readonly ClaimDraft[];
  /** The GitHub Organizations, separated by commas or spaces. */
  organizations: string;
  /** The Discord server (guild) IDs, separated by commas or spaces. */
  guilds: string;
};

const emptyDraft: LoginProviderDraft = {
  kind: "oidc",
  displayName: "",
  issuer: "",
  clientId: "",
  clientSecret: "",
  scopes: "",
  allowSignup: false,
  allowLink: true,
  claims: [],
  organizations: "",
  guilds: "",
};

/** The draft of an existing provider. */
export function draftOf(provider: LoginProvider): LoginProviderDraft {
  return {
    kind: provider.kind,
    displayName: provider.display_name,
    issuer: provider.issuer ?? "",
    clientId: provider.client_id,
    clientSecret: "",
    scopes: provider.scopes.join(" "),
    allowSignup: provider.allow_signup,
    allowLink: provider.allow_link,
    claims: provider.claims.map((condition) => newClaimDraft(condition.claim, condition.values.join(", "))),
    organizations: provider.organizations.join(", "),
    guilds: provider.guilds.join(", "),
  };
}

/** The items of a list typed in one field, split at `separator` and trimmed, without empty ones. */
function items(text: string, separator: RegExp): string[] {
  return text
    .split(separator)
    .map((item) => item.trim())
    .filter((item) => item !== "");
}

/** The conditions of the draft, without the rows left empty; undefined when a row lacks its claim or values. */
function claimsOf(draft: LoginProviderDraft): LoginProviderClaimCondition[] | undefined {
  const conditions: LoginProviderClaimCondition[] = [];
  for (const row of draft.claims) {
    const claim = row.claim.trim();
    const values = items(row.values, /,/u);
    if (claim === "" && values.length === 0) continue;
    if (claim === "" || values.length === 0) return undefined;
    conditions.push({ claim, values });
  }
  return conditions;
}

/**
 * The settings of the draft that both a setup and a change send, only the ones its kind takes; or the field error
 * of a condition row that lacks its claim or its values.
 */
function settingsOf(draft: LoginProviderDraft, incomplete: string, badGuild: string): LoginProviderChange | LoginProviderFieldError {
  const settings: LoginProviderChange = {
    display_name: draft.displayName.trim(),
    client_id: draft.clientId.trim(),
    allow_signup: draft.allowSignup,
    allow_link: draft.allowLink,
  };
  if (draft.kind === "oidc" || draft.kind === "mastodon") settings.issuer = draft.issuer.trim();
  if (draft.kind !== "mastodon") settings.scopes = items(draft.scopes, /\s+/u);
  if (draft.kind === "oidc") {
    const claims = claimsOf(draft);
    if (claims === undefined) return { field: "conditions", message: incomplete };
    settings.claims = claims;
  }
  if (draft.kind === "github") settings.organizations = items(draft.organizations, /[\s,]+/u);
  if (draft.kind === "discord") {
    const guilds = items(draft.guilds, /[\s,]+/u);
    if (!guilds.every((id) => guildId.test(id))) return { field: "conditions", message: badGuild };
    settings.guilds = guilds;
  }
  return settings;
}

/** A Discord server ID: a decimal number of up to 20 digits. */
const guildId = /^\d{1,20}$/u;

function isFieldError(value: LoginProviderChange | LoginProviderFieldError): value is LoginProviderFieldError {
  return "field" in value;
}

/** The form of a new provider. A success empties it and keeps the new provider to show its callback URL. */
export function useLoginProviderForm() {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<LoginProviderDraft>(emptyDraft);
  const [created, setCreated] = useState<LoginProvider | undefined>(undefined);
  const [invalid, setInvalid] = useState<LoginProviderFieldError | undefined>(undefined);
  const mutation = useMutation({
    ...createLoginProviderMutation(api),
    onMutate: () => {
      setCreated(undefined);
    },
    onSuccess: async (provider) => {
      await refreshLoginProviders(queryClient);
      setDraft(emptyDraft);
      setCreated(provider);
    },
  });
  return {
    draft,
    change: (change: Partial<LoginProviderDraft>) => {
      setDraft((previous) => ({ ...previous, ...change }));
    },
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      if (mutation.isPending) return;
      const settings = settingsOf(draft, t("client:admin.login_providers.conditions.incomplete"), t("client:admin.login_providers.guilds.invalid"));
      if (isFieldError(settings)) {
        mutation.reset();
        setInvalid(settings);
        return;
      }
      setInvalid(undefined);
      const { client_id: clientId = "", ...rest } = settings;
      // Without a client ID and secret, a Mastodon provider registers its client at the server.
      const registers = draft.kind === "mastodon" && clientId === "" && draft.clientSecret === "";
      const body: NewLoginProvider = { ...rest, kind: draft.kind, display_name: settings.display_name ?? "", ...(!registers && { client_id: clientId, client_secret: draft.clientSecret }) };
      mutation.mutate(body);
    },
    isPending: mutation.isPending,
    /** Why setting it up failed, next to its field; the draft stays. */
    error: invalid ?? (mutation.isError ? fieldError(mutation.error) : undefined),
    /** The new provider, whose callback URL is shown until the next submit. */
    created,
    /** What the last submit did, for the form's StatusMessage. */
    status: created === undefined ? undefined : t("client:admin.login_providers.created", { name: loginProviderName(created) }),
  };
}

/** Copies a provider's callback URL to the clipboard and says whether that worked in a toast. */
export function useCopyCallbackUrl(): (url: string) => void {
  const t = useT();
  const toast = useToast();
  return (url) => {
    navigator.clipboard.writeText(url).then(
      () => {
        // The toast is announced too.
        toast.show({ message: t("client:admin.login_providers.callback.copied"), tone: "success" });
      },
      () => {
        toast.show({ message: t("client:admin.login_providers.callback.copy_failed"), tone: "danger" });
      },
    );
  };
}

export type LoginProviderActions = {
  /** Changes a provider's settings from a draft; resolves with the refused field, or undefined once it is saved. */
  update: (provider: LoginProvider, draft: LoginProviderDraft) => Promise<LoginProviderFieldError | undefined>;
  /** Gives a provider a new client secret; resolves with the refused field, or undefined once it is saved. */
  rotate: (provider: LoginProvider, secret: string) => Promise<LoginProviderFieldError | undefined>;
  /** Enables or disables a provider; resolves with whether it succeeded. */
  setEnabled: (provider: LoginProvider, enabled: boolean) => Promise<boolean>;
  /** Deletes a provider; resolves with whether it succeeded. */
  remove: (provider: LoginProvider) => Promise<boolean>;
  /** The provider being changed. */
  pendingId: string | undefined;
  /** Why the last change failed, localized, until the next change. */
  error: string | undefined;
  /** What the last change did, for the list's StatusMessage. */
  status: string | undefined;
};

/**
 * Changing the providers of the list. `onDeleted(id)` is called once a deleted provider is about to leave the list
 * (to keep focus, lib/useFocusAfterRemoval); a changed provider keeps its row, which shows the answer.
 */
export function useLoginProviderActions(onDeleted: (id: string) => void): LoginProviderActions {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const announce = useAnnouncer();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const report = (message: string) => {
    setStatus(message);
    announce(message);
  };
  const update = useMutation(updateLoginProviderMutation(api));
  const rotate = useMutation(rotateLoginProviderSecretMutation(api));
  const enable = useMutation(setLoginProviderEnabledMutation(api));
  const remove = useMutation(deleteLoginProviderMutation(api));
  const mutations = [update, rotate, enable, remove] as const;
  const resetAll = () => {
    for (const mutation of mutations) mutation.reset();
  };
  const pendingId = update.isPending
    ? update.variables.id
    : rotate.isPending
      ? rotate.variables.id
      : enable.isPending
        ? enable.variables.id
        : remove.isPending
          ? remove.variables
          : undefined;
  // A refused setting stays in its dialog; only the list's own actions show their failure above it.
  const failed = [enable, remove].find((mutation) => mutation.isError);
  return {
    update: async (provider, draft) => {
      resetAll();
      const change = settingsOf(draft, t("client:admin.login_providers.conditions.incomplete"), t("client:admin.login_providers.guilds.invalid"));
      if (isFieldError(change)) return change;
      try {
        const saved = await update.mutateAsync({ id: provider.id, change });
        await refreshLoginProviders(queryClient);
        report(t("client:admin.login_providers.updated", { name: loginProviderName(saved) }));
        return undefined;
      } catch (error) {
        return fieldError(error);
      }
    },
    rotate: async (provider, secret) => {
      resetAll();
      try {
        await rotate.mutateAsync({ id: provider.id, secret });
        await refreshLoginProviders(queryClient);
        report(t("client:admin.login_providers.rotated", { name: loginProviderName(provider) }));
        return undefined;
      } catch (error) {
        return fieldError(error);
      }
    },
    setEnabled: async (provider, enabled) => {
      resetAll();
      try {
        const saved = await enable.mutateAsync({ id: provider.id, enabled });
        await refreshLoginProviders(queryClient);
        report(t(enabled ? "client:admin.login_providers.enabled_status" : "client:admin.login_providers.disabled_status", { name: loginProviderName(saved) }));
        return true;
      } catch {
        return false;
      }
    },
    remove: async (provider) => {
      resetAll();
      try {
        await remove.mutateAsync(provider.id);
      } catch {
        return false;
      }
      onDeleted(provider.id);
      await refreshLoginProviders(queryClient);
      report(t("client:admin.login_providers.deleted", { name: loginProviderName(provider) }));
      return true;
    },
    pendingId,
    error: failed === undefined ? undefined : errorMessage(failed.error),
    status,
  };
}
