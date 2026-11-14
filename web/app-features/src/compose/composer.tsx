// SPDX-License-Identifier: AGPL-3.0-only

// The composer's view (#3046's rough, ADR-0155, ADR-0156): from the top, the head (the title, then close at its end),
// the post replied to or quoted (both above the text, as context), the content warning, the text with the part over
// the limit marked, the images as rows with their descriptions or the poll as a card, the form's error, and the row
// that sends it: its tools (images, poll, content warning, the audience and the language as glyphs that open their
// menus), the count, and Post. The audience menu holds the visibilities, then apart under their own caption the
// interactions, whose reply, boost and quote permissions open as a submenu. Its state is the hook's (use-composer.ts).

import { isRemote, QuoteContext, ReplyContext } from "@my/app-components";
import type { Audience, Interaction, Visibility } from "@my/app-data";
import {
  AttachButton,
  AttachmentList,
  AttachmentRow,
  Button,
  CharCount,
  CheckField,
  ChoiceBack,
  ChoiceGroup,
  ChoiceOpens,
  ChoicePopup,
  ChoiceSection,
  ChoiceSelect,
  ComposerFrame,
  ComposerText,
  FormAlert,
  IconButton,
  PollCard,
  PollOption,
  PollOptions,
  PollSettings,
  ToolText,
  useLocale,
  WarningField,
  type IconName,
} from "@my/design-system";
import { useId, useState } from "react";
import { stringsIn } from "../locale";
import type { MessageId } from "../messages.gen";
import {
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
const interactions: readonly Interaction[] = ["reply", "announce", "quote"];

/** The languages offered: the post's, the reader's default and the app's, each once. */
const appLanguages = ["en", "ja", "de", "es", "fr", "pt-BR", "zh-CN", "zh-TW"];

/** A language by its name in its own words (English, 日本語), its first letter capitalized as a picker lists it. */
function languageName(tag: string): string {
  try {
    const name = new Intl.DisplayNames([tag], { type: "language" }).of(tag) ?? tag;
    return name.charAt(0).toLocaleUpperCase(tag) + name.slice(1);
  } catch {
    return tag;
  }
}

/** How long a poll runs, as its choice reads. */
function durationName(seconds: number, t: T): string {
  if (seconds < 3600) return t("compose.poll.minutes", { count: seconds / 60 });
  if (seconds < 86400) return t("compose.poll.hours", { count: seconds / 3600 });
  return t("compose.poll.days", { count: seconds / 86400 });
}

/** The audience tool and its menu: the visibilities, then the interactions, whose permissions open as a submenu. */
function Audience({
  c,
  t,
  open,
  setOpen,
}: {
  c: Composer;
  t: T;
  open: "audience" | "permissions" | undefined;
  setOpen: (o: "audience" | "permissions" | undefined) => void;
}) {
  const { draft, settings, answered } = c;
  const remote = draft.target.kind === "quote" && !draft.quoteRemoved && answered !== undefined && isRemote(answered);
  const limited = interactions.some((i) => draft.policy[i] !== "everyone");
  const panel =
    open === "permissions" ? (
      <>
        <ChoiceBack
          label={t("compose.permissions")}
          onPress={() => {
            setOpen("audience");
          }}
        />
        {interactions.map((i) => (
          <ChoiceGroup
            key={i}
            title={t(`compose.interaction.${i}`)}
            items={audiences.map((a) => ({ id: a, icon: audienceIcon[a], label: t(`compose.audience.${a}`) }))}
            chosen={draft.policy[i]}
            onChoose={(a) => {
              c.change({ policy: { ...draft.policy, [i]: a } });
            }}
          />
        ))}
      </>
    ) : (
      <>
        <ChoiceGroup
          title={t("compose.audience")}
          items={visibilities.map((v) => ({
            id: v,
            icon: visibilityIcon[v],
            label: t(`compose.visibility.${v}`),
            about: t(`compose.visibility.${v}.about` as MessageId, { server: settings.domain }),
            disabled: remote && !quotesRemote(v),
          }))}
          chosen={draft.visibility}
          onChoose={(visibility) => {
            c.change({ visibility });
          }}
          note={remote ? t("compose.quote_remote") : undefined}
        />
        <ChoiceSection title={t("compose.interactions")}>
          <ChoiceOpens
            icon="settings"
            label={t("compose.permissions.item")}
            now={t(limited ? "compose.permissions.limited" : "compose.permissions.open")}
            onPress={() => {
              setOpen("permissions");
            }}
          />
        </ChoiceSection>
      </>
    );
  return (
    <ChoicePopup
      open={open !== undefined}
      onClose={() => {
        setOpen(undefined);
      }}
      label={t("compose.audience")}
      trigger={
        <IconButton
          icon={visibilityIcon[draft.visibility]}
          label={t("compose.audience.tool", { visibility: draft.visibility, limited: limited ? "yes" : "no" })}
          aria-haspopup="dialog"
          aria-expanded={open !== undefined}
          onPress={() => {
            setOpen(open === undefined ? "audience" : undefined);
          }}
        />
      }
    >
      {panel}
    </ChoicePopup>
  );
}

/** The language tool, by the language's name, and its menu. */
function Language({
  c,
  t,
  lang,
  open,
  setOpen,
}: {
  c: Composer;
  t: T;
  lang: string;
  open: boolean;
  setOpen: (o: boolean) => void;
}) {
  const current = c.draft.language;
  const tags = [current, c.settings.language, lang, ...appLanguages].filter(
    (tag, i, all) => tag !== "" && tag !== "und" && all.indexOf(tag) === i,
  );
  const shown = current === "" ? lang : current;
  return (
    <ChoicePopup
      open={open}
      onClose={() => {
        setOpen(false);
      }}
      label={t("compose.language")}
      end
      trigger={
        <ToolText
          label={t("compose.language.tool", { language: languageName(shown) })}
          lang={shown}
          expanded={open}
          onPress={() => {
            setOpen(!open);
          }}
        >
          {languageName(shown)}
        </ToolText>
      }
    >
      <ChoiceGroup
        title={t("compose.language")}
        items={tags.map((tag) => ({ id: tag, icon: "translate" as const, label: languageName(tag) }))}
        chosen={shown}
        onChoose={(language) => {
          c.change({ language });
          setOpen(false);
        }}
      />
    </ChoicePopup>
  );
}

/** The poll's card: its options headed by their marks, then how long it runs and how it counts. */
function Poll({
  poll,
  t,
  onChange,
  onRemove,
}: {
  poll: PollDraft;
  t: T;
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

/** The form's error before Post: the first thing that keeps the post from being sent. */
function alertOf(c: Composer, t: T): string | undefined {
  if (c.phase.kind === "failed") return c.phase.reason ?? t("compose.failed");
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
  /** Over the page, or in the flow of a page (a catalog's specimen). */
  layer?: boolean | undefined;
  /** A screen of its own on a phone (the app's choice), or a dialog. */
  phone?: "screen" | "dialog" | undefined;
};

export function ComposerView({ c, onClose, onDismiss, layer = true, phone = "screen" }: ComposerViewProps) {
  const { locale } = useLocale();
  const { t, lang } = stringsIn(locale);
  const [open, setOpen] = useState<"audience" | "permissions" | "language" | undefined>(undefined);
  const { draft, settings, answered, problems } = c;
  const busy = c.phase.kind === "posting";
  const left = settings.maxChars - usedChars(draft);
  const title = t(`compose.title.${draft.target.kind === "quote" && draft.quoteRemoved ? "new" : draft.target.kind}`);
  const alert = alertOf(c, t);
  const handle = answered ? `@${(answered.boost ?? answered).account.acct}` : "";
  return (
    <ComposerFrame
      title={title}
      closeLabel={t("compose.close")}
      onClose={onClose}
      onDismiss={onDismiss}
      layer={layer}
      phone={phone}
      alert={alert === undefined ? undefined : <FormAlert>{alert}</FormAlert>}
      tools={
        <>
          <AttachButton
            label={t("compose.attach")}
            accept="image/*,video/*,audio/*"
            isDisabled={draft.poll !== undefined || draft.attachments.length >= maxAttachments}
            onFiles={c.addFiles}
          />
          <IconButton
            icon="poll"
            label={t(draft.poll ? "compose.poll.off" : "compose.poll.on")}
            pressed={draft.poll !== undefined}
            isDisabled={draft.attachments.length > 0}
            onPress={() => {
              c.change({ poll: draft.poll ? undefined : newPoll });
            }}
          />
          <IconButton
            icon="hidden"
            label={t(draft.cw === undefined ? "compose.cw.on" : "compose.cw.off")}
            pressed={draft.cw !== undefined}
            onPress={() => {
              c.change({ cw: draft.cw === undefined ? "" : undefined });
            }}
          />
          <Audience
            c={c}
            t={t}
            open={open === "audience" || open === "permissions" ? open : undefined}
            setOpen={setOpen}
          />
          <Language
            c={c}
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
        <CharCount
          left={left}
          near={nearChars}
          label={left < 0 ? t("compose.over", { count: -left }) : t("compose.remaining", { count: left })}
        />
      }
      action={
        <Button kind="primary" size="timeline" busy={busy} onPress={c.submit}>
          {t("compose.post")}
        </Button>
      }
    >
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
        <WarningField
          value={draft.cw}
          label={t("compose.cw.label")}
          placeholder={t("compose.cw.placeholder")}
          readOnly={busy}
          onChange={(cw) => {
            c.change({ cw });
          }}
        />
      )}
      <ComposerText
        value={draft.text}
        label={t("compose.text.label")}
        placeholder={t("compose.text.placeholder")}
        over={problems.over}
        error={problems.over > 0 ? t("compose.too_long", { max: settings.maxChars }) : undefined}
        readOnly={busy}
        roomy={draft.attachments.length === 0 && draft.poll === undefined}
        autoFocus={layer}
        onChange={(text) => {
          c.change({ text });
        }}
      />
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
                    : "compose.upload_failed",
              )}
              retry={
                a.state === "failed" && a.file
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
