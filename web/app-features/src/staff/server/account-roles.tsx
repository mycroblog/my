// SPDX-License-Identifier: AGPL-3.0-only

// The roles of a local account as a holder of `manage_roles` grants and revokes them (FR-720), a section of the
// account's Management page, in that page's look: the roles it holds, highest first, each with Revoke where the reader
// may revoke it, and Grant, a choice of the roles the reader may grant it. What FR-720 forbids is not offered: a role
// above the reader's highest, one with a permission the reader lacks, a plan's role (its subscriptions grant it); a
// role the account holds that the reader may not revoke says why in a word. Revoking a role of the reader's own asks
// first, since it may take the permission to give it back. What came of a press is said in the page's status (`say`).

import {
  mayGrant,
  mayRevoke,
  roleManagedByPlan,
  useAccountRoles,
  useAdminRoles,
  useAdminRoleWrites,
  useViewer,
  withinReach,
  type AdminRole,
  type RoleRights,
} from "@my/app-data";
import {
  Button,
  ConfirmDialog,
  FeedNotice,
  ModalPresence,
  RecordTable,
  Row,
  Stack,
  type RecordColumn,
  type RecordRow,
} from "@my/design-system";
import { ProblemError } from "@mycroblog/mykit";
import { useState, type ReactNode } from "react";
import { LabelledSelect } from "../accounts/parts";
import { Titled, useStaffStrings, type T } from "../parts";
import { useStillThere } from "./parts";

/** A word on why the reader may not revoke a role the account holds. */
function keptWord(t: T, role: AdminRole, rights: RoleRights): string {
  if (!withinReach(role, rights)) return t("staff_roles.state.above");
  return t("staff_roles.state.plan");
}

/** Why a grant or a revocation failed, in a sentence. */
function failedWords(t: T, error: unknown, key: string, revoking: boolean): string {
  if (roleManagedByPlan(error)) return t("staff_roles.account.plan", { key });
  if (error instanceof ProblemError && error.status === 409) return t("staff_roles.account.last_admin");
  if (error instanceof ProblemError && error.status === 403) return t("staff_roles.save.forbidden");
  return t(revoking ? "staff_roles.account.revoke_failed" : "staff_roles.account.grant_failed", { key });
}

export type AccountRolesProps = {
  /** The local account's ID. */
  readonly accountId: string;
  /** The account's handle, as a sentence names it (`@kai`). */
  readonly handle: string;
  /** Says in the page's status what a press did, or that it failed (`alert`). */
  readonly say: (words: string, alert?: boolean) => void;
};

type Column = "place" | "act";

/** The roles of the account of `accountId`: those it holds, Revoke, and Grant. */
export function AccountRoles({ accountId, handle, say }: AccountRolesProps): ReactNode {
  const { t } = useStaffStrings();
  const held = useAccountRoles(accountId);
  const all = useAdminRoles();
  const writes = useAdminRoleWrites();
  const me = useViewer()?.id;
  const there = useStillThere();
  const [chosen, setChosen] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState<string | undefined>(undefined);
  const [asking, setAsking] = useState<AdminRole | undefined>(undefined);
  const heading = t("staff_roles.account");
  if (held.held === undefined || all.roles === undefined || all.rights === undefined) {
    const failed = held.status === "failed" || all.status === "failed";
    return (
      <Titled title={heading}>
        <FeedNotice
          alert={failed}
          action={
            failed ? (
              <Button
                size="timeline"
                touch
                onPress={() => {
                  held.retry();
                  all.retry();
                }}
              >
                {t("staff.retry")}
              </Button>
            ) : undefined
          }
        >
          {t(failed ? "staff_roles.failed" : "staff_roles.loading")}
        </FeedNotice>
      </Titled>
    );
  }
  const rights = all.rights;
  const holds = new Set(held.held.map((r) => r.key));
  const grantable = all.roles.filter((r) => !holds.has(r.key) && mayGrant(r, rights));
  const pick = grantable.find((r) => r.key === chosen) ?? grantable[0];
  const act = (role: AdminRole, revoking: boolean) => {
    setBusy(role.key);
    setAsking(undefined);
    (revoking ? writes.revoke(accountId, role.key) : writes.grant(accountId, role.key)).then(
      () => {
        if (!there.current) return;
        setBusy(undefined);
        say(t(revoking ? "staff_roles.account.revoked" : "staff_roles.account.granted", { key: role.key, handle }));
      },
      (error: unknown) => {
        if (!there.current) return;
        setBusy(undefined);
        say(failedWords(t, error, role.key, revoking), true);
      },
    );
  };
  const columns: RecordColumn<Column>[] = [
    { key: "place", label: t("staff_roles.position"), kind: "fit", bare: true },
    { key: "act", label: t("staff_roles.account.act"), kind: "fit", beside: true },
  ];
  const rows = held.held.map((role): RecordRow<Column> => ({
    id: role.id,
    head: role.key,
    cells: {
      place: t("staff_roles.row.place", { position: role.position }),
      act: mayRevoke(role, rights) ? (
        <Button
          size="timeline"
          touch
          busy={busy === role.key}
          isDisabled={busy !== undefined}
          onPress={() => {
            if (accountId === me) setAsking(role);
            else act(role, true);
          }}
        >
          {t("staff_roles.account.revoke")}
        </Button>
      ) : (
        keptWord(t, role, rights)
      ),
    },
  }));
  return (
    <>
      <Titled title={heading}>
        <Stack gap="entry-room">
          <RecordTable
            label={heading}
            head={{ label: t("staff_roles.account.role") }}
            columns={columns}
            rows={rows}
            empty={<FeedNotice>{t("staff_roles.account.none")}</FeedNotice>}
          />
          {pick === undefined ? (
            <FeedNotice>{t("staff_roles.account.none_to_grant")}</FeedNotice>
          ) : (
            <Row align="end" gap="screen-row-gap" wrap>
              <LabelledSelect
                label={t("staff_roles.account.grant_role")}
                options={grantable.map((r) => ({ value: r.key, label: r.key }))}
                value={pick.key}
                onChange={setChosen}
              />
              <Button
                busy={busy === pick.key}
                isDisabled={busy !== undefined}
                onPress={() => {
                  act(pick, false);
                }}
              >
                {t("staff_roles.account.grant")}
              </Button>
            </Row>
          )}
        </Stack>
      </Titled>
      <ModalPresence>
        {asking !== undefined && (
          <ConfirmDialog
            key="revoke-own"
            title={t("staff_roles.account.own.title", { key: asking.key })}
            cancel={t("staff_roles.account.own.cancel")}
            confirm={t("staff_roles.account.revoke")}
            onCancel={() => {
              setAsking(undefined);
            }}
            onConfirm={() => {
              act(asking, true);
            }}
          >
            {t("staff_roles.account.own.text")}
          </ConfirmDialog>
        )}
      </ModalPresence>
    </>
  );
}
