// SPDX-License-Identifier: AGPL-3.0-only

// The confirmation that asks the reader to authenticate again (ADR-0106 §3.7), shared by every screen of the settings
// whose operation the server takes only with a `Confirmation`: changing or removing the password, adding a passkey,
// linking an account, and moving, deactivating or deleting the account. A screen calls `useConfirmation` with its
// operation, places the hook's `dialog` once and its `notice` where the reader looks, and runs its write through
// `ask`: the dialog opens, the reader proves it is them, and the write runs with that proof while the dialog spins.
//
// The ways are the account's own, one at a time, the others a press away under it: the current password (never for
// removing it), a passkey, where the account has one and the browser can use it, and each linked account whose
// provider is enabled. They are offered in that order, GitHub after every other provider: it cannot ask the reader to
// sign in again, so a reader who can confirm another way is offered that first. A proof the server refuses (a wrong
// password, a passkey that does not verify, a confirmation that expired, too many tries) stays in the dialog, in the
// server's words, for another try; anything else the write fails for closes the dialog and is the screen's to say.
//
// A linked account confirms on its provider's page: the app is left for it (`leave`), and the provider sends the
// browser back to the screen's own address (`returnTo`) with the confirmation in the fragment, which the screen reads
// once (`useProviderReturn`). What the reader had entered is gone with the page, so nothing resumes by itself: the
// notice says that they confirmed and asks them to do it again, and that next `ask` runs with the provider's
// confirmation and opens no dialog. The confirmation works once, for ten minutes; one the server no longer takes opens
// the dialog, which says so. A value of the screen's form that is no secret (the account a move names) is kept in the
// tab under the operation while the reader is at the provider, and given back on the return (`kept`).
//
// An operation that is itself an act done at once (moving, deactivating, deleting the account) names it (`act`): the
// dialog asks the act's question, says what the press does, in the text's color, over how to prove it, and its button
// is named by the act wherever the press does it (not where it leaves for a provider). Back from a provider, the next
// `ask` still asks, with no proof: a plain confirmation of the act, named and said as the dialog does.

import {
  confirmationRefused,
  readExternalOutcome,
  refusal,
  useConfirmWithPasskey,
  useHasPassword,
  useLinkedIdentities,
  useLinkedIdentityWrites,
  usePasskeys,
  type Confirmation,
  type ConfirmedOperation,
  type ExternalOutcome,
  type LinkedIdentity,
} from "@my/app-data";
import {
  ConfirmDialog,
  FormAlert,
  ModalPresence,
  ReauthDialog,
  SaveState,
  TextButton,
  TextField,
  useLocale,
} from "@my/design-system";
import { PasskeyError, readTabValue, supportsPasskeys, writeTabValue } from "@my/platform";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { stringsIn } from "../locale";
import type { SettingsAddress } from "./entries";

type T = ReturnType<typeof stringsIn>["t"];

/**
 * What a login provider's return left in the fragment of the screen's address (a link made, a confirmation, or why
 * neither), read once as the screen opens; the fragment is then taken off the address. Null without one.
 */
export function useProviderReturn({
  fragment,
  onFragmentRead,
}: Pick<SettingsAddress, "fragment" | "onFragmentRead">): ExternalOutcome | null {
  const [outcome] = useState(() => readExternalOutcome(fragment));
  // Read as the screen opened; it stays on the address until the app has taken it off.
  const left = outcome !== null && fragment !== "";
  useEffect(() => {
    if (left) onFragmentRead();
  }, [left, onFragmentRead]);
  return outcome;
}

/** What the server said of a write it refused, in the reader's language; the settings' own words for no answer. */
export function saidOf(error: unknown, t: T): string {
  return refusal(error) ?? t("settings.confirm.failed");
}

/** Why a link or a confirmation at a login provider did not happen, as a sentence. */
export function providerFailure(outcome: Extract<ExternalOutcome, { kind: "error" }>, t: T): string {
  return t("settings.confirm.external.error", { reason: outcome.reason });
}

/** A way to confirm: the password, a passkey, or a linked account at its provider. */
type Way =
  | { readonly key: "password" }
  | { readonly key: "passkey" }
  | { readonly key: string; readonly identity: LinkedIdentity };

/**
 * The ways the account can confirm with in this browser, in the order they are offered. The screen reads them as it
 * opens, before the reader asks, so that the dialog opens on the one it will keep.
 */
function useWays(password: boolean): readonly Way[] {
  const hasPassword = useHasPassword();
  const passkeys = usePasskeys();
  const linked = useLinkedIdentities();
  const providers = (linked?.items ?? []).filter((identity) => identity.available);
  // GitHub cannot ask for a new sign-in (ADR-0106 §3.7): every other way comes first.
  const ordered = [
    ...providers.filter((identity) => identity.provider_kind !== "github"),
    ...providers.filter((identity) => identity.provider_kind === "github"),
  ];
  return [
    ...(password && hasPassword === true ? [{ key: "password" } as const] : []),
    ...(passkeys !== undefined && passkeys.length > 0 && supportsPasskeys() ? [{ key: "passkey" } as const] : []),
    ...ordered.map((identity) => ({ key: `provider:${identity.id}`, identity })),
  ];
}

/**
 * Runs `onGone` once the dialog that calls this is out of the document: when the presence around it has let it go,
 * after the motion of its leaving, React takes its nodes out and then runs this effect's cleanup. Until then the
 * dialog holds the focus, and takes back one moved elsewhere. `onGone` is the same function for as long as the dialog
 * is open, or a new one would run the one before it.
 */
export function useGone(onGone: () => void): void {
  useEffect(() => onGone, [onGone]);
}

/** A write that waits for its confirmation, and what follows it. */
type Asked = {
  readonly run: (confirmation: Confirmation) => Promise<unknown>;
  readonly done: (() => void) | undefined;
  readonly failed: ((error: unknown) => void) | undefined;
  readonly left: (() => void) | undefined;
  /** What the dialog says as it opens: the server's refusal of a provider's confirmation. */
  readonly said: string | undefined;
  /** The provider's confirmation an act runs with once the reader confirmed the act alone, with no proof. */
  readonly held: string | undefined;
};

/** What the screen names an act by, where the confirmation does one at once (`act` of `useConfirmation`). */
export type ConfirmationAct = {
  /** The act's question, the dialog's title (Delete this account?). */
  title: string;
  /** The button that does it, named by the act (Delete account). */
  confirm: string;
};

/**
 * Where the tab keeps the value of a screen's form across a provider's page, for `operation` (`kept` of
 * `useConfirmation`): a key of the tab's session storage.
 */
export const confirmationKeptKey = (operation: ConfirmedOperation): string => `my.confirmation.${operation}.kept`;
const keptKey = confirmationKeptKey;

type DialogProps = {
  operation: ConfirmedOperation;
  about: string;
  act: ConfirmationAct | undefined;
  kept: string | undefined;
  returnTo: string;
  leave: (href: string) => void;
  ways: readonly Way[];
  asked: Asked;
  /** The provider's confirmation held was used: it works once. */
  onHeldUsed: () => void;
  /** Cancel, close, Escape: nothing was written. */
  onClose: () => void;
  /** The write ended, done or failed: the dialog leaves, and `asked.left` follows once it has. */
  onEnded: () => void;
  /** The dialog is out of the document, its leaving's motion over. The same function for as long as it is open. */
  onGone: () => void;
};

function ConfirmationDialog({
  operation,
  about,
  act,
  kept,
  returnTo,
  leave,
  ways,
  asked,
  onHeldUsed,
  onClose,
  onEnded,
  onGone,
}: DialogProps) {
  const { t } = stringsIn(useLocale().locale);
  const fieldId = useId();
  useGone(onGone);
  const confirmWithPasskey = useConfirmWithPasskey();
  const { beginConfirmation } = useLinkedIdentityWrites();
  const [chosen, setChosen] = useState<string>();
  const [typed, setTyped] = useState("");
  const [busy, setBusy] = useState(false);
  const [said, setSaid] = useState(asked.said);
  /** The provider's confirmation the act runs with, until the server no longer takes it. */
  const [held, setHeld] = useState(asked.held);
  const way = ways.find((w) => w.key === chosen) ?? ways[0];

  const refuse = (words: string) => {
    setBusy(false);
    setSaid(words);
  };
  /** Runs the write with `confirmation`; a proof the server refuses stays in the dialog, anything else ends it. */
  const run = (confirmation: Confirmation, refused: (words: string) => void) => {
    asked.run(confirmation).then(
      () => {
        onEnded();
        asked.done?.();
      },
      (error: unknown) => {
        if (confirmationRefused(error, confirmation.kind)) {
          refused(saidOf(error, t));
          return;
        }
        onEnded();
        asked.failed?.(error);
      },
    );
  };

  // Back from a provider, an act is still asked, with nothing to prove: its question, what it does, and its button.
  if (act !== undefined && held !== undefined)
    return (
      <ConfirmDialog
        title={act.title}
        closeLabel={t("settings.confirm.close")}
        cancel={t("settings.confirm.cancel")}
        confirm={act.confirm}
        busy={busy}
        onCancel={onClose}
        onConfirm={() => {
          setBusy(true);
          // The provider's confirmation works once.
          onHeldUsed();
          run({ kind: "external", token: held }, (words) => {
            // The server no longer takes it: the dialog asks for a proof, and says why.
            setHeld(undefined);
            refuse(words);
          });
        }}
      >
        {about}
      </ConfirmDialog>
    );

  const cancel = {
    title: act?.title ?? t("settings.confirm.title"),
    closeLabel: t("settings.confirm.close"),
    cancel: t("settings.confirm.cancel"),
    onCancel: onClose,
  };
  // An account that can prove nothing here (its passkeys alone, in a browser without them): the dialog says so, and
  // has nothing to confirm with: Cancel and close are its ways out.
  if (way === undefined)
    return <ReauthDialog {...cancel} about={about} error={<FormAlert>{t("settings.confirm.none")}</FormAlert>} />;
  /** The proof of the way chosen; undefined when the browser's prompt gave none, or the app was left for a provider. */
  const prove = async (): Promise<Confirmation | undefined> => {
    if (way.key === "password") return { kind: "password", password: typed };
    if (!("identity" in way)) {
      try {
        return await confirmWithPasskey();
      } catch (error) {
        const reason = error instanceof PasskeyError ? error.reason : undefined;
        if (reason === "aborted") setBusy(false);
        else refuse(reason === undefined ? saidOf(error, t) : t("settings.confirm.passkey.failed", { reason }));
        return undefined;
      }
    }
    try {
      const start = await beginConfirmation(way.identity.provider_id, operation, returnTo);
      // What the reader entered and is no secret waits in the tab for the return.
      if (kept !== undefined) writeTabValue(keptKey(operation), kept);
      // The dialog keeps spinning until the provider's page takes the app's place.
      leave(start.url);
    } catch (error) {
      refuse(saidOf(error, t));
    }
    return undefined;
  };
  const confirm = () => {
    if (way.key === "password" && typed === "") return;
    setBusy(true);
    setSaid(undefined);
    void prove().then((confirmation) => {
      if (confirmation !== undefined) run(confirmation, refuse);
    });
  };

  const others = ways.filter((w) => w !== way);
  const provider = "identity" in way ? way.identity.provider_name : undefined;
  /** How the way chosen proves it, in a sentence. */
  const proving =
    provider !== undefined
      ? t("settings.confirm.provider.about", { provider })
      : act === undefined
        ? about
        : way.key === "passkey"
          ? t("settings.confirm.way.passkey")
          : t("settings.confirm.way.password");
  return (
    <ReauthDialog
      {...cancel}
      effect={act === undefined ? undefined : about}
      about={proving}
      confirm={
        provider !== undefined
          ? t("settings.confirm.provider", { provider })
          : act !== undefined
            ? act.confirm
            : way.key === "passkey"
              ? t("settings.confirm.passkey")
              : t("settings.confirm.submit")
      }
      onConfirm={confirm}
      busy={busy}
      error={said === undefined ? undefined : <FormAlert>{said}</FormAlert>}
      // Each other way a row of its own (ReauthDialog), so that one is never pressed for the next.
      other={others.map((w) => (
        <TextButton
          key={w.key}
          isDisabled={busy}
          onPress={() => {
            setChosen(w.key);
            setSaid(undefined);
          }}
        >
          {"identity" in w
            ? t("settings.confirm.other.provider", { provider: w.identity.provider_name })
            : w.key === "password"
              ? t("settings.confirm.other.password")
              : t("settings.confirm.other.passkey")}
        </TextButton>
      ))}
    >
      {way.key === "password" && (
        <TextField
          key="password"
          id={fieldId}
          name="password"
          type="password"
          label={t("settings.confirm.password")}
          autoComplete="current-password"
          required
          onChange={setTyped}
        />
      )}
    </ReauthDialog>
  );
}

export type ConfirmationOptions = Pick<SettingsAddress, "leave"> & {
  /** The operation the server confirms. */
  operation: ConfirmedOperation;
  /**
   * Why the dialog asks, in a sentence (To change your password, confirm it's you.); with an `act`, what the press
   * does (The account and its posts are deleted at once…).
   */
  about: string;
  /**
   * The act the confirmation does at once, where it is one (moving, deactivating, deleting the account): the dialog's
   * question and the name of its button. Without it, the dialog is Confirm it's you, its button the way's.
   */
  act?: ConfirmationAct | undefined;
  /**
   * What the reader entered in the screen's form and would enter again after a provider's page, never a secret (the
   * account a move names): kept in the tab under the operation while the app is left, and given back as `kept`.
   */
  kept?: string | undefined;
  /** The app's address a login provider sends the browser back to: the screen's own. */
  returnTo: string;
  /** What a login provider's return left in the screen's address (`useProviderReturn`). */
  returned: ExternalOutcome | null;
  /** Whether the current password confirms the operation: not its own removal. */
  password?: boolean | undefined;
};

export type Confirmed = {
  /**
   * Runs `run` once the reader has confirmed, with their confirmation: after the dialog, or with the confirmation a
   * login provider gave, at once or, with an `act`, once the reader confirmed the act alone. `done` follows a write
   * that succeeded; `failed`, with the error, one that failed for anything but its confirmation; `left` follows either
   * once the dialog is out of the document, the motion of its leaving over (at once where none was shown), which is
   * when the screen can move the focus: until then the dialog keeps it. Cancelling the dialog runs none of them.
   */
  ask: (
    run: (confirmation: Confirmation) => Promise<unknown>,
    then?: { done?: () => void; failed?: (error: unknown) => void; left?: () => void },
  ) => void;
  /** A write runs on a login provider's confirmation, with no dialog: the control that asked spins. */
  busy: boolean;
  /**
   * A login provider's confirmation is held for this operation: the next `ask` asks for no proof (an `act` is still
   * asked, plainly). Until it is used, or the screen is left.
   */
  held: boolean;
  /** What `kept` held as the reader left for a provider, back with them for this operation; undefined otherwise. */
  kept: string | undefined;
  /** The dialog, which the screen places once. */
  dialog: ReactNode;
  /**
   * What came back from a login provider for this operation, for the screen to say where the reader looks: that they
   * confirmed and what to do now, or why the confirmation did not happen. Null with nothing to say.
   */
  notice: ReactNode;
};

/**
 * The name of the provider of `id`, as the administrators wrote it, among the server's providers and the account's
 * links; undefined for one neither lists, of which a screen then speaks in a sentence of its own, with no name.
 */
export function useProviderName(): (id: string | undefined) => string | undefined {
  const linked = useLinkedIdentities();
  return (id) =>
    linked?.providers.find((provider) => provider.id === id)?.display_name ??
    linked?.items.find((identity) => identity.provider_id === id)?.provider_name;
}

/** The confirmation of `operation` for a screen of the settings (see the head of this file). */
export function useConfirmation({
  operation,
  about,
  act,
  kept,
  returnTo,
  returned,
  leave,
  password = true,
}: ConfirmationOptions): Confirmed {
  const { t } = stringsIn(useLocale().locale);
  const nameOf = useProviderName();
  const ways = useWays(password);
  const own = returned !== null && returned.kind !== "linked" && returned.operation === operation ? returned : null;
  const [token, setToken] = useState(own?.kind === "confirmation" ? own.token : undefined);
  /** What the tab kept of the form, back from the provider of this operation. */
  const [back] = useState(() => (own === null ? undefined : (readTabValue(keptKey(operation)) ?? undefined)));
  // Read once, as the screen opened: a later visit starts empty.
  useEffect(() => {
    writeTabValue(keptKey(operation), null);
  }, [operation]);
  const [busy, setBusy] = useState(false);
  const [asked, setAsked] = useState<Asked>();
  /** What follows the dialog's leaving, after a write that ended in it. */
  const afterLeaving = useRef<(() => void) | undefined>(undefined);
  /** Runs it, once: the dialog's own word that it is out of the document. */
  const [gone] = useState(() => () => {
    const follows = afterLeaving.current;
    afterLeaving.current = undefined;
    follows?.();
  });

  const ask: Confirmed["ask"] = (run, then) => {
    const next = { run, done: then?.done, failed: then?.failed, left: then?.left };
    if (token === undefined) {
      setAsked({ ...next, said: undefined, held: undefined });
      return;
    }
    // An act is asked all the same, with no proof: the press on its button does it.
    if (act !== undefined) {
      setAsked({ ...next, said: undefined, held: token });
      return;
    }
    // The provider's confirmation works once.
    setToken(undefined);
    setBusy(true);
    run({ kind: "external", token }).then(
      () => {
        setBusy(false);
        next.done?.();
        next.left?.();
      },
      (error: unknown) => {
        setBusy(false);
        if (confirmationRefused(error, "external")) {
          setAsked({ ...next, said: saidOf(error, t), held: undefined });
          return;
        }
        next.failed?.(error);
        next.left?.();
      },
    );
  };

  const provider = nameOf(own?.provider);
  const notice =
    own?.kind === "error" ? (
      <FormAlert>{providerFailure(own, t)}</FormAlert>
    ) : token !== undefined ? (
      <SaveState state="saved">
        {act !== undefined
          ? provider === undefined
            ? t("settings.confirm.returned.act.unknown")
            : t("settings.confirm.returned.act", { provider })
          : provider === undefined
            ? t("settings.confirm.returned.unknown")
            : t("settings.confirm.returned", { provider })}
      </SaveState>
    ) : null;

  return {
    ask,
    busy,
    held: token !== undefined,
    kept: back,
    notice,
    dialog: (
      <ModalPresence>
        {asked !== undefined && (
          <ConfirmationDialog
            operation={operation}
            about={about}
            act={act}
            // What came back with the reader goes with them again, where they did not change it.
            kept={kept ?? back}
            returnTo={returnTo}
            leave={leave}
            ways={ways}
            asked={asked}
            onHeldUsed={() => {
              setToken(undefined);
            }}
            onClose={() => {
              setAsked(undefined);
            }}
            onEnded={() => {
              afterLeaving.current = asked.left;
              setAsked(undefined);
            }}
            onGone={gone}
          />
        )}
      </ModalPresence>
    ),
  };
}
