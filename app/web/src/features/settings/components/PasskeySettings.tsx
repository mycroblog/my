// SPDX-License-Identifier: AGPL-3.0-only
// The passkeys page below its heading (FR-004; web-client/screens/settings-passkeys.md): the list-and-add part
// (`ListSection`) with the passkeys, oldest first, each with when it was added and its button that removes it after an
// alert dialog (ConfirmDialog: it can no longer sign in), and the button that opens the form that adds one (its name and
// the password that confirms it, or for a user without a password another passkey, then the browser's prompt). A
// browser that cannot create passkeys gets a note in place of the button. What adding did shows above the list, where
// the new row appears; removing is announced and moves focus to the next row. A wrong password is the password field's
// error and any other failure the form's action bar (web-client/README.md E2, web-client/16-look-states-and-wording.md G16, G24).
import { Box, Button, Cluster, ConfirmDialog, FormActionBar, Hint, ListSection, Stack, Text, TextField } from "@my/ui";
import { useRef, useState } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { type Passkey, passkeyNameLimit } from "../api/passkeys";
import {
  type PasskeyForm,
  type RemovePasskey,
  usePasskeyForm,
  usePasskeyList,
  useRemovePasskey,
} from "../hooks/usePasskeys";
import { ExternalConfirmationChoice } from "./ExternalConfirmationChoice";

export function PasskeySettings() {
  const t = useT();
  const passkeys = usePasskeyList();
  const form = usePasskeyForm();
  const remove = useRemovePasskey();
  const listRef = useRef<HTMLUListElement>(null);
  const removed = useFocusAfterRemoval(
    listRef,
    passkeys.map((passkey) => passkey.id),
  );
  const [confirming, setConfirming] = useState<Passkey | null>(null);
  const label = t("client:settings.passkeys.add.title");
  return (
    <>
      <ListSection
        title={t("client:settings.passkeys.list.title")}
        description={t("client:settings.passkeys.description")}
        items={passkeys}
        itemKey={(passkey) => passkey.id}
        renderItem={(passkey) => (
          <PasskeyRow
            passkey={passkey}
            remove={remove}
            onRemove={() => {
              remove.reset();
              setConfirming(passkey);
            }}
          />
        )}
        listRef={listRef}
        countLabel={t("client:settings.passkeys.count", { count: passkeys.length })}
        empty={{
          heading: t("client:settings.passkeys.empty"),
          description: t("client:settings.passkeys.empty.description"),
        }}
        add={
          form.isSupported
            ? {
                label,
                panel: (close) => <AddPasskeyForm form={form} close={close} />,
                // Back from confirming at a linked provider: the form goes on with that confirmation.
                defaultOpen: form.external.confirmed !== undefined || form.external.error !== undefined,
              }
            : { unavailable: t("client:settings.passkeys.unsupported") }
        }
        result={form.status === undefined ? undefined : { tone: "success", message: form.status }}
      />
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

/** The form that adds a passkey, in the panel of the list: closing it on success shows the result above the list. */
function AddPasskeyForm({ form, close }: { form: PasskeyForm; close: () => void }) {
  const t = useT();
  const cancel = <Button onPress={close}>{t("client:settings.list.cancel")}</Button>;
  // A user with neither a password nor a passkey signs in again at a linked provider first.
  if (form.external.isPreferred && form.external.confirmed === undefined)
    return (
      <Stack align="start" gap="4">
        <ExternalConfirmationChoice confirmation={form.external} />
        {cancel}
      </Stack>
    );
  return (
    <Stack
      as="form"
      onSubmit={(event) => {
        form.submit(event, close);
      }}
      gap="4"
    >
      {form.external.isPreferred ? (
        <ExternalConfirmationChoice confirmation={form.external} />
      ) : (
        <Hint>
          {t(
            form.hasPassword
              ? "client:settings.passkeys.add.description"
              : "client:settings.passkeys.add.description_passkey",
          )}
        </Hint>
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
      <FormActionBar result={form.error === undefined ? undefined : { tone: "danger", message: form.error }}>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:settings.passkeys.add.submit")}
        </Button>
        {cancel}
      </FormActionBar>
    </Stack>
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
