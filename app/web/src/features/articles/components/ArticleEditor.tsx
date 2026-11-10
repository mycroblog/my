// SPDX-License-Identifier: AGPL-3.0-only
// The article editor (web-client/screens/article-editor.md): the form beside the preview the server renders, side by
// side in a wide column and one above the other in a narrow one. Its logic is useArticleEditor.
import {
  Box,
  Button,
  Callout,
  Checkbox,
  Cluster,
  EmptyState,
  FileDropZone,
  FileTrigger,
  FormActionBar,
  Grid,
  Hint,
  InlineError,
  Link,
  PreviewImage,
  SaveIndicator,
  type SaveState,
  Select,
  SelectItem,
  Stack,
  Text,
  TextArea,
  type TextAreaProps,
  TextField,
} from "@my/ui";
import type { ClipboardEvent, SubmitEvent } from "react";
import { useIntl } from "react-intl";
import { errorMessage } from "../../../lib/errors";
import { useT } from "../../../lib/i18n";
import {
  acceptedImageTypes,
  SchedulePicker,
  useCanWriteArticles,
  useFormatRemaining,
  visibilityLabel,
} from "../../compose";
import { maxSummaryLength, maxTitleLength } from "../hooks/articleFields";
import type { AutosaveStatus } from "../hooks/autosaver";
import { type ArticleEditor as Editor, type EditorStart, useArticleEditor } from "../hooks/useArticleEditor";
import { useBlankStart, useDraftStart, useEditStart } from "../hooks/useEditorStart";
import { ArticlePreview } from "./ArticlePreview";

/** The key event of the body, as React Aria gives it. */
type BodyKeyEvent = Parameters<NonNullable<TextAreaProps["onKeyDown"]>>[0];

export type NewArticleProps = {
  /** The draft of an article to go on writing (from the drafts page); none starts a new one. */
  draftId: string | undefined;
  /** The id of the page's heading, which names the form. */
  labelledBy: string;
};

/** A new article, from nothing or from its draft; the server's setting may keep the viewer from writing one. */
export function NewArticle({ draftId, labelledBy }: NewArticleProps) {
  const t = useT();
  const canWrite = useCanWriteArticles();
  if (!canWrite) {
    return (
      <EmptyState
        heading={t("client:articles.not_allowed.title")}
        description={t("client:articles.not_allowed.body")}
      />
    );
  }
  // Another draft is another article: nothing of the one before carries over (web-client/README.md R10).
  return draftId === undefined ? (
    <BlankArticle labelledBy={labelledBy} />
  ) : (
    <DraftArticle key={draftId} id={draftId} labelledBy={labelledBy} />
  );
}

function BlankArticle({ labelledBy }: { labelledBy: string }) {
  return <EditorForm start={useBlankStart()} labelledBy={labelledBy} />;
}

function DraftArticle({ id, labelledBy }: { id: string; labelledBy: string }) {
  return <EditorForm start={useDraftStart(id)} labelledBy={labelledBy} />;
}

/** A published article of the viewer, edited in place. */
export function EditArticle({ id, labelledBy }: { id: string; labelledBy: string }) {
  return <EditorForm key={id} start={useEditStart(id)} labelledBy={labelledBy} />;
}

/** The SaveIndicator's state of the autosave's status. */
function saveState(status: AutosaveStatus): SaveState {
  switch (status.kind) {
    case "idle":
      return "idle";
    case "saving":
      return "saving";
    case "saved":
      return "saved";
    case "failed":
    case "conflict":
      return "failed";
  }
}

function EditorForm({ start, labelledBy }: { start: EditorStart; labelledBy: string }) {
  const t = useT();
  const intl = useIntl();
  const formatRemaining = useFormatRemaining();
  const editor = useArticleEditor(start);
  const { fields } = editor;
  const onSubmit = (event: SubmitEvent<HTMLFormElement>) => {
    event.preventDefault();
    editor.publish();
  };
  // Ctrl+B (Cmd+B on macOS) makes the selection bold and Ctrl+K a link, as the buttons do; a key of an IME conversion
  // is not one of them.
  const onBodyKeyDown = (event: BodyKeyEvent) => {
    if (!(event.ctrlKey || event.metaKey) || event.altKey || event.shiftKey || event.nativeEvent.isComposing) return;
    const key = event.key.toLowerCase();
    if (key === "b") {
      event.preventDefault();
      editor.bold();
    } else if (key === "k") {
      event.preventDefault();
      editor.link();
    }
  };
  // Pasted image files go into the body as dropped ones do; a clipboard that also holds text pastes only the text.
  const onBodyPaste = (event: ClipboardEvent) => {
    const files = Array.from(event.clipboardData.files);
    if (files.length === 0 || Array.from(event.clipboardData.types).includes("text/plain")) return;
    event.preventDefault();
    editor.addImages(files);
  };
  const autosave = editor.autosave;
  const scheduledAt = autosave?.link?.scheduledAt ?? null;
  return (
    <Stack gap="6" container>
      <Grid columns={2} at="2xl" gap="6">
        <Stack as="form" gap="4" aria-labelledby={labelledBy} onSubmit={onSubmit}>
          <TextField
            label={t("client:articles.field.title")}
            value={fields.title}
            onChange={(title) => {
              editor.edit({ title });
            }}
            maxLength={maxTitleLength}
            isInvalid={editor.invalid === "title"}
            {...(editor.invalid === "title" && { errorMessage: t("client:articles.blocked.title") })}
            autoFocus={start.mode === "new" && fields.title === ""}
          />
          <TextArea
            label={t("client:articles.field.summary")}
            description={t("client:articles.field.summary.description")}
            value={fields.summary}
            onChange={(summary) => {
              editor.edit({ summary });
            }}
            rows={2}
            characterLimit={maxSummaryLength}
            formatRemaining={formatRemaining}
          />
          <CoverFields editor={editor} />
          <Stack gap="2">
            {/* The buttons act on the body's selection and give focus back to the body, the next field. */}
            <Cluster gap="2">
              <Button size="sm" variant="ghost" onPress={editor.bold} aria-keyshortcuts="Control+B Meta+B">
                {t("client:articles.format.bold")}
              </Button>
              <Button size="sm" variant="ghost" onPress={editor.link} aria-keyshortcuts="Control+K Meta+K">
                {t("client:articles.format.link")}
              </Button>
              <FileTrigger
                acceptedFileTypes={acceptedImageTypes}
                allowsMultiple
                onSelect={(files) => {
                  editor.addImages(files === null ? [] : Array.from(files));
                }}
              >
                <Button size="sm" variant="ghost">
                  {t("client:articles.format.image")}
                </Button>
              </FileTrigger>
            </Cluster>
            <FileDropZone label={t("client:articles.images.drop")} onDropFiles={editor.addImages}>
              <TextArea
                id={editor.bodyId}
                label={t("client:articles.field.body")}
                description={t("client:articles.field.body.description")}
                value={fields.text}
                onChange={(text) => {
                  editor.edit({ text });
                }}
                onKeyDown={onBodyKeyDown}
                onPaste={onBodyPaste}
                rows={16}
                isInvalid={editor.invalid === "text"}
                {...(editor.invalid === "text" && { errorMessage: t("client:articles.blocked.text") })}
              />
            </FileDropZone>
            {editor.uploadingImages > 0 && (
              <Hint>{t("client:articles.images.uploading", { count: editor.uploadingImages })}</Hint>
            )}
            {editor.imageError !== null && <InlineError size="inherit">{editor.imageError}</InlineError>}
          </Stack>
          <TextField
            label={t("client:articles.field.tags")}
            description={t("client:articles.field.tags.description", { max: editor.maxTags })}
            value={fields.tags}
            onChange={(tags) => {
              editor.edit({ tags });
            }}
            isInvalid={editor.invalid === "tags"}
            {...(editor.invalid === "tags" && {
              errorMessage: t("client:articles.blocked.tags", { max: editor.maxTags }),
            })}
          />
          <Grid columns={2} at="sm" gap="4">
            <Select
              label={t("client:articles.field.language")}
              items={editor.languages}
              value={fields.language}
              onChange={(language) => {
                if (typeof language === "string") editor.edit({ language });
              }}
            >
              {(option) => (
                <SelectItem id={option.id} textValue={option.name}>
                  {option.name}
                </SelectItem>
              )}
            </Select>
            {/* The visibility of a published article never changes (editArticle). */}
            {editor.mode === "new" && (
              <Select
                label={t("client:articles.field.visibility")}
                value={fields.visibility}
                onChange={(value) => {
                  const visibility = editor.visibilities.find((v) => v === value);
                  if (visibility !== undefined) editor.edit({ visibility });
                }}
              >
                {editor.visibilities.map((visibility) => (
                  <SelectItem key={visibility} id={visibility} textValue={t(visibilityLabel[visibility])}>
                    {t(visibilityLabel[visibility])}
                  </SelectItem>
                ))}
              </Select>
            )}
          </Grid>
          <Checkbox
            isSelected={fields.sensitive}
            onChange={(sensitive) => {
              editor.edit({ sensitive });
            }}
            description={t("client:articles.field.sensitive.description")}
          >
            {t("client:articles.field.sensitive")}
          </Checkbox>
          {autosave !== null && <AutosaveStatusView autosave={autosave} scheduledAt={scheduledAt} />}
          {/* The bar's buttons are never disabled by the input (G23): pressed while the article cannot be published, it
              says why beside them. */}
          <FormActionBar
            {...(editor.message !== null && { result: { tone: "danger" as const, message: editor.message } })}
          >
            <Button type="submit" variant="primary" isPending={editor.isPublishing}>
              {t(editor.mode === "new" ? "client:articles.publish" : "client:articles.save_edit")}
            </Button>
            {editor.mode === "new" && (
              <SchedulePicker
                scheduledAt={scheduledAt}
                check={editor.scheduleCheck}
                onSchedule={editor.schedule}
                trigger={() => (
                  <Button>
                    {scheduledAt === null
                      ? t("client:articles.schedule")
                      : t("client:articles.schedule.scheduled", {
                          date: intl.formatDate(scheduledAt, { dateStyle: "medium", timeStyle: "short" }),
                        })}
                  </Button>
                )}
              />
            )}
          </FormActionBar>
        </Stack>
        <ArticlePreview editor={editor} />
      </Grid>
      {editor.mode === "new" && (
        <Box self="start">
          <Link href="/drafts" variant="standalone">
            {t("client:drafts.title")}
          </Link>
        </Box>
      )}
    </Stack>
  );
}

/** The cover image: the button that picks it, its thumbnail and description once uploaded, and its removal. */
function CoverFields({ editor }: { editor: Editor }) {
  const t = useT();
  const { cover } = editor;
  return (
    <Stack gap="2">
      <Text as="p" size="sm" weight="medium">
        {t("client:articles.field.cover")}
      </Text>
      {cover.status === "uploaded" && (
        <>
          {cover.image !== undefined && (
            // The thumbnail repeats the image the description describes: decorative here.
            <PreviewImage src={cover.image.sources.still.at(-1)?.url} alt="" />
          )}
          {cover.image === undefined && !cover.failed && <Hint>{t("client:articles.cover.processing")}</Hint>}
          {cover.failed && <InlineError size="inherit">{t("client:articles.cover.failed")}</InlineError>}
          <TextField
            label={t("client:articles.cover.description")}
            value={cover.description}
            onChange={(description) => {
              const current = editor.fields.cover;
              if (current !== null) editor.edit({ cover: { ...current, description } });
            }}
          />
        </>
      )}
      {cover.status === "uploading" && <Hint>{t("client:articles.cover.uploading")}</Hint>}
      <Cluster gap="2">
        <FileTrigger
          acceptedFileTypes={acceptedImageTypes}
          onSelect={(files) => {
            editor.chooseCover(files === null ? [] : Array.from(files));
          }}
        >
          <Button size="sm" isDisabled={cover.status === "uploading"}>
            {cover.status === "none" ? t("client:articles.cover.choose") : t("client:articles.cover.replace")}
          </Button>
        </FileTrigger>
        {cover.status === "uploaded" && (
          <Button size="sm" variant="ghost" onPress={editor.removeCover}>
            {t("client:articles.cover.remove")}
          </Button>
        )}
      </Cluster>
      {editor.coverError !== null && <InlineError size="inherit">{editor.coverError}</InlineError>}
    </Stack>
  );
}

type AutosaveStatusViewProps = {
  autosave: NonNullable<Editor["autosave"]>;
  scheduledAt: string | null;
};

/** Where the autosave of the draft is, and what to do after a conflict or a failure. */
function AutosaveStatusView({ autosave, scheduledAt }: AutosaveStatusViewProps) {
  const t = useT();
  const intl = useIntl();
  const { status } = autosave;
  return (
    <Stack gap="2">
      <Cluster gap="3" align="center">
        <SaveIndicator
          state={saveState(status)}
          labels={{
            saving: t("client:articles.autosave.saving"),
            saved: t("client:articles.autosave.saved"),
            failed: t("client:articles.autosave.failed"),
          }}
        />
        {autosave.link !== null && (
          <Hint>
            {scheduledAt === null
              ? t("client:drafts.editing")
              : t("client:drafts.editing.scheduled", {
                  date: intl.formatDate(scheduledAt, { dateStyle: "medium", timeStyle: "short" }),
                })}
          </Hint>
        )}
      </Cluster>
      {status.kind === "failed" && (
        <InlineError size="inherit">
          {t("client:articles.autosave.failed.detail", {
            reason: errorMessage(status.error),
          })}
        </InlineError>
      )}
      {status.kind === "conflict" && (
        <Callout role="alert">
          <Text as="p">{t("client:articles.autosave.conflict")}</Text>
          <Button size="sm" onPress={autosave.saveAsNew}>
            {t("client:articles.autosave.save_new")}
          </Button>
        </Callout>
      )}
    </Stack>
  );
}
