// SPDX-License-Identifier: AGPL-3.0-only

// Move account, the screen under the settings' entry Account and data at /settings/account/move (FR-009, ADR-0089
// §3, §8), which the closure's banner leads to: moving this account's followers to another account.
//
// - Before a move, one form saved as a whole. It first says what a move does: what moves (the followers), what does
//   not (the posts, the follows, the blocks, mutes and bookmarks, with the way to Export, which carries them as CSV
//   files), what becomes of this account, and for how long it cannot move again. Then what the reader does first on
//   the other account, in the words of that account's Aliases screen, with this account's handle to copy. Then the
//   field that names the account to move to, a handle or the address of a profile, and the button that moves. The
//   server looks the account up, the reader confirms it's them (confirmation.tsx), and the move is sent.
// - What is wrong stands where it is wrong: an empty field, an address the server finds no account at, this account
//   itself, and what the server refuses of the account named (it does not list this account among its aliases, it
//   moved itself, it cannot be moved to) under the field, which takes the focus; anything else before the button, in
//   the server's words.
// - Where the screen knows the account cannot move now (it cancelled the move here, or the server refused a move as
//   too soon after the last one), the act's place says so, with no field and no button. The API says neither when the
//   account moved nor when it may move again, so the screen says the wait in days and no date, and keeps it for this
//   visit alone: after a reload the form is back, and a press finds it out again. The server's refusal names one kind
//   for the two causes it has (moved in the last 30 days, or to that account before), so its sentence leads with the
//   likelier and names both.
// - Once the account has moved, the screen is its state: where it moved and what it can still do, and, in a section
//   of its own, cancelling the move, which asks first and says what it does not undo.
//
// The view that comes with a move, or with its cancellation, says what happened first, and its title takes the focus
// once the dialog over it has left. The account named waits in the tab while the reader confirms at a login
// provider's page, and is in the field again on the return (confirmation.tsx).

import { accountMovePath, dataExportPath, handleOf } from "@my/app-components";
import {
  accountNotFound,
  confirmationFields,
  loadOwnAccount,
  moveCooldownDays,
  moveRefusalOf,
  useAccountMove,
  useOwnAccountRead,
  type Account,
  type AppData,
} from "@my/app-data";
import {
  BackButton,
  Button,
  FormAlert,
  ModalPresence,
  Row,
  SettingsFacts,
  SettingsNote,
  SettingsScreen,
  SettingsSection,
  StatusSummary,
  SubmitBar,
  TextField,
  useLocale,
} from "@my/design-system";
import { useId, useRef, useState, type RefObject } from "react";
import { stringsIn } from "../locale";
import { tied } from "./account";
import { saidOf, useConfirmation, useProviderReturn } from "./confirmation";
import type { SettingsSubProps } from "./entries";
import { Asking, Kept, ReadingNotice, useFaultFocus, useFocusAfterDialog } from "./parts";

type T = ReturnType<typeof stringsIn>["t"];

/** What the route's loader waits for: the reader's account and what it confirms with, from the cache when it has them. */
export async function loadAccountMoveSettings(data: AppData): Promise<void> {
  await loadOwnAccount(data);
}

/** The title of the screen. */
export function useAccountMoveTitle(): string {
  return stringsIn(useLocale().locale).t("settings.account.move");
}

/** What a move does, before the reader makes one: what moves, what stays, what becomes of this account, the wait. */
function MoveEffect({ t }: { t: T }) {
  const heading = t("settings.account.move.effect");
  return (
    <SettingsSection heading={heading}>
      <SettingsFacts
        label={heading}
        items={[
          {
            text: t("settings.account.move.effect.followers"),
            about: [t("settings.account.move.effect.followers.about")],
          },
          { text: t("settings.account.move.effect.stays"), about: [t("settings.account.move.effect.stays.about")] },
          { text: t("settings.account.move.effect.account"), about: [t("settings.account.move.effect.account.about")] },
          {
            text: tied(t("settings.account.move.effect.wait", { days: moveCooldownDays })),
            about: [t("settings.account.move.effect.wait.about")],
          },
        ]}
      />
      <SettingsNote links={[{ href: dataExportPath, label: t("settings.account.export") }]}>
        {t("settings.account.move.effect.export")}
      </SettingsNote>
    </SettingsSection>
  );
}

/** What the reader does first on the account they move to, with this account's handle to enter there. */
function AliasFirst({ account, t }: { account: Account; t: T }) {
  return (
    <SettingsSection heading={t("settings.account.move.alias")} about={t("settings.account.move.alias.about")}>
      <Kept
        label={t("settings.account.move.alias.handle")}
        values={[handleOf(account)]}
        copy={t("settings.account.move.alias.copy")}
      />
    </SettingsSection>
  );
}

/** What has just come of the reader's act on this screen: the account moved, or its move was cancelled. */
type Arrived = "moved" | "cancelled";

/**
 * Why the screen knows the account cannot move now: it cancelled the move here, or the server refused a move to the
 * account of `handle` as too soon.
 */
type Later = { readonly cause: "cancelled" } | { readonly cause: "refused"; readonly handle: string };

/** In the act's place while the account cannot move now: that it cannot, and until when, in days. */
function MoveLater({ later, titleRef, t }: { later: Later; titleRef: RefObject<HTMLHeadingElement | null>; t: T }) {
  const days = moveCooldownDays;
  return (
    <StatusSummary
      icon="caution"
      title={t("settings.account.move.later")}
      titleRef={later.cause === "refused" ? titleRef : undefined}
      about={tied(
        later.cause === "cancelled"
          ? t("settings.account.move.later.cancelled", { days })
          : t("settings.account.move.refused", { reason: "too_soon", handle: later.handle, days }),
      )}
    />
  );
}

export function AccountMoveScreen({ onBack, fragment, onFragmentRead, leave }: SettingsSubProps) {
  const { t } = stringsIn(useLocale().locale);
  const fieldId = useId();
  const { status, retry, account } = useOwnAccountRead();
  const { find, move, cancel } = useAccountMove();
  const returned = useProviderReturn({ fragment, onFragmentRead });
  /** What the reader entered in the field; until they change it, what came back with them from a provider. */
  const [entered, setEntered] = useState<string>();
  /** The account cannot move now, as this visit found out. */
  const [later, setLater] = useState<Later>();
  /** The account the server found at what the reader entered, which the confirmation names. */
  const [target, setTarget] = useState<Account>();
  const [finding, setFinding] = useState(false);
  /** What is wrong with the account named, under its field. */
  const [fault, setFault] = useState<string>();
  /** What was refused and names no field, before the button. */
  const [said, setSaid] = useState<string>();
  const [arrived, setArrived] = useState<Arrived>();
  /**
   * The account the move to which the reader is asked whether to cancel. The question keeps it until it has left: the
   * account read again says no move as soon as the server took the cancellation, before the question closes.
   */
  const [asking, setAsking] = useState<Account>();
  const focusFault = useFaultFocus(fieldId);
  const summary = useRef<HTMLHeadingElement>(null);
  const after = useFocusAfterDialog(summary);
  const days = moveCooldownDays;
  const named = target === undefined ? "" : handleOf(target);
  const moving = useConfirmation({
    operation: "account_move",
    about: tied(t("settings.account.move.confirm", { handle: named, days })),
    act: {
      title: t("settings.account.move.confirm.title", { handle: named }),
      confirm: t("settings.account.move.submit"),
    },
    kept: entered,
    returnTo: accountMovePath,
    returned,
    leave,
  });
  const typed = entered ?? moving.kept ?? "";

  const title = t("settings.account.move");
  const back = <BackButton onPress={onBack}>{t("settings.entry.account")}</BackButton>;

  const refuse = (words: string) => {
    setFault(words);
    focusFault();
  };
  const submit = () => {
    if (account === undefined || finding || moving.busy || later !== undefined) return;
    const address = typed.trim();
    setArrived(undefined);
    setSaid(undefined);
    if (address === "") {
      refuse(t("settings.account.move.target.missing"));
      return;
    }
    setFault(undefined);
    setFinding(true);
    find(address).then(
      (found) => {
        setFinding(false);
        if (found.id === account.id) {
          refuse(t("settings.account.move.target.own"));
          return;
        }
        setTarget(found);
        const handle = handleOf(found);
        moving.ask((confirmation) => move(found.id, confirmationFields(confirmation)), {
          done: () => {
            after.expect();
            setArrived("moved");
          },
          failed: (error) => {
            const reason = moveRefusalOf(error);
            if (reason === undefined) setSaid(saidOf(error, t));
            // The account cannot move now: the act's place says so, and its title takes the focus once the dialog
            // left.
            else if (reason === "too_soon") {
              after.expect();
              setLater({ cause: "refused", handle });
            }
            // What the server refused of the account named stands under its field, which takes the focus once the
            // dialog left.
            else setFault(t("settings.account.move.refused", { reason, handle, days }));
          },
          left: () => {
            focusFault();
            after.settle();
          },
        });
      },
      (error: unknown) => {
        setFinding(false);
        if (accountNotFound(error)) refuse(t("settings.account.move.target.not_found"));
        else setSaid(saidOf(error, t));
      },
    );
  };

  const moved = account?.moved;
  const dialogs = (
    <>
      {moving.dialog}
      <ModalPresence>
        {asking !== undefined && (
          <Asking
            title={t("settings.account.move.cancel.title", { handle: handleOf(asking) })}
            confirm={t("settings.account.move.cancel.submit")}
            cancel={t("settings.account.move.cancel.keep")}
            run={() =>
              cancel().then(() => {
                after.expect();
                setArrived("cancelled");
                // A cancelled move still counts: the account cannot move again until 30 days after it.
                setLater({ cause: "cancelled" });
              })
            }
            onClose={() => {
              setAsking(undefined);
            }}
            onLeft={after.settle}
          >
            {tied(t("settings.account.move.cancel.text", { days }))}
          </Asking>
        )}
      </ModalPresence>
    </>
  );

  if (account === undefined || status !== "read")
    return (
      <SettingsScreen
        sub
        title={title}
        back={back}
        notice={
          <ReadingNotice
            words={{ loading: t("settings.account.loading"), failed: t("settings.account.failed") }}
            status={status === "failed" ? "failed" : "loading"}
            retry={retry}
          />
        }
      />
    );

  if (moved !== undefined) {
    const handle = handleOf(moved);
    return (
      <>
        <SettingsScreen sub title={title} back={back}>
          <StatusSummary
            icon="moved"
            title={t("settings.account.move.moved", { handle })}
            titleRef={arrived === "moved" ? summary : undefined}
            about={t("settings.account.move.moved.about")}
          />
          <SettingsSection
            heading={t("settings.account.move.cancel")}
            about={tied(t("settings.account.move.cancel.about", { handle, days }))}
          >
            <Row justify="start">
              <Button
                onPress={() => {
                  setAsking(moved);
                }}
              >
                {t("settings.account.move.cancel.submit")}
              </Button>
            </Row>
          </SettingsSection>
        </SettingsScreen>
        {dialogs}
      </>
    );
  }

  const cancelled =
    arrived === "cancelled" ? (
      <StatusSummary
        icon="accepted"
        title={t("settings.account.move.cancelled")}
        titleRef={summary}
        about={t("settings.account.move.cancelled.about")}
      />
    ) : null;

  // The account cannot move now: what a move does and what to do first stand, and the act's place says so.
  if (later !== undefined)
    return (
      <>
        <SettingsScreen sub title={title} back={back}>
          {cancelled}
          <MoveEffect t={t} />
          <AliasFirst account={account} t={t} />
          <MoveLater later={later} titleRef={summary} t={t} />
        </SettingsScreen>
        {dialogs}
      </>
    );

  return (
    <>
      <form
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          submit();
        }}
      >
        <SettingsScreen
          sub
          title={title}
          back={back}
          foot={
            <SubmitBar result={said === undefined ? undefined : <FormAlert>{said}</FormAlert>}>
              <Button kind="primary" type="submit" busy={finding || moving.busy}>
                {t("settings.account.move.submit")}
              </Button>
            </SubmitBar>
          }
        >
          {cancelled}
          <MoveEffect t={t} />
          <AliasFirst account={account} t={t} />
          <SettingsSection heading={t("settings.account.move.form")}>
            {moving.notice}
            <TextField
              id={fieldId}
              name="acct"
              label={t("settings.account.move.target")}
              hint={t("settings.account.aliases.add.hint")}
              autoCapitalize="none"
              autoComplete="off"
              defaultValue={typed}
              error={fault}
              readOnly={finding}
              onChange={(value) => {
                setEntered(value);
                setFault(undefined);
              }}
            />
          </SettingsSection>
        </SettingsScreen>
      </form>
      {dialogs}
    </>
  );
}
