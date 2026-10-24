// SPDX-License-Identifier: AGPL-3.0-only
// The development page below its heading (ADR-0090 §1, §2): the form that registers an app, and the user's apps, each
// with its buttons that change it in a dialog, give it a new secret and delete it after a confirmation, and issue
// tokens of it to the user's own account in a dialog. A secret and the tokens are shown once with a copy button and
// a note that they are not shown again: the secret of a new or renewed app in a region above the list that takes focus,
// the tokens in the dialog that issued them (a press outside does not close it). What registering did stays in a
// StatusMessage; removing is announced and moves focus to the next row (web-client/README.md E2).
import {
  Box,
  Button,
  Cluster,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  Dialog,
  DialogActions,
  DialogTrigger,
  EmptyState,
  Heading,
  Hint,
  InlineError,
  Radio,
  RadioGroup,
  Stack,
  StatusMessage,
  Text,
  TextArea,
  TextField,
} from "@my/ui";
import { type ReactNode, useEffect, useId, useRef, useState } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { appNameLimit, appWebsiteLimit, type OAuthApp, type OAuthAppToken } from "../api/oauthApps";
import {
  type AppDraft,
  type DeleteApp,
  draftOf,
  type RegisterAppForm,
  type RotateSecret,
  type ShownSecret,
  useCopyValue,
  useDeleteApp,
  useIssueToken,
  useOAuthAppList,
  useRegisterAppForm,
  useRotateSecret,
  useUpdateApp,
} from "../hooks/useOAuthApps";

/** The most redirect URIs an app has (CreateOAuthAppRequest.redirect_uris). */
const redirectUrisMax = 10;

export function OAuthAppSettings() {
  const t = useT();
  const [shown, setShown] = useState<ShownSecret | undefined>(undefined);
  return (
    <Stack container gap="8">
      <p>{t("client:settings.apps.description")}</p>
      <Section title={t("client:settings.apps.new.title")}>
        <RegisterForm onRegistered={setShown} />
      </Section>
      {shown !== undefined && <SecretNotice shown={shown} />}
      <Section title={t("client:settings.apps.list.title")}>
        <AppList onRotated={setShown} />
      </Section>
    </Stack>
  );
}

/** A titled region of the page below its `h1`: an `h2` that names a `<section>`. */
function Section({ title, children }: { title: ReactNode; children: ReactNode }) {
  const headingId = useId();
  return (
    <Stack as="section" aria-labelledby={headingId} gap="3">
      <Heading level={2} id={headingId}>
        {title}
      </Heading>
      {children}
    </Stack>
  );
}

/** A value to copy: its label, the value on its own line as the user reads it, and the button that copies it. */
function CopyValue({ label, value }: { label: string; value: string }) {
  const t = useT();
  const copy = useCopyValue();
  const labelId = useId();
  return (
    <Stack gap="1">
      <Text as="p" size="sm" weight="medium" id={labelId}>
        {label}
      </Text>
      <Text as="p" wrap="anywhere" mono selectAll aria-labelledby={labelId} dir="ltr">
        {value}
      </Text>
      <div>
        <Button
          size="sm"
          aria-label={t("client:settings.apps.copy.label", { name: label })}
          onPress={() => {
            copy(value);
          }}
        >
          {t("client:settings.apps.copy")}
        </Button>
      </div>
    </Stack>
  );
}

/**
 * The credentials of an app that are shown only now. The region takes focus when it appears, so that the reader finds
 * it (and a screen reader reads its title and the note) before anything else.
 */
function SecretNotice({ shown }: { shown: ShownSecret }) {
  const t = useT();
  const headingId = useId();
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    ref.current?.focus();
  }, [shown]);
  return (
    <Stack as="section" ref={ref} tabIndex={-1} aria-labelledby={headingId} gap="3" border="md" padding="3">
      <Heading level={2} id={headingId}>
        {t("client:settings.apps.secret.title", { name: shown.app.name })}
      </Heading>
      <Hint>
        {shown.secret === undefined
          ? t("client:settings.apps.secret.public", { name: shown.app.name })
          : t("client:settings.apps.secret.notice")}
      </Hint>
      <CopyValue label={t("client:settings.apps.client_id")} value={shown.app.client_id} />
      {shown.secret !== undefined && <CopyValue label={t("client:settings.apps.secret.client_secret")} value={shown.secret} />}
    </Stack>
  );
}

/** The fields of an app; the type is chosen when registering and never changes. */
function AppFields({ draft, change, withType, autoFocus = false }: { draft: AppDraft; change: (change: Partial<AppDraft>) => void; withType: boolean; autoFocus?: boolean }) {
  const t = useT();
  return (
    <>
      <TextField
        label={t("client:settings.apps.name")}
        description={t("client:settings.apps.name.description", { max: String(appNameLimit) })}
        value={draft.name}
        onChange={(name) => {
          change({ name });
        }}
        isRequired
        maxLength={appNameLimit}
        autoComplete="off"
        autoFocus={autoFocus}
        errorMessage={t("client:settings.apps.name.required")}
      />
      <TextField
        type="url"
        label={t("client:settings.apps.website")}
        description={t("client:settings.apps.website.description")}
        value={draft.website}
        onChange={(website) => {
          change({ website });
        }}
        maxLength={appWebsiteLimit}
        autoComplete="off"
        spellCheck="false"
      />
      <TextArea
        label={t("client:settings.apps.redirect_uris")}
        description={t("client:settings.apps.redirect_uris.description", { max: String(redirectUrisMax) })}
        value={draft.redirectUris}
        onChange={(redirectUris) => {
          change({ redirectUris });
        }}
        isRequired
        rows={3}
        autoComplete="off"
        spellCheck="false"
        dir="ltr"
        errorMessage={t("client:settings.apps.redirect_uris.required")}
      />
      <TextField
        label={t("client:settings.apps.scope")}
        description={t("client:settings.apps.scope.description")}
        value={draft.scope}
        onChange={(scope) => {
          change({ scope });
        }}
        isRequired
        autoComplete="off"
        autoCorrect="off"
        spellCheck="false"
        dir="ltr"
        errorMessage={t("client:settings.apps.scope.required")}
      />
      {withType && (
        <RadioGroup
          label={t("client:settings.apps.client_type")}
          value={draft.clientType}
          onChange={(clientType) => {
            change({ clientType: clientType === "confidential" ? "confidential" : "public" });
          }}
        >
          <Radio value="public" description={t("client:settings.apps.client_type.public.description")}>
            {t("client:settings.apps.client_type.public")}
          </Radio>
          <Radio value="confidential" description={t("client:settings.apps.client_type.confidential.description")}>
            {t("client:settings.apps.client_type.confidential")}
          </Radio>
        </RadioGroup>
      )}
    </>
  );
}

function RegisterForm({ onRegistered }: { onRegistered: (shown: ShownSecret) => void }) {
  const t = useT();
  const form: RegisterAppForm = useRegisterAppForm(onRegistered);
  return (
    <Stack as="form" onSubmit={form.submit} gap="4">
      <AppFields draft={form.draft} change={form.change} withType />
      {form.error !== undefined && (
        <InlineError alert size="inherit">
          {form.error}
        </InlineError>
      )}
      <div>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:settings.apps.register")}
        </Button>
      </div>
      <StatusMessage>{form.status}</StatusMessage>
    </Stack>
  );
}

function AppList({ onRotated }: { onRotated: (shown: ShownSecret) => void }) {
  const t = useT();
  const apps = useOAuthAppList();
  const remove = useDeleteApp();
  const rotate = useRotateSecret();
  const listRef = useRef<HTMLUListElement>(null);
  const removed = useFocusAfterRemoval(
    listRef,
    apps.map((app) => app.id),
  );
  if (apps.length === 0) return <EmptyState headingLevel={3} heading={t("client:settings.apps.empty")} />;
  return (
    <Stack as="ul" ref={listRef} divided>
      {apps.map((app) => (
        <Box as="li" key={app.id} data-row-id={app.id} container paddingY="3">
          <AppRow app={app} remove={remove} rotate={rotate} onRotated={onRotated} onRemoved={removed} />
        </Box>
      ))}
    </Stack>
  );
}

type AppRowProps = {
  app: OAuthApp;
  remove: DeleteApp;
  rotate: RotateSecret;
  onRotated: (shown: ShownSecret) => void;
  onRemoved: (id: string) => void;
};

/** An app: what it is called and how it is registered, with the buttons of what the user can do with it. */
function AppRow({ app, remove, rotate, onRotated, onRemoved }: AppRowProps) {
  const t = useT();
  const intl = useIntl();
  const action = (label: string) => t("client:settings.list.action", { action: label, name: app.name });
  const editLabel = t("client:settings.apps.edit");
  const tokenLabel = t("client:settings.apps.token");
  const rotateLabel = t("client:settings.apps.rotate");
  const deleteLabel = t("client:settings.apps.delete");
  return (
    <Stack gap="3">
      <DescriptionList columns={1}>
        <DescriptionItem term={t("client:settings.apps.name")}>
          <Text dir="auto" weight="semibold">
            {app.name}
          </Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:settings.apps.client_id")}>
          <Text mono dir="ltr" wrap="anywhere">
            {app.client_id}
          </Text>
        </DescriptionItem>
        <DescriptionItem term={t("client:settings.apps.client_type")}>
          {app.client_type === "confidential" ? t("client:settings.apps.type.confidential") : t("client:settings.apps.type.public")}
        </DescriptionItem>
        <DescriptionItem term={t("client:settings.apps.scope")}>
          <Text mono dir="ltr" wrap="anywhere">
            {app.scope}
          </Text>
        </DescriptionItem>
      </DescriptionList>
      <Hint as="span">{t("client:settings.apps.created_at", { date: intl.formatDate(app.created_at, { dateStyle: "medium", timeStyle: "short" }) })}</Hint>
      {app.disabled && <Hint>{t("client:settings.apps.disabled")}</Hint>}
      <Cluster gap="2">
        <DialogTrigger>
          <Button size="sm" aria-label={action(editLabel)}>
            {editLabel}
          </Button>
          <Dialog title={t("client:settings.apps.edit.title", { name: app.name })} closeLabel={t("client:settings.apps.edit.close")} isDismissable={false} size="md">
            {({ close }) => <EditAppForm app={app} close={close} />}
          </Dialog>
        </DialogTrigger>
        <DialogTrigger>
          <Button size="sm" aria-label={action(tokenLabel)}>
            {tokenLabel}
          </Button>
          <Dialog title={t("client:settings.apps.token.title", { name: app.name })} closeLabel={t("client:settings.apps.edit.close")} isDismissable={false} size="md">
            {({ close }) => <IssueTokenForm app={app} close={close} />}
          </Dialog>
        </DialogTrigger>
        {app.client_type === "confidential" && (
          <DialogTrigger
            onOpenChange={(isOpen) => {
              if (isOpen) rotate.reset();
            }}
          >
            <Button size="sm" aria-label={action(rotateLabel)}>
              {rotateLabel}
            </Button>
            <ConfirmDialog
              tone="primary"
              title={t("client:settings.apps.rotate.title", { name: app.name })}
              description={t("client:settings.apps.rotate.description")}
              confirmLabel={rotateLabel}
              cancelLabel={t("client:settings.apps.cancel")}
              isPending={rotate.pendingId === app.id}
              {...(rotate.error !== undefined && { errorMessage: rotate.error })}
              onConfirm={(close) => {
                rotate.run(app, (secret) => {
                  onRotated({ app, secret });
                  close();
                });
              }}
            />
          </DialogTrigger>
        )}
        <DialogTrigger
          onOpenChange={(isOpen) => {
            if (isOpen) remove.reset();
          }}
        >
          <Button size="sm" variant="danger" isPending={remove.pendingId === app.id} aria-label={action(deleteLabel)}>
            {deleteLabel}
          </Button>
          <ConfirmDialog
            title={t("client:settings.apps.delete.title", { name: app.name })}
            description={t("client:settings.apps.delete.description")}
            confirmLabel={deleteLabel}
            cancelLabel={t("client:settings.apps.cancel")}
            tone="danger"
            isPending={remove.pendingId === app.id}
            {...(remove.error !== undefined && { errorMessage: remove.error })}
            onConfirm={(close) => {
              remove.run(app, () => {
                onRemoved(app.id);
                close();
              });
            }}
          />
        </DialogTrigger>
      </Cluster>
    </Stack>
  );
}

/** The fields of an app in a dialog; it closes once they are saved, and keeps a refusal above its buttons. */
function EditAppForm({ app, close }: { app: OAuthApp; close: () => void }) {
  const t = useT();
  const update = useUpdateApp();
  const [draft, setDraft] = useState<AppDraft>(() => draftOf(app));
  return (
    <Stack
      as="form"
      container
      gap="4"
      onSubmit={(event) => {
        event.preventDefault();
        if (update.isPending) return;
        update.run(app, draft, close);
      }}
    >
      <Hint>{t("client:settings.apps.edit.description")}</Hint>
      <AppFields
        draft={draft}
        change={(change) => {
          setDraft((previous) => ({ ...previous, ...change }));
        }}
        withType={false}
        autoFocus
      />
      {update.error !== undefined && (
        <InlineError alert size="inherit">
          {update.error}
        </InlineError>
      )}
      <DialogActions>
        <Button onPress={close}>{t("client:settings.apps.cancel")}</Button>
        <Button type="submit" variant="primary" isPending={update.isPending}>
          {t("client:settings.apps.save")}
        </Button>
      </DialogActions>
    </Stack>
  );
}

/** Issuing tokens in a dialog: the scopes, then the tokens, shown once; the dialog does not close on a press outside. */
function IssueTokenForm({ app, close }: { app: OAuthApp; close: () => void }) {
  const t = useT();
  const issue = useIssueToken();
  const [scope, setScope] = useState(app.scope);
  const [tokens, setTokens] = useState<OAuthAppToken | undefined>(undefined);
  if (tokens !== undefined) return <IssuedTokens app={app} tokens={tokens} close={close} />;
  return (
    <Stack
      as="form"
      container
      gap="4"
      onSubmit={(event) => {
        event.preventDefault();
        if (issue.isPending) return;
        issue.run(app, scope, setTokens);
      }}
    >
      <Hint>{t("client:settings.apps.token.description")}</Hint>
      <TextField
        label={t("client:settings.apps.token.scope")}
        description={t("client:settings.apps.token.scope.description")}
        value={scope}
        onChange={setScope}
        autoComplete="off"
        autoCorrect="off"
        spellCheck="false"
        dir="ltr"
        autoFocus
      />
      {issue.error !== undefined && (
        <InlineError alert size="inherit">
          {issue.error}
        </InlineError>
      )}
      <DialogActions>
        <Button onPress={close}>{t("client:settings.apps.cancel")}</Button>
        <Button type="submit" variant="primary" isPending={issue.isPending}>
          {t("client:settings.apps.token.submit")}
        </Button>
      </DialogActions>
    </Stack>
  );
}

/** The tokens that were issued, which are not shown again. */
function IssuedTokens({ app, tokens, close }: { app: OAuthApp; tokens: OAuthAppToken; close: () => void }) {
  const t = useT();
  return (
    <Stack container gap="4">
      <Text as="p" weight="medium">
        {t("client:settings.apps.token.result.title", { name: app.name })}
      </Text>
      <Hint>{t("client:settings.apps.token.result.notice")}</Hint>
      <CopyValue label={t("client:settings.apps.token.access")} value={tokens.access_token} />
      <CopyValue label={t("client:settings.apps.token.refresh")} value={tokens.refresh_token} />
      <Hint>{t("client:settings.apps.token.granted_scope", { scope: tokens.scope })}</Hint>
      <Hint>{t("client:settings.apps.token.result.expires", { minutes: Math.round(tokens.expires_in / 60) })}</Hint>
      <DialogActions>
        <Button variant="primary" onPress={close}>
          {t("client:settings.apps.done")}
        </Button>
      </DialogActions>
    </Stack>
  );
}
