// SPDX-License-Identifier: AGPL-3.0-only
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import type { Status } from "../../status";
import { type AutoDeletePolicy, autoDeletePreviewQueryOptions } from "../api/autoDelete";

/** The estimate of what the policy being edited would delete now. */
export type AutoDeletePreviewState =
  | { status: "loading" }
  | { status: "error"; message: string }
  /** `pending` while the count of a newer policy is on its way and the previous one still shows. */
  | { status: "done"; count: number; more: boolean; examples: readonly Status[]; pending: boolean };

/**
 * Counts what `policy` would delete now, as the form changes (FR-118): the server counts it without saving it. The
 * count does not depend on whether the policy is turned on, so turning it on or off asks nothing again; while the next
 * count is on its way, the previous one stays rather than flashing empty.
 */
export function useAutoDeletePreview(policy: AutoDeletePolicy): AutoDeletePreviewState {
  const api = useApi();
  const result = useQuery({
    ...autoDeletePreviewQueryOptions(api, { ...policy, enabled: true }),
    placeholderData: keepPreviousData,
    // The next change of a condition asks again, so a failed count says so after one retry, not after the backoff.
    retry: 1,
  });
  if (result.isError) return { status: "error", message: errorMessage(result.error) };
  if (result.data === undefined) return { status: "loading" };
  return {
    status: "done",
    count: result.data.count,
    more: result.data.more,
    examples: result.data.examples,
    pending: result.isPlaceholderData,
  };
}
