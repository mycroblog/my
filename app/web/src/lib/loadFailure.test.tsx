// SPDX-License-Identifier: AGPL-3.0-only
import { QueryClient, QueryClientProvider, queryOptions, useQuery } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, test } from "vitest";
import { useLoadFailure } from "./loadFailure";

/** A query per key whose fetches wait for `settle`, with the failure the hook keeps for it. */
function setup() {
  const pending = new Map<string, (result: Error | string) => void>();
  const settle = (id: string, result: Error | string) => {
    pending.get(id)?.(result);
  };
  const load = (id: string) =>
    new Promise<string>((resolve, reject) => {
      pending.set(id, (result) => {
        if (typeof result === "string") resolve(result);
        else reject(result);
      });
    });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const view = renderHook(
    ({ id }: { id: string }) => {
      const query = useQuery(queryOptions({ queryKey: [id], retryOnMount: false, queryFn: () => load(id) }));
      return { query, failure: useLoadFailure(query) };
    },
    { wrapper, initialProps: { id: "a" } },
  );
  return { pending, settle, view };
}

/** Fails the first fetch of `id` with `error`. */
async function failFirst({ pending, settle, view }: ReturnType<typeof setup>, id: string, error: Error) {
  await waitFor(() => {
    expect(pending.has(id)).toBe(true);
  });
  act(() => {
    settle(id, error);
  });
  await waitFor(() => {
    expect(view.result.current.failure.error).toBe(error);
  });
}

describe("useLoadFailure", () => {
  test("has no failure during a first load", () => {
    const { view } = setup();
    expect(view.result.current.query.isFetching).toBe(true);
    expect(view.result.current.failure).toEqual({ error: null, isRetrying: false });
  });

  test("keeps the error while the query loads again, where the query itself has none", async () => {
    const context = setup();
    const { view, settle } = context;
    const first = new Error("first");
    await failFirst(context, "a", first);
    expect(view.result.current.failure).toEqual({ error: first, isRetrying: false });

    act(() => {
      void view.result.current.query.refetch();
    });
    await waitFor(() => {
      expect(view.result.current.query.status).toBe("pending");
    });
    expect(view.result.current.query.error).toBeNull();
    expect(view.result.current.failure).toEqual({ error: first, isRetrying: true });

    const second = new Error("second");
    act(() => {
      settle("a", second);
    });
    await waitFor(() => {
      expect(view.result.current.failure).toEqual({ error: second, isRetrying: false });
    });

    act(() => {
      void view.result.current.query.refetch();
    });
    await waitFor(() => {
      expect(view.result.current.failure).toEqual({ error: second, isRetrying: true });
    });
    act(() => {
      settle("a", "loaded");
    });
    await waitFor(() => {
      expect(view.result.current.query.data).toBe("loaded");
    });
    expect(view.result.current.failure).toEqual({ error: null, isRetrying: false });
  });

  test("does not show one query's error for another after the key changes", async () => {
    const context = setup();
    const { view } = context;
    await failFirst(context, "a", new Error("of a"));

    view.rerender({ id: "b" });
    expect(view.result.current.query.isFetching).toBe(true);
    expect(view.result.current.failure).toEqual({ error: null, isRetrying: false });
  });

  test("does not show another key's error for a query retrying after its own failure", async () => {
    const context = setup();
    const { view } = context;
    await failFirst(context, "a", new Error("of a"));
    view.rerender({ id: "b" });
    await failFirst(context, "b", new Error("of b"));

    view.rerender({ id: "a" });
    act(() => {
      void view.result.current.query.refetch();
    });
    await waitFor(() => {
      expect(view.result.current.query.status).toBe("pending");
    });
    expect(view.result.current.failure).toEqual({ error: null, isRetrying: false });
  });
});
