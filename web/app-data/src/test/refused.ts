// SPDX-License-Identifier: AGPL-3.0-only
// What a refused write rejects with, for the tests that pin which problem a screen can tell apart.
import { ProblemError } from "@mycroblog/mykit";

/** The problem `write` was rejected with; fails the test when it resolved or rejected with anything else. */
export async function refusedWith(write: Promise<unknown>): Promise<ProblemError> {
  const outcome = await write.then(
    () => undefined,
    (error: unknown) => error,
  );
  if (!(outcome instanceof ProblemError)) throw new Error("the write was not refused with a problem");
  return outcome;
}
