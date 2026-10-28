// SPDX-License-Identifier: AGPL-3.0-only
// The user's aliases (FR-009, ADR-0089): the list, adding an alias with a form, and removing one from its row. Both
// changes answer the whole list, which replaces the cached one. Removing is undone as easily as it is done, so it
// asks nothing; it is announced, and focus moves to the next row.
import { useAnnouncer } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { isolate } from "../../profile";
import {
  type AccountAlias,
  addAliasMutation,
  aliasesQueryOptions,
  cacheAliases,
  maxAliases,
  removeAliasMutation,
} from "../api/aliases";

/** What an alias is called in a sentence: its handle, or its address when the instance does not know the handle. */
export function aliasName(alias: AccountAlias): string {
  return alias.acct === undefined ? alias.uri : `@${alias.acct}`;
}

/** The aliases, oldest first, which the route has loaded. */
export function useAliasList(): readonly AccountAlias[] {
  const api = useApi();
  return useSuspenseQuery(aliasesQueryOptions(api)).data.aliases;
}

/** Removing an alias from its row. */
export type RemoveAlias = {
  /** Removes `alias`; `onDone` is called once it is off the list. */
  run: (alias: AccountAlias, onDone: () => void) => void;
  /** The `uri` of the alias being removed. */
  pendingUri: string | undefined;
};

export function useRemoveAlias(): RemoveAlias {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const announce = useAnnouncer();
  const remove = useMutation(removeAliasMutation(api));
  return {
    pendingUri: remove.isPending ? remove.variables : undefined,
    run: (alias, onDone) => {
      remove.mutate(alias.uri, {
        onSuccess: (list) => {
          cacheAliases(api, queryClient, list);
          onDone();
          announce(t("client:account_move.aliases.removed", { name: isolate(aliasName(alias)) }));
        },
        onError: (error) => {
          announce(t("client:account_move.aliases.remove_failed", { reason: errorMessage(error) }), "assertive");
        },
      });
    },
  };
}

/** What the form that adds an alias needs. */
export type AddAliasForm = {
  /** Whether the user has the most aliases, so that the form gives way to a note. */
  isFull: boolean;
  acct: string;
  setAcct: (acct: string) => void;
  /** Adds the alias once the browser has checked the field. */
  submit: (event: SyntheticEvent<HTMLFormElement>) => void;
  isPending: boolean;
  /** Why adding failed, localized, for an alert next to the form; the field keeps what was typed. */
  error: string | undefined;
  /** What the last addition did, for the form's StatusMessage; undefined before the first one. */
  status: string | undefined;
};

export function useAddAliasForm(): AddAliasForm {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const aliases = useAliasList();
  const [acct, setAcct] = useState("");
  const [status, setStatus] = useState<string | undefined>(undefined);
  const add = useMutation({
    ...addAliasMutation(api),
    onSuccess: (list) => {
      cacheAliases(api, queryClient, list);
      // The alias the user added is the newest, and the list is oldest first.
      const added = list.aliases.at(-1);
      setAcct("");
      setStatus(
        added === undefined ? undefined : t("client:account_move.aliases.added", { name: isolate(aliasName(added)) }),
      );
    },
  });
  return {
    isFull: aliases.length >= maxAliases,
    acct,
    setAcct: (value) => {
      setAcct(value);
      if (add.isError) add.reset();
    },
    submit: (event) => {
      event.preventDefault();
      const value = acct.trim();
      if (add.isPending || value === "") return;
      add.reset();
      setStatus(undefined);
      add.mutate(value);
    },
    isPending: add.isPending,
    error: add.isError ? t("client:account_move.aliases.add_failed", { reason: errorMessage(add.error) }) : undefined,
    status,
  };
}
