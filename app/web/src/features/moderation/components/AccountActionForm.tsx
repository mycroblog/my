// SPDX-License-Identifier: AGPL-3.0-only
// The form of a moderation action on an account (FR-703): the kind (React Aria RadioGroup, each with what it
// does), the category of the reason (Select, the WAI-ARIA APG select-only combobox), how long it lasts
// (DurationSelect), the public reason and the private note (ReasonField), whether the account is told
// (Checkbox), whether a ban also blocks sign-ups with the user's email address (Checkbox, FR-719), and the posts of an action on posts (a group of checkboxes). It follows the browser's constraint
// validation (React Aria's native validation). An action that cannot be undone asks first in an alert dialog
// (ConfirmDialog, NFR-MOD-7), whose failure stays in it; a failure of another kind is shown under the form and
// keeps what was filled in.
import {
  Box,
  Button,
  Checkbox,
  ConfirmDialog,
  DurationSelect,
  InlineError,
  PostContent,
  Radio,
  RadioGroup,
  ReasonField,
  Select,
  SelectItem,
  serverSanitizedHtml,
  Stack,
  Text,
  SeverityMark,
} from "@my/ui";
import { useId } from "react";
import { useIntl } from "react-intl";
import { type ClientMessageKey, type Translate, useT } from "../../../lib/i18n";
import type { Status } from "../../status";
import { type AccountAction, type AccountActionKind, accountActionTextLimit, reportCategories } from "../api/reports";
import {
  type ActionTarget,
  type AccountActionForm as Form,
  isPostKind,
  takesDuration,
  takesEmailBlock,
  useAccountActionForm,
  severityOf,
} from "../hooks/useAccountActionForm";
import { accountActionKindLabels, isolate, reportCategoryLabels } from "../hooks/useReports";

const kindDescriptions: Readonly<Record<AccountActionKind, ClientMessageKey>> = {
  warn: "client:admin.account_actions.kind.warn.description",
  mark_statuses_sensitive: "client:admin.account_actions.kind.mark_statuses_sensitive.description",
  delete_statuses: "client:admin.account_actions.kind.delete_statuses.description",
  force_sensitive: "client:admin.account_actions.kind.force_sensitive.description",
  limit: "client:admin.account_actions.kind.limit.description",
  restrict_posting: "client:admin.account_actions.kind.restrict_posting.description",
  restrict_interaction: "client:admin.account_actions.kind.restrict_interaction.description",
  disable_login: "client:admin.account_actions.kind.disable_login.description",
  suspend: "client:admin.account_actions.kind.suspend.description",
  ban: "client:admin.account_actions.kind.ban.description",
};

/** The question, the result and the verb of the confirmation of a kind that cannot be undone (web-client/16-look-states-and-wording.md G22). */
function confirmationOf(
  t: Translate,
  kind: AccountActionKind,
  handle: string,
  count: number,
): { title: string; description: string; confirm: string } {
  if (kind === "ban") {
    return {
      title: t("client:admin.account_actions.confirm_ban.title", { handle }),
      description: t("client:admin.account_actions.confirm_ban.description"),
      confirm: t("client:admin.account_actions.confirm_ban.confirm"),
    };
  }
  return {
    title: t("client:admin.account_actions.confirm_delete_statuses.title", { handle, count }),
    description: t("client:admin.account_actions.confirm_delete_statuses.description"),
    confirm: t("client:admin.account_actions.confirm_delete_statuses.confirm"),
  };
}

/** The durations an action can last, in seconds, from the shortest. */
const day = 86_400;
const durations = [day, 7 * day, 30 * day, 90 * day, 180 * day, 365 * day] as const;

export type AccountActionFormProps = {
  target: ActionTarget;
  /** The handle of the account, for the confirmation's question (`@bob`, `@carol@example.com`). */
  handle: string;
  /** The posts that an action on posts may choose, loaded, in `target.statusIds`' order. */
  posts: readonly Status[];
  /** The server has taken the action. */
  onIssued: (action: AccountAction) => void;
};

export function AccountActionForm({ target, handle, posts, onIssued }: AccountActionFormProps) {
  const t = useT();
  const form = useAccountActionForm(target, onIssued);
  const { draft } = form;
  const formatRemaining = (remaining: number) =>
    remaining < 0
      ? t("client:admin.account_actions.characters_over", { count: -remaining })
      : t("client:admin.account_actions.characters_left", { count: remaining });
  const tooLong = t("client:admin.account_actions.too_long", { max: accountActionTextLimit });
  // The handle reads in its own direction inside the question.
  const confirmation =
    form.confirmation === null
      ? undefined
      : confirmationOf(t, form.confirmation.kind, isolate(handle), form.chosenStatusIds.length);
  return (
    <>
      <Stack as="form" onSubmit={form.submit} gap="6">
        <RadioGroup
          label={t("client:admin.account_actions.kind")}
          value={draft.kind}
          onChange={(kind) => {
            const chosen = form.kinds.find((item) => item === kind);
            if (chosen !== undefined) form.change({ kind: chosen });
          }}
          isRequired
          errorMessage={t("client:admin.account_actions.kind.required")}
          {...(target.remote && { description: t("client:admin.account_actions.kind.remote") })}
        >
          {form.kinds.map((kind) => {
            const severity = severityOf(kind);
            // The kind's place on the scale goes with its description, so that its name stays the kind's alone (A7).
            return (
              <Radio
                key={kind}
                value={kind}
                description={
                  <Stack gap="1">
                    <SeverityMark
                      level={severity.level}
                      max={severity.max}
                      label={t("client:admin.account_actions.severity", severity)}
                      {...(severity.irreversible && {
                        irreversibleLabel: t("client:admin.account_actions.irreversible"),
                      })}
                    />
                    {t(kindDescriptions[kind])}
                  </Stack>
                }
              >
                {t(accountActionKindLabels[kind])}
              </Radio>
            );
          })}
        </RadioGroup>
        {isPostKind(draft.kind) && <PostChoices form={form} posts={posts} />}
        <Select
          label={t("client:admin.account_actions.category")}
          description={t("client:admin.account_actions.category.description")}
          value={draft.category}
          onChange={(key) => {
            const category = reportCategories.find((item) => item === key);
            if (category !== undefined) form.change({ category });
          }}
        >
          {reportCategories.map((category) => (
            <SelectItem key={category} id={category}>
              {t(reportCategoryLabels[category])}
            </SelectItem>
          ))}
        </Select>
        {takesDuration(draft.kind) && (
          <DurationSelect
            label={t("client:admin.account_actions.duration")}
            description={t("client:admin.account_actions.duration.description")}
            durations={durations}
            formatDuration={(seconds) => t("client:admin.account_actions.duration.days", { count: seconds / day })}
            indefiniteLabel={t("client:admin.account_actions.duration.indefinite")}
            value={draft.expiresIn}
            onChange={(expiresIn) => {
              form.change({ expiresIn });
            }}
          />
        )}
        <ReasonField
          label={t("client:admin.account_actions.public_reason")}
          description={t("client:admin.account_actions.public_reason.description")}
          value={draft.publicReason}
          onChange={(publicReason) => {
            form.change({ publicReason });
          }}
          characterLimit={accountActionTextLimit}
          formatRemaining={formatRemaining}
          errorMessage={tooLong}
        />
        <ReasonField
          label={t("client:admin.account_actions.private_note")}
          description={t("client:admin.account_actions.private_note.description")}
          value={draft.privateNote}
          onChange={(privateNote) => {
            form.change({ privateNote });
          }}
          characterLimit={accountActionTextLimit}
          formatRemaining={formatRemaining}
          errorMessage={tooLong}
        />
        {!target.remote && (
          <Checkbox
            isSelected={draft.notify}
            onChange={(notify) => {
              form.change({ notify });
            }}
            description={t("client:admin.account_actions.notify.description")}
          >
            {t("client:admin.account_actions.notify")}
          </Checkbox>
        )}
        {takesEmailBlock(draft.kind, target) && (
          <Checkbox
            name="block_email"
            isSelected={draft.blockEmail}
            onChange={(blockEmail) => {
              form.change({ blockEmail });
            }}
            description={t("client:admin.account_actions.block_email.description")}
          >
            {t("client:admin.account_actions.block_email")}
          </Checkbox>
        )}
        {form.confirmation === null && form.error !== undefined && (
          <InlineError size="inherit" alert>
            {form.error}
          </InlineError>
        )}
        <Box>
          <Button type="submit" variant="primary" isPending={form.isPending && form.confirmation === null}>
            {target.reportId === undefined
              ? t("client:admin.account_actions.submit")
              : t("client:admin.account_actions.submit_and_resolve")}
          </Button>
        </Box>
      </Stack>
      {/* Outside the form: React sends the dialog's own submit event up its component tree, through a portal too. */}
      <ConfirmDialog
        isOpen={form.confirmation !== null}
        onOpenChange={(isOpen) => {
          if (!isOpen) form.confirmation?.cancel();
        }}
        title={confirmation?.title ?? ""}
        description={confirmation?.description ?? ""}
        confirmLabel={confirmation?.confirm ?? ""}
        cancelLabel={t("client:admin.account_actions.confirm.cancel")}
        onConfirm={() => {
          form.confirmation?.confirm();
        }}
        isPending={form.isPending}
        {...(form.confirmation !== null && form.error !== undefined ? { errorMessage: form.error } : {})}
      />
    </>
  );
}

/** The posts of an action on posts: one checkbox per post, named by its date and described by its text. */
function PostChoices({ form, posts }: { form: Form; posts: readonly Status[] }) {
  const t = useT();
  const labelId = useId();
  const errorId = useId();
  return (
    <Stack role="group" aria-labelledby={labelId} {...(form.isPostMissing && { "aria-describedby": errorId })} gap="2">
      <Text id={labelId} weight="medium">
        {t("client:admin.account_actions.posts")}
      </Text>
      <Stack as="ul" divided>
        {posts.map((post) => (
          <PostChoice key={post.id} post={post} form={form} />
        ))}
      </Stack>
      {form.isPostMissing && (
        <InlineError id={errorId} alert>
          {t("client:admin.account_actions.posts.required")}
        </InlineError>
      )}
    </Stack>
  );
}

/** One post to choose, named by its date and described by its text (a post with a CW by the CW only). */
function PostChoice({ post, form }: { post: Status; form: Form }) {
  const t = useT();
  const intl = useIntl();
  const previewId = useId();
  return (
    <Stack as="li" gap="1" paddingY="2">
      <Checkbox
        isSelected={form.chosenStatusIds.includes(post.id)}
        isInvalid={form.isPostMissing}
        onChange={(isSelected) => {
          form.choosePost(post.id, isSelected);
        }}
        aria-describedby={previewId}
      >
        {t("client:admin.account_actions.posts.item", {
          date: intl.formatDate(post.created_at, { dateStyle: "medium", timeStyle: "short" }),
        })}
      </Checkbox>
      {/* The post describes its checkbox; it is inert, so its links are no stops on the way through the list. */}
      <Box id={previewId} inert marginStart="8" textSize="sm">
        {post.spoiler_text === "" ? (
          <PostContent html={serverSanitizedHtml(post.content)} lang={post.language} lineClamp={2} />
        ) : (
          <Text as="p" dir="auto" lang={post.language} align="start" wrap="anywhere">
            {t("client:admin.account_actions.posts.content_warning", { text: post.spoiler_text })}
          </Text>
        )}
      </Box>
    </Stack>
  );
}
