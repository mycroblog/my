// SPDX-License-Identifier: AGPL-3.0-only
// The closure of the service as a holder of `manage_service_closure` runs it (FR-808, FR-809, ADR-0088): where it
// stands, with the schedule in effect and whether its notice is short, and once the server is closed how far the
// `Delete` of every local account has gone, read again while it goes on; what closing would end (the check); and
// announcing it or changing its dates, withdrawing it, and closing the server for good. Every write changes what every
// reader's screens show of the closure (`Instance.closure`: the banner, the closed screen), so it reads the instance
// and the check again.
import { ProblemError, type components } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { instanceQuery, type Instance } from "./instance";
import { useAppData } from "./provider";
import { readOf, type ScreenRead } from "./reads";
import { writeCached } from "./written";

/** The closure of the service as its administrators see it. */
export type AdminClosure = components["schemas"]["AdminServiceClosure"];
/** Where the server is in its closure. */
export type ClosureState = components["schemas"]["ServiceClosureState"];
/** How far the closure's `Delete`s have gone, once the server is closed. */
export type ClosureProgress = components["schemas"]["AdminServiceClosureProgress"];
/** What closing the server would end. */
export type ClosureCheck = components["schemas"]["AdminServiceClosureCheck"];
/** The schedule of an announcement. */
export type AnnounceClosure = components["schemas"]["AnnounceServiceClosureRequest"];

/** How often the closure is read again while its deliveries go on. */
const progressInterval = 10_000;

/** The closure of the service. */
export function adminClosureQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/closure");
}

/** What closing the server would end. */
export function closureCheckQuery({ queries }: AppData) {
  return queries.queryOptions("get", "/admin/closure/check");
}

/** Reads the closure, for a route's loader. */
export async function loadAdminClosure(data: AppData): Promise<void> {
  await data.queryClient.query({ ...adminClosureQuery(data), staleTime: Infinity }).catch(() => undefined);
}

/** Reads the closure and what closing would end, for a route's loader. */
export async function loadClosureCheck(data: AppData): Promise<void> {
  await Promise.all([
    loadAdminClosure(data),
    data.queryClient.query({ ...closureCheckQuery(data), staleTime: Infinity }).catch(() => undefined),
  ]);
}

/** Whether the server is closed: its deliveries go on (`closed`) or have ended. */
export const closureClosed = (state: ClosureState): boolean => state === "closed" || state === "delivery_completed";

/** The closure and where its read is; read again every few seconds while the closure's deliveries go on. */
export function useAdminClosure(): ScreenRead & { readonly closure: AdminClosure | undefined } {
  const query = useQuery({
    ...adminClosureQuery(useAppData()),
    refetchInterval: (q) => (q.state.data?.state === "closed" ? progressInterval : false),
  });
  return { ...readOf([query]), closure: query.data };
}

/** The closure and what closing would end, and where their reads are. */
export function useClosureCheck(): ScreenRead & {
  readonly closure: AdminClosure | undefined;
  readonly check: ClosureCheck | undefined;
} {
  const data = useAppData();
  const closure = useQuery(adminClosureQuery(data));
  const check = useQuery(closureCheckQuery(data));
  return { ...readOf([closure, check]), closure: closure.data, check: check.data };
}

/** Whether `error` is the server's refusal of a closure's operation in the state it is in now (409). */
export function closureStateRefused(error: unknown): boolean {
  return error instanceof ProblemError && error.type === "/api/v1/problems/service-closure-state";
}

/** The parameter of the request a 422 names (`closes_at`, `export_until`, `message`, `confirm_domain`); else none. */
export function closureFieldOf(error: unknown): string | undefined {
  if (!(error instanceof ProblemError) || error.status !== 422) return undefined;
  return error.problem.field;
}

/** `instance` as it says `closure`: none while none is announced, else its state, dates and message. */
function instanceWith(instance: Instance, closure: AdminClosure): Instance {
  const { closure: _was, ...rest } = instance;
  if (closure.state === "none" || closure.closes_at === undefined || closure.export_until === undefined) return rest;
  return {
    ...rest,
    closure: {
      state: closure.state,
      closes_at: closure.closes_at,
      export_until: closure.export_until,
      ...(closure.message === undefined ? {} : { message: closure.message }),
    },
  };
}

/** The writes of the closure; each resolves with the closure as the server answered it, or rejects with its problem. */
export type AdminClosureWrites = {
  /** Announces the closure, or changes the dates and the message of the one in effect. */
  readonly announce: (schedule: AnnounceClosure) => Promise<AdminClosure>;
  /** Withdraws the announcement in effect. */
  readonly withdraw: () => Promise<AdminClosure>;
  /** Closes the server for good, `domain` typed by the reader to confirm it. */
  readonly close: (domain: string) => Promise<AdminClosure>;
};

/** The writes of the closure, for a screen. */
export function useAdminClosureWrites(): AdminClosureWrites {
  const data = useAppData();
  const written = async (closure: AdminClosure | undefined): Promise<AdminClosure> => {
    if (closure === undefined) throw new Error("app-data: a write of the closure answered without the closure");
    const { queryClient } = data;
    const key = adminClosureQuery(data).queryKey;
    await queryClient.cancelQueries({ queryKey: key });
    queryClient.setQueryData(key, closure);
    void queryClient.invalidateQueries({ queryKey: closureCheckQuery(data).queryKey });
    // The instance is cached for a minute on its way (`max-age`): what it says of the closure is put in from the
    // answer, so that this reader's banner and closed screen follow the write at once.
    await writeCached<Instance>(data, instanceQuery(data).queryKey, (instance) => instanceWith(instance, closure));
    return closure;
  };
  return {
    announce: async (schedule) => written((await data.api.PUT("/admin/closure/announcement", { body: schedule })).data),
    withdraw: async () => written((await data.api.DELETE("/admin/closure/announcement")).data),
    close: async (domain) =>
      written((await data.api.POST("/admin/closure/close", { body: { confirm_domain: domain } })).data),
  };
}
