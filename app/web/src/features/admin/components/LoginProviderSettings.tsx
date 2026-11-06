// SPDX-License-Identifier: AGPL-3.0-only
// The login providers screen below its heading (ADR-0106, manage_login_providers), in three pages
// (web-client/14-admin.md 画面の分け方):
// - the list (`LoginProviderSettings`): the providers, newest first, each with its display name linking to its page,
//   its kind, its state and when it was set up; above it "Add a provider" opens the form.
// - the form (`NewLoginProvider`): the kind and the fields of that kind; setting one up opens its page. It asks first
//   when that would start the limit of sign-in to login providers.
// - the page of one provider (`LoginProviderDetail`): its facts and the callback URL to register at the provider (with
//   a copy button), the form of its settings (its kind stays; saved together, the result next to the button), and its
//   actions: a new client secret in a dialog, enabling it (after the same warning), and disabling or deleting it after an
//   alert dialog that says how many linked users can no longer sign in with it. Deleting goes back to the list with a
//   toast.
// What an action did stays next to the actions in a StatusMessage (role="status"), and a failure stays there too
// (role="alert").
import {
  Button,
  ButtonLink,
  Card,
  Cluster,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  Dialog,
  DialogTrigger,
  EmptyState,
  FormActionBar,
  type FormActionResult,
  Hint,
  InlineError,
  Link,
  Radio,
  RadioGroup,
  Stack,
  StatusMessage,
  Text,
  useToast,
} from "@my/ui";
import { useNavigate } from "@tanstack/react-router";
import { type SubmitEvent, useState } from "react";
import { useT } from "../../../lib/i18n";
import { DateTime, Section, type RecordColumns, RecordTable, RequirePermission, wholeList } from "../../moderation";
import { type LoginProvider, loginProviderKinds } from "../api/loginProviders";
import {
  draftOf,
  type LimitStart,
  type LoginProviderActions,
  type LoginProviderDraft,
  type LoginProviderFieldError,
  loginProviderName,
  useEnableLoginProvider,
  useLoginProvider,
  useLoginProviderActions,
  useLoginProviderForm,
  useLoginProviders,
} from "../hooks/useLoginProviders";
import { CallbackUrl, loginProviderKindLabels, LoginProviderFields, SecretField } from "./loginProviderParts";

/** The list of the providers, with the link to the form that adds one. */
export function LoginProviderSettings() {
  const t = useT();
  return (
    <RequirePermission permission="manage_login_providers">
      <Stack gap="6">
        <div>
          <ButtonLink href="/admin/login-providers/new" variant="primary">
            {t("client:admin.login_providers.new.title")}
          </ButtonLink>
        </div>
        <Section surface="card" title={t("client:admin.login_providers.list.title")}>
          <LoginProviderList />
        </Section>
      </Stack>
    </RequirePermission>
  );
}

function LoginProviderList() {
  const t = useT();
  const providers = useLoginProviders();
  const columns: RecordColumns<LoginProvider> = [
    {
      id: "provider",
      header: t("client:admin.login_providers.provider"),
      wrap: "anywhere",
      cell: (provider) => (
        <Link href={`/admin/login-providers/${encodeURIComponent(provider.id)}`} wrap="anywhere">
          <span dir="auto">{provider.display_name}</span>
        </Link>
      ),
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
      id: "created_at",
      header: t("client:admin.login_providers.created_at"),
      cell: (provider) => <DateTime value={provider.created_at} />,
    },
  ];
  return (
    <RecordTable
      list={wholeList(providers)}
      caption={t("client:admin.login_providers.list.title")}
      columns={columns}
      getId={(provider) => provider.id}
      headingLevel={3}
      empty={
        <EmptyState
          headingLevel={3}
          heading={t("client:admin.login_providers.empty")}
          description={t("client:admin.login_providers.empty.description")}
        />
      }
    />
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

/** The form that sets up a provider; setting it up opens its page. */
export function NewLoginProvider() {
  return (
    <RequirePermission permission="manage_login_providers">
      <NewLoginProviderForm />
    </RequirePermission>
  );
}

function NewLoginProviderForm() {
  const t = useT();
  const navigate = useNavigate();
  const form = useLoginProviderForm((provider) => {
    void navigate({ to: "/admin/login-providers/$id", params: { id: provider.id } });
  });
  return (
    <Card>
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
        <FormActionBar>
          <Button type="submit" variant="primary" isPending={form.isPending}>
            {t("client:admin.login_providers.submit")}
          </Button>
        </FormActionBar>
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
      </Stack>
    </Card>
  );
}

/** The page of one provider: its facts and callback URL, its settings, and its actions. */
export function LoginProviderDetail({ id }: { id: string }) {
  return (
    <RequirePermission permission="manage_login_providers">
      <LoginProviderPage id={id} />
    </RequirePermission>
  );
}

function LoginProviderPage({ id }: { id: string }) {
  const t = useT();
  const provider = useLoginProvider(id);
  const navigate = useNavigate();
  const toast = useToast();
  const actions = useLoginProviderActions((deleted) => {
    void navigate({ to: "/admin/login-providers" });
    // The toast is announced too.
    toast.show({
      message: t("client:admin.login_providers.deleted", { name: loginProviderName(deleted) }),
      tone: "success",
    });
  });
  return (
    <Stack gap="8">
      <Section surface="card" title={<span dir="auto">{provider.display_name}</span>}>
        <Card>
          <Stack gap="6">
            <LoginProviderFacts provider={provider} />
            <Stack gap="2">
              <Text as="p">{t("client:admin.login_providers.callback.notice")}</Text>
              <CallbackUrl url={provider.callback_url} name={loginProviderName(provider)} />
            </Stack>
          </Stack>
        </Card>
      </Section>
      <Section surface="card" title={t("client:admin.login_providers.settings.title")}>
        <Card>
          <EditLoginProviderForm provider={provider} actions={actions} />
        </Card>
      </Section>
      <Section surface="card" title={t("client:admin.login_providers.manage.title")}>
        <Card>
          <Stack gap="4">
            <StatusMessage>{actions.status}</StatusMessage>
            {actions.error !== undefined && (
              <InlineError size="inherit" alert>
                {t("client:admin.login_providers.change_failed", { reason: actions.error })}
              </InlineError>
            )}
            <Cluster gap="2" align="center">
              <RotateSecret provider={provider} actions={actions} />
              {provider.state === "enabled" ? (
                <DisableLoginProvider provider={provider} actions={actions} />
              ) : (
                <EnableLoginProvider provider={provider} actions={actions} />
              )}
              <DeleteLoginProvider provider={provider} actions={actions} />
            </Cluster>
          </Stack>
        </Card>
      </Section>
    </Stack>
  );
}

/** The facts of a provider: its kind, issuer, state, what it allows, and its linked users. */
function LoginProviderFacts({ provider }: { provider: LoginProvider }) {
  const t = useT();
  return (
    <DescriptionList columns={2}>
      <DescriptionItem term={t("client:admin.login_providers.kind")}>
        {t(loginProviderKindLabels[provider.kind])}
      </DescriptionItem>
      {provider.issuer !== undefined && (
        <DescriptionItem
          term={t(
            provider.kind === "mastodon"
              ? "client:admin.login_providers.server"
              : "client:admin.login_providers.issuer",
          )}
        >
          <Text mono wrap="anywhere" dir="ltr">
            {provider.issuer}
          </Text>
        </DescriptionItem>
      )}
      <DescriptionItem term={t("client:admin.login_providers.state")}>
        <Cluster as="span" gap="1" rowGap="0">
          <span>
            {t(
              provider.state === "enabled"
                ? "client:admin.login_providers.state.enabled"
                : "client:admin.login_providers.state.disabled",
            )}
          </span>
          <DateTime value={provider.state_changed_at} />
        </Cluster>
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.login_providers.signup")}>
        {t(
          provider.allow_signup
            ? "client:admin.login_providers.state.signup_allowed"
            : "client:admin.login_providers.state.signup_closed",
        )}
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.login_providers.linked_users")}>
        {t("client:admin.login_providers.linked_users.count", { count: provider.linked_users })}
      </DescriptionItem>
      <DescriptionItem term={t("client:admin.login_providers.created_at")}>
        <DateTime value={provider.created_at} />
      </DescriptionItem>
    </DescriptionList>
  );
}

/** The settings of a provider (its kind stays), saved together; a refusal stays on its field. */
function EditLoginProviderForm({ provider, actions }: { provider: LoginProvider; actions: LoginProviderActions }) {
  const t = useT();
  const [draft, setDraft] = useState<LoginProviderDraft>(() => draftOf(provider));
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<LoginProviderFieldError | undefined>(undefined);
  const [result, setResult] = useState<FormActionResult | undefined>(undefined);
  const onSubmit = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (isSaving) return;
    setIsSaving(true);
    setError(undefined);
    setResult(undefined);
    void actions.update(provider, draft).then((refused) => {
      setIsSaving(false);
      if (refused === undefined) {
        setResult({
          tone: "success",
          message: t("client:admin.login_providers.updated", { name: loginProviderName(provider) }),
        });
        return;
      }
      setError(refused);
      if (refused.field === "form") {
        setResult({
          tone: "danger",
          message: t("client:admin.login_providers.save_failed", { reason: refused.message }),
        });
      }
    });
  };
  return (
    <Stack as="form" container gap="6" onSubmit={onSubmit}>
      <Hint size="inherit">
        {t("client:admin.login_providers.edit.kind", { kind: t(loginProviderKindLabels[provider.kind]) })}
      </Hint>
      <LoginProviderFields
        draft={draft}
        change={(change) => {
          setResult(undefined);
          setDraft((previous) => ({ ...previous, ...change }));
        }}
        error={error}
        withSecret={false}
      />
      <FormActionBar result={result}>
        <Button type="submit" variant="primary" isPending={isSaving}>
          {t("client:admin.login_providers.save")}
        </Button>
      </FormActionBar>
    </Stack>
  );
}

type ActionProps = { provider: LoginProvider; actions: LoginProviderActions };

/** Giving a provider the new client secret that it issued, in a dialog; the old one is no longer used. */
function RotateSecret({ provider, actions }: ActionProps) {
  const t = useT();
  const name = loginProviderName(provider);
  return (
    <DialogTrigger>
      <Button isDisabled={actions.pendingId === provider.id}>{t("client:admin.login_providers.rotate")}</Button>
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

function RotateSecretForm({ provider, actions, close }: ActionProps & { close: () => void }) {
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
function EnableLoginProvider({ provider, actions }: ActionProps) {
  const t = useT();
  const enable = useEnableLoginProvider(provider, actions);
  return (
    <>
      <Button isPending={actions.pendingId === provider.id || enable.isChecking} onPress={enable.press}>
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
function DisableLoginProvider({ provider, actions }: ActionProps) {
  const t = useT();
  const name = loginProviderName(provider);
  const [isDisabling, setIsDisabling] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <DialogTrigger>
      <Button isDisabled={actions.pendingId === provider.id}>{t("client:admin.login_providers.disable")}</Button>
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
function DeleteLoginProvider({ provider, actions }: ActionProps) {
  const t = useT();
  const name = loginProviderName(provider);
  const [isDeleting, setIsDeleting] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <DialogTrigger>
      <Button variant="danger" isDisabled={actions.pendingId === provider.id}>
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
