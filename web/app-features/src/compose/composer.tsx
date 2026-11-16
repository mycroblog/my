// SPDX-License-Identifier: AGPL-3.0-only

// The composer's view (#3046's rough, ADR-0155, ADR-0156): from the top, the head (the title, then close at its end),
// the post replied to or quoted (both above the text, as context), the content warning, the text with what the server
// will read in it and the part over the limit marked (the editor of text.tsx, a chunk of its own that comes with this
// one, which layer.tsx loads once the page is idle or the composer first opens), the suggestions for the word at its caret
// (suggest.tsx) and the emoji button at the end of its first line (emoji.tsx), the images as rows with their
// descriptions or the poll as a card, the form's error (what the server refused of an input it named stands under that
// input, the problem's title alone here), and the row
// that sends it: its tools (images, poll, content warning, the audience and the language as glyphs that open their
// menus, the one menu of the design system), the count, and Post. The audience menu holds a row for who can see the
// post, which opens the visibilities, and one for who may reply, boost and quote, whose submenu has a row per kind with
// who may do it now, each opening its audiences and, under them, whether others may ask for the author's approval
// (ADR-0155). A reply to a post whose author approves the reader's replies
// says so in a line over the row. Editing the reader's post, it is titled Edit
// post and saves with Save; what an edit cannot change leaves the row (the poll, the audience), and one muted line over
// the text names the audience and that it stays (a post with a poll is never edited: the server refuses it). A
// redraft is a new post, written from the deleted one. A new post may go out later (ADR-0156): a tool of the row, on
// while a time is set, opens the form of its time within the server's limits, in the reader's zone; while it has one,
// a line over the row says when, and Schedule takes Post's place, which stays alone at the row's end. A menu in the
// head, before close, keeps it as a draft or opens the drafts (drafts.tsx). A draft whose scheduled post failed says
// why and what Post does now, and a save refused for a newer draft saved elsewhere says so and opens that one. Its
// state is the hook's (use-composer.ts).

import { isRemote, QuoteContext, ReplyContext } from "@my/app-components";
import type { Audience, Interaction, InteractionChoice, Visibility } from "@my/app-data";
import {
  AttachButton,
  AttachmentList,
  AttachmentRow,
  Button,
  CaptionLine,
  CharCount,
  CheckField,
  ChoiceSelect,
  ComposerFrame,
  ComposerText,
  FormAlert,
  IconButton,
  MenuButton,
  MenuGroup,
  MenuItem,
  PollCard,
  PollOption,
  PollOptions,
  PollSettings,
  ScheduleTime,
  scheduleTime,
  ScheduleZone,
  TextButton,
  useLocale,
  WarningField,
  type IconName,
} from "@my/design-system";
import type { EditorHandle } from "@my/design-system/editor";
import { useId, useRef, useState } from "react";
import { fieldError } from "../field-errors";
import { stringsIn } from "../locale";
import { formatParts } from "../slots";
import { EditorFailed, PostText, retryEditors, WarningText } from "./editors";
import { ComposerEmoji } from "./emoji";
import { useSuggestions } from "./suggest";
import { Language, Schedule } from "./tools";
import type { MessageId } from "../messages.gen";
import {
  interactions,
  maxAttachments,
  nearChars,
  newPoll,
  pollDurations,
  pollLimits,
  quotesRemote,
  usedChars,
  visibilities,
  type PollDraft,
} from "./model";
import type { Composer } from "./use-composer";

type T = ReturnType<typeof stringsIn>["t"];

export { preloadComposerText } from "./editors";

const visibilityIcon: Record<Visibility, IconName> = {
  public: "public",
  local: "local",
  unlisted: "unlisted",
  private: "followers",
  direct: "direct",
};
const audienceIcon: Record<Audience, IconName> = {
  everyone: "public",
  followers: "followers",
  mentioned: "direct",
  nobody: "profile",
};
const audiences: readonly Audience[] = ["everyone", "followers", "mentioned", "nobody"];
const interactionIcon: Record<Interaction, IconName> = { reply: "reply", announce: "boost", quote: "quote" };

/** How long a poll runs, as its choice reads. */
function durationName(seconds: number, t: T): string {
  if (seconds < 3600) return t("compose.poll.minutes", { count: seconds / 60 });
  if (seconds < 86400) return t("compose.poll.hours", { count: seconds / 3600 });
  return t("compose.poll.days", { count: seconds / 86400 });
}

/**
 * A panel of the audience menu: its first, the visibilities' submenu, the permissions' submenu, or the submenu of one
 * kind of interaction.
 */
export type AudiencePanel = "audience" | "visibility" | "permissions" | Interaction;

/** Who may do it, as the value of its row: the audience, and that others may ask, which anyone needs not. */
const choiceText = (choice: InteractionChoice, t: T) =>
  t("compose.permission.now", {
    audience: t(`compose.audience.${choice.audience}`),
    approval: choice.approval && choice.audience !== "everyone" ? "yes" : "no",
  });

/** Who may reply, boost and quote, as the value of their row: one value for all three, or none where they differ. */
function permissionsText(policy: Composer["draft"]["policy"], t: T): string {
  const [first, ...rest] = interactions.map((i) => choiceText(policy[i], t));
  return first !== undefined && rest.every((text) => text === first) ? first : "";
}

/**
 * The items of the audience menu's panel: its first, a row for who can see the post and one for who can reply, boost
 * and quote it, each with what it holds now; the visibilities, the chosen one checked, a choice closing the menu; the
 * permissions, a row per kind of interaction with who may do it now; and a kind's own, its audiences and, under them
 * once the audience is not anyone, whether others may ask for approval, which stay open as they go together.
 */
function AudienceItems({
  c,
  t,
  panel,
  setOpen,
}: {
  c: Composer;
  t: T;
  panel: AudiencePanel;
  setOpen: (o: AudiencePanel) => void;
}) {
  const { draft, settings, answered } = c;
  const { policy } = draft;
  if (panel === "visibility") {
    const remote = draft.target.kind === "quote" && !draft.quoteRemoved && answered !== undefined && isRemote(answered);
    return (
      // A quote of a remote post says over the visibilities why some cannot be chosen.
      <MenuGroup
        title={remote ? t("compose.quote_remote") : undefined}
        chosen={draft.visibility}
        onChoose={(v) => {
          const visibility = visibilities.find((x) => x === v);
          if (visibility !== undefined) c.change({ visibility });
        }}
      >
        {visibilities.map((v) => (
          <MenuItem
            key={v}
            id={v}
            icon={visibilityIcon[v]}
            about={t(`compose.visibility.${v}.about` as MessageId, { server: settings.domain })}
            disabled={remote && !quotesRemote(v)}
          >
            {t(`compose.visibility.${v}`)}
          </MenuItem>
        ))}
      </MenuGroup>
    );
  }
  if (panel === "permissions")
    return (
      <MenuGroup>
        {interactions.map((i) => (
          <MenuItem
            key={i}
            id={i}
            icon={interactionIcon[i]}
            opens={choiceText(policy[i], t)}
            onAction={() => {
              setOpen(i);
            }}
          >
            {t(`compose.interaction.${i}`)}
          </MenuItem>
        ))}
      </MenuGroup>
    );
  if (panel !== "audience") {
    const choice = policy[panel];
    const change = (next: InteractionChoice) => {
      c.change({ policy: { ...policy, [panel]: next } });
    };
    return (
      <>
        <MenuGroup
          chosen={choice.audience}
          stay
          onChoose={(id) => {
            const audience = audiences.find((a) => a === id);
            if (audience !== undefined) change({ ...choice, audience });
          }}
        >
          {audiences.map((a) => (
            <MenuItem key={a} id={a} icon={audienceIcon[a]}>
              {t(`compose.audience.${a}`)}
            </MenuItem>
          ))}
        </MenuGroup>
        {choice.audience !== "everyone" && (
          <MenuGroup
            checked={choice.approval ? ["approval"] : []}
            stay
            onCheck={(_, approval) => {
              change({ ...choice, approval });
            }}
          >
            <MenuItem id="approval" icon="included">
              {t("compose.approval")}
            </MenuItem>
          </MenuGroup>
        )}
      </>
    );
  }
  return (
    <MenuGroup>
      <MenuItem
        id="visibility"
        icon={visibilityIcon[draft.visibility]}
        opens={t(`compose.visibility.${draft.visibility}`)}
        onAction={() => {
          setOpen("visibility");
        }}
      >
        {t("compose.audience")}
      </MenuItem>
      <MenuItem
        id="permissions"
        icon="quote"
        opens={permissionsText(policy, t)}
        onAction={() => {
          setOpen("permissions");
        }}
      >
        {t("compose.permissions")}
      </MenuItem>
    </MenuGroup>
  );
}

/** The submenu a panel of the audience menu is, by its name and the panel back opens; none for its first. */
function audienceSub(panel: AudiencePanel, t: T, setOpen: (o: AudiencePanel) => void) {
  if (panel === "audience") return undefined;
  const [title, parent]: [string, AudiencePanel] =
    panel === "visibility"
      ? [t("compose.audience"), "audience"]
      : panel === "permissions"
        ? [t("compose.permissions"), "audience"]
        : [t(`compose.interaction.${panel}`), "permissions"];
  return {
    title,
    onBack: () => {
      setOpen(parent);
    },
  };
}

/**
 * The audience tool and its menu: who can see the post, and who can reply, boost and quote it, each opening its
 * submenu.
 */
function Audience({
  c,
  t,
  open,
  setOpen,
}: {
  c: Composer;
  t: T;
  open: AudiencePanel | undefined;
  setOpen: (o: AudiencePanel | undefined) => void;
}) {
  const { draft } = c;
  const limited = interactions.some((i) => draft.policy[i].audience !== "everyone");
  const panel = open ?? "audience";
  return (
    <MenuButton
      icon={visibilityIcon[draft.visibility]}
      label={t("compose.audience.tool", { visibility: draft.visibility, limited: limited ? "yes" : "no" })}
      menuLabel={t("compose.audience_menu")}
      align="start"
      open={open !== undefined}
      onOpenChange={(o) => {
        setOpen(o ? "audience" : undefined);
      }}
      sub={audienceSub(panel, t, setOpen)}
    >
      <AudienceItems c={c} t={t} panel={panel} setOpen={setOpen} />
    </MenuButton>
  );
}

/** The poll's card: its options headed by their marks, then how long it runs and how it counts. */
function Poll({
  poll,
  t,
  errorOf,
  onChange,
  onRemove,
}: {
  poll: PollDraft;
  t: T;
  /** What the server refused of an option, by its index. */
  errorOf: (i: number) => string | undefined;
  onChange: (p: PollDraft) => void;
  onRemove: () => void;
}) {
  const id = useId();
  return (
    <PollCard title={t("compose.poll")} removeLabel={t("compose.poll.off")} onRemove={onRemove}>
      <PollOptions
        add={
          poll.options.length < pollLimits.max
            ? {
                label: t("compose.poll.add"),
                onPress: () => {
                  onChange({ ...poll, options: [...poll.options, ""] });
                },
              }
            : undefined
        }
      >
        {poll.options.map((option, i) => (
          <PollOption
            key={i}
            value={option}
            label={t("compose.poll.option", { n: i + 1 })}
            multiple={poll.multiple}
            maxLength={pollLimits.length}
            error={errorOf(i)}
            onChange={(value) => {
              onChange({ ...poll, options: poll.options.map((o, j) => (j === i ? value : o)) });
            }}
            remove={
              poll.options.length > pollLimits.min
                ? {
                    label: t("compose.poll.remove_option", { n: i + 1 }),
                    onPress: () => {
                      onChange({ ...poll, options: poll.options.filter((_, j) => j !== i) });
                    },
                  }
                : undefined
            }
          />
        ))}
      </PollOptions>
      <PollSettings>
        <ChoiceSelect
          label={t("compose.poll.duration")}
          icon="time"
          value={t("compose.poll.ends_in", { duration: durationName(poll.duration, t) })}
          items={pollDurations.map((d) => ({ id: String(d), icon: "time" as const, label: durationName(d, t) }))}
          chosen={String(poll.duration)}
          onChoose={(d) => {
            onChange({ ...poll, duration: Number(d) });
          }}
        />
        <CheckField
          id={`${id}-multiple`}
          name="multiple"
          value="1"
          label={t("compose.poll.multiple")}
          checked={poll.multiple}
          onChange={(multiple) => {
            onChange({ ...poll, multiple });
          }}
        />
        <CheckField
          id={`${id}-hide`}
          name="hide_totals"
          value="1"
          label={t("compose.poll.hide_totals")}
          checked={poll.hideTotals}
          onChange={(hideTotals) => {
            onChange({ ...poll, hideTotals });
          }}
        />
      </PollSettings>
    </PollCard>
  );
}

/** Why a scheduled post was not posted, in a sentence with the time it was for. */
const failureOf = (failure: NonNullable<Composer["draft"]["failure"]>, locale: string, t: T) =>
  t("compose.schedule.failed", {
    time: scheduleTime(failure.publishAt, locale),
    reason: t(`compose.schedule.failed.${failure.reason}`),
  });

/** The form's error before Post: the first thing that keeps the post from being sent. */
function alertOf(c: Composer, t: T, locale: string): string | undefined {
  if (c.phase.kind === "conflict") return t("compose.conflict");
  if (c.phase.kind === "gone") return t("compose.gone");
  if (c.phase.kind === "failed")
    return c.phase.reason ?? t(c.draft.target.kind === "edit" ? "compose.save_failed" : "compose.failed");
  if (c.draft.failure !== undefined && c.draft.scheduledAt === undefined)
    return t("compose.schedule.failed.next", { failure: failureOf(c.draft.failure, locale, t) });
  if (!c.checked) return undefined;
  if (c.problems.waiting) return t("compose.wait_uploads");
  if (c.problems.pollInvalid)
    return t("compose.poll.invalid", { min: pollLimits.min, max: pollLimits.max, length: pollLimits.length });
  return undefined;
}

export type ComposerViewProps = {
  c: Composer;
  /** Close or Escape: the reader means to leave. */
  onClose: () => void;
  /** The backdrop: the reader left by accident. */
  onDismiss: () => void;
  /** Over the page, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
  /** A screen of its own on a phone (the app's choice), or a dialog. */
  phone?: "screen" | "dialog" | undefined;
  /** The emoji picker opens in a sheet (the app's choice on a phone's viewport), else in a popover. */
  pickerSheet?: boolean | undefined;
  /** Opens the list of the reader's drafts and scheduled posts. */
  onDrafts?: (() => void) | undefined;
  /** Keeps what is written as a draft, and closes. */
  onSaveDraft?: (() => void) | undefined;
  /** Opens the newer draft saved elsewhere, in place of this one (a conflict). */
  onNewer?: (() => void) | undefined;
  /**
   * A specimen opens with this open: the schedule's form, the drafts' menu in the head, or the audience menu on one of
   * its panels.
   */
  opened?: "schedule" | "drafts" | AudiencePanel | undefined;
  /** A specimen shows this as if its chunk could not be fetched: the whole view (the layer's), or its editors. */
  unloaded?: "composer" | "editor" | undefined;
};

export function ComposerView({
  c,
  onClose,
  onDismiss,
  layer = true,
  phone = "screen",
  pickerSheet = false,
  onDrafts,
  onSaveDraft,
  onNewer,
  opened,
  unloaded,
}: ComposerViewProps) {
  const { locale } = useLocale();
  const textRef = useRef<EditorHandle>(null);
  const cwRef = useRef<EditorHandle>(null);
  const errorId = useId();
  const cwErrorId = useId();
  const { t, lang } = stringsIn(locale);
  const [open, setOpen] = useState<AudiencePanel | "language" | "schedule" | undefined>(
    opened === "drafts" ? undefined : opened,
  );
  const audienceOpen = open === "language" || open === "schedule" ? undefined : open;
  const [draftsOpen, setDraftsOpen] = useState(opened === "drafts");
  // The content warning was added by the writer here: it takes the focus as it mounts (one opened with it does not).
  const [cwFocus, setCwFocus] = useState(false);
  // The editors' chunk could not be fetched (or a specimen shows so): said once, in the text's place, Post disabled and
  // the count away until Try again brings them; each try mounts them anew.
  const [editorsFailed, setEditorsFailed] = useState(false);
  const [editorsTry, setEditorsTry] = useState(0);
  const editorsDown = unloaded === "editor" || editorsFailed;
  const failEditors = () => {
    setEditorsFailed(true);
  };
  const { draft, settings, answered, problems } = c;
  const busy = c.phase.kind === "posting";
  // Its draft gone from the server: Post posts what is written as a new post, and says so.
  const gone = c.phase.kind === "gone";
  const editing = draft.target.kind === "edit";
  const left = settings.maxChars - usedChars(draft);
  // A redraft is a new post; a quote whose quoted post was taken out is one too.
  const kind = draft.target.kind;
  const title = t(`compose.title.${kind === "redraft" || (kind === "quote" && draft.quoteRemoved) ? "new" : kind}`);
  const alert = alertOf(c, t, locale);
  const handle = answered ? `@${(answered.boost ?? answered).account.acct}` : "";
  // A reply to a post whose author approves the reader's replies says so over the row, before it is sent.
  const waits = kind === "reply" && (answered?.boost ?? answered)?.permissions?.reply === "pending";
  const over = problems.over > 0;
  // What the server refused of an input it named, beside that input (field-errors.ts).
  const refused = c.phase.kind === "failed" ? c.phase : undefined;
  const textError =
    over && !editorsDown ? t("compose.too_long", { max: settings.maxChars }) : fieldError(refused, "text");
  const cwError = fieldError(refused, "cw");
  const roomy = draft.attachments.length === 0 && draft.poll === undefined;
  // The text completes accounts, hashtags and emoji; the content warning, emoji alone (ADR-0092 §5).
  const textSuggest = useSuggestions({ field: textRef, text: draft.text, triggers: ["@", "#", ":"], locale });
  const cwSuggest = useSuggestions({ field: cwRef, text: draft.cw ?? "", triggers: [":"], locale });
  const scheduled = draft.scheduledAt;
  const setSchedule = (o: boolean) => {
    setOpen(o ? "schedule" : undefined);
  };
  // The drafts, a place apart from sending this post, in the head: keep this one as a draft, or open the list. An edit
  // is no draft: it is saved at once, and never scheduled.
  const draftsMenu = !editing && (
    <MenuButton icon="drafts" label={t("compose.later")} align="end" open={draftsOpen} onOpenChange={setDraftsOpen}>
      <MenuGroup>
        <MenuItem id="save" icon="saveDraft" onAction={onSaveDraft}>
          {t("compose.drafts.save")}
        </MenuItem>
        <MenuItem id="drafts" icon="drafts" onAction={onDrafts}>
          {t("compose.drafts.open")}
        </MenuItem>
      </MenuGroup>
    </MenuButton>
  );
  return (
    <ComposerFrame
      title={title}
      closeLabel={t("compose.close")}
      onClose={onClose}
      onDismiss={onDismiss}
      busy={busy}
      layer={layer}
      phone={phone}
      headEnd={draftsMenu || undefined}
      note={
        waits || scheduled !== undefined ? (
          <>
            {waits && (
              <CaptionLine icon="time" lang={lang}>
                {t("compose.reply_waits", { handle })}
              </CaptionLine>
            )}
            {scheduled !== undefined && (
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
                {/* The zone is stated once on the screen: by the form while it is open, else here. */}
                {open === "schedule"
                  ? formatParts(t, "compose.schedule.at", {}, { time: <ScheduleTime date={scheduled} /> })
                  : formatParts(
                      t,
                      "compose.schedule.at_zone",
                      {},
                      { time: <ScheduleTime date={scheduled} />, zone: <ScheduleZone /> },
                    )}
              </CaptionLine>
            )}
          </>
        ) : undefined
      }
      alert={
        alert === undefined ? undefined : (
          <FormAlert
            // A draft gone is told in the text's weight: Post, right after it, posts what is written as a new post.
            regular={gone}
            action={
              c.phase.kind === "conflict" ? (
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
            label={t("compose.attach")}
            accept="image/*,video/*,audio/*"
            isDisabled={draft.poll !== undefined || draft.attachments.length >= maxAttachments}
            onFiles={c.addFiles}
          />
          {!editing && (
            <IconButton
              icon="poll"
              label={t(draft.poll ? "compose.poll.off" : "compose.poll.on")}
              pressed={draft.poll !== undefined}
              isDisabled={draft.attachments.length > 0}
              onPress={() => {
                c.change({ poll: draft.poll ? undefined : newPoll });
              }}
            />
          )}
          <IconButton
            icon="hidden"
            label={t(draft.cw === undefined ? "compose.cw.on" : "compose.cw.off")}
            pressed={draft.cw !== undefined}
            onPress={() => {
              // The field added takes the focus, to be written at once; taken out, the focus goes back to the text.
              const adding = draft.cw === undefined;
              setCwFocus(adding);
              c.change({ cw: adding ? "" : undefined });
              if (!adding) textRef.current?.focus();
            }}
          />
          {!editing && <Audience c={c} t={t} open={audienceOpen} setOpen={setOpen} />}
          {!editing && (
            <Schedule
              at={c.draft.scheduledAt}
              limits={settings.schedule}
              onSet={(scheduledAt) => {
                c.change({ scheduledAt });
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
                  aria-haspopup="dialog"
                  aria-expanded={open === "schedule"}
                  onPress={() => {
                    setSchedule(open !== "schedule");
                  }}
                />
              }
            />
          )}
          <Language
            current={draft.language}
            preferred={settings.language}
            onChoose={(language) => {
              c.change({ language });
            }}
            t={t}
            lang={lang}
            open={open === "language"}
            setOpen={(o) => {
              setOpen(o ? "language" : undefined);
            }}
          />
        </>
      }
      count={
        editorsDown ? undefined : (
          <CharCount
            left={left}
            near={nearChars}
            label={left < 0 ? t("compose.over", { count: -left }) : t("compose.remaining", { count: left })}
          />
        )
      }
      action={
        <Button kind="primary" size="timeline" busy={busy} isDisabled={editorsDown} onPress={c.submit}>
          {t(
            editing
              ? "compose.save"
              : gone
                ? "compose.gone.post"
                : scheduled === undefined
                  ? "compose.post"
                  : "compose.schedule.post",
          )}
        </Button>
      }
    >
      {editing && (
        <CaptionLine icon={visibilityIcon[draft.visibility]} lang={lang}>
          {t("compose.edit.fixed", { visibility: t(`compose.visibility.${draft.visibility}`) })}
        </CaptionLine>
      )}
      {draft.target.kind === "reply" && answered && (
        <ReplyContext status={answered} note={t("compose.replying_to", { handle })} />
      )}
      {draft.target.kind === "quote" && !draft.quoteRemoved && answered && (
        <QuoteContext
          status={answered}
          removeLabel={t("compose.quote.remove")}
          onRemove={() => {
            c.change({ quoteRemoved: true });
          }}
        />
      )}
      {draft.cw !== undefined && (
        <WarningField error={cwError} errorId={cwErrorId}>
          {!editorsDown && (
            <WarningText
              key={editorsTry}
              onFailed={failEditors}
              value={draft.cw}
              label={t("compose.cw.label")}
              placeholder={t("compose.cw.placeholder")}
              invalid={cwError !== undefined}
              describedBy={cwError === undefined ? undefined : cwErrorId}
              handle={cwRef}
              autoFocus={cwFocus}
              suggest={cwSuggest.suggest}
              {...cwSuggest.caret}
              readOnly={busy}
              onChange={(cw) => {
                c.change({ cw });
              }}
            />
          )}
        </WarningField>
      )}
      <ComposerText
        error={textError}
        errorId={errorId}
        roomy={roomy}
        end={editorsDown ? undefined : <ComposerEmoji text={textRef} sheet={pickerSheet} disabled={busy} />}
      >
        {editorsDown ? (
          <EditorFailed
            t={t}
            retry={() => {
              retryEditors();
              setEditorsFailed(false);
              setEditorsTry((n) => n + 1);
            }}
          />
        ) : (
          <PostText
            key={editorsTry}
            onFailed={failEditors}
            value={draft.text}
            label={t("compose.text.label")}
            placeholder={t("compose.text.placeholder")}
            over={problems.over}
            describedBy={textError === undefined ? undefined : errorId}
            invalid={textError !== undefined}
            readOnly={busy}
            roomy={roomy}
            autoFocus={layer}
            ended
            handle={textRef}
            suggest={textSuggest.suggest}
            {...textSuggest.caret}
            onChange={(value) => {
              c.change({ text: value });
            }}
          />
        )}
      </ComposerText>
      {draft.attachments.length > 0 && (
        <AttachmentList>
          {draft.attachments.map((a) => (
            <AttachmentRow
              key={a.key}
              src={a.preview}
              state={a.state}
              status={t(
                a.state === "uploading"
                  ? "compose.uploading"
                  : a.state === "processing"
                    ? "compose.processing"
                    : a.id === undefined
                      ? "compose.upload_failed"
                      : "compose.media_failed",
              )}
              retry={
                a.state === "failed" && (a.file !== undefined || a.id !== undefined)
                  ? {
                      label: t("compose.retry"),
                      onPress: () => {
                        c.retry(a.key);
                      },
                    }
                  : undefined
              }
              description={a.description}
              onDescription={(value) => {
                c.describe(a.key, value);
              }}
              descriptionLabel={t("compose.description")}
              placeholder={t("compose.description.hint")}
              missing={
                a.description.trim() === ""
                  ? settings.requireDescriptions && c.checked
                    ? { text: t("compose.description.required"), required: true }
                    : { text: t("compose.description.missing"), required: false }
                  : undefined
              }
              error={fieldError(refused, `media:${a.key}`)}
              removeLabel={t("compose.remove")}
              onRemove={() => {
                c.remove(a.key);
              }}
            />
          ))}
        </AttachmentList>
      )}
      {draft.poll && (
        <Poll
          poll={draft.poll}
          t={t}
          errorOf={(i) => fieldError(refused, `option:${String(i)}`)}
          onChange={(poll) => {
            c.change({ poll });
          }}
          onRemove={() => {
            c.change({ poll: undefined });
          }}
        />
      )}
    </ComposerFrame>
  );
}
