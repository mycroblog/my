// SPDX-License-Identifier: AGPL-3.0-only

// The composer's logic, without its view (ADR-0025 §4): the draft being written (model.ts), the uploads of its images
// (each with a failed upload's retry), the post replied to or quoted, read from the cache or the server, and the post
// itself, or the edit of the reader's post. A post the server refuses keeps the draft and says why in the server's own
// words; one that got no answer says that it did not finish, and Post sends it again with the same Idempotency-Key, so
// that it is posted once (an edit sent again is the same edit). A post saved as a draft on the server (ADR-0156) is
// saved over it and published from it; one with a time is saved and scheduled instead, and Save as draft keeps it so;
// a save refused for a newer draft saved elsewhere is the conflict (§9). A draft no longer on the server may have been
// posted (by its schedule, or by a publication whose answer was lost): it is not saved again as a new one, which would
// post it twice, and what was written stays, to be posted as a new post only when the reader asks for it.

import {
  DraftConflictError,
  DraftGoneError,
  refusal,
  useComposeSettings,
  useComposeWrites,
  useStatus,
  type ComposeSettings,
  type SavedDraft,
  type Status,
} from "@my/app-data";
import { useEffect, useRef, useState } from "react";
import {
  answering,
  maxAttachments,
  onServer,
  postable,
  problemsOf,
  toDraft,
  toEdit,
  toStatus,
  type Attachment,
  type Draft,
  type Problems,
} from "./model";
import { rememberTags } from "./recent-tags";

/**
 * Where the post is: being written, being sent, refused (with what the server said, if it said anything), its draft
 * refused for a newer one saved elsewhere since it was opened (ADR-0156 §9), or its draft gone from the server.
 */
export type Phase =
  | { kind: "editing" }
  | { kind: "posting" }
  | { kind: "failed"; reason: string | undefined }
  | { kind: "conflict" }
  | { kind: "gone" };

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
  /**
   * Posts the draft, or schedules it while it has a time, or shows what keeps it from being sent; `onPosted` hears the
   * posted status, `onKept` the draft scheduled. With its draft gone from the server, it lets that draft go and posts
   * what is written now as a new post, with a new Idempotency-Key.
   */
  readonly submit: () => void;
  /** Keeps the draft on the server as it is, with its time if it has one; `onKept` hears it. */
  readonly keep: () => void;
};

/** What the composer tells of a post that left it. */
export type ComposerEvents = {
  /** It was posted (or the edit saved): the status, and the draft it came from. */
  readonly onPosted: (status: Status, draft: Draft) => void;
  /** It was kept on the server as a draft, or scheduled. */
  readonly onKept: (draft: Draft) => void;
};

let next = 0;
const newKey = () => `a${String((next += 1))}`;

/** Whether two times (ISO 8601), either of them none, are the same moment. */
const sameTime = (a: string | undefined, b: string | undefined) =>
  a === undefined || b === undefined ? a === b : Date.parse(a) === Date.parse(b);

/** The composer of `initial`, which tells what left it to `events`. */
export function useComposer(
  initial: Draft,
  { onPosted, onKept }: ComposerEvents,
  initialPhase: Phase = { kind: "editing" },
): Composer {
  const settings = useComposeSettings();
  const writes = useComposeWrites();
  const [draft, setDraft] = useState(initial);
  const [phase, setPhase] = useState<Phase>(initialPhase);
  const [checked, setChecked] = useState(false);
  const id = initial.target.kind === "reply" || initial.target.kind === "quote" ? initial.target.id : undefined;
  const { status: answered } = useStatus(id);

  // A composer that was left (its drafts opened in its place, or the app closed it) hears nothing of what it sent: what
  // left it decided already what becomes of the post.
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const whileOpen =
    <A extends unknown[]>(f: (...args: A) => void) =>
    (...args: A) => {
      if (mounted.current) f(...args);
    };

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

  /** Reads a draft's file of `id` again, which could not be read when the draft was opened. */
  const reread = (key: string, id: string) => {
    update(key, { state: "processing" });
    writes.media(id).then(
      (media) => {
        // One the server could not process is no image the post may carry, nor one to keep.
        if (media.state === "failed") update(key, { state: "failed", id: undefined, media });
        else update(key, { state: "ready", id: undefined, media, preview: media.preview_url ?? media.url });
      },
      // Not read again: it stays the server's, kept by its ID, with its retry.
      () => {
        update(key, { state: "failed" });
      },
    );
  };

  /**
   * Saves `d` over the draft it was saved as, refused for a newer one saved elsewhere (ADR-0156 §9), then, with
   * `schedule`, sets, moves or takes back its schedule to match its time. Each answer is kept with the draft, so that
   * after a failure the next save goes over the draft saved.
   */
  const store = async (d: Draft, schedule: boolean): Promise<SavedDraft> => {
    const hold = (s: SavedDraft) => {
      setDraft((x) => ({ ...x, saved: s }));
      return s;
    };
    // An image still uploading, or failed, is not kept: the server has no file of it (but a draft's it could not read).
    const ready = { ...d, attachments: d.attachments.filter(onServer) };
    const saved = hold(await writes.saveDraft(toDraft(ready, settings), d.saved, { onConflict: "refuse" }));
    if (!schedule || sameTime(d.scheduledAt, saved.scheduledAt)) return saved;
    return hold(
      d.scheduledAt === undefined ? await writes.unschedule(saved.id) : await writes.schedule(saved.id, d.scheduledAt),
    );
  };
  /** A write refused: for a newer draft saved elsewhere, for a draft gone, or with what the server said. */
  const refused = whileOpen((e: unknown) => {
    setPhase(
      e instanceof DraftConflictError
        ? { kind: "conflict" }
        : e instanceof DraftGoneError
          ? { kind: "gone" }
          : { kind: "failed", reason: refusal(e) },
    );
  });
  const kept = whileOpen(onKept);

  /** Posts `d`, or schedules it while it has a time, or shows what keeps it from being sent. */
  const send = (d: Draft) => {
    if (phase.kind === "posting") return;
    if (!postable(problemsOf(d, settings))) {
      setChecked(true);
      return;
    }
    setPhase({ kind: "posting" });
    const { target } = d;
    if (d.scheduledAt !== undefined && target.kind !== "edit") {
      store(d, true).then(() => {
        kept(d);
      }, refused);
      return;
    }
    // A post saved as a draft is published from it, as it is written now; the server deletes the draft.
    const sent =
      target.kind === "edit"
        ? writes.edit(target.id, toEdit(d))
        : d.saved === undefined
          ? writes.post(toStatus(d, settings), d.key)
          : store(d, false).then((saved) => writes.publish(saved.id));
    sent.then((status) => {
      rememberTags(d.text);
      if (mounted.current) onPosted(status, d);
    }, refused);
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
      const a = draft.attachments.find((x) => x.key === key);
      if (a?.file) upload(key, a.file);
      else if (a?.id !== undefined) reread(key, a.id);
    },
    describe: (key, description) => {
      update(key, { description });
    },
    remove: (key) => {
      setDraft((d) => ({ ...d, attachments: d.attachments.filter((a) => a.key !== key) }));
    },
    submit: () => {
      if (phase.kind !== "gone") {
        send(draft);
        return;
      }
      // The draft gone is let go: what is written is posted now as a post of its own, under a key of its own.
      const fresh: Draft = {
        ...draft,
        saved: undefined,
        stored: undefined,
        scheduledAt: undefined,
        failure: undefined,
        key: crypto.randomUUID(),
      };
      setDraft(fresh);
      send(fresh);
    },
    keep: () => {
      if (phase.kind === "posting" || draft.target.kind === "edit") return;
      setPhase({ kind: "posting" });
      store(draft, true).then(() => {
        kept(draft);
      }, refused);
    },
  };
}
