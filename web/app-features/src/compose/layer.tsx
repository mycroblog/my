// SPDX-License-Identifier: AGPL-3.0-only

// The composer over the screen, and how it is left (#3046's decisions, ADR-0156): the app opens it for a target (a new
// post, a reply, a quote, an edit of the reader's post, a redraft) or for a draft of the reader's on the server, and
// closes it when asked; this decides what leaving does.
//
// - Post: the post goes into the home timeline at once (app-data), and the composer closes.
// - Save (an edit): every copy of the post shows the edit (app-data), the composer closes, and a notice says so. An
//   edit closed goes without a word; left by accident it is kept on this page alone, never as a server draft.
// - Schedule, or Save as draft: the post is kept on the server, scheduled if it has a time, and the composer closes.
// - Close or Escape with a written post: the post is discarded, and a notice offers Undo for a moment, which opens the
//   composer again with the post. Nothing is asked: Escape is often pressed by mistake around an input method. A draft
//   opened from the server closes without a word, and the server keeps it as it was last saved.
// - Leaving by accident (the backdrop, or the app closing it without a word): the
//   post is saved as a draft on the server, without a word, and kept here, so that opening the composer for the same
//   target (New post, or Reply or Quote on the same post) brings it back.
// - While a post or a save is in flight, none of these leaves it, nor do the drafts open; a composer the app closed
//   then hears nothing of its answer.
//
// The composer's head opens the reader's drafts and scheduled posts in its place (drafts.tsx), which goes back to the
// post being written, or opens one of them in the composer, read again from the server with its images, so that it
// goes on from any device; the post being written is then left as by accident. A save refused for a newer draft saved
// elsewhere opens the newer one in place of what was written (§9).

import {
  useComposeSettings,
  useComposeWrites,
  useDrafts,
  type ComposeSettings,
  type ComposeWrites,
  type Status,
} from "@my/app-data";
import { ModalPresence, UndoNotice, useLocale } from "@my/design-system";
import { useEffect, useRef, useState } from "react";
import { stringsIn } from "../locale";
import { ComposerView, type ComposerViewProps } from "./composer";
import { DraftsView } from "./drafts";
import { KeptDrafts } from "./kept";
import { emptyDraft, fromServer, targetKey, written, type Draft, type Opening } from "./model";
import { useComposer, type Phase } from "./use-composer";

/** How long the notice of a discarded post offers Undo. */
const undoMs = 6000;

/** The drafts left by accident, by their target's key, with their writes on the server in order. */
const kept = new KeptDrafts();

/** Reads the reader's draft of `id` from the server, with its images, as the composer's draft. */
async function load(id: string, writes: ComposeWrites, settings: ComposeSettings): Promise<Draft> {
  const { draft, saved } = await writes.readDraft(id);
  const media = await Promise.all(draft.media.map((m) => writes.media(m.id).catch(() => undefined)));
  return fromServer(draft, saved, media, settings, crypto.randomUUID());
}

/** How the composer was left. */
type Leaving = "posted" | "kept" | "close" | "dismiss";

function Open({
  initial,
  layer,
  phone,
  pickerSheet,
  track,
  onLeave,
  onDrafts,
  onNewer,
  phase,
  opened,
}: {
  initial: Draft;
  layer: boolean;
  phone: "screen" | "dialog";
  pickerSheet: boolean;
  track: (d: Draft) => void;
  onLeave: (how: Leaving, d: Draft, status?: Status) => void;
  onDrafts: (d: Draft) => void;
  onNewer: (d: Draft) => void;
  phase: Phase | undefined;
  opened: ComposerViewProps["opened"];
}) {
  const c = useComposer(
    initial,
    {
      onPosted: (status, draft) => {
        onLeave("posted", draft, status);
      },
      onKept: (draft) => {
        onLeave("kept", draft);
      },
    },
    phase,
  );
  useEffect(() => {
    track(c.draft);
  });
  return (
    <ComposerView
      c={c}
      layer={layer}
      phone={phone}
      pickerSheet={pickerSheet}
      opened={opened}
      onClose={() => {
        onLeave("close", c.draft);
      }}
      onDismiss={() => {
        onLeave("dismiss", c.draft);
      }}
      onSaveDraft={c.keep}
      onDrafts={() => {
        // Not while what was written is being sent: it would be left with its answer unheard.
        if (c.phase.kind !== "posting") onDrafts(c.draft);
      }}
      onNewer={() => {
        onNewer(c.draft);
      }}
    />
  );
}

/** The reader's drafts and scheduled posts in the composer's place, as the server lists them. */
function Drafts({
  layer,
  phone,
  onBack,
  onClose,
  onDismiss,
  onOpen,
}: {
  layer: boolean;
  phone: "screen" | "dialog";
  onBack: () => void;
  onClose: () => void;
  onDismiss: () => void;
  onOpen: (id: string, schedule: boolean) => void;
}) {
  const drafts = useDrafts();
  const state = drafts.lists
    ? { kind: "ready" as const, ...drafts.lists }
    : drafts.failed
      ? { kind: "failed" as const }
      : { kind: "loading" as const };
  return (
    <DraftsView
      state={state}
      layer={layer}
      phone={phone}
      onBack={onBack}
      onClose={onClose}
      onDismiss={onDismiss}
      onRetry={drafts.retry}
      onOpen={(id, schedule = false) => {
        onOpen(id, schedule);
      }}
      // A failure leaves the list as the server reads it again.
      onUnschedule={(id) => {
        drafts.unschedule(id).catch(() => undefined);
      }}
      onDelete={(id) => {
        drafts.remove(id).catch(() => undefined);
      }}
    />
  );
}

export type ComposeLayerProps = {
  /** What the composer is open for; none while it is closed. */
  target: Opening | undefined;
  /** Opens the composer for a target (Undo). */
  onOpen: (target: Opening) => void;
  /** Closes it, once it was left. */
  onClose: () => void;
  /** A screen of its own on a phone, or a dialog: the app's choice by the viewport. */
  phone?: "screen" | "dialog" | undefined;
  /** The emoji picker opens in a sheet: the app's choice on a phone's viewport; else in a popover. */
  pickerSheet?: boolean | undefined;
  /** In the flow of a page instead of over it (a story's specimen). */
  inline?: boolean | undefined;
  /** The draft it opens with, in place of the one kept or an empty one (a story's specimen). */
  initial?: Draft | undefined;
  /** What a story's specimen opens in: its phase, and what is open in it. */
  specimen?: ComposeSpecimen | undefined;
};

/** A story's specimen of the composer: the phase it opens in, and what is open in it. */
export type ComposeSpecimen = {
  readonly phase?: Phase | undefined;
  readonly opened?: ComposerViewProps["opened"];
};

/**
 * What the layer shows in place of the target's composer: a draft in the composer (`id` tells one opening from the
 * next), or the drafts, with the post that was being written to go back to.
 */
type Screen =
  { kind: "compose"; draft: Draft; id: string; schedule: boolean } | { kind: "drafts"; back: Draft | undefined };

/** The composer for `target`, over the screen, and the notice of a post discarded from it. */
export function ComposeLayer({
  target,
  onOpen,
  onClose,
  phone = "screen",
  pickerSheet = false,
  inline = false,
  initial,
  specimen = {},
}: ComposeLayerProps) {
  const { t } = stringsIn(useLocale().locale);
  const settings = useComposeSettings();
  const writes = useComposeWrites();
  const [discarded, setDiscarded] = useState<Draft | undefined>(undefined);
  const [saved, setSaved] = useState(false);
  // What shows in place of the target's composer, for the target it was opened over.
  const [screens, setScreens] = useState<{ key: string; screen: Screen } | undefined>(undefined);
  // The draft of the composer shown, as it was last drawn, by its target's key.
  const latest = useRef<{ key: string; draft: Draft } | undefined>(undefined);
  const left = useRef(false);
  const key = target && targetKey(target);
  const screen = screens !== undefined && screens.key === key ? screens.screen : undefined;
  const show = (s: Screen) => {
    if (key !== undefined) setScreens({ key, screen: s });
  };
  /** Closes the layer; what showed in place of the composer goes with it, so the next opening starts at the composer. */
  const close = () => {
    setScreens(undefined);
    onClose();
  };

  // A new target opens a new composer; the one before was left already, or is left by accident now (the app closed it
  // without a word).
  const shown = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (shown.current === key) return;
    const before = latest.current;
    if (!left.current && before && before.key === shown.current && written(before.draft))
      kept.keep(before.draft, writes, settings);
    shown.current = key;
    left.current = false;
  });

  // A draft the app opens (a notification of its failed schedule) is read from the server, with the session's writes
  // and settings, once per opening; one that cannot be read (deleted since) leaves the reader at the list.
  const session = useRef({ writes, settings });
  useEffect(() => {
    session.current = { writes, settings };
  });
  const draftId = target?.kind === "draft" ? target.id : undefined;
  const draftSchedule = target?.kind === "draft" && target.schedule === true;
  useEffect(() => {
    if (draftId === undefined || key === undefined) return undefined;
    const closed = new AbortController();
    load(draftId, session.current.writes, session.current.settings).then(
      (draft) => {
        if (!closed.signal.aborted)
          setScreens({ key, screen: { kind: "compose", draft, id: draft.key, schedule: draftSchedule } });
      },
      () => {
        if (!closed.signal.aborted) setScreens({ key, screen: { kind: "drafts", back: undefined } });
      },
    );
    return () => {
      closed.abort();
    };
  }, [draftId, key, draftSchedule]);

  useEffect(() => {
    if ((discarded === undefined && !saved) || inline) return undefined;
    const timer = setTimeout(() => {
      setDiscarded(undefined);
      setSaved(false);
    }, undoMs);
    return () => {
      clearTimeout(timer);
    };
  }, [discarded, saved, inline]);

  const leave = (how: Leaving, d: Draft) => {
    left.current = true;
    const editing = d.target.kind === "edit";
    if (how === "posted" || how === "kept") {
      kept.settle(d, writes);
      if (editing) {
        setDiscarded(undefined);
        setSaved(true);
      }
    } else if (editing) {
      // An edit is no post of its own: left by accident it is kept here alone, and closed it goes without a word, the
      // post as it was.
      if (how === "dismiss") kept.set(targetKey(d.target), d);
      else kept.delete(targetKey(d.target));
    } else if (how === "dismiss") {
      if (written(d)) kept.keep(d, writes, settings);
    } else if (d.stored === true) {
      // A draft opened from the server stays there as it was last saved.
      kept.delete(targetKey(d.target));
    } else {
      kept.forget(d, writes);
      if (written(d)) setDiscarded({ ...d, saved: undefined });
    }
    close();
  };

  /** Opens the reader's draft of `id` in the composer once it is read; the post being written, `back`, is left. */
  const openDraft = (id: string, schedule: boolean, back: Draft | undefined) => {
    load(id, writes, settings).then(
      (draft) => {
        if (back !== undefined && back.saved?.id !== id && written(back)) kept.keep(back, writes, settings);
        show({ kind: "compose", draft, id: draft.key, schedule });
      },
      // Not read (deleted since): the list is read again, without it.
      () => undefined,
    );
  };

  const start =
    target &&
    target.kind !== "draft" &&
    (initial ?? (key && kept.get(key)) ?? emptyDraft(target, settings, crypto.randomUUID()));
  const composing =
    screen?.kind === "compose"
      ? screen
      : screen === undefined && start
        ? { kind: "compose" as const, draft: start, id: key ?? "", schedule: false }
        : undefined;
  const track = (d: Draft) => {
    latest.current = { key: targetKey(d.target), draft: d };
  };
  return (
    <>
      {/* A composer that was left leaves by its motion, then unmounts. */}
      <ModalPresence>
        {target && composing && (
          <Open
            // One composer per opening: it mounts once when opened, and a closed one comes back as a new one.
            key={composing.id}
            initial={composing.draft}
            layer={!inline}
            phone={phone}
            pickerSheet={pickerSheet}
            track={track}
            onLeave={leave}
            onDrafts={(d) => {
              show({ kind: "drafts", back: d });
            }}
            onNewer={(d) => {
              if (d.saved) openDraft(d.saved.id, false, undefined);
            }}
            phase={screen === undefined ? specimen.phase : undefined}
            opened={composing.schedule ? "schedule" : screen === undefined ? specimen.opened : undefined}
          />
        )}
        {target && screen?.kind === "drafts" && (
          <Drafts
            key="drafts"
            layer={!inline}
            phone={phone}
            onBack={() => {
              const back = screen.back;
              if (back === undefined) close();
              else show({ kind: "compose", draft: back, id: crypto.randomUUID(), schedule: false });
            }}
            onClose={() => {
              if (screen.back === undefined) close();
              else leave("close", screen.back);
            }}
            onDismiss={() => {
              if (screen.back === undefined) close();
              else leave("dismiss", screen.back);
            }}
            onOpen={(id, schedule) => {
              openDraft(id, schedule, screen.back);
            }}
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
      {saved && <UndoNotice inline={inline}>{t("compose.saved")}</UndoNotice>}
    </>
  );
}
