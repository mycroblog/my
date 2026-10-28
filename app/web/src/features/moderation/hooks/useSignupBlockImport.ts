// SPDX-License-Identifier: AGPL-3.0-only
// The import of a list of sign-up blocks (FR-719): one block for each value of a list pasted into the form or
// read from a text file, all with the same kind, severity, comment and duration. The server adds what it can
// and says which lines it skipped and why; the result stays on the page until the next import.
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type SyntheticEvent, useState } from "react";
import { useApi } from "../../../lib/auth/SessionProvider";
import { errorMessage } from "../../../lib/errors";
import { type Translate, useT } from "../../../lib/i18n";
import { refreshAuditLog } from "../api/staff";
import {
  countSignupBlockListValues,
  importSignupBlocksMutation,
  refreshSignupBlockLists,
  type SignupBlockImport,
  signupBlockImportByteLimit,
  type SignupBlockImportResult,
  signupBlockImportValueLimit,
  type SignupBlockKind,
  type SignupBlockSeverity,
} from "../api/signupBlocks";

/** The form of an import, filled in by the moderator. */
type Draft = {
  kind: SignupBlockKind;
  /** The list: one value per line, or CSV rows. */
  text: string;
  severity: SignupBlockSeverity | null;
  comment: string;
  /** The seconds each block lasts, or null until it is lifted. */
  expiresIn: number | null;
};

function emptyDraft(kind: SignupBlockKind): Draft {
  return { kind, text: "", severity: null, comment: "", expiresIn: null };
}

/** The request of a draft whose severity is chosen; an empty comment is left out. */
function requestOf(draft: Draft, severity: SignupBlockSeverity): SignupBlockImport {
  return {
    kind: draft.kind,
    text: draft.text,
    severity,
    ...(draft.comment === "" ? {} : { comment: draft.comment }),
    ...(draft.expiresIn === null ? {} : { expires_in: draft.expiresIn }),
  };
}

/** The megabytes of the byte limit, for the messages. */
const megabytes = signupBlockImportByteLimit / 1_000_000;

/**
 * Why a list cannot be sent, localized, or null when it can: it has no value, more values than one import
 * holds, or more bytes than the server reads. The server counts the values the same way.
 */
export function validateSignupBlockList(t: Translate, text: string): string | null {
  const count = countSignupBlockListValues(text);
  if (count === 0) return t("client:admin.signup_blocks.import.list.required");
  if (count > signupBlockImportValueLimit)
    return t("client:admin.signup_blocks.import.list.too_many", { count, max: signupBlockImportValueLimit });
  if (new TextEncoder().encode(text).length > signupBlockImportByteLimit)
    return t("client:admin.signup_blocks.import.list.too_large", { max: megabytes });
  return null;
}

/** What reading the chosen file did, for next to its button: nothing yet, read, or failed. */
type FileState = { status: "none" } | { status: "read"; message: string } | { status: "failed"; message: string };

/** The outcome of the last import, for its result: the kind it was for and what the server answered. */
export type SignupBlockImportOutcome = SignupBlockImportResult & {
  kind: SignupBlockKind;
  severity: SignupBlockSeverity;
  /** Changes with every import, to start the result's own state afresh (a `key`). */
  id: number;
};

/** What the import form needs. */
export type SignupBlockImportForm = {
  draft: Draft;
  change: (change: Partial<Draft>) => void;
  /** Reads a text file into the list, in place of what it held. */
  chooseFile: (file: File) => void;
  file: FileState;
  /** Imports the list once the browser has checked the fields. */
  submit: (event: SyntheticEvent<HTMLFormElement>) => void;
  isPending: boolean;
  /** Why the import failed, localized, for under the form; the draft stays. */
  error: string | undefined;
  /** What the last import did; undefined before the first one. */
  outcome: SignupBlockImportOutcome | undefined;
};

/**
 * The import form. A success empties the list and the comment (keeping the kind, the severity and the
 * duration, which the next list often shares, such as the second half of a long one) and shows what was added
 * and skipped once every cached list of blocks and the audit log are fetched again.
 */
export function useSignupBlockImportForm(): SignupBlockImportForm {
  const api = useApi();
  const t = useT();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState(() => emptyDraft("email_domain"));
  const [file, setFile] = useState<FileState>({ status: "none" });
  const mutation = useMutation({
    ...importSignupBlocksMutation(api),
    onSuccess: async () => {
      // The blocks are in the lists of both tabs and in the audit log: the import ends once they are fetched again.
      await Promise.all([refreshSignupBlockLists(queryClient), refreshAuditLog(queryClient)]);
      setDraft((current) => ({ ...current, text: "", comment: "" }));
      setFile({ status: "none" });
    },
  });
  const sent = mutation.variables;
  const outcome: SignupBlockImportOutcome | undefined =
    mutation.isSuccess && sent !== undefined
      ? { ...mutation.data, kind: sent.kind, severity: sent.severity, id: mutation.submittedAt }
      : undefined;
  return {
    draft,
    change: (change) => {
      setDraft({ ...draft, ...change });
    },
    chooseFile: (chosen) => {
      if (chosen.size > signupBlockImportByteLimit) {
        setFile({
          status: "failed",
          message: t("client:admin.signup_blocks.import.file.too_large", { name: chosen.name, max: megabytes }),
        });
        return;
      }
      chosen.text().then(
        (text) => {
          setDraft((current) => ({ ...current, text }));
          setFile({
            status: "read",
            message: t("client:admin.signup_blocks.import.file.read", {
              name: chosen.name,
              count: countSignupBlockListValues(text),
            }),
          });
        },
        () => {
          setFile({
            status: "failed",
            message: t("client:admin.signup_blocks.import.file.failed", { name: chosen.name }),
          });
        },
      );
    },
    file,
    submit: (event) => {
      event.preventDefault();
      if (mutation.isPending || draft.severity === null) return;
      mutation.mutate(requestOf(draft, draft.severity));
    },
    isPending: mutation.isPending,
    error: mutation.isError ? errorMessage(mutation.error) : undefined,
    outcome,
  };
}
