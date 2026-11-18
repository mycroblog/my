// SPDX-License-Identifier: AGPL-3.0-only
// Where the reads of a screen are, as one status: a settings screen shows its sections once every read it shows has
// answered, and until then one notice, that they load or that one could not be read, with the way to try again.

/** What a read says of where it is (a query's result). */
type Read = {
  readonly data: unknown;
  readonly status: "pending" | "error" | "success";
  readonly isFetching: boolean;
  readonly refetch: () => unknown;
};

/** Where the reads of a screen are, and the way to read again those that failed. */
export type ScreenRead = { status: "loading" | "failed" | "read"; retry: () => void };

/**
 * Where `reads` are, as one: `read` once the server answered them all, `failed` when it did not answer one, with
 * `retry`, which reads again those that failed, and `loading` until then. What was read stays read: a later read of
 * it that fails leaves the screen what it had.
 */
export function readOf(reads: readonly Read[]): ScreenRead {
  return {
    status: reads.every((read) => read.data !== undefined)
      ? "read"
      : reads.some((read) => read.status === "error" && !read.isFetching)
        ? "failed"
        : "loading",
    retry: () => {
      for (const read of reads) if (read.status === "error") void read.refetch();
    },
  };
}
