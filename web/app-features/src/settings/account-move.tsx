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
//   moved itself, it cannot be moved to) under the field, which takes the focus; a move too soon after the last one,
//   and anything else, before the button, in the app's words where the refusal has a kind and the server's otherwise.
// - Once the account has moved, the screen is its state: where it moved and what it can still do, and, in a section
//   of its own, cancelling the move, which asks first and says what it does not undo. The API says neither when the
//   account moved nor when it may move again, so the screen says the wait in days and no date.
//
// The view that comes with a move, or with its cancellation, says what happened first, and its title takes the focus
// once the dialog over it has left.

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
import { useId, useRef, useState } from "react";
import { stringsIn } from "../locale";
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
            text: t("settings.account.move.effect.wait", { days: moveCooldownDays }),
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

export function AccountMoveScreen({ onBack, fragment, onFragmentRead, leave }: SettingsSubProps) {
  const { t } = stringsIn(useLocale().locale);
  const fieldId = useId();
  const { status, retry, account } = useOwnAccountRead();
  const { find, move, cancel } = useAccountMove();
  const returned = useProviderReturn({ fragment, onFragmentRead });
  const [typed, setTyped] = useState("");
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
  const moving = useConfirmation({
    operation: "account_move",
    about: t("settings.account.move.confirm", { handle: target === undefined ? "" : handleOf(target) }),
    returnTo: accountMovePath,
    returned,
    leave,
  });

  const title = t("settings.account.move");
  const back = <BackButton onPress={onBack}>{t("settings.entry.account")}</BackButton>;
  const days = moveCooldownDays;

  const refuse = (words: string) => {
    setFault(words);
    focusFault();
  };
  const submit = () => {
    if (account === undefined || finding || moving.busy) return;
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
            else if (reason === "too_soon") setSaid(t("settings.account.move.refused", { reason, handle, days }));
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
              })
            }
            onClose={() => {
              setAsking(undefined);
            }}
            onLeft={after.settle}
          >
            {t("settings.account.move.cancel.text", { days })}
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
            about={t("settings.account.move.cancel.about", { handle, days })}
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
          {arrived === "cancelled" && (
            <StatusSummary
              icon="accepted"
              title={t("settings.account.move.cancelled")}
              titleRef={summary}
              about={t("settings.account.move.cancelled.about")}
            />
          )}
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
              error={fault}
              readOnly={finding}
              onChange={(value) => {
                setTyped(value);
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
