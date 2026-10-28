// SPDX-License-Identifier: AGPL-3.0-only
// The aliases page below its heading (FR-009, ADR-0089; web-client/screens/settings-aliases.md): the list-and-add part
// (`ListSection`) with the aliases, each with the button that removes it, and the button that opens the form that adds
// one by handle or address. At the most aliases, a note stands in place of the button. The form follows the browser's
// constraint validation; an alias is removed as easily as it is added, so neither asks first. What adding did, and a
// failed removal, show above the list; removing is announced and moves focus to the next row
// (web-client/16-look-states-and-wording.md G24).
import { Button, Cluster, FormActionBar, Hint, Link, ListSection, Stack, Text, TextField } from "@my/ui";
import { useRef } from "react";
import { useIntl } from "react-intl";
import { useT } from "../../../lib/i18n";
import { accountMovePath } from "../../../lib/paths";
import { useFocusAfterRemoval } from "../../../lib/useFocusAfterRemoval";
import { type AccountAlias, aliasInputLimit, maxAliases } from "../api/aliases";
import {
  type AddAliasForm,
  aliasName,
  type RemoveAlias,
  useAddAliasForm,
  useAliasList,
  useRemoveAlias,
} from "../hooks/useAliases";
import { isolate } from "../../profile";

export function AliasSettings() {
  const t = useT();
  const aliases = useAliasList();
  const form = useAddAliasForm();
  const remove = useRemoveAlias();
  const listRef = useRef<HTMLUListElement>(null);
  const removed = useFocusAfterRemoval(
    listRef,
    aliases.map((alias) => alias.uri),
  );
  let result: { tone: "success" | "danger"; message: string } | undefined;
  if (remove.error !== undefined) result = { tone: "danger", message: remove.error };
  else if (form.status !== undefined) result = { tone: "success", message: form.status };
  return (
    <ListSection
      title={t("client:account_move.aliases.list.title")}
      description={
        <>
          {t("client:account_move.aliases.description")}{" "}
          <Link href={accountMovePath}>{t("client:account_move.aliases.move_link")}</Link>
        </>
      }
      items={aliases}
      itemKey={(alias) => alias.uri}
      renderItem={(alias) => (
        <AliasRow
          alias={alias}
          remove={remove}
          onRemoved={() => {
            removed(alias.uri);
          }}
        />
      )}
      listRef={listRef}
      countLabel={t("client:account_move.aliases.count", { count: aliases.length })}
      empty={{
        heading: t("client:account_move.aliases.empty"),
        description: t("client:account_move.aliases.empty.description"),
      }}
      add={
        form.isFull
          ? { unavailable: t("client:account_move.aliases.limit", { max: String(maxAliases) }) }
          : {
              label: t("client:account_move.aliases.add.title"),
              panel: (close) => <AddAliasFormFields form={form} close={close} />,
            }
      }
      result={result}
    />
  );
}

/** The form that adds an alias, in the panel of the list. */
function AddAliasFormFields({ form, close }: { form: AddAliasForm; close: () => void }) {
  const t = useT();
  return (
    <Stack
      as="form"
      onSubmit={(event) => {
        form.submit(event, close);
      }}
      gap="4"
    >
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
      <FormActionBar result={form.error === undefined ? undefined : { tone: "danger", message: form.error }}>
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {t("client:account_move.aliases.add.submit")}
        </Button>
        <Button onPress={close}>{t("client:settings.list.cancel")}</Button>
      </FormActionBar>
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
          {t("client:account_move.aliases.item.since", {
            date: intl.formatDate(alias.created_at, { dateStyle: "medium", timeStyle: "short" }),
          })}
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
