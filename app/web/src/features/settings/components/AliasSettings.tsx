// SPDX-License-Identifier: AGPL-3.0-only
// The aliases page below its heading (FR-009, ADR-0089): what an alias is, the form that adds one by handle or address,
// and the aliases with the button that removes each. At the most aliases, a note replaces the form. The form follows the
// browser's constraint validation; an alias is removed as easily as it is added, so neither asks first. What adding did
// stays on the page in a StatusMessage (role="status") under the form; removing is announced and moves focus to the next row.
import { Box, Button, Cluster, EmptyState, Heading, Hint, InlineError, Link, Stack, StatusMessage, Text, TextField } from "@my/ui";
import { useId, useRef } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { accountMovePath } from "../../../lib/paths";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { type AccountAlias, aliasInputLimit, maxAliases } from "../api/aliases";
import { aliasName, type RemoveAlias, useAddAliasForm, useAliasList, useRemoveAlias } from "../hooks/useAliases";
import { isolate } from "../../profile";

export function AliasSettings() {
  const t = useT();
  const listHeadingId = useId();
  const formHeadingId = useId();
  return (
    <Stack gap="8">
      <p>{t("client:account_move.aliases.description")}</p>
      <Stack as="section" aria-labelledby={listHeadingId} gap="3">
        <Heading level={2} id={listHeadingId}>
          {t("client:account_move.aliases.list.title")}
        </Heading>
        <AliasList />
      </Stack>
      <Stack as="section" aria-labelledby={formHeadingId} gap="3">
        <Heading level={2} id={formHeadingId}>
          {t("client:account_move.aliases.add.title")}
        </Heading>
        <AddAliasForm />
      </Stack>
      <p>
        <Link href={accountMovePath} variant="standalone">
          {t("client:account_move.aliases.move_link")}
        </Link>
      </p>
    </Stack>
  );
}

function AddAliasForm() {
  const t = useT();
  const form = useAddAliasForm();
  if (form.isFull) return <p>{t("client:account_move.aliases.limit", { max: String(maxAliases) })}</p>;
  return (
    <Stack as="form" onSubmit={form.submit} gap="4">
      <TextField
        label={t("client:account_move.aliases.add.field")}
        description={t("client:account_move.aliases.add.description", { max: String(maxAliases) })}
        value={form.acct}
        onChange={form.setAcct}
        isRequired
        maxLength={aliasInputLimit}
        autoComplete="off"
        autoCorrect="off"
        spellCheck="false"
        inputMode="url"
        errorMessage={t("client:account_move.aliases.add.required")}
      />
      {form.error !== undefined && (
        <InlineError alert size="inherit">
          {form.error}
        </InlineError>
      )}
      <div>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:account_move.aliases.add.submit")}
        </Button>
      </div>
      <StatusMessage>{form.status}</StatusMessage>
    </Stack>
  );
}

function AliasList() {
  const t = useT();
  const aliases = useAliasList();
  const remove = useRemoveAlias();
  const listRef = useRef<HTMLUListElement>(null);
  const removed = useFocusAfterRemoval(
    listRef,
    aliases.map((alias) => alias.uri),
  );
  if (aliases.length === 0) return <EmptyState heading={t("client:account_move.aliases.empty")} />;
  return (
    <Stack as="ul" ref={listRef} divided>
      {aliases.map((alias) => (
        <Box as="li" key={alias.uri} data-row-id={alias.uri} container paddingY="3">
          <AliasRow
            alias={alias}
            remove={remove}
            onRemoved={() => {
              removed(alias.uri);
            }}
          />
        </Box>
      ))}
    </Stack>
  );
}

type AliasRowProps = { alias: AccountAlias; remove: RemoveAlias; onRemoved: () => void };

/** An alias: its handle (or its address when the handle is not known) and when it was added, with the button that removes it. */
function AliasRow({ alias, remove, onRemoved }: AliasRowProps) {
  const t = useT();
  const intl = useIntl();
  const name = aliasName(alias);
  return (
    <Stack gap="3" rowAt="md">
      <Stack as="p" shrink grow>
        {/* A handle and an address read left to right in every locale. */}
        <Text dir="ltr" wrap="anywhere" align="start" weight="semibold">
          {name}
        </Text>
        {alias.acct !== undefined && (
          <Text dir="ltr" wrap="anywhere" align="start" size="sm" tone="muted">
            {alias.uri}
          </Text>
        )}
        <Hint as="span">
          {t("client:account_move.aliases.item.since", { date: intl.formatDate(alias.created_at, { dateStyle: "medium", timeStyle: "short" }) })}
        </Hint>
      </Stack>
      <Cluster gap="2">
        <Button
          size="sm"
          isPending={remove.pendingUri === alias.uri}
          aria-label={t("client:account_move.aliases.remove_of", { name: isolate(name) })}
          onPress={() => {
            remove.run(alias, onRemoved);
          }}
        >
          {t("client:account_move.aliases.remove")}
        </Button>
      </Cluster>
    </Stack>
  );
}
