// SPDX-License-Identifier: AGPL-3.0-only

// Reporting to the moderators (FR-701, FR-709): a panel over the page (a dialog from `tablet`, a screen of its own on a
// phone) that reports an account, a voice chat room (its host) or an event (its organizer). It says first who reads it
// and that the account is never told who sent it; then why (spam, illegal content, a break of the server's rules, which
// it then asks for, or something else), for an account the posts of it to include (the one it was opened from chosen
// already, more of its posts on request, 20 at most), a comment, and for a remote account whether to send the report
// to its server too, without the reader's name. What is missing is said under what it is about when the reader sends,
// and what the server refused right before Send. The forward choice, the refusal and the buttons are the panel's foot,
// in sight under the choices as they scroll. Once sent, it says so in its title and offers to mute the account, done
// at once, or to block it, which the feature asks first; each said in its place once done. It only displays: the
// feature holds what the reader chose and sends it.
//
// For assistive technology: the choices are groups named by their headings; a missing choice or a comment the server
// refused takes the focus with its error; once sent, the panel's live region, there from the start, says so, and the
// focus goes to Done; a mute or a block done says so in its button's place, with its focus.

import {
  Box,
  Button,
  CheckField,
  ChoiceList,
  Excerpt,
  FieldError,
  focusRefusedField,
  FormAlert,
  GroupHeading,
  PanelDialog,
  RelativeTime,
  Row,
  Spoken,
  Stack,
  StateText,
  TextButton,
  TextField,
  useLocale,
} from "@my/design-system";
import { useEffect, useId, useRef, useState } from "react";
import { sentenceHandleOf } from "./account";
import { stringsIn } from "./locale";
import { excerptHtml, PostContent, type ContentEmoji } from "./post-content";

/** Why an account is reported (the API's ReportCategory). */
export type ReportCategory = "spam" | "legal" | "violation" | "other";

const categories: readonly ReportCategory[] = ["spam", "legal", "violation", "other"];

/** The account a report is about, as a status embeds it. */
export type ReportAccount = { id: string; username: string; acct: string; domain: string };

/** What is reported: an account (with its posts), a voice chat room and its host, or an event and its organizer. */
export type ReportTarget =
  { kind: "account"; account: ReportAccount } | { kind: "room" | "event"; account: ReportAccount; name: string };

/** A post of the account that a report may include (the part of the API's Status it shows). */
export type ReportPost = {
  id: string;
  content: string;
  spoiler_text?: string | undefined;
  emojis: readonly ContentEmoji[];
  created_at: string;
  language?: string | null | undefined;
  article?: { title: string } | undefined;
};

/** A rule of the server that a report of a violation cites. */
export type ReportRule = { id: string; text: string; hint?: string | undefined };

/** What the reader chose and wrote so far. */
export type ReportDraft = {
  category: ReportCategory | undefined;
  /** The IDs of the rules cited, for a violation. */
  rules: readonly string[];
  /** The IDs of the posts included, for an account. */
  posts: readonly string[];
  comment: string;
  /** Send it to a remote account's server too. */
  forward: boolean;
};

/** An empty draft, with the post it was opened from chosen. */
export const reportDraft = (post?: string): ReportDraft => ({
  category: undefined,
  rules: [],
  posts: post === undefined ? [] : [post],
  comment: "",
  forward: false,
});

/** The account's posts to choose from, as far as they were read. */
export type ReportPosts = {
  items: readonly ReportPost[];
  status: "pending" | "error" | "success";
  /** There are older posts to read. */
  more: boolean;
  /** The older posts are being read. */
  loadingMore: boolean;
  onMore: () => void;
  onRetry: () => void;
};

/** Mute or block after the report: its state, and its press. */
export type ReportAfter = { state: "ready" | "busy" | "done" | "failed"; onPress: () => void };

/** The limits of a report, as the server checks them. */
export const reportPanelLimits = { posts: 20, comment: 1000 } as const;

export type ReportPanelProps = {
  target: ReportTarget;
  /** The server's name, which says whose moderators read it. */
  instance: string;
  /** The server's rules in effect; without any, a violation of them is not offered. */
  rules: readonly ReportRule[];
  /** The account's posts to choose from, for an account. */
  posts?: ReportPosts | undefined;
  draft: ReportDraft;
  onChange: (draft: ReportDraft) => void;
  /** Being written, being sent, or sent. */
  phase: "writing" | "sending" | "sent";
  /** What the server said of a refused report, right before Send. */
  refusal?: string | undefined;
  /** The server refused the comment as it was sent. */
  commentRefused?: boolean | undefined;
  /** Sends the draft, once nothing is missing. */
  onSend: () => void;
  /** Cancel, close, Escape, a press outside, or Done. */
  onClose: () => void;
  /** Mute the account, once sent; none where the reader mutes it already. */
  mute?: ReportAfter | undefined;
  /** Block the account, once sent; none where the reader blocks it already. */
  block?: ReportAfter | undefined;
  /** Over the page, or in the flow of a page (a story's specimen). */
  layer?: boolean | undefined;
};

/** How many code points `text` holds, as the server counts a comment. */
const codePoints = (text: string) => Array.from(text).length;

type T = ReturnType<typeof stringsIn>["t"];

/** A choice's name with what it means under it, muted. */
function Named({ name, hint }: { name: string; hint?: string | undefined }) {
  return (
    <Stack as="span" gap="hair">
      <span>{name}</span>
      {hint !== undefined && hint !== "" && (
        <Box as="span" tone="muted">
          {hint}
        </Box>
      )}
    </Stack>
  );
}

/** A post as a choice: when it was posted, and its first words (its content warning, where it has one). */
function PostChoice({ post, t }: { post: ReportPost; t: T }) {
  const warning = post.spoiler_text ?? "";
  const html = excerptHtml(post);
  return (
    <Stack as="span" gap="hair">
      <Box as="span" text="caption" tone="muted">
        <RelativeTime date={post.created_at} />
      </Box>
      <Excerpt tone="text" lang={post.language ?? undefined}>
        {warning !== "" ? (
          <span dir="auto">{t("report.post_warning", { warning })}</span>
        ) : html === "" ? (
          <span>{t("report.post_no_text")}</span>
        ) : (
          <PostContent html={html} emojis={post.emojis} lang={undefined} />
        )}
      </Excerpt>
    </Stack>
  );
}

/** The posts of the account to include: those read, the chosen ones kept, and more on request. */
function Posts({
  posts,
  draft,
  onChange,
  disabled,
  t,
}: {
  posts: ReportPosts;
  draft: ReportDraft;
  onChange: (draft: ReportDraft) => void;
  disabled: boolean;
  t: T;
}) {
  const id = useId();
  const max = reportPanelLimits.posts;
  const heading = t("report.posts");
  return (
    <Stack gap="entry-line">
      <GroupHeading>{heading}</GroupHeading>
      <Box as="p" id={`${id}-hint`} tone="muted">
        {t("report.posts_hint", { max, count: draft.posts.length })}
      </Box>
      {posts.items.length > 0 && (
        <ChoiceList
          label={heading}
          multiple
          describedBy={`${id}-hint`}
          disabled={disabled}
          items={posts.items.map((post) => ({ key: post.id, label: <PostChoice post={post} t={t} /> }))}
          chosen={posts.items.flatMap((post, i) => (draft.posts.includes(post.id) ? [i] : []))}
          onChange={(chosen) => {
            const ids = chosen.flatMap((i) => posts.items[i]?.id ?? []);
            // Past the limit, a further post is not taken.
            if (ids.length > max) return;
            onChange({ ...draft, posts: ids });
          }}
        />
      )}
      {posts.status === "pending" && posts.items.length === 0 && (
        <Box as="p" tone="muted" role="status">
          {t("report.posts_loading")}
        </Box>
      )}
      {posts.status === "success" && posts.items.length === 0 && (
        <Box as="p" tone="muted">
          {t("report.posts_none")}
        </Box>
      )}
      {posts.status === "error" && (
        <Row wrap align="baseline" gapX="entry-caption">
          <Box as="p">{t("report.posts_failed")}</Box>
          <TextButton onPress={posts.onRetry}>{t("report.retry")}</TextButton>
        </Row>
      )}
      {posts.more && posts.status !== "error" && (
        <Box>
          <Button size="timeline" busy={posts.loadingMore} onPress={posts.onMore}>
            {t("report.posts_more")}
          </Button>
        </Box>
      )}
    </Stack>
  );
}

/**
 * Mute or Block… once sent, named with the account for assistive technology, or, once done, that it was, in the
 * button's place and with the focus the button leaves; a failure says so, over the button.
 */
function After({ what, after, handle, t }: { what: "mute" | "block"; after: ReportAfter; handle: string; t: T }) {
  const done = useRef<HTMLSpanElement>(null);
  const was = useRef(after.state);
  useEffect(() => {
    if (after.state === "done" && was.current !== "done") done.current?.focus();
    was.current = after.state;
  }, [after.state]);
  if (after.state === "done")
    return (
      <Box>
        <StateText ref={done}>{t(what === "mute" ? "report.muted" : "report.blocked", { handle })}</StateText>
      </Box>
    );
  return (
    <Stack gap="entry-line">
      {after.state === "failed" && (
        <FormAlert regular>{t(what === "mute" ? "report.mute_failed" : "report.block_failed", { handle })}</FormAlert>
      )}
      <Box>
        <Button
          size="form"
          busy={after.state === "busy"}
          aria-label={t(what === "mute" ? "report.mute" : "report.block", { handle })}
          onPress={after.onPress}
        >
          {t(what === "mute" ? "report.mute_short" : "report.block_short")}
        </Button>
      </Box>
    </Stack>
  );
}

/** What follows the sending: under its heading, which names the account, mute and Block…; Done is the panel's foot. */
function Sent({
  target,
  mute,
  block,
  t,
}: {
  target: ReportTarget;
  mute: ReportAfter | undefined;
  block: ReportAfter | undefined;
  t: T;
}) {
  const handle = sentenceHandleOf(target.account);
  if (mute === undefined && block === undefined) return null;
  return (
    <Stack gap="entry-line">
      <GroupHeading>{t("report.also", { handle })}</GroupHeading>
      <Row wrap align="start" gap="dialog-row">
        {mute !== undefined && <After what="mute" after={mute} handle={handle} t={t} />}
        {block !== undefined && <After what="block" after={block} handle={handle} t={t} />}
      </Row>
    </Stack>
  );
}

/** Done, the sent panel's foot, which takes the focus once the form that was sent has gone. */
function DoneFoot({ onClose, t }: { onClose: () => void; t: T }) {
  const place = useRef<HTMLDivElement>(null);
  useEffect(() => {
    place.current?.querySelector<HTMLElement>("button")?.focus();
  }, []);
  return (
    <Row as={<div ref={place} />} justify="end">
      <Button kind="primary" size="form" onPress={onClose}>
        {t("report.done")}
      </Button>
    </Row>
  );
}

/** What is missing of a draft, which the reader is told as they send. */
type Missing = "category" | "rules" | null;

const missingOf = (draft: ReportDraft, rules: readonly ReportRule[]): Missing => {
  if (draft.category === undefined) return "category";
  if (draft.category === "violation" && rules.length > 0 && draft.rules.length === 0) return "rules";
  return null;
};

/** The panel that reports an account, a voice chat room or an event to the moderators. */
export function ReportPanel({
  target,
  instance,
  rules,
  posts,
  draft,
  onChange,
  phase,
  refusal,
  commentRefused = false,
  onSend,
  onClose,
  mute,
  block,
  layer = true,
}: ReportPanelProps) {
  const { t } = stringsIn(useLocale().locale);
  const id = useId();
  const [tried, setTried] = useState(false);
  const handle = sentenceHandleOf(target.account);
  const sending = phase === "sending";
  const commentId = `${id}-comment`;
  useEffect(() => {
    if (commentRefused) focusRefusedField(commentId);
  }, [commentRefused, commentId]);
  const title =
    phase === "sent"
      ? t("report.sent_title")
      : target.kind === "account"
        ? t("report.title_account", { handle })
        : t(target.kind === "room" ? "report.title_room" : "report.title_event", { name: target.name });
  // The panel's one polite live region, there from the start: it says that the report was sent.
  const said = (
    <Box role="status">
      <Spoken>{phase === "sent" ? t("report.sent_title") : ""}</Spoken>
    </Box>
  );
  if (phase === "sent")
    return (
      <PanelDialog
        title={title}
        closeLabel={t("report.close")}
        onClose={onClose}
        layer={layer}
        foot={<DoneFoot onClose={onClose} t={t} />}
      >
        {said}
        <Sent target={target} mute={mute} block={block} t={t} />
      </PanelDialog>
    );
  const offered = rules.length > 0 ? categories : categories.filter((c) => c !== "violation");
  const missing = tried ? missingOf(draft, rules) : null;
  const tooLong = codePoints(draft.comment) > reportPanelLimits.comment;
  const remote = target.kind === "account" && target.account.acct.includes("@");
  const categoryId = `${id}-category`;
  const rulesId = `${id}-rules`;
  const forwardId = `${id}-forward`;
  const send = () => {
    if (sending) return;
    setTried(true);
    const now = missingOf(draft, rules);
    if (now === "category") focusRefusedField(categoryId);
    else if (now === "rules") focusRefusedField(rulesId);
    else if (tooLong) focusRefusedField(commentId);
    else onSend();
  };
  const commentError = tooLong || commentRefused;
  return (
    <PanelDialog
      title={title}
      closeLabel={t("report.close")}
      onClose={() => {
        if (!sending) onClose();
      }}
      layer={layer}
      foot={
        <Stack gap="entry-line">
          {remote && (
            <Stack gap="entry-line">
              <CheckField
                id={forwardId}
                name="forward"
                value="1"
                label={t("report.forward", { domain: target.account.domain })}
                checked={draft.forward}
                disabled={sending}
                describedBy={`${forwardId}-hint`}
                onChange={(forward) => {
                  onChange({ ...draft, forward });
                }}
              />
              <Box as="p" id={`${forwardId}-hint`} tone="muted">
                {t("report.forward_hint")}
              </Box>
            </Stack>
          )}
          {refusal !== undefined && <FormAlert>{refusal}</FormAlert>}
          <Row wrap align="center" justify="end" gap="dialog-row">
            <Button kind="quiet" size="form" isDisabled={sending} onPress={onClose}>
              {t("report.cancel")}
            </Button>
            <Button kind="primary" size="form" busy={sending} onPress={send}>
              {t("report.send")}
            </Button>
          </Row>
        </Stack>
      }
    >
      {said}
      <Stack gap="entry-room">
        <Box as="p" dir="auto">
          {t("report.intro", { kind: target.kind, instance, handle })}
        </Box>
        <Stack gap="entry-line">
          <GroupHeading>{t("report.category")}</GroupHeading>
          <ChoiceList
            id={categoryId}
            label={t("report.category")}
            multiple={false}
            disabled={sending}
            describedBy={missing === "category" ? `${categoryId}-error` : undefined}
            items={offered.map((c) => ({
              key: c,
              label: (
                <Named
                  name={t("report.category_name", { category: c })}
                  hint={c === "spam" ? t("report.spam_hint") : undefined}
                />
              ),
            }))}
            chosen={draft.category === undefined ? [] : [offered.indexOf(draft.category)]}
            onChange={([i]) => {
              const category = offered[i ?? -1];
              if (category !== undefined)
                onChange({ ...draft, category, rules: category === "violation" ? draft.rules : [] });
            }}
          />
          {missing === "category" && <FieldError id={`${categoryId}-error`}>{t("report.category_missing")}</FieldError>}
        </Stack>
        {draft.category === "violation" && rules.length > 0 && (
          <Stack gap="entry-line">
            <GroupHeading>{t("report.rules")}</GroupHeading>
            <ChoiceList
              id={rulesId}
              label={t("report.rules")}
              multiple
              disabled={sending}
              describedBy={missing === "rules" ? `${rulesId}-error` : undefined}
              items={rules.map((r) => ({ key: r.id, label: <Named name={r.text} hint={r.hint} /> }))}
              chosen={rules.flatMap((r, i) => (draft.rules.includes(r.id) ? [i] : []))}
              onChange={(chosen) => {
                onChange({ ...draft, rules: chosen.flatMap((i) => rules[i]?.id ?? []) });
              }}
            />
            {missing === "rules" && <FieldError id={`${rulesId}-error`}>{t("report.rules_missing")}</FieldError>}
          </Stack>
        )}
        {target.kind === "account" && posts !== undefined && (
          <Posts posts={posts} draft={draft} onChange={onChange} disabled={sending} t={t} />
        )}
        <Stack gap="entry-line">
          <GroupHeading>{t("report.comment")}</GroupHeading>
          <TextField
            id={commentId}
            name="comment"
            label={t("report.comment")}
            labelHidden
            hint={commentError ? undefined : t("report.comment_hint", { max: reportPanelLimits.comment })}
            error={commentError ? t("report.comment_rule", { max: reportPanelLimits.comment }) : undefined}
            defaultValue={draft.comment}
            multiline
            rows={3}
            readOnly={sending}
            onChange={(comment) => {
              onChange({ ...draft, comment });
            }}
          />
        </Stack>
      </Stack>
    </PanelDialog>
  );
}
