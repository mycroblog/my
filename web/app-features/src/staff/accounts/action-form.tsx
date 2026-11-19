// SPDX-License-Identifier: AGPL-3.0-only

// The form of a new action on an account (FR-703, FR-708, FR-713, FR-719): its kind (on a remote account limit,
// suspend or force sensitive alone; on posts only from a report, whose posts it names), the kind of reason, how long it
// lasts (up to ten years; a ban and an action on posts do not end), the reason the account is told and the moderators'
// note, whether a local account is told, and for a ban of a local account whether its email address is blocked from
// signing up again. A ban asks first, with the handle typed, and says that it cannot be lifted, only an appeal can
// overturn it; the
// deletion of posts asks first too, since it is final. What the server refuses stands under its field, or beside the
// button for what names none; what the press did is said in the form's status.

import { plainText, sentenceHandleOf } from "@my/app-components";
import {
  actionCategories,
  actionTextLimit,
  localActionKinds,
  postActionKinds,
  refusedMember,
  remoteActionKinds,
  useAccountWrites,
  useActionReport,
  type AccountActionCategory,
  type AccountActionKind,
  type ModeratedAccountDetail,
  type NewAccountAction,
} from "@my/app-data";
import {
  Box,
  Button,
  CheckField,
  ConfirmDialog,
  FieldError,
  FormAlert,
  ModalPresence,
  Row,
  Stack,
  TextField,
} from "@my/design-system";
import { useId, useState } from "react";
import { useStaffStrings } from "../parts";
import {
  actionCategoryWords,
  kindWords,
  LabelledSelect,
  lengthOptions,
  refusalWords,
  secondsOf,
  useSaid,
} from "./parts";

/** The members of the request a refusal may name, each with the field that says it. */
type Faults = Partial<Record<"kind" | "status_ids" | "expires_in" | "public_reason" | "private_note", string>>;

/** The form of a new action on `account`, from the report of `report` where the reader came from one. */
export function ActionForm({ account, report }: { account: ModeratedAccountDetail; report: string | undefined }) {
  const { t } = useStaffStrings();
  const { impose } = useAccountWrites();
  const id = useId();
  const { say, status } = useSaid();
  // The report's posts, which an action on posts names; none from another account's report.
  const reported = useActionReport(report);
  const from = reported?.target_account_id === account.id ? reported : undefined;
  const posts = from?.statuses.filter((s) => s.deleted_at === undefined) ?? [];
  const kinds = (account.local ? localActionKinds : remoteActionKinds).filter(
    (k) => !postActionKinds.has(k) || posts.length > 0,
  );
  const [round, setRound] = useState(0);
  const [kind, setKind] = useState<AccountActionKind>("warn");
  const [category, setCategory] = useState<AccountActionCategory>("other");
  const [length, setLength] = useState("0");
  const [publicReason, setPublicReason] = useState("");
  const [privateNote, setPrivateNote] = useState("");
  const [notify, setNotify] = useState(true);
  const [blockEmail, setBlockEmail] = useState(false);
  const [picked, setPicked] = useState<ReadonlySet<string>>(new Set());
  const [faults, setFaults] = useState<Faults>({});
  const [alert, setAlert] = useState<string | undefined>(undefined);
  const [asking, setAsking] = useState(false);
  const [busy, setBusy] = useState(false);
  const shown = kinds.includes(kind) ? kind : (kinds[0] ?? "warn");
  const onPosts = postActionKinds.has(shown);
  const lasts = shown !== "ban" && !onPosts;
  const handle = sentenceHandleOf(account.account);

  const request = (): NewAccountAction => {
    const seconds = lasts ? secondsOf(length) : undefined;
    return {
      kind: shown,
      reason_category: category,
      ...(publicReason.trim() === "" ? {} : { public_reason: publicReason.trim() }),
      ...(privateNote.trim() === "" ? {} : { private_note: privateNote.trim() }),
      ...(from === undefined ? {} : { report_id: from.id }),
      notify_target: account.local && notify,
      ...(seconds === undefined ? {} : { expires_in: seconds }),
      ...(onPosts ? { status_ids: [...picked] } : {}),
      ...(shown === "ban" && account.local && blockEmail ? { block_email: true } : {}),
    };
  };

  const send = () => {
    setBusy(true);
    impose(account.id, request()).then(
      (action) => {
        setBusy(false);
        setAsking(false);
        setFaults({});
        setAlert(undefined);
        setPublicReason("");
        setPrivateNote("");
        setPicked(new Set());
        setBlockEmail(false);
        setRound((n) => n + 1);
        say(t("staff_accounts.act.done", { kind: kindWords(t, action.kind) }));
      },
      (e: unknown) => {
        setBusy(false);
        setAsking(false);
        const member = refusedMember(e);
        if (member !== undefined && member.member in fieldOf) {
          setFaults({ [member.member]: member.said });
          setAlert(t("staff_accounts.act.not_taken"));
          document.getElementById(`${id}-${fieldOf[member.member as keyof Faults]}`)?.focus();
          return;
        }
        setAlert(refusalWords(t, e));
      },
    );
  };

  const submit = () => {
    if (busy) return;
    setFaults({});
    setAlert(undefined);
    if (onPosts && picked.size === 0) {
      setFaults({ status_ids: t("staff_accounts.act.posts.none") });
      document.getElementById(`${id}-posts`)?.focus();
      return;
    }
    if (shown === "ban" || shown === "delete_statuses") {
      setAsking(true);
      return;
    }
    send();
  };

  return (
    <Stack
      gap="screen-field-gap"
      as={
        <form
          onSubmit={(e) => {
            e.preventDefault();
            submit();
          }}
        />
      }
    >
      <LabelledSelect
        id={`${id}-kind`}
        label={t("staff_accounts.act.kind")}
        options={kinds.map((k) => ({ value: k, label: kindWords(t, k) }))}
        value={shown}
        onChange={(v) => {
          setKind(v as AccountActionKind);
        }}
        error={faults.kind}
      />
      <LabelledSelect
        label={t("staff_accounts.act.category")}
        options={actionCategories.map((c) => ({ value: c, label: actionCategoryWords(t, c) }))}
        value={category}
        onChange={(v) => {
          setCategory(v as AccountActionCategory);
        }}
      />
      {lasts && (
        <LabelledSelect
          id={`${id}-length`}
          label={t("staff_accounts.act.length")}
          options={lengthOptions(t)}
          value={length}
          onChange={setLength}
          error={faults.expires_in}
        />
      )}
      {onPosts && (
        <Stack
          gap="entry-line"
          as={<div role="group" id={`${id}-posts`} tabIndex={-1} aria-labelledby={`${id}-posts-label`} />}
        >
          <Box as="span" id={`${id}-posts-label`} weight="label">
            {t("staff_accounts.act.posts")}
          </Box>
          {posts.map((p, i) => (
            <CheckField
              key={p.status.id}
              id={`${id}-post-${i}`}
              name="status_ids"
              value={p.status.id}
              touch
              label={t("staff_accounts.act.post", {
                n: i + 1,
                excerpt: plainText(p.status.content).slice(0, 80),
              })}
              checked={picked.has(p.status.id)}
              onChange={(on) => {
                setPicked((now) => {
                  const next = new Set(now);
                  if (on) next.add(p.status.id);
                  else next.delete(p.status.id);
                  return next;
                });
              }}
            />
          ))}
          {faults.status_ids !== undefined && <FieldError id={`${id}-posts-error`}>{faults.status_ids}</FieldError>}
        </Stack>
      )}
      <TextField
        key={`public-${round}`}
        id={`${id}-public_reason`}
        name="public_reason"
        label={t("staff_accounts.act.public_reason")}
        multiline
        rows={3}
        maxLength={actionTextLimit}
        error={faults.public_reason}
        onChange={setPublicReason}
      />
      <TextField
        key={`private-${round}`}
        id={`${id}-private_note`}
        name="private_note"
        label={t("staff_accounts.act.private_note")}
        multiline
        rows={2}
        maxLength={actionTextLimit}
        error={faults.private_note}
        onChange={setPrivateNote}
      />
      {account.local && (
        <CheckField
          id={`${id}-notify`}
          name="notify_target"
          value="true"
          touch
          label={t("staff_accounts.act.notify")}
          checked={notify}
          onChange={setNotify}
        />
      )}
      {shown === "ban" && account.local && (
        <CheckField
          id={`${id}-block_email`}
          name="block_email"
          value="true"
          touch
          label={t("staff_accounts.act.block_email")}
          checked={blockEmail}
          onChange={setBlockEmail}
        />
      )}
      {alert !== undefined && <FormAlert regular>{alert}</FormAlert>}
      <Row wrap align="center" gap="screen-tab-gap">
        <Button type="submit" kind="primary" busy={busy && !asking}>
          {shown === "ban" ? t("staff_accounts.act.ban") : t("staff_accounts.act.submit")}
        </Button>
      </Row>
      {status}
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            title={
              shown === "ban"
                ? t("staff_accounts.act.ban.confirm", { handle })
                : t("staff_accounts.act.delete.confirm", { count: picked.size })
            }
            cancel={t("staff_accounts.cancel")}
            confirm={shown === "ban" ? t("staff_accounts.act.ban") : t("staff_accounts.act.delete")}
            phrase={shown === "ban" ? handle : undefined}
            busy={busy}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={send}
          >
            {shown === "ban" ? t("staff_accounts.act.ban.about") : t("staff_accounts.act.delete.about")}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </Stack>
  );
}

/** The field of each member a refusal names, by the end of its element's id. */
const fieldOf: Readonly<Record<keyof Faults, string>> = {
  kind: "kind",
  status_ids: "posts",
  expires_in: "length",
  public_reason: "public_reason",
  private_note: "private_note",
};
