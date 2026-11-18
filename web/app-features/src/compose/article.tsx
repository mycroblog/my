// SPDX-License-Identifier: AGPL-3.0-only

// The article editor (FR-122, FR-123, ADR-0160, #3091's settled layout): a screen of its own, not the composer, opened
// for a new article, for one of the reader's drafts of an article, or for the reader's published article. Its head
// holds its title, the drafts' menu (keep it as a draft, or open the drafts of articles) and close; what is written is
// the title, the Markdown body in its editor (fetched as a chunk of its own), and after it the article's details (the
// summary, the cover and the hashtags); the article as the server will publish it stands beside it from `desktop`, and
// behind Write | Preview under it, from `POST /articles/preview` once the writer pauses, with the body's limits under
// what is written before they are hit. The row at its end holds the tools (an image for the body, the composer's
// audience menu of who may see it, never direct, and who may reply, boost and quote it; sensitive, the schedule, the
// language) and Publish, Schedule or Save. An edit keeps who may see it and interact with it, and is never a draft.
//
// Leaving: Publish (or Save, an edit) tells the app the status; Schedule, or Save as draft, keeps it on the server and
// tells the app it was kept; Close keeps a new article that was written or changed as a draft on the server, without a
// word, so that a long text is never lost, and leaves an edit as it was. A reader the server keeps from writing new
// articles (`article_authoring`) gets the one sentence why in place of the form; an edit of an earlier one still opens.

import { PostContent, scheduleTime, ScheduleTime, ScheduleZone } from "@my/app-components";
import {
  articleFieldLimits,
  useArticleAuthoring,
  useComposeSettings,
  useComposeWrites,
  useArticleSource,
  useDrafts,
  useStatus,
  type ComposeSettings,
  type ComposeWrites,
  type Status,
} from "@my/app-data";
import {
  ArticleDetails,
  ArticleField,
  ArticleFrame,
  ArticleLimits,
  ArticleMode,
  ArticlePane,
  ArticlePreview,
  ArticleTextArea,
  ArticleTitleInput,
  AttachButton,
  Button,
  CaptionLine,
  CharCount,
  ComposerText,
  CoverAdd,
  CoverRemove,
  CoverRow,
  FeedNotice,
  FormAlert,
  FormText,
  IconButton,
  MenuButton,
  MenuGroup,
  MenuItem,
  ModalPresence,
  TagInput,
  TextButton,
  useLocale,
  useTimeZone,
} from "@my/design-system";
import type { EditorHandle, PlainTextEditorProps } from "@my/design-system/editor";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { fieldError } from "../field-errors";
import { stringsIn } from "../locale";
import { formatParts } from "../slots";
import {
  articleFromServer,
  articleFromSource,
  articleVisibilities,
  charsOf,
  emptyArticle,
  lengthRatio,
  limitState,
  usage,
  type ArticleDraft,
} from "./article-model";
import { AudienceTool, visibilityIcon, type AudiencePanel } from "./audience";
import { ChunkBoundary, retryingLazy } from "./chunk";
import { DraftsView } from "./drafts";
import { Language, Schedule } from "./tools";
import { useArticleEditor, type ArticleEditor } from "./use-article";
import type { Phase } from "./use-composer";

type T = ReturnType<typeof stringsIn>["t"];

/** The body's editor, a chunk of its own with Lexical, fetched as the editor first shows. */
const lazyEditor = retryingLazy<PlainTextEditorProps>(() =>
  import("@my/design-system/editor").then((m) => m.PlainTextEditor),
);

/** The editor of the body: drawn as its chunk comes; one that could not be fetched says so, with Try again. */
function BodyEditor(props: PlainTextEditorProps & { t: T }) {
  const { t, ...rest } = props;
  const [Editor] = useState(() => lazyEditor.get());
  return (
    <ChunkBoundary
      onRetry={lazyEditor.retry}
      fallback={(retry) => (
        <FeedNotice alert start action={<TextButton onPress={retry}>{t("compose.load_retry")}</TextButton>}>
          {t("compose.editor_failed")}
        </FeedNotice>
      )}
    >
      <Editor {...rest} />
    </ChunkBoundary>
  );
}

/** The form's error before the action: the first thing that keeps the article from being sent. */
function alertOf(e: ArticleEditor, t: T, locale: string, timeZone: string | undefined): string | undefined {
  const { phase, problems, draft } = e;
  if (phase.kind === "conflict") return t("compose.conflict");
  if (phase.kind === "gone") return t("compose.gone");
  if (phase.kind === "failed")
    return phase.reason ?? t(draft.target.kind === "edit" ? "compose.save_failed" : "compose.article.failed");
  if (draft.failure !== undefined && draft.scheduledAt === undefined)
    return t("compose.schedule.failed.next", {
      failure: t("compose.schedule.failed", {
        time: scheduleTime(draft.failure.publishAt, locale, timeZone),
        reason: t(`compose.schedule.failed.${draft.failure.reason}`),
      }),
    });
  if (!e.checked) return undefined;
  if (problems.titleMissing || problems.bodyMissing) return t("compose.article.missing");
  if (problems.titleOver > 0 || problems.summaryOver > 0 || problems.bytesOver || problems.imagesOver)
    return t("compose.article.over");
  if (problems.coverWaiting || problems.coverFailed) return t("compose.article.cover.waiting");
  return undefined;
}

/** A count of a field near its limit, past it negative; none until it is near. */
function Count({ left, near, t }: { left: number; near: number; t: T }) {
  if (left > near) return undefined;
  return (
    <CharCount
      left={left}
      near={near}
      label={
        left < 0 ? t("compose.article.field.over", { count: -left }) : t("compose.article.field.left", { count: left })
      }
    />
  );
}

export type ArticleViewProps = {
  e: ArticleEditor;
  /** Close: the reader means to leave. */
  onClose: () => void;
  /** Opens the list of the reader's drafts of articles. */
  onDrafts?: (() => void) | undefined;
  /** Opens the newer draft saved elsewhere, in place of this one (a conflict). */
  onNewer?: (() => void) | undefined;
  /**
   * What it opens with: what is written, or the preview (under `desktop`), and the schedule's form or a panel of the
   * audience menu open.
   */
  mode?: "write" | "preview" | undefined;
  opened?: "schedule" | AudiencePanel | undefined;
};

/** The article editor's screen, for the state of `e` (use-article.ts). */
export function ArticleView({ e, onClose, onDrafts, onNewer, mode: initialMode, opened }: ArticleViewProps) {
  const { locale } = useLocale();
  const timeZone = useTimeZone();
  const { t, lang } = stringsIn(locale);
  const id = useId();
  const bodyRef = useRef<EditorHandle>(null);
  const [mode, setMode] = useState<"write" | "preview">(initialMode ?? "write");
  const [open, setOpen] = useState<AudiencePanel | "schedule" | "language" | undefined>(opened);
  const audienceOpen = open === "language" || open === "schedule" ? undefined : open;
  const { draft, settings, problems, preview, phase } = e;
  const busy = phase.kind === "posting";
  const gone = phase.kind === "gone";
  const editing = draft.target.kind === "edit";
  const scheduled = draft.scheduledAt;
  const shown = preview.preview;
  const used = usage(shown, e.limits);
  const titleLeft = articleFieldLimits.title - charsOf(draft.title);
  const summaryLeft = articleFieldLimits.summary - charsOf(draft.summary);
  const tagsLeft = articleFieldLimits.tags - draft.tags.length;
  const textLang = draft.language === "" ? undefined : draft.language;
  const alert = alertOf(e, t, locale, timeZone);
  // What the server refused of an input it named, beside that input (field-errors.ts); the preview gives way to what
  // is written, which holds it.
  const refused = phase.kind === "failed" ? phase : undefined;
  const placed = refused?.field !== undefined;
  useEffect(() => {
    if (placed) setMode("write");
  }, [placed]);
  const bodyError = e.checked && problems.bodyMissing ? t("compose.article.body.missing") : fieldError(refused, "text");
  const summaryError =
    summaryLeft < 0
      ? t("compose.article.summary.over", { max: articleFieldLimits.summary })
      : fieldError(refused, "summary");
  const setSchedule = (o: boolean) => {
    setOpen(o ? "schedule" : undefined);
  };

  const title = (
    <ArticleTitleInput
      id={`${id}-title`}
      label={t("compose.article.field.title")}
      placeholder={t("compose.article.field.title")}
      value={draft.title}
      readOnly={busy}
      onChange={(value) => {
        e.change({ title: value });
      }}
      count={<Count left={titleLeft} near={20} t={t} />}
      error={
        titleLeft < 0
          ? t("compose.article.field.title.over", { max: articleFieldLimits.title })
          : e.checked && problems.titleMissing
            ? t("compose.article.field.title.missing")
            : fieldError(refused, "title")
      }
    />
  );
  const body = (
    <ComposerText error={bodyError} errorId={`${id}-body-error`}>
      <BodyEditor
        t={t}
        namespace="article"
        look="composer"
        roomy
        value={draft.text}
        onChange={(text) => {
          e.change({ text });
        }}
        label={t("compose.article.body")}
        placeholder={t("compose.article.body.placeholder")}
        readOnly={busy}
        invalid={bodyError !== undefined}
        describedBy={bodyError === undefined ? undefined : `${id}-body-error`}
        handle={bodyRef}
      />
    </ComposerText>
  );
  const uploads =
    e.uploading > 0 ? (
      <CaptionLine icon="attach" busy>
        {t("compose.article.attach.uploading", { count: e.uploading })}
      </CaptionLine>
    ) : e.uploadFailed ? (
      <CaptionLine icon="error" alert>
        {t("compose.article.attach.failed")}
      </CaptionLine>
    ) : undefined;
  const summary = (
    <ArticleField
      id={`${id}-summary`}
      label={t("compose.article.summary")}
      hint={t("compose.article.summary.hint")}
      end={<Count left={summaryLeft} near={50} t={t} />}
      error={summaryError}
    >
      <ArticleTextArea
        label={t("compose.article.summary")}
        placeholder={t("compose.article.summary.placeholder")}
        value={draft.summary}
        readOnly={busy}
        onChange={(value) => {
          e.change({ summary: value });
        }}
        invalid={summaryError !== undefined}
        describedBy={summaryError === undefined ? `${id}-summary-hint` : `${id}-summary-hint ${id}-summary-error`}
      />
    </ArticleField>
  );
  const coverDraft = draft.cover;
  const cover = (
    <ArticleField
      id={`${id}-cover`}
      label={t("compose.article.cover")}
      error={fieldError(refused, "cover")}
      end={
        coverDraft === undefined ? undefined : (
          <CoverRemove label={t("compose.article.cover.remove")} onPress={e.removeCover} />
        )
      }
    >
      {coverDraft === undefined ? (
        <CoverAdd label={t("compose.article.cover.add")} onFile={e.setCover} />
      ) : (
        <>
          <CoverRow
            id={`${id}-cover-description`}
            src={coverDraft.preview}
            description={coverDraft.description}
            onDescription={e.describeCover}
            descriptionLabel={t("compose.article.cover.description")}
            placeholder={t("compose.article.cover.description.hint")}
            status={
              coverDraft.state === "uploading"
                ? { text: t("compose.uploading"), busy: true }
                : coverDraft.state === "processing"
                  ? { text: t("compose.processing"), busy: true }
                  : coverDraft.state === "failed"
                    ? { text: t(coverDraft.file ? "compose.upload_failed" : "compose.media_failed"), busy: false }
                    : undefined
            }
          />
          {coverDraft.state === "failed" && coverDraft.file !== undefined && (
            <span>
              <TextButton onPress={e.retryCover}>{t("compose.retry")}</TextButton>
            </span>
          )}
        </>
      )}
    </ArticleField>
  );
  const tags = (
    <ArticleField
      id={`${id}-tags`}
      label={t("compose.article.tags")}
      error={fieldError(refused, "tags")}
      end={
        tagsLeft <= 2 ? (
          <CharCount left={tagsLeft} near={2} label={t("compose.article.tags.left", { count: tagsLeft })} />
        ) : undefined
      }
    >
      <TagInput
        label={t("compose.article.tags")}
        items={draft.tags}
        removeLabel={(tag) => t("compose.article.tags.remove", { tag })}
        placeholder={t(tagsLeft <= 0 ? "compose.article.tags.full" : "compose.article.tags.placeholder")}
        full={tagsLeft <= 0}
        onAdd={e.addTag}
        onRemove={e.removeTag}
      />
    </ArticleField>
  );
  const writing = (
    <>
      {title}
      {body}
      {uploads}
      <ArticleDetails id={`${id}-details`} heading={t("compose.article.details")}>
        {summary}
        {cover}
        {tags}
      </ArticleDetails>
    </>
  );
  const coverShown =
    coverDraft?.preview === undefined ? undefined : { src: coverDraft.preview, alt: coverDraft.description };
  const summaryShown = shown?.summary ?? draft.summary;
  const published = (
    <ArticlePreview
      title={draft.title}
      summary={summaryShown === "" ? undefined : summaryShown}
      cover={coverShown}
      empty={t("compose.article.preview.empty")}
      lang={textLang}
      busy={preview.busy}
      notice={
        preview.failed ? (
          <FeedNotice alert start action={<TextButton onPress={preview.retry}>{t("compose.load_retry")}</TextButton>}>
            {t("compose.article.preview.failed")}
          </FeedNotice>
        ) : undefined
      }
    >
      {shown === undefined ? undefined : <PostContent html={shown.content} lang={textLang} article />}
    </ArticlePreview>
  );
  const modes = [
    { id: "write" as const, name: t("compose.article.mode.write") },
    { id: "preview" as const, name: t("compose.article.mode.preview") },
  ];
  // The drafts, a place apart from publishing this article, in the head: keep it as a draft, or open the list. An
  // edit is no draft: it is saved at once, and never scheduled.
  const draftsMenu = !editing && (
    <MenuButton icon="drafts" label={t("compose.later")} align="end">
      <MenuGroup>
        <MenuItem id="save" icon="saveDraft" onAction={e.keep}>
          {t("compose.drafts.save")}
        </MenuItem>
        <MenuItem id="drafts" icon="drafts" onAction={onDrafts}>
          {t("compose.drafts.open")}
        </MenuItem>
      </MenuGroup>
    </MenuButton>
  );
  return (
    <ArticleFrame
      title={t(editing ? "compose.article.title.edit" : "compose.article.title.new")}
      closeLabel={t("compose.close")}
      onClose={onClose}
      headEnd={draftsMenu || undefined}
      preview={{ caption: t("compose.article.preview.caption"), children: published }}
      limits={
        // Counted only by the server's preview: while a written body has none (it renders, or it failed), no figure.
        shown === undefined && draft.text.trim() !== "" ? undefined : (
          <ArticleLimits
            items={[
              {
                id: "bytes",
                text: t("compose.article.limit.bytes", { ratio: lengthRatio(used.bytes, used.maxBytes) }),
                state: limitState(used.bytes, used.maxBytes),
              },
              {
                id: "images",
                text: t("compose.article.limit.images", { used: used.images, max: used.maxImages }),
                state: limitState(used.images, used.maxImages),
              },
            ]}
          />
        )
      }
      note={
        scheduled !== undefined ? (
          <CaptionLine
            icon="schedule"
            lang={lang}
            end={
              <TextButton
                onPress={() => {
                  setSchedule(true);
                }}
              >
                {t("compose.schedule.change")}
              </TextButton>
            }
          >
            {open === "schedule"
              ? formatParts(t, "compose.schedule.at", {}, { time: <ScheduleTime date={scheduled} /> })
              : formatParts(
                  t,
                  "compose.schedule.at_zone",
                  {},
                  { time: <ScheduleTime date={scheduled} />, zone: <ScheduleZone /> },
                )}
          </CaptionLine>
        ) : editing ? (
          <CaptionLine icon={visibilityIcon[draft.visibility]} lang={lang}>
            {t("compose.edit.fixed", { visibility: t(`compose.visibility.${draft.visibility}`) })}
          </CaptionLine>
        ) : undefined
      }
      alert={
        alert === undefined ? undefined : (
          <FormAlert
            regular={gone}
            action={
              phase.kind === "conflict" ? (
                <TextButton onPress={() => onNewer?.()}>{t("compose.conflict.open")}</TextButton>
              ) : undefined
            }
          >
            {alert}
          </FormAlert>
        )
      }
      tools={
        <>
          <AttachButton
            label={t("compose.article.attach")}
            accept="image/*"
            isDisabled={busy}
            onFiles={(files) => {
              e.addImages(files, (markdown) => {
                // Each image a paragraph of its own, where the caret was.
                bodyRef.current?.insert(
                  (before) =>
                    `${before === "" || before.endsWith("\n\n") ? "" : before.endsWith("\n") ? "\n" : "\n\n"}${markdown}\n\n`,
                  { focus: false },
                );
              });
            }}
          />
          {!editing && (
            <AudienceTool
              visibility={draft.visibility}
              policy={draft.policy}
              visibilities={articleVisibilities}
              onChange={e.change}
              domain={settings.domain}
              t={t}
              open={audienceOpen}
              setOpen={setOpen}
            />
          )}
          <IconButton
            icon="hidden"
            label={t(draft.sensitive ? "compose.article.sensitive.off" : "compose.article.sensitive.on")}
            pressed={draft.sensitive}
            onPress={() => {
              e.change({ sensitive: !draft.sensitive });
            }}
          />
          {!editing && (
            <Schedule
              at={scheduled}
              limits={settings.schedule}
              onSet={(scheduledAt) => {
                e.change({ scheduledAt });
              }}
              t={t}
              locale={locale}
              open={open === "schedule"}
              setOpen={setSchedule}
              trigger={
                <IconButton
                  icon="schedule"
                  label={t(scheduled === undefined ? "compose.schedule.tool" : "compose.schedule.tool_set")}
                  pressed={scheduled !== undefined}
                />
              }
            />
          )}
          <Language
            current={draft.language}
            preferred={settings.language}
            lang={lang}
            onChoose={(language) => {
              e.change({ language });
            }}
            t={t}
            open={open === "language"}
            setOpen={(o) => {
              setOpen(o ? "language" : undefined);
            }}
          />
        </>
      }
      action={
        <Button kind="primary" size="timeline" busy={busy} onPress={e.submit}>
          {t(
            editing
              ? "compose.save"
              : gone
                ? "compose.article.gone.publish"
                : scheduled === undefined
                  ? "compose.article.publish"
                  : "compose.schedule.post",
          )}
        </Button>
      }
    >
      <ArticleMode label={t("compose.article.mode")} items={modes} current={mode} onChange={setMode} />
      <ArticlePane shown={mode === "write" ? "always" : "desktop"}>{writing}</ArticlePane>
      <ArticlePane shown={mode === "preview" ? "narrow" : "never"}>{published}</ArticlePane>
    </ArticleFrame>
  );
}

/** What the app opens the article editor for: a new article, a draft of one on the server, or the reader's article. */
export type ArticleOpening = { kind: "new" } | { kind: "draft"; id: string } | { kind: "edit"; id: string };

/** How the article editor was left: published (or the edit saved), kept on the server, or closed. */
export type ArticleLeaving = { kind: "published"; status: Status } | { kind: "kept" } | { kind: "closed" };

/** What a story's specimen of the editor opens with. */
export type ArticleSpecimen = {
  readonly initial?: ArticleDraft | undefined;
  readonly phase?: Phase | undefined;
  readonly mode?: "write" | "preview" | undefined;
  readonly opened?: "schedule" | "drafts" | AudiencePanel | undefined;
};

export type ArticleEditorProps = {
  opening: ArticleOpening;
  /** It was left: the app goes to the article published, or back. */
  onLeave: (how: ArticleLeaving) => void;
  /** A draft of an article was chosen in the drafts' list: the app opens the editor for it. */
  onOpenDraft: (id: string) => void;
  specimen?: ArticleSpecimen | undefined;
};

/** Reads the reader's draft of `id` from the server, with its cover, as the editor's article. */
async function loadDraft(id: string, writes: ComposeWrites, settings: ComposeSettings): Promise<ArticleDraft> {
  const { draft, saved } = await writes.readDraft(id);
  const coverId = draft.article?.cover?.id;
  const cover = coverId === undefined ? undefined : await writes.media(coverId).catch(() => undefined);
  return articleFromServer(draft, saved, cover, settings, crypto.randomUUID());
}

/** The editor's frame with one notice in place of the form: it loads, it failed, or the reader may not write. */
function Notice({ editing, onClose, children }: { editing: boolean; onClose: () => void; children: ReactNode }) {
  const { t } = stringsIn(useLocale().locale);
  return (
    <ArticleFrame
      title={t(editing ? "compose.article.title.edit" : "compose.article.title.new")}
      closeLabel={t("compose.close")}
      onClose={onClose}
    >
      {children}
    </ArticleFrame>
  );
}

/** The reader's drafts of articles, over the editor, as the server lists them. */
function ArticleDrafts({ onBack, onOpen }: { onBack: () => void; onOpen: (id: string) => void }) {
  const drafts = useDrafts();
  const lists = drafts.lists;
  const state = lists
    ? {
        kind: "ready" as const,
        drafts: lists.drafts.filter((d) => d.article !== undefined),
        scheduled: lists.scheduled.filter((d) => d.article !== undefined),
      }
    : drafts.failed
      ? { kind: "failed" as const }
      : { kind: "loading" as const };
  return (
    <DraftsView
      state={state}
      phone="dialog"
      onBack={onBack}
      onClose={onBack}
      onDismiss={onBack}
      onRetry={drafts.retry}
      onOpen={onOpen}
      onUnschedule={(id) => {
        drafts.unschedule(id).catch(() => undefined);
      }}
      onDelete={(id) => {
        drafts.remove(id).catch(() => undefined);
      }}
    />
  );
}

/** The editor open on `initial`, with the drafts' list over it when asked for. */
function Open({
  initial,
  onLeave,
  onOpenDraft,
  specimen,
}: {
  initial: ArticleDraft;
  onLeave: (how: ArticleLeaving) => void;
  onOpenDraft: (id: string) => void;
  specimen: ArticleSpecimen;
}) {
  const [drafts, setDrafts] = useState(specimen.opened === "drafts");
  const e = useArticleEditor(
    initial,
    {
      onPublished: (status) => {
        onLeave({ kind: "published", status });
      },
      onKept: () => {
        onLeave({ kind: "kept" });
      },
    },
    specimen.phase,
  );
  return (
    <>
      <ArticleView
        e={e}
        mode={specimen.mode}
        opened={specimen.opened === "drafts" ? undefined : specimen.opened}
        onClose={() => {
          void e.keepQuietly().then(() => {
            onLeave({ kind: "closed" });
          });
        }}
        onDrafts={() => {
          setDrafts(true);
        }}
        onNewer={() => {
          if (e.draft.saved) onOpenDraft(e.draft.saved.id);
        }}
      />
      <ModalPresence>
        {drafts && (
          <ArticleDrafts
            key="drafts"
            onBack={() => {
              setDrafts(false);
            }}
            onOpen={(id) => {
              setDrafts(false);
              if (id === e.draft.saved?.id) return;
              void e.keepQuietly().then(() => {
                onOpenDraft(id);
              });
            }}
          />
        )}
      </ModalPresence>
    </>
  );
}

/**
 * The article editor for `opening`: a new article at once, a draft or the reader's article once read from the server
 * (that it loads, or failed with Try again, in the form's place), or why the reader may not write a new one.
 */
export function ArticleEditor({ opening, onLeave, onOpenDraft, specimen = {} }: ArticleEditorProps) {
  const { t } = stringsIn(useLocale().locale);
  const settings = useComposeSettings();
  const writes = useComposeWrites();
  const editing = opening.kind === "edit";
  const allowed = useArticleAuthoring(!editing);
  const editId = opening.kind === "edit" ? opening.id : undefined;
  const source = useArticleSource(editId);
  const { status, failed: statusFailed } = useStatus(editId);
  const draftId = opening.kind === "draft" ? opening.id : undefined;
  const [loaded, setLoaded] = useState<{ id: string; draft: ArticleDraft } | { id: string; failed: true }>();
  const [attempt, setAttempt] = useState(0);
  const session = useRef({ writes, settings });
  useEffect(() => {
    session.current = { writes, settings };
  });
  useEffect(() => {
    if (draftId === undefined) return undefined;
    const closed = new AbortController();
    loadDraft(draftId, session.current.writes, session.current.settings).then(
      (draft) => {
        if (!closed.signal.aborted) setLoaded({ id: draftId, draft });
      },
      () => {
        if (!closed.signal.aborted) setLoaded({ id: draftId, failed: true });
      },
    );
    return () => {
      closed.abort();
    };
  }, [draftId, attempt]);
  // A new article starts once, from the settings as they are when it opens.
  const [fresh] = useState(() => emptyArticle(settings, crypto.randomUUID()));
  const close = () => {
    onLeave({ kind: "closed" });
  };
  const notice = (children: ReactNode) => (
    <Notice editing={editing} onClose={close}>
      {children}
    </Notice>
  );
  const loading = notice(<FeedNotice>{t("compose.article.loading")}</FeedNotice>);
  const failedNotice = (retry: () => void) =>
    notice(
      <FeedNotice alert start action={<TextButton onPress={retry}>{t("compose.load_retry")}</TextButton>}>
        {t("compose.article.load_failed")}
      </FeedNotice>,
    );

  if (!editing && allowed === false) return notice(<FormText>{t("compose.article.not_allowed")}</FormText>);
  if (!editing && allowed === undefined && specimen.initial === undefined) return loading;

  let initial: ArticleDraft | undefined = specimen.initial;
  if (initial === undefined && opening.kind === "new") initial = fresh;
  if (initial === undefined && opening.kind === "draft") {
    if (loaded === undefined || loaded.id !== opening.id) return loading;
    if ("failed" in loaded)
      return failedNotice(() => {
        setLoaded(undefined);
        setAttempt((n) => n + 1);
      });
    initial = loaded.draft;
  }
  if (initial === undefined && opening.kind === "edit") {
    if (source.failed || statusFailed) return failedNotice(source.retry);
    if (source.source === undefined || status === undefined) return loading;
    initial = articleFromSource(status, source.source, settings, crypto.randomUUID());
  }
  if (initial === undefined) return loading;
  const key = opening.kind === "new" ? "new" : `${opening.kind}:${opening.id}`;
  return <Open key={key} initial={initial} onLeave={onLeave} onOpenDraft={onOpenDraft} specimen={specimen} />;
}
