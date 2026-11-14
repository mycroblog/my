// SPDX-License-Identifier: AGPL-3.0-only

// The composer's logic, without its view (ADR-0025 §4): the draft being written (model.ts), the uploads of its images
// (each with a failed upload's retry), the post replied to or quoted, read from the cache or the server, and the post
// itself. A post the server refuses keeps the draft and says why in the server's own words; one that got no answer
// says that it did not finish, and Post sends it again with the same Idempotency-Key, so that it is posted once.

import {
  refusal,
  useComposeSettings,
  useComposeWrites,
  useStatus,
  type ComposeSettings,
  type Status,
} from "@my/app-data";
import { useEffect, useRef, useState } from "react";
import {
  answering,
  maxAttachments,
  postable,
  problemsOf,
  toStatus,
  type Attachment,
  type Draft,
  type Problems,
} from "./model";

/** Where the post is: being written, being sent, or refused (with what the server said, if it said anything). */
type Phase = { kind: "editing" } | { kind: "posting" } | { kind: "failed"; reason: string | undefined };

export type Composer = {
  readonly draft: Draft;
  readonly settings: ComposeSettings;
  /** The post replied to or quoted, once read. */
  readonly answered: Status | undefined;
  readonly problems: Problems;
  readonly phase: Phase;
  /** Post was pressed with a problem: the problems show. */
  readonly checked: boolean;
  readonly change: (patch: Partial<Draft>) => void;
  readonly addFiles: (files: readonly File[]) => void;
  readonly retry: (key: string) => void;
  readonly describe: (key: string, description: string) => void;
  readonly remove: (key: string) => void;
  /** Posts the draft, or shows what keeps it from being posted; `onPosted` hears the posted status. */
  readonly submit: () => void;
};

let next = 0;
const newKey = () => `a${String((next += 1))}`;

/** The composer of `initial`; `onPosted` hears the posted status and the draft it was posted from. */
export function useComposer(initial: Draft, onPosted: (status: Status, draft: Draft) => void): Composer {
  const settings = useComposeSettings();
  const writes = useComposeWrites();
  const [draft, setDraft] = useState(initial);
  const [phase, setPhase] = useState<Phase>({ kind: "editing" });
  const [checked, setChecked] = useState(false);
  const id = initial.target.kind === "new" ? undefined : initial.target.id;
  const { status: answered } = useStatus(id);

  // A reply mentions whom it answers, once the post is read; the reader's own writing is never replaced.
  const took = useRef(false);
  useEffect(() => {
    if (answered === undefined || took.current) return;
    took.current = true;
    setDraft((d) => answering(d, answered, settings.viewer));
  }, [answered, settings.viewer]);

  const change = (patch: Partial<Draft>) => {
    setDraft((d) => ({ ...d, ...patch }));
    if (phase.kind === "failed") setPhase({ kind: "editing" });
  };
  const update = (key: string, patch: Partial<Attachment>) => {
    setDraft((d) => ({ ...d, attachments: d.attachments.map((a) => (a.key === key ? { ...a, ...patch } : a)) }));
  };

  const upload = (key: string, file: File) => {
    update(key, { state: "uploading" });
    writes
      .upload(file, {
        onProcessing: () => {
          update(key, { state: "processing" });
        },
      })
      .then(
        (media) => {
          update(key, {
            state: "ready",
            media,
            ...(media.preview_url === undefined ? {} : { preview: media.preview_url }),
          });
        },
        // Refused, unprocessable, or no answer: the row offers to upload it again.
        () => {
          update(key, { state: "failed" });
        },
      );
  };

  return {
    draft,
    settings,
    answered,
    problems: problemsOf(draft, settings),
    phase,
    checked,
    change,
    addFiles: (files) => {
      const room = maxAttachments - draft.attachments.length;
      const added = files.slice(0, Math.max(0, room)).map((file): Attachment => ({
        key: newKey(),
        file,
        preview: file.type.startsWith("image/") ? URL.createObjectURL(file) : undefined,
        state: "uploading",
        description: "",
      }));
      setDraft((d) => ({ ...d, poll: undefined, attachments: [...d.attachments, ...added] }));
      for (const a of added) if (a.file) upload(a.key, a.file);
    },
    retry: (key) => {
      const file = draft.attachments.find((a) => a.key === key)?.file;
      if (file) upload(key, file);
    },
    describe: (key, description) => {
      update(key, { description });
    },
    remove: (key) => {
      setDraft((d) => ({ ...d, attachments: d.attachments.filter((a) => a.key !== key) }));
    },
    submit: () => {
      if (phase.kind === "posting") return;
      const problems = problemsOf(draft, settings);
      if (!postable(problems)) {
        setChecked(true);
        return;
      }
      setPhase({ kind: "posting" });
      writes.post(toStatus(draft, settings), draft.key).then(
        (status) => {
          // The draft it was saved as is published now: it goes, and a failure leaves only a stale draft.
          if (draft.saved) writes.deleteDraft(draft.saved.id).catch(() => undefined);
          onPosted(status, draft);
        },
        (e: unknown) => {
          setPhase({ kind: "failed", reason: refusal(e) });
        },
      );
    },
  };
}
