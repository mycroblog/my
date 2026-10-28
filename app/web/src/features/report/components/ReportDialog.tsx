// SPDX-License-Identifier: AGPL-3.0-only
// The dialog that reports an account to the moderators (FR-701, FR-709), opened from the menu of one of its
// posts or of its profile. It is a modal dialog (WAI-ARIA APG, dialog pattern) that goes through one step at a
// time: the reason, the rules it breaks (for a rule violation), the posts, then a comment and, for an account
// of another server, forwarding; or, from a voice chat room's page, reports the room and its host without the
// posts step (ADR-0100 §11). Each step is a form whose heading takes the focus when the step appears, so a
// screen reader reads where the user is and Tab goes on from there; Back keeps the answers. Above the heading, a
// Stepper shows how far the report has come, in words and as segments.
import {
  Button,
  Checkbox,
  Dialog,
  DialogActions,
  Heading,
  Hint,
  InlineError,
  PostContent,
  Radio,
  RadioGroup,
  RegionError,
  serverSanitizedHtml,
  Spinner,
  Stack,
  Stepper,
  Text,
  TextArea,
} from "@my/ui";
import { type ReactNode, useEffect, useId, useRef } from "react";
import { useIntl } from "react-intl";
import { type ClientMessageKey, useT } from "../../../lib/i18n";
import {
  type Account,
  maxReportComment,
  maxReportItems,
  reportCategories,
  type ReportCategory,
  type Status,
} from "../api/reports";
import { type ReportedVoiceRoom, type ReportForm, type ReportStep, useReportForm } from "../hooks/useReportForm";

export type ReportDialogProps = {
  /** The account reported. */
  account: Account;
  /** The post the report starts from, chosen at first; none from a profile. */
  status?: Status | undefined;
  /** The voice chat room the report is about, whose host is `account`; its report cites no posts (ADR-0100 §11). */
  voiceRoom?: ReportedVoiceRoom | undefined;
  isOpen: boolean;
  onOpenChange: (isOpen: boolean) => void;
};

/** The name of an account in the UI, wrapped in directional isolates (FSI … PDI) for a sentence around it. */
function isolatedName(account: Account): string {
  return `⁨${account.display_name || account.username}⁩`;
}

/** A domain in a sentence: it reads left to right in every locale (LRI … PDI). */
function isolatedDomain(domain: string): string {
  return `⁦${domain}⁩`;
}

const categoryLabels: Readonly<Record<ReportCategory, { label: ClientMessageKey; description: ClientMessageKey }>> = {
  spam: { label: "client:report.category.spam", description: "client:report.category.spam.description" },
  legal: { label: "client:report.category.legal", description: "client:report.category.legal.description" },
  violation: { label: "client:report.category.violation", description: "client:report.category.violation.description" },
  other: { label: "client:report.category.other", description: "client:report.category.other.description" },
};

const stepHeadings: Readonly<Record<ReportStep, ClientMessageKey>> = {
  category: "client:report.category.heading",
  rules: "client:report.rules.heading",
  posts: "client:report.posts.heading",
  comment: "client:report.comment.heading",
  sent: "client:report.sent.heading",
};

/** The report dialog. It loads the rules and the posts when it opens, and every opening starts a new draft. */
export function ReportDialog({ account, status, voiceRoom, isOpen, onOpenChange }: ReportDialogProps) {
  const t = useT();
  const title =
    voiceRoom === undefined
      ? t("client:report.title", { name: isolatedName(account) })
      : t("client:report.title_room", { room: `⁨${voiceRoom.name}⁩` });
  return (
    <Dialog
      title={title}
      closeLabel={t("client:report.close")}
      isDismissable={false}
      isOpen={isOpen}
      onOpenChange={onOpenChange}
    >
      {/* The steps mount with the dialog's content, so closing the dialog drops the draft. */}
      {({ close }) => <ReportSteps account={account} status={status} voiceRoom={voiceRoom} close={close} />}
    </Dialog>
  );
}

type ReportStepsProps = {
  account: Account;
  status: Status | undefined;
  voiceRoom: ReportedVoiceRoom | undefined;
  close: () => void;
};

function ReportSteps({ account, status, voiceRoom, close }: ReportStepsProps) {
  const t = useT();
  const form = useReportForm(account, status, voiceRoom);
  const headingId = useId();
  // A new key per step mounts a new heading, which takes the focus.
  const heading = (
    <StepHeading key={form.step} id={headingId}>
      {t(
        voiceRoom !== undefined && form.step === "category"
          ? "client:report.room.category.heading"
          : stepHeadings[form.step],
      )}
    </StepHeading>
  );

  if (form.step === "sent") {
    return (
      <Stack gap="4">
        {heading}
        <p>{t("client:report.sent.body")}</p>
        {form.report?.forwarded === true && form.remoteDomain !== null && (
          <Text as="p" wrap="anywhere">
            {t("client:report.sent.forwarded", { domain: isolatedDomain(form.remoteDomain) })}
          </Text>
        )}
        <DialogActions>
          <Button variant="primary" onPress={close}>
            {t("client:report.done")}
          </Button>
        </DialogActions>
      </Stack>
    );
  }

  const isLast = form.position.current === form.position.total;
  return (
    <Stack as="form" onSubmit={form.next} aria-labelledby={headingId} gap="4">
      <Stack gap="1">
        <Stepper
          current={form.position.current}
          total={form.position.total}
          label={t("client:report.step", { current: form.position.current, total: form.position.total })}
        />
        {heading}
      </Stack>
      {voiceRoom !== undefined && form.step === "category" && (
        <Text as="p" wrap="anywhere">
          {t("client:report.room.target", { name: isolatedName(account) })}
        </Text>
      )}
      <StepFields form={form} headingId={headingId} />
      {form.error !== undefined && (
        <InlineError alert size="inherit">
          {t("client:report.failed", { reason: form.error })}
        </InlineError>
      )}
      <DialogActions>
        {form.position.current > 1 && <Button onPress={form.back}>{t("client:report.back")}</Button>}
        <Button type="submit" variant="primary" isPending={form.isPending}>
          {isLast ? t("client:report.submit") : t("client:report.next")}
        </Button>
      </DialogActions>
    </Stack>
  );
}

/**
 * A step's heading. It takes the focus when it mounts (react.dev, "Synchronizing with Effects", focusing a
 * field on mount), which is before the dialog's own effect, so the dialog leaves the focus on it at opening.
 */
function StepHeading({ id, children }: { id: string; children: ReactNode }) {
  const ref = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    ref.current?.focus();
  }, []);
  // The dialog's title is an h2, so the steps are one level below it.
  return (
    <Heading level={3} ref={ref} id={id} wrap="pretty" focusable>
      {children}
    </Heading>
  );
}

function StepFields({ form, headingId }: { form: ReportForm; headingId: string }) {
  switch (form.step) {
    case "category":
      return <CategoryStep form={form} headingId={headingId} />;
    case "rules":
      return <RulesStep form={form} headingId={headingId} />;
    case "posts":
      return <PostsStep form={form} headingId={headingId} />;
    case "comment":
      return <CommentStep form={form} />;
    case "sent":
      return null;
  }
}

function CategoryStep({ form, headingId }: { form: ReportForm; headingId: string }) {
  const t = useT();
  // A rule violation needs a rule to cite: without rules in effect, the reason is not offered.
  const hasRules = form.rules.status === "ready" && form.rules.value.length > 0;
  const categories = reportCategories.filter((category) => category !== "violation" || hasRules);
  return (
    <RadioGroup
      aria-labelledby={headingId}
      value={form.draft.category}
      onChange={(value) => {
        const category = categories.find((item) => item === value);
        if (category !== undefined) form.change({ category });
      }}
      isRequired
      errorMessage={t("client:report.category.required")}
    >
      {categories.map((category) => (
        <Radio key={category} value={category} description={t(categoryLabels[category].description)}>
          {t(categoryLabels[category].label)}
        </Radio>
      ))}
    </RadioGroup>
  );
}

function RulesStep({ form, headingId }: { form: ReportForm; headingId: string }) {
  const t = useT();
  const descriptionId = useId();
  const errorId = useId();
  if (form.rules.status !== "ready") return null;
  const chosen = new Set(form.draft.ruleIds);
  return (
    <Stack
      role="group"
      aria-labelledby={headingId}
      aria-describedby={form.isRuleMissing ? `${descriptionId} ${errorId}` : descriptionId}
      gap="3"
    >
      <Hint id={descriptionId}>{t("client:report.rules.description")}</Hint>
      {form.rules.value.map((rule) => (
        <Checkbox
          key={rule.id}
          isSelected={chosen.has(rule.id)}
          isInvalid={form.isRuleMissing}
          isDisabled={!chosen.has(rule.id) && chosen.size >= maxReportItems}
          onChange={(isSelected) => {
            form.change({
              ruleIds: isSelected
                ? [...form.draft.ruleIds, rule.id]
                : form.draft.ruleIds.filter((id) => id !== rule.id),
            });
          }}
          {...(rule.hint !== undefined && {
            description: (
              <span dir="auto" lang="">
                {rule.hint}
              </span>
            ),
          })}
        >
          <span dir="auto" lang="">
            {rule.text}
          </span>
        </Checkbox>
      ))}
      {form.isRuleMissing && (
        <InlineError id={errorId} alert>
          {t("client:report.rules.required")}
        </InlineError>
      )}
    </Stack>
  );
}

function PostsStep({ form, headingId }: { form: ReportForm; headingId: string }) {
  const t = useT();
  const descriptionId = useId();
  const { posts } = form;
  return (
    <Stack
      role="group"
      aria-labelledby={headingId}
      aria-describedby={descriptionId}
      aria-busy={posts.status === "loading"}
      gap="3"
    >
      <Hint id={descriptionId}>{t("client:report.posts.description")}</Hint>
      {posts.status === "loading" && <Spinner label={t("client:report.posts.loading")} />}
      {posts.status === "error" && (
        <RegionError
          heading={t("client:report.posts.failed")}
          headingLevel={4}
          reason={posts.message}
          retryLabel={t("client:report.posts.retry")}
          onRetry={posts.retry}
          isRetrying={posts.isRetrying}
        />
      )}
      {posts.status === "ready" &&
        (posts.value.length === 0 ? (
          <Hint>{t("client:report.posts.empty")}</Hint>
        ) : (
          <Stack as="ul" divided>
            {posts.value.map((post) => (
              <PostChoice key={post.id} post={post} form={form} />
            ))}
          </Stack>
        ))}
    </Stack>
  );
}

function PostChoice({ post, form }: { post: Status; form: ReportForm }) {
  const t = useT();
  const intl = useIntl();
  const previewId = useId();
  const isSelected = form.draft.statusIds.includes(post.id);
  return (
    <Stack as="li" gap="1" paddingY="3">
      <Checkbox
        isSelected={isSelected}
        isDisabled={!isSelected && form.draft.statusIds.length >= maxReportItems}
        onChange={(selected) => {
          form.change({
            statusIds: selected
              ? [...form.draft.statusIds, post.id]
              : form.draft.statusIds.filter((id) => id !== post.id),
          });
        }}
        aria-describedby={previewId}
      >
        {t("client:report.posts.item", {
          date: intl.formatDate(post.created_at, { dateStyle: "medium", timeStyle: "short" }),
        })}
      </Checkbox>
      {/* The post describes its checkbox; it is inert, so its links are no stops on the way through the list. */}
      <Stack id={previewId} inert marginStart="8" gap="1" textSize="sm">
        {post.spoiler_text === "" ? (
          <PostContent html={serverSanitizedHtml(post.content)} lang={post.language} lineClamp={3} />
        ) : (
          <Text as="p" dir="auto" lang={post.language} wrap="anywhere">
            {t("client:report.posts.content_warning", { text: post.spoiler_text })}
          </Text>
        )}
        {post.media_attachments.length > 0 && (
          <Hint size="inherit">{t("client:report.posts.media", { count: post.media_attachments.length })}</Hint>
        )}
      </Stack>
    </Stack>
  );
}

function CommentStep({ form }: { form: ReportForm }) {
  const t = useT();
  return (
    <>
      <TextArea
        label={t("client:report.comment.label")}
        description={t("client:report.comment.description")}
        value={form.draft.comment}
        onChange={(comment) => {
          form.change({ comment });
        }}
        rows={4}
        characterLimit={maxReportComment}
        formatRemaining={(remaining) =>
          remaining < 0
            ? t("client:report.comment.characters_over", { count: -remaining })
            : t("client:report.comment.characters_left", { count: remaining })
        }
        errorMessage={t("client:report.comment.too_long", { max: maxReportComment })}
      />
      {form.remoteDomain !== null && (
        <Checkbox
          isSelected={form.draft.forward}
          onChange={(forward) => {
            form.change({ forward });
          }}
          description={t("client:report.forward.description", { domain: isolatedDomain(form.remoteDomain) })}
        >
          <Text wrap="anywhere">{t("client:report.forward", { domain: isolatedDomain(form.remoteDomain) })}</Text>
        </Checkbox>
      )}
    </>
  );
}
