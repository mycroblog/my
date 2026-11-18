// SPDX-License-Identifier: AGPL-3.0-only

// A post that this server's automated review refused, in the composer (ADR-0147 §9): the refusal is appealed here,
// where the post still is, since the server keeps no refused post and so no list of them. The form's alert says the
// server's words, which end with that the author may appeal, and Appeal after them; it asks first, since a refusal is
// appealed once and the appeal sends the post's text to the moderators. Once appealed, the alert's place says where
// the appeal stands, read from the server: that it waits, that the moderators kept the refusal, or that they lifted
// it, after which Post sends the post again. An appeal the server did not take says why there, with what to do. The
// refusal, and what is said of its appeal, last as long as the post stays as it was refused: a change of it is a new
// post to send.

import { appealRefusalOf, refusedField, useAppeals, useHookAppeal, type HookRefusal } from "@my/app-data";
import { ConfirmDialog, FormAlert, TextButton, useLocale } from "@my/design-system";
import { useState } from "react";
import { stringsIn } from "../locale";

/** Why an appeal was not taken: the server's refusal of it, no text to send with it, or no answer. */
type Failure = "appealed" | "closed" | "not_appealable" | "text" | "other";

/** Where the appeal the reader asked for is: not asked, being confirmed, on its way, or not taken. */
type Asked = { kind: "idle" | "asking" | "sending" } | { kind: "failed"; reason: Failure };

/**
 * The alert of a refused post its author may appeal: the server's words (`words`) with Appeal, then where the appeal
 * stands. `text` is the post's text, which the appeal carries.
 */
export function RefusedAppeal({ refusal, words, text }: { refusal: HookRefusal; words: string; text: string }) {
  const { t } = stringsIn(useLocale().locale);
  const appeal = useHookAppeal(refusal.decision);
  const appeals = useAppeals();
  const [asked, setAsked] = useState<Asked>({ kind: "idle" });
  const send = () => {
    if (asked.kind === "sending") return;
    if (text.trim() === "") {
      setAsked({ kind: "failed", reason: "text" });
      return;
    }
    setAsked({ kind: "sending" });
    appeals.hook(refusal.decision, { status_text: text }).then(
      () => {
        setAsked({ kind: "idle" });
      },
      (error: unknown) => {
        const reason = appealRefusalOf(error);
        setAsked({
          kind: "failed",
          // A refusal that is not the reader's, or no longer kept, cannot be appealed either.
          reason:
            reason === "gone" ? "not_appealable" : (reason ?? (refusedField(error) === undefined ? "other" : "text")),
        });
      },
    );
  };
  // Once appealed, where the appeal stands; the moderators' lifting it is no error: Post, right after, posts again.
  if (appeal !== undefined && appeal !== null)
    return (
      <FormAlert regular={appeal.state === "overturned"}>
        {t("compose.refused.appeal.state", { state: appeal.state })}
      </FormAlert>
    );
  const retry = asked.kind === "failed" && asked.reason === "other";
  return (
    <>
      <FormAlert
        action={
          // Offered once the server said there is no appeal yet, and again after one that got no answer.
          appeal === null && (asked.kind !== "failed" || retry) ? (
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
        {asked.kind === "failed" ? t("compose.refused.appeal.failed", { reason: asked.reason }) : words}
      </FormAlert>
      {(asked.kind === "asking" || asked.kind === "sending") && (
        <ConfirmDialog
          title={t("compose.refused.appeal.title")}
          cancel={t("compose.refused.appeal.cancel")}
          confirm={t("compose.refused.appeal.confirm")}
          busy={asked.kind === "sending"}
          onCancel={() => {
            if (asked.kind === "asking") setAsked({ kind: "idle" });
          }}
          onConfirm={send}
        >
          {t("compose.refused.appeal.text")}
        </ConfirmDialog>
      )}
    </>
  );
}
