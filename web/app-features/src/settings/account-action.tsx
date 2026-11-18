// SPDX-License-Identifier: AGPL-3.0-only

// One moderators' action on the reader's account, the screen under Account status at /settings/account/status/<id>
// (FR-715, ADR-0058 §9, §10): where the reader reads an action whole and appeals it.
//
// - What it does, in the server's words for its kind; under it, in the text's color, where it stands (when it ends,
//   ended or was lifted), then when the moderators took it.
// - The moderators' reason: its kind, and their explanation to the reader where they gave one. Neither who reported
//   the account nor the moderators' own notes: the server does not send them.
// - The posts it is about, for an action on posts: each a link to its page where the server still shows it to the
//   reader, and how many it no longer does.
// - Its appeal. While it can be appealed, a form saved as a whole by the submit bar: what the reader wants the
//   moderators to know, up to the server's limit, with that it is sent once and the last moment to send it said over
//   the field. Once appealed, where the appeal stands, in a sentence that reads whole by itself, when it was sent and
//   decided, a line each, what the reader wrote, and the moderators' answer. Never appealed and no longer appealable,
//   that it is so, with the day the time passed.
//
// What came of sending is said in the appeal's status, a polite live region there from the start, which takes the
// focus: the form is gone once the appeal is sent, the status says only that it was, and the cache holds the action as
// the server answered it, so the lists and the menu's count follow. What is wrong with what the reader wrote (nothing,
// too much, what the server refuses of it) stands under the field, which takes the focus and comes into view with it,
// the submit bar saying only that nothing was sent. A server that refuses the appeal itself (the time passed, the
// action no longer appealable, appealed from elsewhere meanwhile, the action gone) ends the form: no field, no submit
// bar, no last day; the status says what happened and what to do, with the link to the server's About page where
// that is where to turn, and what the reader wrote stays under it to be read and copied; the cache holds the action
// as no longer appealable, or the action is read again where the server has more to say of it. A server that could
// not be reached says so in the submit bar, and the form stays as it was. Leaving with an appeal written that can
// still be sent is the app's to notice (`onUnsaved`): it holds the reader back and gives `leaving`, and the screen
// asks whether to leave.

import { plainText, statusPath } from "@my/app-components";
import {
  appealLimits,
  appealRefusalOf,
  ownModerationActionsQuery,
  refusedField,
  useActionStatuses,
  useAppeals,
  useOwnModerationAction,
  type AppData,
  type AppealRefusal,
  type ModerationAction,
} from "@my/app-data";
import {
  BackButton,
  Box,
  Button,
  ConfirmDialog,
  FeedNotice,
  focusRefusedField,
  FormAlert,
  ModalPresence,
  SettingsEmpty,
  SettingsList,
  SettingsListRow,
  SettingsScreen,
  SettingsSection,
  Stack,
  SubmitBar,
  TextField,
  TextLink,
  useLocale,
} from "@my/design-system";
import { useEffect, useId, useRef, useState } from "react";
import { stringsIn } from "../locale";
import { LimitCount } from "../profile-identity";
import { formatParts } from "../slots";
import { accountStatusPath, useActionDate } from "./account";
import { appealPassed, endsLine, useStatusWords } from "./account-status";
import { ReadingNotice } from "./parts";

type T = ReturnType<typeof stringsIn>["t"];

/** The address of the server's About page, where its contact is: the one the frame's footer leads to. */
const aboutPath = "/about";

/** What the route's loader waits for: the newest of the actions on the account, from the cache when it has them. */
export async function loadModerationAction(data: AppData): Promise<void> {
  await data.queryClient
    .infiniteQuery({ ...ownModerationActionsQuery(data), staleTime: Infinity })
    .catch(() => undefined);
}

/** The title of the screen. */
export function useModerationActionTitle(): string {
  return stringsIn(useLocale().locale).t("settings.account.action.title");
}

export type ModerationActionProps = {
  /** The ID of the action. */
  id: string;
  /** Goes back to Account status. */
  onBack: () => void;
  /** Hears whether an appeal is written and not sent, as that comes and goes; false before it leaves. */
  onUnsaved?: ((unsaved: boolean) => void) | undefined;
  /**
   * The reader is leaving with an appeal written and not sent, and is held back: the screen asks whether to leave
   * (`onLeave`, what was written lost) or stay (`onStay`).
   */
  leaving?: { onStay: () => void; onLeave: () => void } | undefined;
};

/** How much of a post's text names it in the list of an action's posts. */
const excerptLength = 100;

/** The start of the text of a post, as plain text on one line; undefined for a post without text. */
function excerptOf(content: string): string | undefined {
  const text = plainText(content).replace(/\s+/g, " ");
  if (text === "") return undefined;
  const letters = Array.from(text);
  return letters.length > excerptLength ? `${letters.slice(0, excerptLength).join("")}…` : text;
}

/** The posts an action is about: each one the server still shows a link to its page, and how many it no longer does. */
function Posts({ ids, t }: { ids: readonly string[]; t: T }) {
  const { lang } = stringsIn(useLocale().locale);
  const date = useActionDate();
  const { statuses, gone, pending } = useActionStatuses(ids);
  const heading = t("settings.account.action.section.posts");
  return (
    <SettingsSection heading={heading} count={new Intl.NumberFormat(lang).format(ids.length)}>
      {pending ? (
        <SettingsEmpty>{t("settings.account.action.posts.loading")}</SettingsEmpty>
      ) : (
        statuses.length > 0 && (
          <SettingsList label={heading}>
            {statuses.map((status) => (
              <SettingsListRow
                key={status.id}
                title={t("settings.account.action.post", { date: date(status.created_at) })}
                caption={excerptOf(status.content)}
                href={statusPath(status)}
              />
            ))}
          </SettingsList>
        )
      )}
      {gone > 0 && (
        <Box as="p" tone="muted" breaks="phrase">
          {t("settings.account.action.posts.gone", { count: gone })}
        </Box>
      )}
    </SettingsSection>
  );
}

/** A text someone wrote under what names it: the reader's appeal, the moderators' answer. */
function Written({ label, children }: { label: string; children: string }) {
  return (
    <Stack gap="hair">
      <Box as="p" text="caption" tone="muted">
        {label}
      </Box>
      <Box as="p" dir="auto" keeps="lines" breaks="anywhere">
        {children}
      </Box>
    </Stack>
  );
}

/** Where the form's last send is; one that was not sent, with what is wrong with what the reader wrote, if that is why. */
type Sent = { readonly kind: "idle" | "sending" } | { readonly kind: "refused"; readonly field: string | undefined };

function ActionView({
  action,
  reread,
  onBack,
  onUnsaved,
  leaving,
}: Omit<ModerationActionProps, "id"> & { action: ModerationAction; reread: () => void }) {
  const { t } = stringsIn(useLocale().locale);
  const date = useActionDate();
  const id = useId();
  const appeals = useAppeals();
  const [text, setText] = useState("");
  const [sent, setSent] = useState<Sent>({ kind: "idle" });
  // What came of sending (that it was sent, or why the server refused it), each a new element of the status, which
  // takes the focus.
  const [said, setSaid] = useState<{ readonly what: "sent" | AppealRefusal; readonly n: number } | undefined>(
    undefined,
  );
  const count = useRef(0);
  // What takes the focus once the screen is drawn again: the status, or the field that was refused.
  const focusNext = useRef<"status" | "field" | undefined>(undefined);
  const fieldId = `${id}-appeal`;
  const statusId = `${id}-status`;
  useEffect(() => {
    if (focusNext.current === "field") focusRefusedField(fieldId);
    else if (focusNext.current === "status") document.getElementById(statusId)?.focus();
    focusNext.current = undefined;
  });

  const { appeal } = action;
  // A refusal of the appeal itself ends the form, whatever the action read before it says.
  const ended = said !== undefined && said.what !== "sent";
  const open = action.appealable && appeal === undefined && !ended;
  const busy = sent.kind === "sending";
  const unsaved = open && text.trim() !== "";
  useEffect(() => {
    onUnsaved?.(unsaved);
  }, [onUnsaved, unsaved]);
  useEffect(
    () => () => {
      onUnsaved?.(false);
    },
    [onUnsaved],
  );

  const say = (what: "sent" | AppealRefusal) => {
    count.current += 1;
    setSaid({ what, n: count.current });
    setSent({ kind: "idle" });
    focusNext.current = "status";
  };
  const refuse = (field: string | undefined) => {
    setSent({ kind: "refused", field });
    if (field !== undefined) focusNext.current = "field";
  };
  const left = appealLimits.text - Array.from(text.trim()).length;
  const send = () => {
    if (busy) return;
    const written = text.trim();
    if (written === "") {
      refuse(t("settings.account.appeal.missing"));
      return;
    }
    if (left < 0) {
      refuse(t("settings.account.appeal.over", { max: appealLimits.text }));
      return;
    }
    setSent({ kind: "sending" });
    appeals.action(action.id, written).then(
      () => {
        // The cache holds the action with its appeal: the form is gone, and the status says what came of it.
        onUnsaved?.(false);
        say("sent");
      },
      (error: unknown) => {
        const reason = appealRefusalOf(error);
        if (reason !== undefined) {
          // The form ends, and the status says what the server said. An action that can no longer be appealed is
          // held so in the cache; one appealed from elsewhere, or gone, is read again for what the server has of it.
          if (reason === "appealed" || reason === "gone") reread();
          say(reason);
          return;
        }
        const named = refusedField(error);
        refuse(named?.member === "text" ? named.detail : undefined);
      },
    );
  };

  const deadline = action.appeal_deadline === undefined ? "" : date(action.appeal_deadline);
  const status = (
    <Box as="p" id={statusId} role="status" tabIndex={-1} breaks="phrase">
      {said !== undefined && (
        <span key={said.n}>
          {said.what === "sent"
            ? t("settings.account.appeal.sent")
            : formatParts(
                t,
                "settings.account.appeal.refused",
                { reason: said.what === "gone" ? "other" : said.what },
                { about: <TextLink href={aboutPath}>{t("settings.account.appeal.about_page")}</TextLink> },
              )}
        </span>
      )}
    </Box>
  );
  return (
    <>
      <SettingsScreen
        sub
        title={t("settings.account.action.title")}
        back={<BackButton onPress={onBack}>{t("settings.account.status")}</BackButton>}
        foot={
          open ? (
            <SubmitBar
              result={
                sent.kind === "refused" ? (
                  <FormAlert>
                    {t("settings.account.appeal.not_sent", { reason: sent.field === undefined ? "other" : "field" })}
                  </FormAlert>
                ) : undefined
              }
            >
              <Button kind="primary" busy={busy} onPress={send}>
                {t("settings.account.appeal.submit")}
              </Button>
            </SubmitBar>
          ) : undefined
        }
      >
        <SettingsSection heading={t("settings.account.action.section.what")}>
          <Stack gap="hair">
            <Box as="p" breaks="phrase">
              {action.kind_label}
            </Box>
            <Box as="p" breaks="phrase">
              {endsLine(action, date, t)}
            </Box>
            <Box as="p" text="caption" tone="muted">
              {t("settings.account.action.issued", { date: date(action.issued_at) })}
            </Box>
          </Stack>
        </SettingsSection>
        <SettingsSection heading={t("settings.account.action.section.reason")}>
          <Stack gap="hair">
            <Box as="p" breaks="phrase">
              {action.reason_category_label}
            </Box>
            {action.public_reason === undefined || action.public_reason === "" ? (
              <Box as="p" tone="muted" breaks="phrase">
                {t("settings.account.action.reason.none")}
              </Box>
            ) : (
              <Box as="p" dir="auto" keeps="lines" breaks="anywhere">
                {action.public_reason}
              </Box>
            )}
          </Stack>
        </SettingsSection>
        {action.status_ids.length > 0 && <Posts ids={action.status_ids} t={t} />}
        <SettingsSection
          heading={t("settings.account.appeal")}
          about={
            open
              ? t("settings.account.appeal.about", { deadline: deadline === "" ? "no" : "yes", date: deadline })
              : undefined
          }
        >
          {/* The status keeps its place from the start, with no room of its own until it says something. */}
          <Stack gap={said === undefined ? undefined : "screen-row-gap"}>
            {status}
            {appeal !== undefined ? (
              <Stack gap="screen-row-gap">
                <Stack gap="hair">
                  <Box as="p" breaks="phrase">
                    {t("settings.account.appeal.state", { state: appeal.state })}
                  </Box>
                  <Box as="p" text="caption" tone="muted">
                    {t("settings.account.appeal.sent_on", { date: date(appeal.submitted_at) })}
                  </Box>
                  {appeal.decided_at !== undefined && (
                    <Box as="p" text="caption" tone="muted">
                      {t("settings.account.appeal.decided_on", { date: date(appeal.decided_at) })}
                    </Box>
                  )}
                </Stack>
                <Written label={t("settings.account.appeal.label")}>{appeal.text}</Written>
                {appeal.decision_reason !== undefined && appeal.decision_reason !== "" && (
                  <Written label={t("settings.account.appeal.answer")}>{appeal.decision_reason}</Written>
                )}
              </Stack>
            ) : open ? (
              <TextField
                id={fieldId}
                name="text"
                label={t("settings.account.appeal.label")}
                hint={t("settings.account.appeal.hint", { max: appealLimits.text })}
                multiline
                rows={6}
                onChange={(value) => {
                  setText(value);
                  // What the last send said was about the appeal as it was.
                  setSent((was) => (was.kind === "refused" ? { kind: "idle" } : was));
                }}
                readOnly={busy}
                end={<LimitCount left={left} near={100} />}
                error={sent.kind === "refused" ? sent.field : undefined}
              />
            ) : ended ? (
              // The status said why nothing was sent: what the reader wrote stays, to be read and copied.
              text.trim() !== "" && (
                <Stack gap="hair">
                  <Written label={t("settings.account.appeal.unsent")}>{text.trim()}</Written>
                  <Box as="p" text="caption" tone="muted" breaks="phrase">
                    {t("settings.account.appeal.unsent.about")}
                  </Box>
                </Stack>
              )
            ) : (
              <Box as="p" tone="muted" breaks="phrase">
                {t("settings.account.appeal.closed", { deadline: appealPassed(action) ? "yes" : "no", date: deadline })}
              </Box>
            )}
          </Stack>
        </SettingsSection>
      </SettingsScreen>
      <ModalPresence>
        {leaving !== undefined && (
          <ConfirmDialog
            key="leave"
            title={t("settings.account.appeal.leave.title")}
            cancel={t("settings.account.appeal.leave.cancel")}
            confirm={t("settings.account.appeal.leave.confirm")}
            onCancel={leaving.onStay}
            onConfirm={leaving.onLeave}
          >
            {t("settings.account.appeal.leave.text")}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </>
  );
}

/** The screen: the action once it is found among the reader's, and until then that it loads. */
export function ModerationActionScreen({ id, onBack, onUnsaved, leaving }: ModerationActionProps) {
  const { t } = stringsIn(useLocale().locale);
  const words = useStatusWords();
  const { action, status, retry, reread } = useOwnModerationAction(id);
  if (action !== undefined)
    return <ActionView action={action} reread={reread} onBack={onBack} onUnsaved={onUnsaved} leaving={leaving} />;
  return (
    <SettingsScreen
      sub
      title={t("settings.account.action.title")}
      back={<BackButton onPress={onBack}>{t("settings.account.status")}</BackButton>}
      notice={
        status === "missing" ? (
          <FeedNotice alert lead action={<TextLink href={accountStatusPath}>{t("settings.account.status")}</TextLink>}>
            {t("settings.account.action.not_found")}
          </FeedNotice>
        ) : (
          <ReadingNotice words={words} status={status === "error" ? "failed" : "loading"} retry={retry} />
        )
      }
    />
  );
}
