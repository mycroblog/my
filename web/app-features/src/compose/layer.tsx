// SPDX-License-Identifier: AGPL-3.0-only

// The composer over the screen, and how it is left (#3046's decisions, ADR-0156): the app opens it for a target (a new
// post, a reply, a quote) and closes it when asked; this decides what leaving does.
//
// - Post: the post goes into the home timeline at once (app-data), and the composer closes.
// - Close or Escape with a written post: the post is discarded, and a notice offers Undo for a moment, which opens the
//   composer again with the post. Nothing is asked: Escape is often pressed by mistake around an input method.
// - Leaving by accident (the backdrop, or the app closing it without a word): the
//   post is saved as a draft on the server, without a word, and kept here, so that opening the composer for the same
//   target (New post, or Reply or Quote on the same post) brings it back.
//
// A draft kept for a target lives as long as the page: the server keeps it across devices, which a list of drafts
// will read (not built yet).

import {
  useComposeSettings,
  useComposeWrites,
  type ComposeSettings,
  type ComposeWrites,
  type Status,
} from "@my/app-data";
import { ModalPresence, UndoNotice, useLocale } from "@my/design-system";
import { useEffect, useRef, useState } from "react";
import { stringsIn } from "../locale";
import { ComposerView } from "./composer";
import { emptyDraft, targetKey, toDraft, written, type Draft, type Target } from "./model";
import { useComposer } from "./use-composer";

/** How long the notice of a discarded post offers Undo. */
const undoMs = 6000;

/** The drafts left by accident, by their target's key. */
const kept = new Map<string, Draft>();

/** Saves a draft left by accident: kept here at once, and on the server; the server's answer is kept with it. */
function keep(d: Draft, writes: ComposeWrites, settings: ComposeSettings): void {
  const key = targetKey(d.target);
  // An image still uploading, or failed, is not kept: the server has no file of it.
  const draft = { ...d, attachments: d.attachments.filter((a) => a.state === "ready") };
  kept.set(key, draft);
  writes.saveDraft(toDraft(draft, settings), draft.saved).then(
    (saved) => {
      if (kept.get(key) === draft) kept.set(key, { ...draft, saved });
    },
    // Not saved on the server: the page still keeps it.
    () => undefined,
  );
}

/** Forgets a draft: here, and on the server. */
function forget(d: Draft, writes: ComposeWrites): void {
  kept.delete(targetKey(d.target));
  if (d.saved) writes.deleteDraft(d.saved.id).catch(() => undefined);
}

function Open({
  initial,
  layer,
  phone,
  track,
  onLeave,
}: {
  initial: Draft;
  layer: boolean;
  phone: "screen" | "dialog";
  track: (d: Draft) => void;
  onLeave: (how: "posted" | "close" | "dismiss", d: Draft, status?: Status) => void;
}) {
  const c = useComposer(initial, (status, draft) => {
    onLeave("posted", draft, status);
  });
  useEffect(() => {
    track(c.draft);
  });
  return (
    <ComposerView
      c={c}
      layer={layer}
      phone={phone}
      onClose={() => {
        onLeave("close", c.draft);
      }}
      onDismiss={() => {
        onLeave("dismiss", c.draft);
      }}
    />
  );
}

export type ComposeLayerProps = {
  /** What the composer is open for; none while it is closed. */
  target: Target | undefined;
  /** Opens the composer for a target (Undo). */
  onOpen: (target: Target) => void;
  /** Closes it, once it was left. */
  onClose: () => void;
  /** A screen of its own on a phone, or a dialog: the app's choice by the viewport. */
  phone?: "screen" | "dialog" | undefined;
  /** In the flow of a page instead of over it (a story's specimen). */
  inline?: boolean | undefined;
  /** The draft it opens with, in place of the one kept or an empty one (a story's specimen). */
  initial?: Draft | undefined;
};

/** The composer for `target`, over the screen, and the notice of a post discarded from it. */
export function ComposeLayer({
  target,
  onOpen,
  onClose,
  phone = "screen",
  inline = false,
  initial,
}: ComposeLayerProps) {
  const { t } = stringsIn(useLocale().locale);
  const settings = useComposeSettings();
  const writes = useComposeWrites();
  const [discarded, setDiscarded] = useState<Draft | undefined>(undefined);
  // The draft of the composer shown, as it was last drawn, by its target's key.
  const latest = useRef<{ key: string; draft: Draft } | undefined>(undefined);
  const left = useRef(false);
  const key = target && targetKey(target);

  // A new target opens a new composer; the one before was left already, or is left by accident now (the app closed it
  // without a word).
  const shown = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (shown.current === key) return;
    const before = latest.current;
    if (!left.current && before && before.key === shown.current && written(before.draft))
      keep(before.draft, writes, settings);
    shown.current = key;
    left.current = false;
  });

  useEffect(() => {
    if (discarded === undefined || inline) return undefined;
    const timer = setTimeout(() => {
      setDiscarded(undefined);
    }, undoMs);
    return () => {
      clearTimeout(timer);
    };
  }, [discarded, inline]);

  const leave = (how: "posted" | "close" | "dismiss", d: Draft) => {
    left.current = true;
    if (how === "posted") {
      kept.delete(targetKey(d.target));
    } else if (how === "dismiss") {
      if (written(d)) keep(d, writes, settings);
    } else {
      forget(d, writes);
      if (written(d)) setDiscarded({ ...d, saved: undefined });
    }
    onClose();
  };

  const start = target && (initial ?? (key && kept.get(key)) ?? emptyDraft(target, settings, crypto.randomUUID()));
  return (
    <>
      {/* A composer that was left leaves by its motion, then unmounts. */}
      <ModalPresence>
        {target && start && (
          <Open
            // One composer per target: it mounts once when opened, and a closed one comes back as a new one.
            key={key}
            initial={start}
            layer={!inline}
            phone={phone}
            track={(d) => {
              latest.current = { key: targetKey(d.target), draft: d };
            }}
            onLeave={leave}
          />
        )}
      </ModalPresence>
      {discarded && (
        <UndoNotice
          action={t("compose.undo")}
          inline={inline}
          onAction={() => {
            kept.set(targetKey(discarded.target), discarded);
            setDiscarded(undefined);
            onOpen(discarded.target);
          }}
        >
          {t("compose.discarded")}
        </UndoNotice>
      )}
    </>
  );
}
