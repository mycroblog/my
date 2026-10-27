// SPDX-License-Identifier: AGPL-3.0-only
// The account page below its heading (FR-008): the archives of the user's data, each with its state, when it was
// asked for, until when it can be downloaded and its size, and deleting the account, which asks for the password,
// or for a passkey when the user has no password, in an alert dialog (ConfirmDialog: the deletion cannot be undone). Dates are in the user's locale and time zone.
import {
  Box,
  Button,
  ConfirmDialog,
  DescriptionItem,
  DescriptionList,
  EmptyState,
  Heading,
  InlineError,
  Stack,
  StatusMessage,
  TextField,
} from "@my/ui";
import { type ReactNode, useId } from "react";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import type { DataExport, DataExportState } from "../api/account";
import { type AccountDeletion, type DataExports, useAccountDeletion, useDataExports } from "../hooks/useOwnAccount";
import { ExternalConfirmationChoice } from "./ExternalConfirmationChoice";

const stateLabels: Readonly<Record<DataExportState, ClientMessageKey>> = {
  pending: "client:settings.account.export.state.pending",
  ready: "client:settings.account.export.state.ready",
  failed: "client:settings.account.export.state.failed",
};

/** The format of every date and time on the page. */
const dateTimeFormat = { dateStyle: "medium", timeStyle: "short" } as const;

export function OwnAccount() {
  const t = useT();
  const exports = useDataExports();
  const deletion = useAccountDeletion();
  return (
    <Stack gap="8">
      <Section title={t("client:settings.account.export.title")}>
        <ExportsView exports={exports} />
      </Section>
      <Section title={t("client:settings.account.delete.title")}>
        <DeletionView deletion={deletion} />
      </Section>
    </Stack>
  );
}

/** The archives of the user's data alone, for the screen of a closed server, where nothing else of the account page is open. */
export function DataExportsSection() {
  const t = useT();
  const exports = useDataExports();
  return (
    <Section title={t("client:settings.account.export.title")}>
      <ExportsView exports={exports} />
    </Section>
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

function ExportsView({ exports }: { exports: DataExports }) {
  const t = useT();
  return (
    <Stack gap="4">
      <p>{t("client:settings.account.export.description")}</p>
      <StatusMessage tone="success">{exports.status}</StatusMessage>
      {exports.error !== undefined && (
        <InlineError alert size="inherit">
          {exports.error}
        </InlineError>
      )}
      <div>
        <Button variant="primary" onPress={exports.request} isPending={exports.isRequesting} isDisabled={!exports.canRequest}>
          {t("client:settings.account.export.request")}
        </Button>
      </div>
      {exports.items.length === 0 ? (
        <EmptyState headingLevel={3} heading={t("client:settings.account.export.empty")} />
      ) : (
        <Stack as="ul" divided>
          {exports.items.map((item) => (
            <Box as="li" key={item.id} container paddingY="3">
              <ExportRow item={item} exports={exports} />
            </Box>
          ))}
        </Stack>
      )}
    </Stack>
  );
}

function ExportRow({ item, exports }: { item: DataExport; exports: DataExports }) {
  const t = useT();
  const intl = useIntl();
  const requested = intl.formatDate(item.requested_at, dateTimeFormat);
  return (
    <Stack as="article" aria-labelledby={`export-${item.id}`} gap="3">
      <Heading level={3} id={`export-${item.id}`}>
        {t("client:settings.account.export.item", { date: requested })}
      </Heading>
      <DescriptionList columns={3}>
        <DescriptionItem term={t("client:settings.account.export.state")}>{t(stateLabels[item.state])}</DescriptionItem>
        {item.state === "ready" && item.expires_at !== undefined && (
          <DescriptionItem term={t("client:settings.account.export.expires_at")}>
            <time dateTime={item.expires_at}>{intl.formatDate(item.expires_at, dateTimeFormat)}</time>
          </DescriptionItem>
        )}
        {item.file_size !== undefined && (
          <DescriptionItem term={t("client:settings.account.export.size")}>
            {intl.formatNumber(item.file_size / 1_000_000, { style: "unit", unit: "megabyte", maximumFractionDigits: 1 })}
          </DescriptionItem>
        )}
      </DescriptionList>
      {item.state === "ready" && (
        <div>
          <Button
            size="sm"
            aria-label={t("client:settings.account.export.download_of", { date: requested })}
            isPending={exports.downloadingId === item.id}
            onPress={() => {
              exports.download(item);
            }}
          >
            {t("client:settings.account.export.download")}
          </Button>
        </div>
      )}
    </Stack>
  );
}

function DeletionView({ deletion }: { deletion: AccountDeletion }) {
  const t = useT();
  // Without a password, the dialog has no field (a passkey confirms the deletion), so its children are undefined.
  const passwordField = deletion.hasPassword ? (
    <TextField
      label={t("client:settings.account.delete.password")}
      type="password"
      autoComplete="current-password"
      value={deletion.password}
      onChange={deletion.changePassword}
      isRequired
      errorMessage={t("client:settings.account.delete.password.required")}
    />
  ) : undefined;
  return (
    <Stack align="start" gap="4">
      <p>{t("client:settings.account.delete.description")}</p>
      {/* A user with neither a password nor a passkey signs in again at a linked provider first. */}
      {deletion.external.isPreferred && <ExternalConfirmationChoice confirmation={deletion.external} />}
      {(!deletion.external.isPreferred || deletion.external.confirmed !== undefined) && (
        <Button variant="danger" onPress={deletion.open}>
          {t("client:settings.account.delete.open")}
        </Button>
      )}
      <ConfirmDialog
        isOpen={deletion.isOpen}
        onOpenChange={(isOpen) => {
          if (!isOpen) deletion.cancel();
        }}
        title={t("client:settings.account.delete.confirm.title")}
        description={t(
          deletion.hasPassword
            ? "client:settings.account.delete.confirm.description"
            : deletion.external.isPreferred
              ? "client:settings.account.delete.confirm.description_external"
              : "client:settings.account.delete.confirm.description_passkey",
        )}
        confirmLabel={t("client:settings.account.delete.confirm")}
        cancelLabel={t("client:settings.account.delete.cancel")}
        tone="danger"
        onConfirm={() => {
          deletion.submit();
        }}
        isPending={deletion.isPending}
        {...(deletion.error === undefined ? {} : { errorMessage: deletion.error })}
      >
        {passwordField}
      </ConfirmDialog>
    </Stack>
  );
}
