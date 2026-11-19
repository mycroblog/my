// SPDX-License-Identifier: AGPL-3.0-only

// The registrations that wait for approval, at /moderation/registrations (FR-721, ADR-0058 §9): the longest waiting
// first, each the account's handle, its email address (shown here without asking, for the review alone, and whether it
// is confirmed), why the person wants to join, and when; picked rows approved or rejected, with a note for the
// moderators, each asked first, since a registration is decided once. A decided row stays where it was with its
// decision until the screen reads the list again. A one-person server has none (ADR-0152), and its entry is not shown.

import { moderatedAccountPath, sentenceHandleOf } from "@my/app-components";
import {
  registrationNoteLimit,
  useDecideRegistrations,
  useRegistrations,
  type ModeratedRegistration,
} from "@my/app-data";
import {
  Button,
  FactList,
  Row,
  Stack,
  ConfirmDialog,
  FeedNotice,
  ModalPresence,
  RecordTable,
  RelativeTime,
  ScreenStack,
  ScreenTitle,
  StateMark,
  TextField,
  type RecordColumn,
  type RecordRow,
} from "@my/design-system";
import { useId, useState, type ReactNode } from "react";
import type { StaffAddress } from "../entries";
import { ReadNotice, useStaffStrings } from "../parts";
import { cursorOf, Pages, refusalWords, useSaid } from "./parts";

type Column = "email" | "reason" | "submitted" | "state";

/** A registration's state, marked: waiting, approved (in force), or rejected. */
function RegistrationStateMark({ registration }: { registration: ModeratedRegistration }) {
  const { t } = useStaffStrings();
  const { state } = registration;
  return (
    <StateMark state={state === "pending" ? "pending" : state === "approved" ? "active" : "disabled"}>
      {t("staff_accounts.registration_state", { state })}
    </StateMark>
  );
}

/** The registrations that wait: the table with its picked rows' actions, and the pages. */
export function RegistrationsScreen(address: StaffAddress): ReactNode {
  const { t } = useStaffStrings();
  const cursor = cursorOf(address.search);
  const page = useRegistrations(cursor);
  const decide = useDecideRegistrations(cursor);
  const noteId = useId();
  const { say, status } = useSaid();
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  // The decision asked, of the registrations of `ids`: the picked rows, or one row's own.
  const [asking, setAsking] = useState<{ decision: "approve" | "reject"; ids: readonly string[] } | undefined>(
    undefined,
  );
  // The registrations whose email address the reader asked to see (ADR-0058 §9: shown on demand alone).
  const [revealed, setRevealed] = useState<ReadonlySet<string>>(new Set());
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const head = <ScreenTitle>{t("staff_accounts.entry.registrations")}</ScreenTitle>;
  if (page.forbidden) return <FeedNotice lead>{t("staff_accounts.forbidden")}</FeedNotice>;
  if (page.status !== "success")
    return (
      <ScreenStack>
        {head}
        <ReadNotice
          status={page.status}
          loading={t("staff_accounts.registrations.loading")}
          failed={t("staff_accounts.registrations.failed")}
          retry={page.refetch}
        />
      </ScreenStack>
    );
  const pending = page.items.filter((r) => r.state === "pending");
  const picked = pending.filter((r) => selected.has(r.id));
  const send = () => {
    if (asking === undefined) return;
    const { decision, ids } = asking;
    setBusy(true);
    void decide(ids, decision, note.trim()).then((results) => {
      setBusy(false);
      setAsking(undefined);
      setNote("");
      setSelected(new Set());
      const failed = results.find((r) => r.error !== undefined);
      const done = results.length - results.filter((r) => r.error !== undefined).length;
      if (failed === undefined)
        say(
          t(
            decision === "approve" ? "staff_accounts.registrations.approved" : "staff_accounts.registrations.rejected",
            { count: done },
          ),
        );
      else say(refusalWords(t, failed.error), true);
    });
  };
  const columns: RecordColumn<Column>[] = [
    { key: "submitted", label: t("staff_accounts.registrations.column.submitted"), kind: "fit", bare: true },
    { key: "state", label: t("staff_accounts.registrations.column.state"), kind: "fit", beside: true },
  ];
  const rows = page.items.map((r): RecordRow<Column> => ({
    id: r.id,
    head:
      r.username === undefined ? t("staff.account_gone") : sentenceHandleOf({ username: r.username, acct: r.username }),
    caption: r.email_verified === false ? t("staff_accounts.registrations.unconfirmed") : undefined,
    ...(r.account_id === undefined ? {} : { href: moderatedAccountPath(r.account_id) }),
    cells: {
      submitted: <RelativeTime date={r.submitted_at} />,
      state: <RegistrationStateMark registration={r} />,
    },
    // Across the row: the email address, kept until shown, why they want to join, and the row's own decision.
    detail: (
      <Stack gap="entry-room">
        <FactList
          label={t("staff_accounts.registrations.facts")}
          items={[
            {
              term: t("staff_accounts.registrations.column.email"),
              code: true,
              ...(r.email === undefined
                ? { value: t("staff_accounts.registrations.no_email") }
                : revealed.has(r.id)
                  ? { value: r.email }
                  : {
                      reveal: {
                        onReveal: () => {
                          setRevealed((now) => new Set([...now, r.id]));
                        },
                      },
                    }),
            },
            {
              term: t("staff_accounts.registrations.column.reason"),
              value: r.reason === undefined ? t("staff_accounts.registrations.no_reason") : <bdi>{r.reason}</bdi>,
            },
          ]}
        />
        {r.state === "pending" && (
          <Row wrap gap="screen-tab-gap">
            <Button
              kind="primary"
              size="timeline"
              touch
              busy={busy && asking?.decision === "approve" && asking.ids.length === 1 && asking.ids[0] === r.id}
              onPress={() => {
                setAsking({ decision: "approve", ids: [r.id] });
              }}
            >
              {t("staff_accounts.registrations.approve")}
            </Button>
            <Button
              size="timeline"
              touch
              busy={busy && asking?.decision === "reject" && asking.ids.length === 1 && asking.ids[0] === r.id}
              onPress={() => {
                setAsking({ decision: "reject", ids: [r.id] });
              }}
            >
              {t("staff_accounts.registrations.reject")}
            </Button>
          </Row>
        )}
      </Stack>
    ),
  }));
  const decision = asking?.decision;
  return (
    <ScreenStack busy={page.stale}>
      {head}
      {status}
      <RecordTable
        label={t("staff_accounts.registrations.table")}
        head={{ label: t("staff_accounts.registrations.column.account") }}
        columns={columns}
        rows={rows}
        busy={page.stale}
        selection={
          pending.length === 0
            ? undefined
            : {
                selected,
                onChange: (next) => {
                  // A decided row is not picked again.
                  setSelected(new Set([...next].filter((id) => pending.some((r) => r.id === id))));
                },
                actions: [
                  {
                    id: "approve",
                    label: t("staff_accounts.registrations.approve"),
                    onPress: () => {
                      if (picked.length > 0) setAsking({ decision: "approve", ids: picked.map((r) => r.id) });
                    },
                    busy: busy && decision === "approve" && (asking?.ids.length ?? 0) > 1,
                  },
                  {
                    id: "reject",
                    label: t("staff_accounts.registrations.reject"),
                    onPress: () => {
                      if (picked.length > 0) setAsking({ decision: "reject", ids: picked.map((r) => r.id) });
                    },
                    busy: busy && decision === "reject" && (asking?.ids.length ?? 0) > 1,
                  },
                ],
              }
        }
        empty={<FeedNotice>{t("staff_accounts.registrations.empty")}</FeedNotice>}
      />
      <Pages address={address} next={page.next} />
      <ModalPresence>
        {asking !== undefined && (
          <ConfirmDialog
            title={t(
              asking.decision === "approve"
                ? "staff_accounts.registrations.approve.confirm"
                : "staff_accounts.registrations.reject.confirm",
              { count: asking.ids.length },
            )}
            cancel={t("staff_accounts.cancel")}
            confirm={t(
              asking.decision === "approve"
                ? "staff_accounts.registrations.approve"
                : "staff_accounts.registrations.reject",
            )}
            busy={busy}
            onCancel={() => {
              setAsking(undefined);
            }}
            onConfirm={send}
          >
            <TextField
              id={noteId}
              name="note"
              label={t("staff_accounts.registrations.note")}
              hint={t(
                asking.decision === "approve"
                  ? "staff_accounts.registrations.approve.about"
                  : "staff_accounts.registrations.reject.about",
              )}
              multiline
              rows={2}
              maxLength={registrationNoteLimit}
              onChange={setNote}
            />
          </ConfirmDialog>
        )}
      </ModalPresence>
    </ScreenStack>
  );
}
