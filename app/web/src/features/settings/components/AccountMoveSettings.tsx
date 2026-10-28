// SPDX-License-Identifier: AGPL-3.0-only
// The page that moves the account below its heading (FR-009, ADR-0089). An account that has not moved gets what a move
// does, what to do first (list the account as an alias of the new one), and the form that names the new account; it
// opens an alert dialog (ConfirmDialog; the move cannot be undone, only cancelled and not for the followers who moved)
// that says what moves (the followers) and what stays (the posts, follows, blocks, mutes, and bookmarks, which the CSV
// export takes along), and asks for the password, or a passkey for a user without one. An account that has moved gets
// where it moved to and the dialog that cancels the move. What a move or its cancellation did stays on the page in one
// StatusMessage (role="status") at its top, which stays in place when the page turns from one kind into the other.
import { Box, Button, ConfirmDialog, InlineError, Link, Section, Stack, StatusMessage, Text, TextField } from "@my/ui";
import { useState } from "react";
import { useIntl } from "react-intl";
import { handleOf } from "../../../lib/handle";
import { useT } from "../../../lib/i18n";
import { importExportPath, profilePath } from "../../../lib/paths";
import { isolate } from "../../profile";
import { targetInputLimit } from "../api/move";
import {
  type AccountMove,
  type MoveCancellation,
  useAccountMove,
  useMoveCancellation,
  useMovedTo,
  useOwnAddress,
} from "../hooks/useAccountMove";
import { ExternalConfirmationChoice } from "./ExternalConfirmationChoice";

export function AccountMoveSettings() {
  const [status, setStatus] = useState<string | undefined>(undefined);
  const movedTo = useMovedTo();
  return (
    <Stack container gap="8">
      <StatusMessage>{status}</StatusMessage>
      {movedTo === undefined ? (
        <MoveAccount onStatus={setStatus} />
      ) : (
        <MovedAccount target={movedTo} onStatus={setStatus} />
      )}
    </Stack>
  );
}

/** The account has not moved: what a move does, what comes first, and the form that names the new account. */
function MoveAccount({ onStatus }: { onStatus: (status: string | undefined) => void }) {
  const t = useT();
  const intl = useIntl();
  const move = useAccountMove(onStatus);
  const url = useOwnAddress();
  return (
    <>
      <p>{t("client:account_move.move.description")}</p>
      <Section title={t("client:account_move.move.form.title")}>
        <p>
          {/* The address is no running text: it reads left to right whatever the sentence around it (Y10). */}
          {intl.formatMessage(
            { id: "client:account_move.move.prerequisite" },
            {
              url: (
                <bdi key="url" dir="ltr">
                  {url}
                </bdi>
              ),
            },
          )}
        </p>
        {/* A user with neither a password nor a passkey signs in again at a linked provider first. */}
        {move.external.isPreferred && <ExternalConfirmationChoice confirmation={move.external} />}
        {(!move.external.isPreferred || move.external.confirmed !== undefined) && (
          <Stack as="form" onSubmit={move.submit} align="start" gap="4">
            <Box width="sm">
              <TextField
                label={t("client:account_move.move.target.field")}
                description={t("client:account_move.move.target.description")}
                value={move.input}
                onChange={move.setInput}
                isRequired
                maxLength={targetInputLimit}
                autoComplete="off"
                autoCorrect="off"
                spellCheck="false"
                inputMode="url"
                errorMessage={t("client:account_move.move.target.required")}
              />
            </Box>
            {move.lookupError !== undefined && (
              <InlineError alert size="inherit">
                {move.lookupError}
              </InlineError>
            )}
            <Button type="submit" variant="primary" isPending={move.isLookingUp}>
              {t("client:account_move.move.target.submit")}
            </Button>
          </Stack>
        )}
      </Section>
      <MoveDialog move={move} />
    </>
  );
}

/** The dialog that confirms the move, for the account that `move.target` is. */
function MoveDialog({ move }: { move: AccountMove }) {
  const t = useT();
  const values = { handle: move.targetHandle };
  const passwordField = move.hasPassword ? (
    <TextField
      label={t("client:account_move.move.password.field")}
      type="password"
      autoComplete="current-password"
      value={move.password}
      onChange={move.changePassword}
      isRequired
      errorMessage={t("client:account_move.move.password.required")}
    />
  ) : undefined;
  return (
    <ConfirmDialog
      isOpen={move.isOpen}
      onOpenChange={(isOpen) => {
        if (!isOpen) move.cancel();
      }}
      title={t("client:account_move.move.confirm.title", values)}
      description={t(
        move.hasPassword
          ? "client:account_move.move.confirm.description"
          : move.external.isPreferred
            ? "client:account_move.move.confirm.description_external"
            : "client:account_move.move.confirm.description_passkey",
        values,
      )}
      confirmLabel={t("client:account_move.move.confirm.submit")}
      cancelLabel={t("client:account_move.move.confirm.cancel")}
      tone="danger"
      onConfirm={() => {
        move.confirm();
      }}
      isPending={move.isPending}
      {...(move.error === undefined ? {} : { errorMessage: move.error })}
    >
      {/* The password field comes first so that it takes the focus; a user without a password gets the link. */}
      {passwordField}
      <Stack gap="2">
        <Text as="p" weight="semibold">
          {t("client:account_move.move.confirm.moves")}
        </Text>
        <Stack as="ul" gap="1">
          <li>{t("client:account_move.move.confirm.moves.followers")}</li>
        </Stack>
      </Stack>
      <Stack gap="2">
        <Text as="p" weight="semibold">
          {t("client:account_move.move.confirm.stays")}
        </Text>
        <Stack as="ul" gap="1">
          <li>{t("client:account_move.move.confirm.stays.items")}</li>
        </Stack>
        <p>
          <Link href={importExportPath} variant="standalone">
            {t("client:account_move.move.confirm.export")}
          </Link>
        </p>
      </Stack>
    </ConfirmDialog>
  );
}

/** The account has moved: where to, what it still does, and the dialog that cancels the move. */
function MovedAccount({
  target,
  onStatus,
}: {
  target: { acct: string };
  onStatus: (status: string | undefined) => void;
}) {
  const t = useT();
  const cancellation = useMoveCancellation(onStatus);
  const handle = isolate(handleOf(target.acct));
  return (
    <Section title={t("client:account_move.moved.title")}>
      <p>{t("client:account_move.moved.description", { handle })}</p>
      <p>
        <Link href={profilePath(target.acct)} variant="standalone">
          {t("client:account_move.moved.target", { handle })}
        </Link>
      </p>
      <CancelMove cancellation={cancellation} handle={handle} />
    </Section>
  );
}

function CancelMove({ cancellation, handle }: { cancellation: MoveCancellation; handle: string }) {
  const t = useT();
  return (
    <Stack align="start" gap="4">
      <Button variant="primary" onPress={cancellation.open}>
        {t("client:account_move.cancel.open")}
      </Button>
      <ConfirmDialog
        isOpen={cancellation.isOpen}
        onOpenChange={(isOpen) => {
          if (!isOpen) cancellation.cancel();
        }}
        title={t("client:account_move.cancel.title")}
        description={t("client:account_move.cancel.description", { handle })}
        confirmLabel={t("client:account_move.cancel.confirm")}
        cancelLabel={t("client:account_move.cancel.close")}
        tone="primary"
        onConfirm={() => {
          cancellation.confirm();
        }}
        isPending={cancellation.isPending}
        {...(cancellation.error === undefined ? {} : { errorMessage: cancellation.error })}
      />
    </Stack>
  );
}
