// SPDX-License-Identifier: AGPL-3.0-only
// The user's lists on their page (web-client/screens/lists.md, ADR-0122): the lists, the form that creates one, renaming
// one in its row, and deleting one after asking. Every change takes effect at once; deleting asks first, since the
// members' choice is lost. The hooks return no JSX.
import { useNotice } from "@my/ui";
import { useMutation, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  cacheDeletedList,
  cacheSavedList,
  createListMutation,
  deleteListMutation,
  type List,
  listFailure,
  listNameProblem,
  listQueryOptions,
  listsQueryOptions,
  maxLists,
  renameListMutation,
} from "../api/lists";
import { isolate, useLimits } from "./names";

/** The user's lists, oldest first, which the route has loaded. */
export function useListCollection(): readonly List[] {
  const api = useApi();
  return useSuspenseQuery(listsQueryOptions(api)).data.items;
}

/** One list of the user, which the route has loaded (a list that is not the user's fails the route with a 404). */
export function useList(id: string): List {
  const api = useApi();
  return useSuspenseQuery(listQueryOptions(api, id)).data;
}

/** A form with one name field: creating a list, or renaming one. */
export type ListNameForm = {
  name: string;
  changeName: (name: string) => void;
  /** Sends the name once the browser has checked the field; `onSaved` is called once the list shows it. */
  submit: (event: SyntheticEvent<HTMLFormElement>, onSaved: () => void) => void;
  isPending: boolean;
  /** Why the name cannot be saved, localized, for the form's action bar; undefined while nothing failed. */
  error: string | undefined;
};

/** The localized reason of a failed save of a name. */
function useNameError() {
  const t = useT();
  const limits = useLimits();
  return (error: unknown): string => {
    if (listFailure(error) === "too-many-lists") return t("client:lists.limit", { max: limits.lists });
    return t("client:lists.save_failed", { reason: errorMessage(error) });
  };
}

/** The reason a typed name is refused before it is sent, or undefined when it may be sent. */
function useNameCheck() {
  const t = useT();
  const limits = useLimits();
  return (name: string): string | undefined => {
    switch (listNameProblem(name)) {
      case "empty":
        return t("client:lists.name.required");
      case "too-long":
        return t("client:lists.name.too_long", { max: limits.name });
      case "invalid":
        return t("client:lists.name.invalid");
      case undefined:
        return undefined;
    }
  };
}

/** Creating a list: the form in the panel of the list section, and what it did last, shown above the rows. */
export type CreateListForm = ListNameForm & {
  /** Whether the user has the most lists, so that the button gives way to a note. */
  isFull: boolean;
  /** What the last creation did, shown above the list; undefined before the first one. */
  status: string | undefined;
};

export function useCreateListForm(): CreateListForm {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const lists = useListCollection();
  const check = useNameCheck();
  const failed = useNameError();
  const [name, setName] = useState("");
  const [refused, setRefused] = useState<string | undefined>(undefined);
  const [status, setStatus] = useState<string | undefined>(undefined);
  // The cache is updated before the mutation settles (S1).
  const create = useMutation({
    ...createListMutation(api),
    onSuccess: (list) => cacheSavedList(api, queryClient, list),
  });
  return {
    isFull: lists.length >= maxLists,
    name,
    changeName: (value) => {
      setName(value);
      setRefused(undefined);
      if (create.isError) create.reset();
    },
    submit: (event, onSaved) => {
      event.preventDefault();
      if (create.isPending) return;
      const value = name.trim();
      const problem = check(value);
      setRefused(problem);
      if (problem !== undefined) return;
      create.reset();
      setStatus(undefined);
      create.mutate(value, {
        onSuccess: (list) => {
          setName("");
          setStatus(t("client:lists.created", { name: isolate(list.name) }));
          onSaved();
        },
      });
    },
    isPending: create.isPending,
    error: refused ?? (create.isError ? failed(create.error) : undefined),
    status,
  };
}

/** Renaming `list`: the form that opens in its row. */
export function useRenameListForm(list: List): ListNameForm {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const check = useNameCheck();
  const failed = useNameError();
  const [name, setName] = useState(list.name);
  const [refused, setRefused] = useState<string | undefined>(undefined);
  const rename = useMutation({
    ...renameListMutation(api),
    onSuccess: (saved) => cacheSavedList(api, queryClient, saved),
  });
  return {
    name,
    changeName: (value) => {
      setName(value);
      setRefused(undefined);
      if (rename.isError) rename.reset();
    },
    submit: (event, onSaved) => {
      event.preventDefault();
      if (rename.isPending) return;
      const value = name.trim();
      const problem = check(value);
      setRefused(problem);
      if (problem !== undefined) return;
      rename.mutate(
        { id: list.id, name: value },
        {
          onSuccess: (saved) => {
            onSaved();
            notice.inPlace(t("client:lists.renamed", { name: isolate(saved.name) }));
          },
        },
      );
    },
    isPending: rename.isPending,
    error: refused ?? (rename.isError ? failed(rename.error) : undefined),
  };
}

/** Deleting a list, after the confirmation. */
export type DeleteList = {
  /** Deletes `list`; `onDone(true)` is called once it is off the list, `onDone(false)` when it failed (a toast says why). */
  run: (list: List, onDone: (deleted: boolean) => void) => void;
  /** The ID of the list being deleted. */
  pendingId: string | undefined;
};

export function useDeleteList(): DeleteList {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const notice = useNotice();
  const remove = useMutation({
    ...deleteListMutation(api),
    onSuccess: (list) => cacheDeletedList(api, queryClient, list),
  });
  return {
    pendingId: remove.isPending ? remove.variables.id : undefined,
    run: (list, onDone) => {
      remove.mutate(list, {
        onSuccess: () => {
          onDone(true);
          notice.inPlace(t("client:lists.deleted", { name: isolate(list.name) }));
        },
        onError: (error) => {
          onDone(false);
          notice.toast({
            tone: "danger",
            message: t("client:lists.delete_failed", { name: isolate(list.name), reason: errorMessage(error) }),
          });
        },
      });
    },
  };
}
