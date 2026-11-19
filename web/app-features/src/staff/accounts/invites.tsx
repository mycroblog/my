// SPDX-License-Identifier: AGPL-3.0-only

// The invites as a holder of `manage_invites` keeps them (FR-011): their list at /moderation/invites, newest first, the
// staff's and the members' alike, each its code, who created it (a member, the staff, the command line, or an account
// gone since), how many signed up with it of how many it allows, when it ends and its state; picked invites revoked,
// asked first, since a revoked invite never works again; and Create an invite (/moderation/invites/new), how many may
// sign up with it and for how long, which then shows its link to pass on and its code. Who signed up with an invite is
// on that account's screen. A one-person server has none (ADR-0152).

import { sentenceHandleOf } from "@my/app-components";
import {
  inviteUsesLimit,
  refusedMember,
  useStaffInvites,
  useStaffInviteWrites,
  type ModeratedInvite,
} from "@my/app-data";
import {
  BackButton,
  Button,
  ConfirmDialog,
  FactList,
  FeedNotice,
  FieldLabel,
  FormAlert,
  LinkButton,
  ModalPresence,
  NumberField,
  RecordTable,
  Row,
  ScreenStack,
  ScreenTitle,
  Stack,
  StateMark,
  FieldError,
  type RecordColumn,
  type RecordRow,
  type RecordState,
} from "@my/design-system";
import { useId, useRef, useState, type ReactNode } from "react";
import type { StaffAddress } from "../entries";
import { ReadNotice, useMoment, useStaffStrings, type T } from "../parts";
import {
  CopyValue,
  cursorOf,
  LabelledSelect,
  lengthOptions,
  Pages,
  refusalWords,
  secondsOf,
  useEnd,
  useSaid,
} from "./parts";

/** The address of the list of invites. */
const invitesPath = "/moderation/invites";

/** Who created an invite, in words. */
function creatorOf(t: T, invite: ModeratedInvite): string {
  if (invite.created_by !== undefined)
    return t("staff_accounts.invites.by", { handle: sentenceHandleOf(invite.created_by), kind: invite.creator_kind });
  return t(invite.created_by_deleted ? "staff_accounts.invites.by_gone" : "staff_accounts.invites.by_cli");
}

/** How an invite's state is marked: usable is in force, the rest is over. */
const inviteMark = (state: ModeratedInvite["state"]): RecordState =>
  state === "usable"
    ? "active"
    : state === "revoked"
      ? "lifted"
      : state === "creator_ineligible"
        ? "disabled"
        : "expired";

type Column = "uses" | "ends" | "state";

/** The list of invites: the way to create one, the table with its picked rows' Revoke, and the pages. */
export function InvitesScreen(address: StaffAddress): ReactNode {
  const { t } = useStaffStrings();
  const end = useEnd();
  const cursor = cursorOf(address.search);
  const page = useStaffInvites(cursor);
  const { revoke } = useStaffInviteWrites();
  const { say, status } = useSaid();
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [asking, setAsking] = useState(false);
  const [busy, setBusy] = useState(false);
  const head = (
    <ScreenTitle
      actions={
        <LinkButton href={`${invitesPath}/new`} size="timeline">
          {t("staff_accounts.invites.create")}
        </LinkButton>
      }
    >
      {t("staff_accounts.entry.invites")}
    </ScreenTitle>
  );
  if (page.forbidden) return <FeedNotice lead>{t("staff_accounts.forbidden")}</FeedNotice>;
  if (page.status !== "success")
    return (
      <ScreenStack>
        {head}
        <ReadNotice
          status={page.status}
          loading={t("staff_accounts.invites.loading")}
          failed={t("staff_accounts.invites.failed")}
          retry={page.refetch}
        />
      </ScreenStack>
    );
  const usable = page.items.filter((i) => i.state === "usable" || i.state === "creator_ineligible");
  const picked = usable.filter((i) => selected.has(i.id));
  const columns: RecordColumn<Column>[] = [
    { key: "uses", label: t("staff_accounts.invites.column.uses"), kind: "figures" },
    { key: "ends", label: t("staff_accounts.invites.column.ends"), kind: "fit" },
    // A state wraps before a code is cut short: a code is what is copied.
    { key: "state", label: t("staff_accounts.invites.column.state"), beside: true },
  ];
  const rows = page.items.map((i): RecordRow<Column> => ({
    id: i.id,
    head: i.code,
    caption: creatorOf(t, i),
    cells: {
      uses:
        i.max_uses === undefined
          ? t("staff_accounts.invites.uses", { uses: i.uses })
          : t("staff_accounts.invites.uses_of", { uses: i.uses, max: i.max_uses }),
      ends: i.expires_at === undefined ? t("staff_accounts.invites.never") : end(i.expires_at),
      state: <StateMark state={inviteMark(i.state)}>{t("staff_accounts.invite_state", { state: i.state })}</StateMark>,
    },
  }));
  const send = () => {
    setBusy(true);
    void revoke(
      cursor,
      picked.map((i) => i.id),
    ).then((results) => {
      setBusy(false);
      setAsking(false);
      setSelected(new Set());
      const failed = results.find((r) => r.error !== undefined);
      if (failed === undefined) say(t("staff_accounts.invites.revoked", { count: results.length }));
      else say(refusalWords(t, failed.error), true);
    });
  };
  return (
    <ScreenStack busy={page.stale}>
      {head}
      {status}
      <RecordTable
        label={t("staff_accounts.invites.table")}
        head={{ label: t("staff_accounts.invites.column.code") }}
        columns={columns}
        rows={rows}
        busy={page.stale}
        selection={
          usable.length === 0
            ? undefined
            : {
                selected,
                onChange: setSelected,
                // A revoked, expired or used-up invite is not revoked again.
                selectable: (id) => usable.some((i) => i.id === id),
                actions: [
                  {
                    id: "revoke",
                    label: t("staff_accounts.invites.revoke"),
                    onPress: () => {
                      if (picked.length > 0) setAsking(true);
                    },
                    busy,
                  },
                ],
              }
        }
        empty={<FeedNotice>{t("staff_accounts.invites.empty")}</FeedNotice>}
      />
      <Pages address={address} next={page.next} />
      <ModalPresence>
        {asking && (
          <ConfirmDialog
            title={t("staff_accounts.invites.revoke.confirm", { count: picked.length })}
            cancel={t("staff_accounts.cancel")}
            confirm={t("staff_accounts.invites.revoke")}
            busy={busy}
            onCancel={() => {
              setAsking(false);
            }}
            onConfirm={send}
          >
            {t("staff_accounts.invites.revoke.about")}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </ScreenStack>
  );
}

/** Create an invite: how many may sign up and for how long, then its link and its code in the form's place. */
export function NewInviteScreen({ onBack }: { onBack: () => void }): ReactNode {
  const { t } = useStaffStrings();
  const moment = useMoment();
  const { create } = useStaffInviteWrites();
  const usesId = useId();
  const titleRef = useRef<HTMLHeadingElement>(null);
  const [uses, setUses] = useState<number | null>(null);
  const [length, setLength] = useState("7");
  const [faults, setFaults] = useState<Partial<Record<"max_uses" | "expires_in", string>>>({});
  const [alert, setAlert] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const [made, setMade] = useState<ModeratedInvite | undefined>(undefined);
  const back = <BackButton onPress={onBack}>{t("staff_accounts.invites.back")}</BackButton>;
  const submit = () => {
    if (busy) return;
    setBusy(true);
    setFaults({});
    setAlert(undefined);
    const seconds = secondsOf(length);
    create({
      ...(uses === null ? {} : { max_uses: uses }),
      ...(seconds === undefined ? {} : { expires_in: seconds }),
    }).then(
      (invite) => {
        setBusy(false);
        setMade(invite);
        requestAnimationFrame(() => titleRef.current?.focus());
      },
      (e: unknown) => {
        setBusy(false);
        const member = refusedMember(e);
        if (member?.member === "max_uses" || member?.member === "expires_in") {
          setFaults({ [member.member]: member.said });
          if (member.member === "max_uses") document.getElementById(usesId)?.focus();
        } else setAlert(refusalWords(t, e));
      },
    );
  };
  if (made !== undefined)
    return (
      <ScreenStack>
        <ScreenTitle back={back} ref={titleRef}>
          {t("staff_accounts.invites.made")}
        </ScreenTitle>
        <CopyValue label={t("staff_accounts.invites.link")} value={made.url} copy={t("staff_accounts.invites.copy")} />
        <FactList
          label={t("staff_accounts.invites.made")}
          items={[
            { term: t("staff_accounts.invites.code"), value: made.code, code: true },
            {
              term: t("staff_accounts.invites.column.uses"),
              value:
                made.max_uses === undefined
                  ? t("staff_accounts.invites.unlimited")
                  : t("staff_accounts.invites.uses_of", { uses: made.uses, max: made.max_uses }),
            },
            {
              term: t("staff_accounts.invites.column.ends"),
              value: made.expires_at === undefined ? t("staff_accounts.invites.never") : moment(made.expires_at),
            },
          ]}
        />
        <Row>
          <LinkButton href={invitesPath}>{t("staff_accounts.invites.to_list")}</LinkButton>
        </Row>
      </ScreenStack>
    );
  return (
    <ScreenStack>
      <ScreenTitle back={back}>{t("staff_accounts.invites.create")}</ScreenTitle>
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
        <Stack gap="entry-line">
          <FieldLabel htmlFor={usesId} id={`${usesId}-label`}>
            {t("staff_accounts.invites.max_uses")}
          </FieldLabel>
          <NumberField
            id={usesId}
            labelledBy={`${usesId}-label`}
            describedBy={faults.max_uses === undefined ? undefined : `${usesId}-error`}
            value={uses}
            min={1}
            max={inviteUsesLimit}
            onChange={setUses}
          />
          {faults.max_uses !== undefined && <FieldError id={`${usesId}-error`}>{faults.max_uses}</FieldError>}
        </Stack>
        <LabelledSelect
          label={t("staff_accounts.invites.length")}
          options={lengthOptions(t).map((o) =>
            o.value === "0" ? { ...o, label: t("staff_accounts.invites.never") } : o,
          )}
          value={length}
          onChange={setLength}
          error={faults.expires_in}
        />
        {alert !== undefined && <FormAlert regular>{alert}</FormAlert>}
        <Row>
          <Button type="submit" kind="primary" busy={busy}>
            {t("staff_accounts.invites.create.submit")}
          </Button>
        </Row>
      </Stack>
    </ScreenStack>
  );
}
