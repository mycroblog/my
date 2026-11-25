// SPDX-License-Identifier: AGPL-3.0-only

// A post or an article that this server's automated review refused, in the composer or the article editor (ADR-0147
// §9): the refusal is appealed here, where it still is, since the server keeps no refused post and so no list of
// them. The form's alert says, in the app's own sentence, that the review did not accept it, and Appeal after it (the
// server's sentence says that the author may appeal, for an app with no such action: said beside Appeal it would say
// it twice); it asks first, since a refusal is appealed once and the appeal sends the text to the moderators: a post's
// text, an article's Markdown body (the text the review judged; never its title or summary). The confirmation holds a
// field for what the author wants the moderators to know, which may stay empty: what it holds, its edges trimmed, goes
// with the text, and an empty one is left out of the appeal. One longer than the server takes is said under the field
// as Send appeal is pressed, and nothing is sent; one the server refuses is said there in the server's words, the
// confirmation still open. Once appealed, the alert's place says where the appeal stands, read from the server: that
// it waits, and what becomes of the post meanwhile (the server does not keep it: the reader posts it again once the
// refusal is lifted, and saves it as a draft to keep it until then); that the moderators kept the refusal; or that
// they lifted it, after which Post sends the post again. While the appeal waits or the refusal was kept, the post as
// it is would be refused again, so Post is disabled (`postHeld`) until the post is changed. An appeal the server did
// not take says why there, with what to do. The refusal, and what is said of its appeal, last as long as the post
// stays as it was refused: a change of it is a new post to send.
//
// Appeal is offered for as long as that read has not said that there is an appeal: while it is on its way, and when
// it failed, too. The server is the one that knows: it refuses a second appeal itself, and the alert then says that
// the refusal was appealed already; a way to read again would be one more state for the same end. A refusal whose
// last moment to appeal, where the server said one, has passed offers none, and the alert says that the time passed.

import {
  appealLimits,
  appealRefusalOf,
  refusedField,
  useAppeals,
  type HookAppeal,
  type HookRefusal,
} from "@my/app-data";
import { ConfirmDialog, focusRefusedField, FormAlert, TextButton, TextField, useLocale } from "@my/design-system";
import { useEffect, useId, useRef, useState } from "react";
import { stringsIn } from "../locale";

/** Why an appeal was not taken: the server's refusal of it, no text to send with it, or no answer. */
type Failure = "appealed" | "closed" | "not_appealable" | "text" | "other";

/**
 * Where the appeal the reader asked for is: not asked, being confirmed (with what is wrong with what they wrote for
 * the moderators, where something is), on its way, or not taken.
 */
type Asked =
  | { kind: "idle" | "sending" }
  | { kind: "asking"; commentError?: string | undefined }
  | { kind: "failed"; reason: Failure };

/** What is refused: the composer's post, or the article editor's article, whose sentences say so. */
type RefusedKind = "post" | "article";

/** The sentences of the alert and of the confirmation that name what was refused, by its kind. */
const sentences = {
  post: {
    reviewed: "compose.refused.reviewed",
    text: "compose.refused.appeal.text",
    state: "compose.refused.appeal.state",
    failed: "compose.refused.appeal.failed",
  },
  article: {
    reviewed: "compose.article.refused.reviewed",
    text: "compose.article.refused.appeal.text",
    state: "compose.article.refused.appeal.state",
    failed: "compose.article.refused.appeal.failed",
  },
} as const;

/**
 * Whether the post as it was refused is held back by its appeal: one that waits for the moderators, or that they
 * answered by keeping the refusal. Sent again as it is, it would be refused again.
 */
export const postHeld = (appeal: HookAppeal | null | undefined): boolean =>
  appeal?.state === "pending" || appeal?.state === "upheld";

type RefusedAppealProps = {
  refusal: HookRefusal;
  /**
   * The appeal as the server has it: null for none, undefined until it answered and when it could not be read (Appeal
   * is offered then as for none).
   */
  appeal: HookAppeal | null | undefined;
  /** The text the appeal carries: the post's text, or the article's Markdown body. */
  text: string;
  /** A post of the composer (the default), or an article of the article editor. */
  kind?: RefusedKind | undefined;
};

/**
 * The alert of a refused post or article its author may appeal: that the review did not accept it, with Appeal, then
 * where the appeal stands.
 */
export function RefusedAppeal({ refusal, appeal, text, kind = "post" }: RefusedAppealProps) {
  const { t } = stringsIn(useLocale().locale);
  const say = sentences[kind];
  const appeals = useAppeals();
  const fieldId = `${useId()}-comment`;
  const [asked, setAsked] = useState<Asked>({ kind: "idle" });
  // What the reader wrote for the moderators, kept while the confirmation closes and opens again.
  const [comment, setComment] = useState("");
  // Whether the field refused takes the focus once the confirmation is drawn again.
  const focusField = useRef(false);
  useEffect(() => {
    if (focusField.current) focusRefusedField(fieldId);
    focusField.current = false;
  });
  const refuseComment = (why: string) => {
    focusField.current = true;
    setAsked({ kind: "asking", commentError: why });
  };
  const send = () => {
    if (asked.kind === "sending") return;
    if (text.trim() === "") {
      setAsked({ kind: "failed", reason: "text" });
      return;
    }
    const written = comment.trim();
    // Counted as the server counts it, in code points.
    if (Array.from(written).length > appealLimits.comment) {
      refuseComment(t("compose.refused.appeal.comment.over", { max: appealLimits.comment }));
      return;
    }
    setAsked({ kind: "sending" });
    appeals.hook(refusal.decision, { status_text: text, ...(written === "" ? {} : { comment: written }) }).then(
      () => {
        setAsked({ kind: "idle" });
      },
      (error: unknown) => {
        const reason = appealRefusalOf(error);
        const named = refusedField(error);
        // What the server refused of the comment stands under it, in its words, the confirmation still open.
        if (reason === undefined && named?.member === "comment") {
          refuseComment(named.detail);
          return;
        }
        setAsked({
          kind: "failed",
          // A refusal that is not the reader's, or no longer kept, cannot be appealed either.
          reason: reason === "gone" ? "not_appealable" : (reason ?? (named === undefined ? "other" : "text")),
        });
      },
    );
  };
  // Once appealed, where the appeal stands; the moderators' lifting it is no error: Post, right after, posts again.
  if (appeal !== undefined && appeal !== null)
    return <FormAlert regular={appeal.state === "overturned"}>{t(say.state, { state: appeal.state })}</FormAlert>;
  const retry = asked.kind === "failed" && asked.reason === "other";
  // The last moment to appeal, where the refusal said one, has passed: nothing is offered, and the alert says so.
  const passed = refusal.deadline !== undefined && Date.parse(refusal.deadline) <= Date.now();
  if (passed) return <FormAlert>{t(say.failed, { reason: "closed" })}</FormAlert>;
  const busy = asked.kind === "sending";
  return (
    <>
      <FormAlert
        action={
          // Offered while no appeal of the refusal is known, whether the server said there is none, has not
          // answered yet, or could not be asked (it says itself that one was made already, which the alert then
          // says), and again after one that got no answer.
          asked.kind !== "failed" || retry ? (
            <TextButton
              onPress={() => {
                setAsked({ kind: "asking" });
              }}
            >
              {t("compose.refused.appeal")}
            </TextButton>
          ) : undefined
        }
      >
        {asked.kind === "failed" ? t(say.failed, { reason: asked.reason }) : t(say.reviewed)}
      </FormAlert>
      {(asked.kind === "asking" || busy) && (
        <ConfirmDialog
          title={t("compose.refused.appeal.title")}
          cancel={t("compose.refused.appeal.cancel")}
          confirm={t("compose.refused.appeal.confirm")}
          busy={busy}
          onCancel={() => {
            if (asked.kind === "asking") setAsked({ kind: "idle" });
          }}
          onConfirm={send}
          field={
            <TextField
              id={fieldId}
              name="comment"
              label={t("compose.refused.appeal.comment")}
              multiline
              defaultValue={comment}
              readOnly={busy}
              hint={
                asked.kind === "asking" && asked.commentError !== undefined
                  ? undefined
                  : t("compose.refused.appeal.comment.hint", { max: appealLimits.comment })
              }
              error={asked.kind === "asking" ? asked.commentError : undefined}
              onChange={(value) => {
                setComment(value);
                // What was said of the comment was about it as it was.
                setAsked((was) => (was.kind === "asking" && was.commentError !== undefined ? { kind: "asking" } : was));
              }}
            />
          }
        >
          {t(say.text)}
        </ConfirmDialog>
      )}
    </>
  );
}
