// SPDX-License-Identifier: AGPL-3.0-only

// The reply in place at the end of a conversation of direct messages (FR-212): a reply to its latest post that keeps
// `direct` and its people, each of them mentioned before what the reader writes, which the line over the text names
// instead of the text holding them. The text completes accounts, hashtags and emoji as the composer's does, and takes
// images; Reply sends it, and the box empties for the next one. A reply the server refuses keeps what was written and
// says why in the server's words. A newer latest post arriving while the reader writes is what the reply answers then.
// Its logic is the composer's (use-composer.ts).

import { ConversationReply } from "@my/app-components";
import { useComposeSettings, type Status } from "@my/app-data";
import {
  AttachButton,
  AttachmentList,
  AttachmentRow,
  Button,
  CharCount,
  ComposerText,
  FormAlert,
  useLocale,
} from "@my/design-system";
import type { EditorHandle } from "@my/design-system/editor";
import { useEffect, useId, useRef, useState } from "react";
import { stringsIn } from "../locale";
import { EditorFailed, PostText, retryEditors } from "./editors";
import { ComposerEmoji } from "./emoji";
import { emptyDraft, maxAttachments, nearChars, usedChars, type Draft } from "./model";
import { useSuggestions } from "./suggest";
import { useComposer } from "./use-composer";

export type InPlaceReplyProps = {
  /** The ID of the post it replies to: the conversation's latest. */
  to: string;
  /** The handles (`acct`) of the conversation's other accounts, whom it mentions. */
  people: readonly string[];
  /** The other accounts' names as the conversation's head lists them, as plain text ("Mika and Ren"). */
  names: string;
  /** The emoji picker opens in a sheet (the app's choice on a phone's viewport), else in a popover. */
  pickerSheet?: boolean | undefined;
  /** Hears each reply posted. */
  onPosted?: ((status: Status) => void) | undefined;
};

/** The mentions of `people` before a reply's text: each handle, a space after. */
const mentionsOf = (people: readonly string[]) => people.map((acct) => `@${acct} `).join("");

/** The reply in place, an empty one after each reply posted. */
export function InPlaceReply(props: InPlaceReplyProps) {
  const [round, setRound] = useState(0);
  return (
    <Box
      key={round}
      {...props}
      onPosted={(status) => {
        setRound(round + 1);
        props.onPosted?.(status);
      }}
    />
  );
}

function Box({ to, people, names, pickerSheet = false, onPosted }: InPlaceReplyProps) {
  const { locale } = useLocale();
  const { t } = stringsIn(locale);
  const settings = useComposeSettings();
  const [initial] = useState<Draft>(() => ({
    ...emptyDraft({ kind: "reply", id: to }, settings, crypto.randomUUID()),
    visibility: "direct",
    mentions: mentionsOf(people),
  }));
  const c = useComposer(initial, {
    onPosted: (status) => {
      onPosted?.(status);
    },
    onKept: () => undefined,
  });
  const { draft, problems } = c;
  // A newer latest post, or the people with it, is what the reply answers and mentions from then on.
  const mentions = mentionsOf(people);
  const { change } = c;
  const target = draft.target.kind === "reply" ? draft.target.id : undefined;
  useEffect(() => {
    if (target !== to || draft.mentions !== mentions) change({ target: { kind: "reply", id: to }, mentions });
  }, [to, mentions, target, draft.mentions, change]);
  const textRef = useRef<EditorHandle>(null);
  const errorId = useId();
  const [editorsFailed, setEditorsFailed] = useState(false);
  const [editorsTry, setEditorsTry] = useState(0);
  const suggest = useSuggestions({ field: textRef, text: draft.text, triggers: ["@", "#", ":"], locale });
  const busy = c.phase.kind === "posting";
  const left = c.settings.maxChars - usedChars(draft);
  const over = problems.over > 0;
  const alert =
    c.phase.kind === "failed"
      ? (c.phase.reason ?? t("compose.failed"))
      : c.checked && problems.waiting
        ? t("compose.wait_uploads")
        : undefined;
  return (
    <ConversationReply
      label={t("compose.in_place.form", { names })}
      to={t("compose.in_place.to", { names })}
      visibility={t("compose.visibility.direct")}
      tools={
        <>
          <AttachButton
            label={t("compose.attach")}
            accept="image/*,video/*,audio/*"
            isDisabled={busy || draft.attachments.length >= maxAttachments}
            onFiles={c.addFiles}
          />
          {!editorsFailed && <ComposerEmoji text={textRef} sheet={pickerSheet} disabled={busy} />}
        </>
      }
      count={
        editorsFailed ? undefined : (
          <CharCount
            left={left}
            near={nearChars}
            label={left < 0 ? t("compose.over", { count: -left }) : t("compose.remaining", { count: left })}
          />
        )
      }
      action={
        <Button
          kind="primary"
          size="timeline"
          busy={busy}
          isDisabled={editorsFailed}
          onPress={c.submit}
          aria-label={t("compose.in_place.send_to", { names })}
        >
          {t("compose.in_place.send")}
        </Button>
      }
    >
      <ComposerText
        error={over && !editorsFailed ? t("compose.too_long", { max: c.settings.maxChars }) : undefined}
        errorId={errorId}
        roomy={false}
      >
        {editorsFailed ? (
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
            onFailed={() => {
              setEditorsFailed(true);
            }}
            value={draft.text}
            label={t("compose.in_place.label")}
            placeholder={t("compose.in_place.placeholder")}
            over={problems.over}
            describedBy={over ? errorId : undefined}
            invalid={over}
            readOnly={busy}
            roomy={false}
            autoFocus={false}
            ended={false}
            handle={textRef}
            suggest={suggest.suggest}
            {...suggest.caret}
            onChange={(value) => {
              change({ text: value });
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
                    : "compose.upload_failed",
              )}
              retry={
                a.state === "failed" && a.file !== undefined
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
                  ? c.settings.requireDescriptions && c.checked
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
      {alert !== undefined && <FormAlert>{alert}</FormAlert>}
    </ConversationReply>
  );
}
