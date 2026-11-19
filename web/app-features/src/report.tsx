// SPDX-License-Identifier: AGPL-3.0-only

// Reporting to the moderators (FR-701, FR-709), the same from every place that offers it (a post's menu, a profile's,
// a notification's, a voice chat room's and an event's): the report panel of @my/app-components over what the reader
// was reading, which this holds. It reads the server's rules (the instance's information) and, for an account, its
// posts the reader sees, the one it was opened from chosen first even where it is older than the posts read; it sends
// the report through @my/app-data and says what the server refused, in its words, the panel staying as it was. Once
// sent, the reader may mute the account from it, or block it after the confirmation the profile asks: by the
// relationship's write, or by what the caller does for it (a post's menu leaves its line in the post's place,
// post-actions.tsx). It is shared by the features, not a feature
// of its own: a feature never imports another.

import {
  reportDraft,
  ReportPanel,
  sentenceHandleOf,
  type ReportAccount,
  type ReportAfter,
  type ReportDraft,
  type ReportPost,
  type ReportTarget,
} from "@my/app-components";
import {
  refusal,
  reportFaultField,
  useAccountStatuses,
  useChangeRelationship,
  useFileReport,
  useInstance,
  useRelationship,
  type CreateReport,
} from "@my/app-data";
import { ConfirmDialog, ModalPresence, useLocale } from "@my/design-system";
import { useState } from "react";
import { stringsIn } from "./locale";

/**
 * What a report is about: an account, opened from one of its posts or none; a voice chat room of this server, with its
 * host; or an event of this server, with its organizer.
 */
export type ReportSubject =
  | { kind: "account"; account: ReportAccount; post?: ReportPost | undefined }
  | { kind: "room" | "event"; account: ReportAccount; id: string; name: string };

/** What the reader may do to the reported account once the report is sent. */
export type ReportAct = (action: "mute" | "block") => Promise<void>;

/** The request a draft sends about `subject`. */
export function reportRequest(subject: ReportSubject, draft: ReportDraft): CreateReport {
  const comment = draft.comment.trim();
  const account = subject.kind === "account";
  return {
    account_id: subject.account.id,
    category: draft.category ?? "other",
    ...(draft.category === "violation" && draft.rules.length > 0 ? { rule_ids: [...draft.rules] } : {}),
    ...(comment === "" ? {} : { comment }),
    ...(account && draft.posts.length > 0 ? { status_ids: [...draft.posts] } : {}),
    ...(subject.kind === "room" ? { voice_room_id: subject.id } : {}),
    ...(subject.kind === "event" ? { event_id: subject.id } : {}),
    // A room or an event is never forwarded, and a local account has no other server.
    forward: account && draft.forward && subject.account.acct.includes("@"),
  };
}

/** The account's posts to choose from: the post it was opened from first, then those read, newest first. */
function usePosts(subject: ReportSubject) {
  const account = subject.kind === "account";
  const list = useAccountStatuses(subject.account.id, "posts", account);
  if (!account) return undefined;
  // A boost is not the account's post: the reader reports what it wrote.
  const read = list.items.filter((s) => s.boost === undefined);
  const first = subject.post;
  const items = first === undefined ? read : [first, ...read.filter((s) => s.id !== first.id)];
  return {
    items,
    status: list.status,
    more: list.hasNextPage,
    loadingMore: list.isFetchingNextPage,
    onMore: list.fetchNextPage,
    onRetry: list.refetch,
  };
}

/** Where a mute or a block after the report stands. */
type AfterState = ReportAfter["state"];

/**
 * The report panel about `subject`, over the page: `onClose` takes it away (Cancel, Done, close). `act` mutes or blocks
 * the account after the report where the caller does it its own way; else the relationship's write does.
 */
export function ReportDialog({
  subject,
  onClose,
  act,
  layer = true,
}: {
  subject: ReportSubject;
  onClose: () => void;
  act?: ReportAct | undefined;
  /** Over the page, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
}) {
  const { t } = stringsIn(useLocale().locale);
  const instance = useInstance();
  const file = useFileReport();
  const change = useChangeRelationship();
  const relationship = useRelationship(subject.account.id);
  const posts = usePosts(subject);
  const [draft, setDraft] = useState<ReportDraft>(() =>
    reportDraft(subject.kind === "account" ? subject.post?.id : undefined),
  );
  const [phase, setPhase] = useState<"writing" | "sending" | "sent">("writing");
  const [refused, setRefused] = useState<{ words: string; comment: boolean } | undefined>(undefined);
  // Block…, asked first as the profile's is.
  const [asking, setAsking] = useState(false);
  const [after, setAfter] = useState<Readonly<Record<"mute" | "block", AfterState>>>({ mute: "ready", block: "ready" });
  const target: ReportTarget =
    subject.kind === "account"
      ? { kind: "account", account: subject.account }
      : { kind: subject.kind, account: subject.account, name: subject.name };
  const send = () => {
    setPhase("sending");
    setRefused(undefined);
    file(reportRequest(subject, draft)).then(
      () => {
        setPhase("sent");
      },
      (error: unknown) => {
        setPhase("writing");
        setRefused({ words: refusal(error) ?? t("report.failed"), comment: reportFaultField(error) === "comment" });
      },
    );
  };
  const doAfter = (action: "mute" | "block") => () => {
    if (after[action] === "busy" || after[action] === "done") return;
    setAfter((now) => ({ ...now, [action]: "busy" }));
    (act ?? ((a: "mute" | "block") => change(subject.account.id, a)))(action).then(
      () => {
        setAfter((now) => ({ ...now, [action]: "done" }));
      },
      () => {
        setAfter((now) => ({ ...now, [action]: "failed" }));
      },
    );
  };
  // What the reader does already is not offered again; a block mutes all a mute would.
  const blocking = relationship?.blocking === true && after.block !== "done";
  const muting = relationship?.muting === true && after.mute !== "done";
  return (
    <>
      <ReportPanel
        target={target}
        instance={instance?.name || instance?.domain || ""}
        rules={instance?.rules ?? []}
        posts={posts}
        draft={draft}
        onChange={(next) => {
          setDraft(next);
          if (refused?.comment === true && next.comment !== draft.comment) setRefused(undefined);
        }}
        phase={phase}
        refusal={refused?.words}
        commentRefused={refused?.comment === true}
        onSend={send}
        onClose={onClose}
        mute={muting || blocking ? undefined : { state: after.mute, onPress: doAfter("mute") }}
        block={
          blocking
            ? undefined
            : {
                state: after.block,
                onPress: () => {
                  if (after.block !== "busy" && after.block !== "done") setAsking(true);
                },
              }
        }
        layer={layer}
      />
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            key="block"
            title={t("profile.block_confirm_title", { handle: sentenceHandleOf(subject.account) })}
            cancel={t("profile.cancel")}
            confirm={t("profile.block_confirm")}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={() => {
              setAsking(false);
              doAfter("block")();
            }}
          >
            {t("profile.block_confirm_text")}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </>
  );
}
