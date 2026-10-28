// SPDX-License-Identifier: AGPL-3.0-only
// The linked accounts page below its heading (FR-010; web-client/screens/settings-linked-accounts.md): the list-and-add
// part (`ListSection`) with the accounts the user links, oldest first, each with when it was linked and its button
// that unlinks it after an alert dialog (ConfirmDialog), and the button that opens the providers that the user can link
// an account at, each with its button that opens the dialog that confirms the link (the password, a passkey, or for a
// user with neither a linked provider first) and leaves for the provider. Provider names are shown as the
// administrators wrote them (dir="auto"). What the provider's round trip did shows above the list, where the new row
// appears; unlinking is announced and moves focus to the next row (web-client/README.md E2,
// web-client/16-look-states-and-wording.md G16, G24).
import { Button, Cluster, ConfirmDialog, Hint, ListSection, Stack, Text, TextField } from "@my/ui";
import { useRef, useState } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import type { LinkedIdentity } from "../api/linkedIdentities";
import {
  type LinkAccount,
  type UnlinkAccount,
  useLinkAccount,
  useLinkedAccountList,
  useLinkOutcome,
  useUnlinkAccount,
} from "../hooks/useLinkedAccounts";
import { ExternalConfirmationChoice } from "./ExternalConfirmationChoice";

export function LinkedAccountSettings() {
  const t = useT();
  const outcome = useLinkOutcome();
  const links = useLinkedAccountList();
  const link = useLinkAccount();
  const unlink = useUnlinkAccount();
  const listRef = useRef<HTMLUListElement>(null);
  const removed = useFocusAfterRemoval(
    listRef,
    links.map((item) => item.id),
  );
  const [confirming, setConfirming] = useState<LinkedIdentity | null>(null);
  let result: { tone: "success" | "danger"; message: string } | undefined;
  if (outcome.error !== undefined) result = { tone: "danger", message: outcome.error };
  else if (outcome.status !== undefined) result = { tone: "success", message: outcome.status };
  return (
    <>
      <ListSection
        title={t("client:settings.linked_accounts.list.title")}
        description={t("client:settings.linked_accounts.description")}
        items={links}
        itemKey={(item) => item.id}
        renderItem={(item) => (
          <LinkedAccountRow
            link={item}
            unlink={unlink}
            onUnlink={() => {
              unlink.reset();
              setConfirming(item);
            }}
          />
        )}
        listRef={listRef}
        countLabel={t("client:settings.linked_accounts.count", { count: links.length })}
        empty={{
          heading: t("client:settings.linked_accounts.empty"),
          description: t("client:settings.linked_accounts.empty.description"),
        }}
        add={
          link.providers.length === 0
            ? { unavailable: t("client:settings.linked_accounts.add.none") }
            : {
                label: t("client:settings.linked_accounts.add.title"),
                panel: (close) => <AddLinkedAccount link={link} close={close} />,
                // Back from confirming at a linked provider: choosing the provider to link goes on.
                defaultOpen: link.external.confirmed !== undefined || link.external.error !== undefined,
              }
        }
        result={result}
      />
      <ConfirmDialog
        isOpen={confirming !== null}
        onOpenChange={(isOpen) => {
          if (!isOpen) setConfirming(null);
        }}
        title={t("client:settings.linked_accounts.unlink.title", { provider: confirming?.provider_name ?? "" })}
        description={t("client:settings.linked_accounts.unlink.description")}
        confirmLabel={t("client:settings.linked_accounts.unlink")}
        cancelLabel={t("client:settings.linked_accounts.unlink.cancel")}
        tone="danger"
        isPending={confirming !== null && unlink.pendingId === confirming.id}
        {...(unlink.error === undefined ? {} : { errorMessage: unlink.error })}
        onConfirm={(close) => {
          if (confirming === null) return;
          unlink.run(confirming, () => {
            removed(confirming.id);
            close();
          });
        }}
      />
    </>
  );
}

/** A linked account: its provider and when it was linked, with the button that unlinks it. */
function LinkedAccountRow({
  link,
  unlink,
  onUnlink,
}: {
  link: LinkedIdentity;
  unlink: UnlinkAccount;
  onUnlink: () => void;
}) {
  const t = useT();
  const intl = useIntl();
  const label = t("client:settings.linked_accounts.unlink");
  return (
    <Stack gap="3" rowAt="md">
      <Stack as="p" shrink grow>
        <Text dir="auto" wrap="truncate" align="start" weight="semibold">
          {link.provider_name}
        </Text>
        <Hint as="span">
          {t("client:settings.linked_accounts.linked_at", {
            date: intl.formatDate(link.linked_at, { dateStyle: "medium", timeStyle: "short" }),
          })}
        </Hint>
        {!link.available && <Hint as="span">{t("client:settings.linked_accounts.unavailable")}</Hint>}
      </Stack>
      <Cluster gap="2">
        <Button
          size="sm"
          variant="danger"
          isPending={unlink.pendingId === link.id}
          aria-label={t("client:settings.list.action", { action: label, name: link.provider_name })}
          onPress={onUnlink}
        >
          {label}
        </Button>
      </Cluster>
    </Stack>
  );
}

/** The providers to link an account at, in the panel of the list, and the dialog that confirms the one chosen. */
function AddLinkedAccount({ link, close }: { link: LinkAccount; close: () => void }) {
  const t = useT();
  return (
    <Stack align="start" gap="4">
      <p>{t("client:settings.linked_accounts.add.description")}</p>
      {/* A user with neither a password nor a passkey confirms with a linked provider before choosing one to link. */}
      {link.method === "external" && <ExternalConfirmationChoice confirmation={link.external} />}
      {(link.method !== "external" || link.external.confirmed !== undefined) && (
        <Cluster gap="2">
          {link.providers.map((provider) => (
            <Button
              key={provider.id}
              onPress={() => {
                link.open(provider);
              }}
            >
              <span dir="auto">{t("client:settings.linked_accounts.link", { provider: provider.display_name })}</span>
            </Button>
          ))}
        </Cluster>
      )}
      <Button onPress={close}>{t("client:settings.list.cancel")}</Button>
      <LinkDialog link={link} />
    </Stack>
  );
}

/** The dialog that confirms a link and leaves for the provider: the password field for a user with a password. */
function LinkDialog({ link }: { link: LinkAccount }) {
  const t = useT();
  const provider = link.linking?.display_name ?? "";
  const description = {
    password: t("client:settings.linked_accounts.link.description", { provider }),
    passkey: t("client:settings.linked_accounts.link.description_passkey", { provider }),
    external: t("client:settings.linked_accounts.link.description_external", { provider }),
  }[link.method];
  return (
    <ConfirmDialog
      isOpen={link.linking !== undefined}
      onOpenChange={(isOpen) => {
        if (!isOpen) link.cancel();
      }}
      title={t("client:settings.linked_accounts.link.title", { provider })}
      description={description}
      confirmLabel={t("client:settings.linked_accounts.link.continue")}
      cancelLabel={t("client:settings.linked_accounts.link.cancel")}
      tone="primary"
      onConfirm={() => {
        link.submit();
      }}
      isPending={link.isPending}
      {...(link.error === undefined ? {} : { errorMessage: link.error })}
    >
      {link.method === "password" ? (
        <TextField
          label={t("client:settings.linked_accounts.link.password")}
          type="password"
          autoComplete="current-password"
          value={link.password}
          onChange={link.changePassword}
          isRequired
          errorMessage={t("client:settings.linked_accounts.link.password.required")}
        />
      ) : undefined}
    </ConfirmDialog>
  );
}
