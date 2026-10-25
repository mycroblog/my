// SPDX-License-Identifier: AGPL-3.0-only
// The service closure (FR-808, FR-809, ADR-0088, manage_service_closure): its state with the schedule and, once the
// server has closed, the progress of its deliveries (read again every few seconds while they go on); announcing it
// or changing its dates with the operators' message; withdrawing the announcement; the check of what closing ends;
// and closing the server for good after the server's domain is typed. Every change goes to the audit log.
import { useMutation, useQuery, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useConfig } from "../../../lib/config";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import { shellQueryReader } from "../../../lib/query";
import {
  type AdminServiceClosure,
  type AnnounceServiceClosure,
  adminClosureCheckQueryOptions,
  adminClosureQueryOptions,
  announceClosureMutation,
  closeServiceMutation,
  closureMessageLimit,
  progressIntervalMs,
  refreshClosure,
  shortNoticeDays,
  withdrawClosureMutation,
} from "../api/closure";
import { characterCount } from "./languages";

const day = 86_400_000;

/** The state of the closure; while the deliveries go on, it is read again every few seconds. */
export function useAdminClosure(): AdminServiceClosure {
  const api = useApi();
  return useSuspenseQuery({
    ...adminClosureQueryOptions(api),
    refetchInterval: (query) => (query.state.data?.state === "closed" ? progressIntervalMs : false),
  }).data;
}

/** What closing the server would end. */
export function useClosureCheck() {
  const api = useApi();
  return useSuspenseQuery(adminClosureCheckQueryOptions(api)).data;
}

/** A moment as the value of an `<input type="datetime-local">`: in the viewer's time zone, to the minute. */
function toLocalInput(value: string | undefined): string {
  if (value === undefined) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${String(date.getFullYear())}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

/** The moment that a value of `<input type="datetime-local">` means in the viewer's time zone; undefined when it is not one. */
function fromLocalInput(value: string): Date | undefined {
  if (value.trim() === "") return undefined;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? undefined : date;
}

/** Whether the closing date `closesAt` gives less than the days of a short notice from `now`. */
function isShortNotice(closesAt: Date, now: number): boolean {
  return closesAt.getTime() - now < shortNoticeDays * day;
}

type FormErrors = { closesAt?: string | undefined; exportUntil?: string | undefined; message?: string | undefined };
const noErrors: FormErrors = {};

/** The form that announces the closure or changes the dates of the announcement in effect. */
export function useAnnounceForm(current: AdminServiceClosure, onDone: (status: string) => void) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [closesAt, setClosesAt] = useState(() => toLocalInput(current.closes_at));
  const [exportUntil, setExportUntil] = useState(() => toLocalInput(current.export_until));
  const [message, setMessage] = useState(current.message ?? "");
  // When the form opened, which the notice's length is counted from (the server counts from the time of sending).
  const [openedAt] = useState(() => Date.now());
  const [submitted, setSubmitted] = useState(false);
  const announce = useMutation({
    ...announceClosureMutation(api),
    onSuccess: async () => {
      await refreshClosure(queryClient);
      setSubmitted(false);
      onDone(t(current.state === "announced" ? "client:admin.closure.changed" : "client:admin.closure.announced"));
    },
  });
  const opens = fromLocalInput(closesAt);
  const until = fromLocalInput(exportUntil);
  const errors: FormErrors = {
    closesAt: opens === undefined ? t("client:admin.closure.closes_at.required") : undefined,
    exportUntil:
      exportUntil.trim() !== "" && until === undefined
        ? t("client:admin.closure.export_until.invalid")
        : opens !== undefined && until !== undefined && until < opens
          ? t("client:admin.closure.export_until.before")
          : undefined,
    message: characterCount(message) > closureMessageLimit ? t("client:admin.closure.message.too_long", { max: String(closureMessageLimit) }) : undefined,
  };
  return {
    closesAt,
    changeClosesAt: setClosesAt,
    exportUntil,
    changeExportUntil: setExportUntil,
    message,
    changeMessage: setMessage,
    errors: submitted ? errors : noErrors,
    /** Whether the closing date gives the users less than 30 days from now (the server decides from the time of sending). */
    shortNotice: opens !== undefined && isShortNotice(opens, openedAt),
    isChange: current.state === "announced",
    submit: (event: SyntheticEvent<HTMLFormElement>) => {
      event.preventDefault();
      setSubmitted(true);
      if (announce.isPending || opens === undefined || errors.exportUntil !== undefined || errors.message !== undefined) return;
      const body: AnnounceServiceClosure = {
        closes_at: opens.toISOString(),
        ...(until === undefined ? {} : { export_until: until.toISOString() }),
        ...(message.trim() === "" ? {} : { message }),
      };
      announce.mutate(body);
    },
    isPending: announce.isPending,
    /** Why announcing failed, localized; the form stays. */
    failure: announce.isError ? errorMessage(announce.error) : undefined,
  };
}

/** Withdrawing the announcement: it resolves whether it worked, and says why it did not. */
export function useWithdrawClosure(onDone: (status: string) => void) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const withdraw = useMutation({ ...withdrawClosureMutation(api) });
  const [error, setError] = useState<string | undefined>(undefined);
  return {
    isPending: withdraw.isPending,
    error,
    clearError: () => {
      setError(undefined);
    },
    withdraw: async (): Promise<boolean> => {
      setError(undefined);
      try {
        await withdraw.mutateAsync();
        await refreshClosure(queryClient);
        onDone(t("client:admin.closure.withdrawn"));
        return true;
      } catch (failure) {
        setError(errorMessage(failure));
        return false;
      }
    },
  };
}

/** Whether `typed` is the server's domain, which closing asks for; the server compares without regard to case. */
function isServerDomain(typed: string, domain: string): boolean {
  return typed.trim().toLowerCase() === domain.toLowerCase();
}

/** Closing the server for good: the domain to type, and the closing that resolves whether it worked. */
export function useCloseService(onDone: (status: string) => void) {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const { accountDomain } = useConfig();
  // The shell reads GET /instance on every page; this only reads its answer. The domain is the one that the server compares.
  const instance = useQuery({ ...api.query.queryOptions("get", "/instance"), ...shellQueryReader });
  const domain = instance.data?.domain ?? accountDomain;
  const close = useMutation({ ...closeServiceMutation(api) });
  const [typed, setTyped] = useState("");
  // Whether the last attempt had a domain that does not match. It is kept until the next edit, never cleared on blur:
  // the error under the field going away as the press moves the focus would shrink the dialog and move the confirm
  // button from under the pointer before it is released, which loses the press.
  const [mismatch, setMismatch] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  return {
    domain,
    typed,
    mismatch,
    changeTyped: (next: string) => {
      setTyped(next);
      setMismatch(false);
    },
    isPending: close.isPending,
    error,
    /** Empties what was typed, the mismatch and the failure, when the dialog closes. */
    reset: () => {
      setTyped("");
      setMismatch(false);
      setError(undefined);
    },
    /** Closes the server if the typed domain matches: `mismatch` when it does not, `failed` when the request fails. */
    close: async (): Promise<"closed" | "mismatch" | "failed"> => {
      if (!isServerDomain(typed, domain)) {
        setMismatch(true);
        return "mismatch";
      }
      setError(undefined);
      try {
        await close.mutateAsync(typed.trim());
        await refreshClosure(queryClient);
        onDone(t("client:admin.closure.closed"));
        return "closed";
      } catch (failure) {
        setError(errorMessage(failure));
        return "failed";
      }
    },
  };
}
