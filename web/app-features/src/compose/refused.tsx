// SPDX-License-Identifier: AGPL-3.0-only

// A post that this server's automated review refused, in the composer (ADR-0147 §9): the refusal is appealed here,
// where the post still is, since the server keeps no refused post and so no list of them. The form's alert says, in
// the app's own sentence, that the review did not accept the post, and Appeal after it (the server's sentence says
// that the author may appeal, for an app with no such action: said beside Appeal it would say it twice); it asks
// first, since a refusal is appealed once and the appeal sends the post's text to the moderators. Once appealed, the
// alert's place says where the appeal stands, read from the server: that it waits, and what becomes of the post
// meanwhile (the server does not keep it: the reader posts it again once the refusal is lifted, and saves it as a
// draft to keep it until then); that the moderators kept the refusal; or that they lifted it, after which Post sends
// the post again. While the appeal waits or the refusal was kept, the post as it is would be refused again, so Post
// is disabled (`postHeld`) until the post is changed. An appeal the server did not take says why there, with what to
// do. The refusal, and what is said of its appeal, last as long as the post stays as it was refused: a change of it
// is a new post to send.
//
// Appeal is offered for as long as that read has not said that there is an appeal: while it is on its way, and when
// it failed, too. The server is the one that knows: it refuses a second appeal itself, and the alert then says that
// the refusal was appealed already; a way to read again would be one more state for the same end. A refusal whose
// last moment to appeal, where the server said one, has passed offers none, and the alert says that the time passed.

import { appealRefusalOf, refusedField, useAppeals, type HookAppeal, type HookRefusal } from "@my/app-data";
import { ConfirmDialog, FormAlert, TextButton, useLocale } from "@my/design-system";
import { useState } from "react";
import { stringsIn } from "../locale";

/** Why an appeal was not taken: the server's refusal of it, no text to send with it, or no answer. */
type Failure = "appealed" | "closed" | "not_appealable" | "text" | "other";

/** Where the appeal the reader asked for is: not asked, being confirmed, on its way, or not taken. */
type Asked = { kind: "idle" | "asking" | "sending" } | { kind: "failed"; reason: Failure };

/**
 * Whether the post as it was refused is held back by its appeal: one that waits for the moderators, or that they
 * answered by keeping the refusal. Sent again as it is, it would be refused again.
 */
export const postHeld = (appeal: HookAppeal | null | undefined): boolean =>
  appeal?.state === "pending" || appeal?.state === "upheld";

/**
 * The alert of a refused post its author may appeal: that the review did not accept it, with Appeal, then where the
 * appeal stands. `appeal` is the appeal as the server has it (null for none, undefined until it answered and when it
 * could not be read: Appeal is offered then as for none), and `text` the post's text, which the appeal carries.
 */
export function RefusedAppeal({
  refusal,
  appeal,
  text,
}: {
  refusal: HookRefusal;
  appeal: HookAppeal | null | undefined;
  text: string;
}) {
  const { t } = stringsIn(useLocale().locale);
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
  // The last moment to appeal, where the refusal said one, has passed: nothing is offered, and the alert says so.
  const passed = refusal.deadline !== undefined && Date.parse(refusal.deadline) <= Date.now();
  if (passed) return <FormAlert>{t("compose.refused.appeal.failed", { reason: "closed" })}</FormAlert>;
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
        {asked.kind === "failed"
          ? t("compose.refused.appeal.failed", { reason: asked.reason })
          : t("compose.refused.reviewed")}
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
