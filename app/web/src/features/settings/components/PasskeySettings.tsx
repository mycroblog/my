// SPDX-License-Identifier: AGPL-3.0-only
// The passkeys page below its heading (FR-004): what a passkey does, the form that adds one (its name and the
// password that confirms it, or for a user without a password another passkey, then the browser's prompt), and the
// passkeys, oldest first, each with when it was added
// and its button that removes it after an alert dialog (ConfirmDialog: it can no longer sign in). A browser that
// cannot create passkeys gets a note instead of the form. What adding did stays on the page in a StatusMessage
// (role="status"); removing is announced and moves focus to the next row. A wrong password is the password field's
// error and any other failure an alert next to the form (web-client/README.md E2, web-client/16-look-states-and-wording.md G16).
import { Box, Button, Cluster, ConfirmDialog, EmptyState, Heading, Hint, InlineError, Stack, StatusMessage, Text, TextField } from "@my/ui";
import { type ReactNode, useId, useRef, useState } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { type Passkey, passkeyNameLimit } from "../api/passkeys";
import { type RemovePasskey, usePasskeyForm, usePasskeyList, useRemovePasskey } from "../hooks/usePasskeys";
import { ExternalConfirmationChoice } from "./ExternalConfirmationChoice";

export function PasskeySettings() {
  const t = useT();
  return (
    <Stack container gap="8">
      <p>{t("client:settings.passkeys.description")}</p>
      <Section title={t("client:settings.passkeys.add.title")}>
        <AddPasskeyForm />
      </Section>
      <Section title={t("client:settings.passkeys.list.title")}>
        <PasskeyList />
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

function AddPasskeyForm() {
  const t = useT();
  const form = usePasskeyForm();
  if (!form.isSupported) return <p>{t("client:settings.passkeys.unsupported")}</p>;
  // A user with neither a password nor a passkey signs in again at a linked provider first.
  if (form.external.isPreferred && form.external.confirmed === undefined) return <ExternalConfirmationChoice confirmation={form.external} />;
  return (
    <Stack as="form" onSubmit={form.submit} align="start" gap="4">
      {form.external.isPreferred ? (
        <ExternalConfirmationChoice confirmation={form.external} />
      ) : (
        <Hint>{t(form.hasPassword ? "client:settings.passkeys.add.description" : "client:settings.passkeys.add.description_passkey")}</Hint>
      )}
      <Box width="sm">
        <TextField
          label={t("client:settings.passkeys.name")}
          description={t("client:settings.passkeys.name.description")}
          value={form.name}
          onChange={form.changeName}
          isRequired
          maxLength={passkeyNameLimit}
          autoComplete="off"
          errorMessage={t("client:settings.passkeys.name.required")}
        />
      </Box>
      {form.hasPassword && (
        <Box width="sm">
          <TextField
            label={t("client:settings.passkeys.password")}
            description={t("client:settings.passkeys.password.description")}
            type="password"
            autoComplete="current-password"
            value={form.password}
            onChange={form.changePassword}
            isRequired
            isInvalid={form.passwordError !== undefined}
            errorMessage={form.passwordError ?? t("client:settings.passkeys.password.required")}
          />
        </Box>
      )}
      {form.error !== undefined && (
        <InlineError alert size="inherit">
          {form.error}
        </InlineError>
      )}
      <Button type="submit" variant="primary" isPending={form.isPending}>
        {t("client:settings.passkeys.add.submit")}
      </Button>
      <StatusMessage>{form.status}</StatusMessage>
    </Stack>
  );
}

function PasskeyList() {
  const t = useT();
  const passkeys = usePasskeyList();
  const remove = useRemovePasskey();
  const listRef = useRef<HTMLUListElement>(null);
  const removed = useFocusAfterRemoval(
    listRef,
    passkeys.map((passkey) => passkey.id),
  );
  const [confirming, setConfirming] = useState<Passkey | null>(null);
  if (passkeys.length === 0) return <EmptyState headingLevel={3} heading={t("client:settings.passkeys.empty")} />;
  return (
    <>
      <Stack as="ul" ref={listRef} divided>
        {passkeys.map((passkey) => (
          <Box as="li" key={passkey.id} data-row-id={passkey.id} container paddingY="3">
            <PasskeyRow
              passkey={passkey}
              remove={remove}
              onRemove={() => {
                remove.reset();
                setConfirming(passkey);
              }}
            />
          </Box>
        ))}
      </Stack>
      <ConfirmDialog
        isOpen={confirming !== null}
        onOpenChange={(isOpen) => {
          if (!isOpen) setConfirming(null);
        }}
        title={t("client:settings.passkeys.remove.title", { name: confirming?.name ?? "" })}
        description={t("client:settings.passkeys.remove.description")}
        confirmLabel={t("client:settings.passkeys.remove")}
        cancelLabel={t("client:settings.passkeys.remove.cancel")}
        tone="danger"
        isPending={confirming !== null && remove.pendingId === confirming.id}
        {...(remove.error === undefined ? {} : { errorMessage: remove.error })}
        onConfirm={(close) => {
          if (confirming === null) return;
          remove.run(confirming, () => {
            removed(confirming.id);
            close();
          });
        }}
      />
    </>
  );
}

/** A passkey: its name and when it was added, with the button that removes it. */
function PasskeyRow({ passkey, remove, onRemove }: { passkey: Passkey; remove: RemovePasskey; onRemove: () => void }) {
  const t = useT();
  const intl = useIntl();
  const label = t("client:settings.passkeys.remove");
  return (
    <Stack gap="3" rowAt="md">
      <Stack as="p" shrink grow>
        <Text dir="auto" wrap="truncate" align="start" weight="semibold">
          {passkey.name}
        </Text>
        <Hint as="span">
          {t("client:settings.passkeys.added_at", {
            date: intl.formatDate(passkey.created_at, { dateStyle: "medium", timeStyle: "short" }),
          })}
        </Hint>
      </Stack>
      <Cluster gap="2">
        <Button
          size="sm"
          variant="danger"
          isPending={remove.pendingId === passkey.id}
          aria-label={t("client:settings.list.action", { action: label, name: passkey.name })}
          onPress={onRemove}
        >
          {label}
        </Button>
      </Cluster>
    </Stack>
  );
}
