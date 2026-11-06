// SPDX-License-Identifier: AGPL-3.0-only
// The login providers screen below its heading (ADR-0106, manage_login_providers): the form that sets up an
// external login provider of a kind, then the providers, newest first. A new provider's callback URL is shown with a
// copy button to register at the provider. Each provider shows its kind, display name, state, number of linked users
// and callback URL; it is changed in a dialog, given a new client secret in a dialog, enabled, and disabled or
// deleted after an alert dialog that says how many linked users can no longer sign in with it. Setting one up or
// enabling one asks first when that would start the limit of sign-in to login providers. What a change did
// stays above the list in a StatusMessage (role="status"), and a failure stays there too (role="alert").
import {
  Button,
  Card,
  Cluster,
  ConfirmDialog,
  Dialog,
  DialogTrigger,
  EmptyState,
  InlineError,
  Radio,
  RadioGroup,
  Stack,
  StatusMessage,
  Text,
} from "@my/ui";
import { type SubmitEvent, useRef, useState } from "react";
import { useT } from "../../../lib/i18n";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { Section, type RecordColumns, RecordTable, RequirePermission, wholeList } from "../../moderation";
import { type LoginProvider, loginProviderKinds } from "../api/loginProviders";
import {
  draftOf,
  type LimitStart,
  type LoginProviderActions,
  type LoginProviderDraft,
  type LoginProviderFieldError,
  loginProviderName,
  useEnableLoginProvider,
  useLoginProviderActions,
  useLoginProviderForm,
  useLoginProviders,
} from "../hooks/useLoginProviders";
import { CallbackUrl, loginProviderKindLabels, LoginProviderFields, SecretField } from "./loginProviderParts";

export function LoginProviderSettings() {
  const t = useT();
  return (
    <RequirePermission permission="manage_login_providers">
      <Stack gap="8">
        <Section surface="card" title={t("client:admin.login_providers.new.title")}>
          <Card>
            <NewLoginProviderForm />
          </Card>
        </Section>
        <Section surface="card" title={t("client:admin.login_providers.list.title")}>
          <LoginProviderList />
        </Section>
      </Stack>
    </RequirePermission>
  );
}

function NewLoginProviderForm() {
  const t = useT();
  const form = useLoginProviderForm();
  return (
    <Stack as="form" container gap="6" onSubmit={form.submit}>
      <RadioGroup
        label={t("client:admin.login_providers.kind")}
        orientation="horizontal"
        value={form.draft.kind}
        onChange={(value) => {
          const kind = loginProviderKinds.find((k) => k === value);
          if (kind !== undefined) form.change({ kind });
        }}
        isRequired
      >
        {loginProviderKinds.map((kind) => (
          <Radio key={kind} value={kind}>
            {t(loginProviderKindLabels[kind])}
          </Radio>
        ))}
      </RadioGroup>
      <LoginProviderFields draft={form.draft} change={form.change} error={form.error} withSecret />
      <div>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:admin.login_providers.submit")}
        </Button>
      </div>
      <LimitStartDialog
        limitStart={form.limitStart}
        confirmLabel={
          form.limitStart?.impact === undefined
            ? t("client:admin.login_providers.submit")
            : t("client:admin.login_providers.limit_start.confirm_add")
        }
        onConfirm={(close) => {
          form.confirmLimitStart();
          close();
        }}
        onCancel={form.cancelLimitStart}
      />
      <StatusMessage>{form.status}</StatusMessage>
      {form.created !== undefined && (
        <Stack gap="2" border="md" padding="3">
          <Text as="p">{t("client:admin.login_providers.callback.notice")}</Text>
          <CallbackUrl url={form.created.callback_url} name={loginProviderName(form.created)} />
        </Stack>
      )}
    </Stack>
  );
}

function LoginProviderList() {
  const t = useT();
  const listRef = useRef<HTMLDivElement>(null);
  const providers = useLoginProviders();
  const removed = useFocusAfterRemoval(
    listRef,
    providers.map((provider) => provider.id),
  );
  const actions = useLoginProviderActions(removed);
  const columns: RecordColumns<LoginProvider> = [
    {
      id: "provider",
      header: t("client:admin.login_providers.provider"),
      wrap: "anywhere",
      cell: (provider) => <LoginProviderName provider={provider} />,
    },
    {
      id: "kind",
      header: t("client:admin.login_providers.kind"),
      cell: (provider) => t(loginProviderKindLabels[provider.kind]),
    },
    {
      id: "state",
      header: t("client:admin.login_providers.state"),
      cell: (provider) => <LoginProviderState provider={provider} />,
    },
    {
      id: "linked_users",
      header: t("client:admin.login_providers.linked_users"),
      cell: (provider) => t("client:admin.login_providers.linked_users.count", { count: provider.linked_users }),
    },
    {
      id: "actions",
      kind: "actions",
      header: t("client:admin.list.column.actions"),
      cell: (provider) => <LoginProviderRowActions provider={provider} actions={actions} />,
    },
  ];
  return (
    <Stack gap="3">
      <StatusMessage>{actions.status}</StatusMessage>
      {actions.error !== undefined && (
        <InlineError size="inherit" alert>
          {t("client:admin.login_providers.change_failed", { reason: actions.error })}
        </InlineError>
      )}
      <RecordTable
        list={wholeList(providers)}
        caption={t("client:admin.login_providers.list.title")}
        columns={columns}
        getId={(provider) => provider.id}
        headingLevel={3}
        listRef={listRef}
        empty={
          <EmptyState
            headingLevel={3}
            heading={t("client:admin.login_providers.empty")}
            description={t("client:admin.login_providers.empty.description")}
          />
        }
      />
    </Stack>
  );
}

function LoginProviderName({ provider }: { provider: LoginProvider }) {
  return (
    <Stack gap="2">
      <Stack gap="1">
        <Text weight="semibold" wrap="anywhere" dir="auto">
          {provider.display_name}
        </Text>
        {provider.issuer !== undefined && (
          <Text size="sm" tone="muted" mono wrap="anywhere" dir="ltr">
            {provider.issuer}
          </Text>
        )}
      </Stack>
      <CallbackUrl url={provider.callback_url} name={loginProviderName(provider)} />
    </Stack>
  );
}

function LoginProviderState({ provider }: { provider: LoginProvider }) {
  const t = useT();
  return (
    <Stack gap="1">
      <span>
        {t(
          provider.state === "enabled"
            ? "client:admin.login_providers.state.enabled"
            : "client:admin.login_providers.state.disabled",
        )}
      </span>
      <Text size="sm" tone="muted">
        {t(
          provider.allow_signup
            ? "client:admin.login_providers.state.signup_allowed"
            : "client:admin.login_providers.state.signup_closed",
        )}
      </Text>
    </Stack>
  );
}

function LoginProviderRowActions({ provider, actions }: { provider: LoginProvider; actions: LoginProviderActions }) {
  const isPending = actions.pendingId === provider.id;
  return (
    <Cluster gap="2" align="center">
      <EditLoginProvider provider={provider} actions={actions} isPending={isPending} />
      <RotateSecret provider={provider} actions={actions} isPending={isPending} />
      {provider.state === "enabled" ? (
        <DisableLoginProvider provider={provider} actions={actions} isPending={isPending} />
      ) : (
        <EnableLoginProvider provider={provider} actions={actions} isPending={isPending} />
      )}
      <DeleteLoginProvider provider={provider} actions={actions} isPending={isPending} />
    </Cluster>
  );
}

type RowActionProps = { provider: LoginProvider; actions: LoginProviderActions; isPending: boolean };

/** The settings of a provider in a dialog (its kind stays); it closes once they are saved, and keeps a refusal on its field. */
function EditLoginProvider({ provider, actions, isPending }: RowActionProps) {
  const t = useT();
  const name = loginProviderName(provider);
  return (
    <DialogTrigger>
      <Button size="sm" isDisabled={isPending} aria-label={t("client:admin.login_providers.edit_provider", { name })}>
        {t("client:admin.login_providers.edit")}
      </Button>
      <Dialog
        title={t("client:admin.login_providers.edit.title", { name })}
        closeLabel={t("client:admin.login_providers.close")}
        isDismissable={false}
        size="md"
      >
        {({ close }) => <EditLoginProviderForm provider={provider} actions={actions} close={close} />}
      </Dialog>
    </DialogTrigger>
  );
}

function EditLoginProviderForm({
  provider,
  actions,
  close,
}: {
  provider: LoginProvider;
  actions: LoginProviderActions;
  close: () => void;
}) {
  const t = useT();
  const [draft, setDraft] = useState<LoginProviderDraft>(() => draftOf(provider));
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<LoginProviderFieldError | undefined>(undefined);
  const onSubmit = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault();
    setIsSaving(true);
    setError(undefined);
    void actions.update(provider, draft).then((refused) => {
      setIsSaving(false);
      if (refused === undefined) close();
      else setError(refused);
    });
  };
  return (
    <Stack as="form" container gap="4" onSubmit={onSubmit}>
      <Text as="p" size="sm" tone="muted">
        {t("client:admin.login_providers.edit.kind", { kind: t(loginProviderKindLabels[provider.kind]) })}
      </Text>
      <LoginProviderFields
        draft={draft}
        change={(change) => {
          setDraft((previous) => ({ ...previous, ...change }));
        }}
        error={error}
        withSecret={false}
        autoFocus
      />
      <Cluster gap="2" justify="end">
        <Button onPress={close}>{t("client:admin.login_providers.cancel")}</Button>
        <Button type="submit" variant="primary" isPending={isSaving}>
          {t("client:admin.login_providers.save")}
        </Button>
      </Cluster>
    </Stack>
  );
}

/** Giving a provider the new client secret that it issued, in a dialog; the old one is no longer used. */
function RotateSecret({ provider, actions, isPending }: RowActionProps) {
  const t = useT();
  const name = loginProviderName(provider);
  return (
    <DialogTrigger>
      <Button size="sm" isDisabled={isPending} aria-label={t("client:admin.login_providers.rotate_provider", { name })}>
        {t("client:admin.login_providers.rotate")}
      </Button>
      <Dialog
        title={t("client:admin.login_providers.rotate.title", { name })}
        closeLabel={t("client:admin.login_providers.close")}
        isDismissable={false}
        size="sm"
      >
        {({ close }) => <RotateSecretForm provider={provider} actions={actions} close={close} />}
      </Dialog>
    </DialogTrigger>
  );
}

function RotateSecretForm({
  provider,
  actions,
  close,
}: {
  provider: LoginProvider;
  actions: LoginProviderActions;
  close: () => void;
}) {
  const t = useT();
  const [secret, setSecret] = useState("");
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<LoginProviderFieldError | undefined>(undefined);
  const onSubmit = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault();
    setIsSaving(true);
    setError(undefined);
    void actions.rotate(provider, secret).then((refused) => {
      setIsSaving(false);
      if (refused === undefined) close();
      else setError(refused);
    });
  };
  return (
    <Stack as="form" container gap="4" onSubmit={onSubmit}>
      <Text as="p">{t("client:admin.login_providers.rotate.description")}</Text>
      <SecretField value={secret} onChange={setSecret} error={error} autoFocus />
      {error !== undefined && error.field !== "client_secret" && (
        <InlineError size="inherit" alert>
          {t("client:admin.login_providers.save_failed", { reason: error.message })}
        </InlineError>
      )}
      <Cluster gap="2" justify="end">
        <Button onPress={close}>{t("client:admin.login_providers.cancel")}</Button>
        <Button type="submit" variant="primary" isPending={isSaving}>
          {t("client:admin.login_providers.rotate.submit")}
        </Button>
      </Cluster>
    </Stack>
  );
}

/**
 * The warning before an action starts the limit of sign-in to login providers (an alert dialog, as for disabling): what
 * stops, how many active users could no longer sign in, whether the administrator is one of them, and how to undo it.
 * It warns and does not refuse.
 */
function LimitStartDialog({
  limitStart,
  confirmLabel,
  isPending = false,
  errorMessage,
  onConfirm,
  onCancel,
}: {
  limitStart: LimitStart | null;
  confirmLabel: string;
  isPending?: boolean;
  errorMessage?: string | undefined;
  onConfirm: (close: () => void) => void;
  onCancel: () => void;
}) {
  const t = useT();
  const impact = limitStart?.impact;
  return (
    <ConfirmDialog
      isOpen={limitStart !== null}
      onOpenChange={(open) => {
        if (!open) onCancel();
      }}
      title={t("client:admin.login_providers.limit_start.title")}
      description={
        impact === undefined ? (
          t("client:admin.login_providers.limit_start.failed")
        ) : (
          <>
            {t("client:admin.login_providers.limit_start.lead")}{" "}
            {t("client:admin.login_providers.limit_start.affected", { count: impact.users_without_provider })}{" "}
            {impact.includes_viewer && (
              <>
                <Text weight="semibold">{t("client:admin.login_providers.limit_start.self")}</Text>{" "}
              </>
            )}
            {t("client:admin.login_providers.limit_start.recovery")}
          </>
        )
      }
      confirmLabel={confirmLabel}
      cancelLabel={t("client:admin.login_providers.cancel")}
      isPending={isPending}
      {...(errorMessage !== undefined && { errorMessage })}
      onConfirm={onConfirm}
    />
  );
}

/** Enabling a provider: at once, or after the warning when that starts the limit of sign-in to login providers. */
function EnableLoginProvider({ provider, actions, isPending }: RowActionProps) {
  const t = useT();
  const name = loginProviderName(provider);
  const enable = useEnableLoginProvider(provider, actions);
  return (
    <>
      <Button
        size="sm"
        isPending={isPending || enable.isChecking}
        aria-label={t("client:admin.login_providers.enable_provider", { name })}
        onPress={enable.press}
      >
        {t("client:admin.login_providers.enable")}
      </Button>
      <LimitStartDialog
        limitStart={enable.limitStart}
        confirmLabel={
          enable.limitStart?.impact === undefined
            ? t("client:admin.login_providers.enable")
            : t("client:admin.login_providers.limit_start.confirm_enable")
        }
        isPending={enable.isEnabling}
        errorMessage={
          enable.failed && actions.error !== undefined
            ? t("client:admin.login_providers.change_failed", { reason: actions.error })
            : undefined
        }
        onConfirm={enable.confirm}
        onCancel={enable.cancel}
      />
    </>
  );
}

/** Disabling a provider, after an alert dialog that says how many linked users can no longer sign in with it. */
function DisableLoginProvider({ provider, actions, isPending }: RowActionProps) {
  const t = useT();
  const name = loginProviderName(provider);
  const [isDisabling, setIsDisabling] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <DialogTrigger>
      <Button
        size="sm"
        isDisabled={isPending}
        aria-label={t("client:admin.login_providers.disable_provider", { name })}
      >
        {t("client:admin.login_providers.disable")}
      </Button>
      <ConfirmDialog
        title={t("client:admin.login_providers.disable.title", { name })}
        description={t("client:admin.login_providers.disable.description", { count: provider.linked_users })}
        confirmLabel={t("client:admin.login_providers.disable")}
        cancelLabel={t("client:admin.login_providers.cancel")}
        isPending={isDisabling}
        {...(failed &&
          actions.error !== undefined && {
            errorMessage: t("client:admin.login_providers.change_failed", { reason: actions.error }),
          })}
        onConfirm={(close) => {
          setIsDisabling(true);
          setFailed(false);
          void actions.setEnabled(provider, false).then((disabled) => {
            setIsDisabling(false);
            if (disabled) close();
            else setFailed(true);
          });
        }}
      />
    </DialogTrigger>
  );
}

/** Deleting a provider, after an alert dialog: it cannot be undone, and its linked users can no longer sign in with it. */
function DeleteLoginProvider({ provider, actions, isPending }: RowActionProps) {
  const t = useT();
  const name = loginProviderName(provider);
  const [isDeleting, setIsDeleting] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <DialogTrigger>
      <Button
        size="sm"
        variant="danger"
        isDisabled={isPending}
        aria-label={t("client:admin.login_providers.delete_provider", { name })}
      >
        {t("client:admin.login_providers.delete")}
      </Button>
      <ConfirmDialog
        title={t("client:admin.login_providers.delete.title", { name })}
        description={t("client:admin.login_providers.delete.description", { count: provider.linked_users })}
        confirmLabel={t("client:admin.login_providers.delete")}
        cancelLabel={t("client:admin.login_providers.cancel")}
        isPending={isDeleting}
        {...(failed &&
          actions.error !== undefined && {
            errorMessage: t("client:admin.login_providers.change_failed", { reason: actions.error }),
          })}
        onConfirm={(close) => {
          setIsDeleting(true);
          setFailed(false);
          void actions.remove(provider).then((deleted) => {
            setIsDeleting(false);
            if (deleted) close();
            else setFailed(true);
          });
        }}
      />
    </DialogTrigger>
  );
}
