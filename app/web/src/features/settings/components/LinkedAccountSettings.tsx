// SPDX-License-Identifier: AGPL-3.0-only
// The linked accounts page below its heading (FR-010): what a linked account does, the accounts the user links,
// oldest first, each with when it was linked and its button that unlinks it after an alert dialog (ConfirmDialog), and
// the providers that the user can link an account at, each with its button that opens the dialog that confirms the
// link (the password, a passkey, or for a user with neither a linked provider first) and leaves for the provider.
// Provider names are shown as the administrators wrote them (dir="auto"). What the provider's round trip did stays on
// the page in a StatusMessage or an alert at its top; unlinking is announced and moves focus to the next row
// (web-client/README.md E2, web-client/16-look-states-and-wording.md G16).
import {
  Box,
  Button,
  Cluster,
  ConfirmDialog,
  EmptyState,
  Hint,
  InlineError,
  Section,
  Stack,
  StatusMessage,
  Text,
  TextField,
} from "@my/ui";
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
  return (
    <Stack container gap="8">
      <StatusMessage>{outcome.status}</StatusMessage>
      {outcome.error !== undefined && (
        <InlineError alert size="inherit">
          {outcome.error}
        </InlineError>
      )}
      <p>{t("client:settings.linked_accounts.description")}</p>
      <Section title={t("client:settings.linked_accounts.list.title")}>
        <LinkedAccountList />
      </Section>
      <Section title={t("client:settings.linked_accounts.add.title")}>
        <AddLinkedAccount />
      </Section>
    </Stack>
  );
}

function LinkedAccountList() {
  const t = useT();
  const links = useLinkedAccountList();
  const unlink = useUnlinkAccount();
  const listRef = useRef<HTMLUListElement>(null);
  const removed = useFocusAfterRemoval(
    listRef,
    links.map((link) => link.id),
  );
  const [confirming, setConfirming] = useState<LinkedIdentity | null>(null);
  if (links.length === 0) return <EmptyState headingLevel={3} heading={t("client:settings.linked_accounts.empty")} />;
  return (
    <>
      <Stack as="ul" ref={listRef} divided>
        {links.map((link) => (
          <Box as="li" key={link.id} data-row-id={link.id} container paddingY="3">
            <LinkedAccountRow
              link={link}
              unlink={unlink}
              onUnlink={() => {
                unlink.reset();
                setConfirming(link);
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

function AddLinkedAccount() {
  const t = useT();
  const link = useLinkAccount();
  if (link.providers.length === 0) return <p>{t("client:settings.linked_accounts.add.none")}</p>;
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
