// SPDX-License-Identifier: AGPL-3.0-only
// Requesting the refetch of remote posts from their servers (FR-407, federation.md §6.4). The server queues the
// fetches; what the request did stays on the page in a StatusMessage, and a failure is a toast that leaves the
// button (web-client/README.md E2).
import { useToast } from "@my/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { cacheRefetchRequested, refetchMutation, type RefetchTarget } from "../api/refetch";

export type { RefetchTarget } from "../api/refetch";

export type Refetch = {
  /** Queues the refetch of `target`; once the audit log is fetched again, `status` says it is queued. */
  request: (target: RefetchTarget) => void;
  /** Whether the refetch of `target` is being requested. */
  isPending: (target: RefetchTarget) => boolean;
  /** What the last request did, for a StatusMessage. */
  status: string | undefined;
};

function sameTarget(a: RefetchTarget, b: RefetchTarget): boolean {
  if (a.kind === "domain" || b.kind === "domain")
    return a.kind === "domain" && b.kind === "domain" && a.domain === b.domain;
  return a.kind === b.kind && a.id === b.id;
}

export function useRefetch(): Refetch {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const toast = useToast();
  const [status, setStatus] = useState<string | undefined>(undefined);
  const mutation = useMutation({
    ...refetchMutation(api),
    onSuccess: async (_, target) => {
      await cacheRefetchRequested(queryClient);
      setStatus(t(target.kind === "status" ? "client:admin.refetch.post.queued" : "client:admin.refetch.queued"));
    },
    onError: (error) => {
      toast.show({ message: t("client:admin.refetch.failed", { reason: errorMessage(error) }), tone: "danger" });
    },
  });
  return {
    request: (target) => {
      if (mutation.isPending) return;
      setStatus(undefined);
      mutation.mutate(target);
    },
    isPending: (target) => mutation.isPending && sameTarget(mutation.variables, target),
    status,
  };
}
