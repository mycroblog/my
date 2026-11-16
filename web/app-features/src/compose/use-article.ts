// SPDX-License-Identifier: AGPL-3.0-only

// The article editor's logic, without its view (ADR-0025 §4, ADR-0160): the article being written (article-model.ts),
// the preview of its body from the server once the writer pauses, the upload of its cover and of the images of its
// body, and its publication, or the edit of the reader's article. Publish judges the body's limits by the preview of
// the body as it is then, asked for at once (busy meanwhile) when the one shown is of an earlier body; it never passes
// or refuses on an earlier body's count. A publication the server refuses keeps what was
// written and says why in the server's own words, beside the input the server named; one that got no answer says so, and Publish sends it again with the
// same Idempotency-Key. As a post's, an article saved as a draft on the server (ADR-0156) is saved over it and published
// from it; one with a time is saved and scheduled instead, and Save as draft keeps it so; a save refused for a newer
// draft saved elsewhere is the conflict (§9), and a draft gone may have been published, so it is not saved again.

import {
  articleFieldLimits,
  useArticleLimits,
  useArticlePreview,
  useArticleWrites,
  useComposeSettings,
  useComposeWrites,
  type ArticleLimits,
  type ArticlePreview,
  type ArticlePreviewState,
  type ComposeSettings,
  type MediaAttachment,
  type SavedDraft,
  type Status,
} from "@my/app-data";
import { useEffect, useRef, useState } from "react";
import {
  articleMembers,
  articleProblems,
  imageMarkdown,
  publishable,
  toArticleDraft,
  toCreateArticle,
  toEditArticle,
  writtenArticle,
  type ArticleCover,
  type ArticleDraft,
  type ArticleProblems,
} from "./article-model";
import { failedPhase, type Phase } from "./use-composer";

/** How long the writer pauses before the preview is asked for again. */
const previewPauseMs = 600;

export type ArticleEditor = {
  readonly draft: ArticleDraft;
  readonly settings: ComposeSettings;
  readonly limits: ArticleLimits;
  /** The preview of the body (and of the summary made from it) as the server renders it. */
  readonly preview: ArticlePreviewState;
  readonly problems: ArticleProblems;
  readonly phase: Phase;
  /** Publish was pressed with a problem: the problems show. */
  readonly checked: boolean;
  /** How many images of the body are on their way to the server. */
  readonly uploading: number;
  /** An image of the body could not be uploaded. */
  readonly uploadFailed: boolean;
  readonly change: (patch: Partial<ArticleDraft>) => void;
  readonly setCover: (file: File) => void;
  readonly retryCover: () => void;
  readonly removeCover: () => void;
  readonly describeCover: (description: string) => void;
  readonly addTag: (tag: string) => void;
  readonly removeTag: (tag: string) => void;
  /** Uploads images for the body, and gives `insert` the Markdown of each once the server holds it. */
  readonly addImages: (files: readonly File[], insert: (markdown: string) => void) => void;
  /**
   * Publishes the article, or schedules it while it has a time, or saves the edit, or shows what keeps it from being
   * sent; `onPublished` hears the status, `onKept` the draft scheduled.
   */
  readonly submit: () => void;
  /** Keeps the article on the server as a draft as it is, with its time if it has one; `onKept` hears it. */
  readonly keep: () => void;
  /**
   * Saves a new article written or changed since the server last held it as a draft, without a word, before the editor
   * is left (Close, or another draft opened): a refusal or a newer draft elsewhere saves it as a new one.
   */
  readonly keepQuietly: () => Promise<void>;
};

/** What the editor tells of an article that left it. */
export type ArticleEvents = {
  /** It was published (or the edit saved). */
  readonly onPublished: (status: Status, draft: ArticleDraft) => void;
  /** It was kept on the server as a draft, or scheduled. */
  readonly onKept: (draft: ArticleDraft) => void;
};

/** Whether two times (ISO 8601), either of them none, are the same moment. */
const sameTime = (a: string | undefined, b: string | undefined) =>
  a === undefined || b === undefined ? a === b : Date.parse(a) === Date.parse(b);

/** `value`, once it has not changed for `ms`. */
function usePaused<T>(value: T, ms: number): T {
  const [settled, setSettled] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => {
      setSettled(value);
    }, ms);
    return () => {
      clearTimeout(timer);
    };
  }, [value, ms]);
  return settled;
}

/** The article editor of `initial`, which tells what left it to `events`. */
export function useArticleEditor(
  initial: ArticleDraft,
  { onPublished, onKept }: ArticleEvents,
  initialPhase: Phase = { kind: "editing" },
): ArticleEditor {
  const settings = useComposeSettings();
  const limits = useArticleLimits();
  const writes = useComposeWrites();
  const articles = useArticleWrites();
  const [draft, setDraft] = useState(initial);
  const [phase, setPhase] = useState<Phase>(initialPhase);
  const [checked, setChecked] = useState(false);
  const [uploading, setUploading] = useState(0);
  const [uploadFailed, setUploadFailed] = useState(false);
  const asked = usePaused(`${draft.text}\u0000${draft.summary}`, previewPauseMs);
  const [text = "", summary = ""] = asked.split("\u0000");
  const preview = useArticlePreview(text, summary);
  // The preview Publish asked for of the body as it was then, when the one shown was of an earlier body.
  const [judged, setJudged] = useState<{ text: string; summary: string; preview: ArticlePreview } | undefined>();
  // What the server holds of the article as a draft, to tell what leaving would lose: as it opened, then as each save
  // left it.
  const stored = useRef(JSON.stringify(toArticleDraft(initial, settings)));

  // An editor that was left hears nothing of what it sent: what left it decided already what becomes of the article.
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

  const change = (patch: Partial<ArticleDraft>) => {
    setDraft((d) => ({ ...d, ...patch }));
    if (phase.kind === "failed") setPhase({ kind: "editing" });
  };
  const changeCover = (patch: Partial<ArticleCover>) => {
    setDraft((d) => (d.cover === undefined ? d : { ...d, cover: { ...d.cover, ...patch } }));
  };

  /** Uploads `file` and waits until the server has processed it. */
  const upload = (file: File, onProcessing: () => void): Promise<MediaAttachment> =>
    writes.upload(file, { onProcessing });

  const uploadCover = (file: File) => {
    upload(file, () => {
      changeCover({ state: "processing" });
    }).then(
      (media) => {
        setDraft((d) =>
          d.cover?.file !== file
            ? d
            : { ...d, cover: { ...d.cover, state: "ready", id: media.id, preview: media.preview_url ?? media.url } },
        );
      },
      // Refused, unprocessable, or no answer: the cover offers to upload it again.
      () => {
        setDraft((d) => (d.cover?.file !== file ? d : { ...d, cover: { ...d.cover, state: "failed" } }));
      },
    );
  };

  /**
   * Saves `d` over the draft it was saved as, refused for a newer one saved elsewhere (ADR-0156 §9), then, with
   * `schedule`, sets, moves or takes back its schedule to match its time.
   */
  const store = async (d: ArticleDraft, schedule: boolean): Promise<SavedDraft> => {
    const hold = (s: SavedDraft) => {
      setDraft((x) => ({ ...x, saved: s }));
      return s;
    };
    const body = toArticleDraft(d, settings);
    const saved = hold(await writes.saveDraft(body, d.saved, { onConflict: "refuse" }));
    stored.current = JSON.stringify(body);
    if (!schedule || sameTime(d.scheduledAt, saved.scheduledAt)) return saved;
    return hold(
      d.scheduledAt === undefined ? await writes.unschedule(saved.id) : await writes.schedule(saved.id, d.scheduledAt),
    );
  };
  const refused = whileOpen((e: unknown) => {
    setPhase(failedPhase(e, articleMembers));
  });
  const kept = whileOpen(onKept);
  /** The preview of `d`'s body and summary as they are, when one is at hand: never an earlier body's. */
  const previewOf = (d: ArticleDraft) => {
    const same = (of: { text: string; summary: string } | undefined) =>
      of !== undefined && of.text === d.text && of.summary === d.summary;
    if (same(preview.of)) return preview.preview;
    return same(judged) ? judged?.preview : undefined;
  };
  const problems = articleProblems(draft, previewOf(draft));

  /** Sends `d`, once checked against the preview of its body as it is now. */
  const send = (d: ArticleDraft) => {
    if (phase.kind === "posting") return;
    // An edit keeps its hashtags and title checked too; a schedule is checked again when it is published.
    const judge = (counted: ArticlePreview | undefined) => {
      if (publishable(articleProblems(d, counted))) return true;
      setChecked(true);
      return false;
    };
    // What the body's count does not change refuses at once.
    if (!judge(undefined)) return;
    // The limits are the server's count of this very body: the preview shown may be of an earlier one (it waits for a
    // pause, then for the server), so Publish asks for this body's at once when it has none, busy meanwhile. One the
    // server could not render is left to the publication, which the server checks too.
    const at = previewOf(d);
    if (at !== undefined || d.text.trim() === "") {
      if (judge(at)) deliver(d);
      return;
    }
    const before = phase;
    setPhase({ kind: "posting" });
    void preview
      .now(d.text, d.summary)
      .then(
        (p) => {
          setJudged({ text: d.text, summary: d.summary, preview: p });
          return p;
        },
        () => undefined,
      )
      .then((counted) => {
        if (!mounted.current) return;
        if (judge(counted)) deliver(d);
        else setPhase(before);
      });
  };

  /** Publishes `d`, or schedules it while it has a time, or saves the edit. */
  const deliver = (d: ArticleDraft) => {
    setPhase({ kind: "posting" });
    const { target } = d;
    if (d.scheduledAt !== undefined && target.kind !== "edit") {
      store(d, true).then(() => {
        kept(d);
      }, refused);
      return;
    }
    const sent =
      target.kind === "edit"
        ? articles.edit(target.id, toEditArticle(d))
        : d.saved === undefined
          ? articles.post(toCreateArticle(d, settings), d.key)
          : store(d, false).then((saved) => writes.publish(saved.id));
    sent.then((status) => {
      if (mounted.current) onPublished(status, d);
    }, refused);
  };

  return {
    draft,
    settings,
    limits,
    preview,
    problems,
    phase,
    checked,
    uploading,
    uploadFailed,
    change,
    setCover: (file) => {
      setDraft((d) => ({
        ...d,
        cover: {
          file,
          preview: file.type.startsWith("image/") ? URL.createObjectURL(file) : undefined,
          state: "uploading",
          description: d.cover?.description ?? "",
        },
      }));
      uploadCover(file);
    },
    retryCover: () => {
      const file = draft.cover?.file;
      if (file === undefined) return;
      changeCover({ state: "uploading" });
      uploadCover(file);
    },
    removeCover: () => {
      setDraft((d) => ({ ...d, cover: undefined }));
    },
    describeCover: (description) => {
      changeCover({ description });
    },
    addTag: (tag) => {
      setDraft((d) =>
        d.tags.length >= articleFieldLimits.tags || d.tags.some((x) => x.toLowerCase() === tag.toLowerCase())
          ? d
          : { ...d, tags: [...d.tags, tag] },
      );
    },
    removeTag: (tag) => {
      setDraft((d) => ({ ...d, tags: d.tags.filter((x) => x !== tag) }));
    },
    addImages: (files, insert) => {
      setUploadFailed(false);
      for (const file of files) {
        setUploading((n) => n + 1);
        upload(file, () => undefined)
          .then(
            (media) => {
              if (mounted.current) insert(imageMarkdown(media.id, media.description ?? ""));
            },
            () => {
              if (mounted.current) setUploadFailed(true);
            },
          )
          .finally(() => {
            if (mounted.current) setUploading((n) => n - 1);
          });
      }
    },
    submit: () => {
      if (phase.kind !== "gone") {
        send(draft);
        return;
      }
      // The draft gone is let go: what is written is published now as an article of its own, under a key of its own.
      const fresh: ArticleDraft = {
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
    keepQuietly: async () => {
      if (draft.target.kind === "edit" || phase.kind === "posting" || !writtenArticle(draft)) return;
      const body = toArticleDraft(draft, settings);
      if (JSON.stringify(body) === stored.current) return;
      await writes.saveDraft(body, draft.saved, { onConflict: "new" }).catch(() => undefined);
    },
  };
}
