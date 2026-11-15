// SPDX-License-Identifier: AGPL-3.0-only
// The reader's lists (FR-211, ADR-0013 §10–§15): the lists they made, oldest first, and one of them. Making and
// changing a list is list management's.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";

/** A list of the reader's: its name and how many accounts are in it. */
export type List = components["schemas"]["List"];

/** The reader's lists, oldest first. */
export function listsQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/lists");
}

/** The reader's list of `id`. */
export function listQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/lists/{id}", { params: { path: { id } } });
}

/** What a screen of the reader's lists shows: the lists, whether they are being read or failed, and trying again. */
export type Lists = {
  readonly lists: readonly List[];
  readonly status: "pending" | "error" | "success";
  readonly refetch: () => void;
};

/** The reader's lists, empty until the server answered. */
export function useLists(): Lists {
  const query = useQuery(listsQuery(useAppData()));
  return {
    lists: query.data?.items ?? [],
    status: query.status,
    refetch: () => {
      void query.refetch();
    },
  };
}

/** What a list's screen shows of the list: the list once read, why it could not be, and reading it again. */
export type OneList = {
  /** The list, undefined until the server answered with it. */
  readonly list: List | undefined;
  /** `pending` until the server answered, `gone` when it has no such list (deleted), `error` when reading it failed. */
  readonly status: "pending" | "gone" | "error" | "success";
  /** Reads the list again: the reader's try again after it failed. */
  readonly refetch: () => void;
};

/** The reader's list of `id`. */
export function useList(id: string): OneList {
  const query = useQuery(listQuery(useAppData(), id));
  const { error } = query;
  return {
    list: query.data,
    status:
      query.status === "error"
        ? error instanceof ProblemError && error.status === 404
          ? "gone"
          : "error"
        : query.status,
    refetch: () => {
      void query.refetch();
    },
  };
}
