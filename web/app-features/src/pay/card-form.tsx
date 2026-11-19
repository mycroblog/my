// SPDX-License-Identifier: AGPL-3.0-only

// The card form of the PAY.JP checkout and card pages (ADR-0074 §11): the email address and the card holder's name,
// which the card's 3-D Secure needs, and PAY.JP's card element, which payjp.js draws in its own iframe. On submit, the
// name is checked against the rule PAY.JP takes before anything is sent; then payjp.js runs the 3-D Secure in an
// iframe and gives a token, which the form posts with the signed state; the card's number, expiry and code never reach
// this page or the server. A refusal of the server or of PAY.JP stands right before the button, in the catalog's words
// (PAY.JP's own message is in its language, not the reader's).

import {
  Button,
  FieldLabel,
  focusRefusedField,
  Form,
  FormActions,
  FormAlert,
  FormText,
  TextField,
  WidgetSlot,
} from "@my/design-system";
import { createContext, use, useEffect, useRef, useState, type ComponentProps } from "react";
import type { PayCardForm } from "./data";
import { loadPayjp, type Payjp, type PayjpCardElement, type PayjpFactory } from "./payjp";
import { FocusOnLoadContext, usePayStrings } from "./strings";

/** Where the card form gets payjp.js from: the script of PAY.JP on a page, a stand-in in a story. */
export const PayjpContext = createContext<() => Promise<PayjpFactory>>(loadPayjp);

/** The class of the element that payjp.js draws the card element into. */
const cardSlot = "pay-card-element";

/**
 * Whether PAY.JP takes `name` as a card holder's name: 2 to 45 Latin letters, digits, spaces, periods and hyphens, the
 * server's rule too (internal/adapter/payment/payjp, validCardName).
 */
const cardHolderName = (name: string) => /^[A-Za-z0-9 .-]{2,45}$/.test(name.trim());

/**
 * The form of `card`, sent by the button `submit`, with `then` on the line above it (what is charged after this
 * payment) and what the server refused of it (`error`).
 */
export function CardForm({
  card,
  submit,
  then,
  error,
}: {
  card: PayCardForm;
  submit: string;
  then?: string | undefined;
  error?: string | undefined;
}) {
  const t = usePayStrings();
  const focus = use(FocusOnLoadContext);
  const load = use(PayjpContext);
  const form = useRef<HTMLFormElement>(null);
  const token = useRef<HTMLInputElement>(null);
  const payjp = useRef<{ payjp: Payjp; element: PayjpCardElement } | null>(null);
  const [ready, setReady] = useState<"loading" | "ready" | "unavailable">("loading");
  // That PAY.JP took no token, in place of what the server refused before.
  const [refused, setRefused] = useState(false);
  const [nameRefused, setNameRefused] = useState(false);
  const [sending, setSending] = useState(false);

  useEffect(() => {
    const gone = new AbortController();
    load()
      .then((factory) => {
        if (gone.signal.aborted) return;
        const instance = factory(card.publicKey, { threeDSecureWorkflow: "iframe" });
        const element = instance.elements().create("card");
        element.mount(`.${cardSlot}`);
        payjp.current = { payjp: instance, element };
        setReady("ready");
      })
      .catch(() => {
        if (!gone.signal.aborted) setReady("unavailable");
      });
    return () => {
      gone.abort();
      payjp.current?.element.unmount?.();
      payjp.current = null;
    };
  }, [load, card.publicKey]);

  const onSubmit: ComponentProps<"form">["onSubmit"] = (event) => {
    // The second submit is the form's own, with the token in place.
    if (token.current?.value) return;
    event.preventDefault();
    const target = form.current;
    const session = payjp.current;
    if (target === null || session === null || sending || !target.reportValidity()) return;
    const field = (name: string) => (target.elements.namedItem(name) as HTMLInputElement | null)?.value.trim() ?? "";
    if (!cardHolderName(field("name"))) {
      setNameRefused(true);
      focusRefusedField("pay-name");
      return;
    }
    setNameRefused(false);
    setSending(true);
    setRefused(false);
    session.payjp
      .createToken(session.element, { three_d_secure: true, card: { name: field("name"), email: field("email") } })
      .then((answer) => {
        if (answer.error !== undefined || token.current === null) throw new Error("payjp.js took no token");
        token.current.value = answer.id;
        target.submit();
      })
      .catch(() => {
        setRefused(true);
        setSending(false);
      });
  };

  const alert =
    ready === "unavailable" ? t("pay.card.unavailable") : sending ? undefined : refused ? t("pay.card.failed") : error;
  return (
    <Form action={card.action} onSubmit={onSubmit} ref={form}>
      <input type="hidden" name="s" value={card.state} />
      <input type="hidden" name="token" defaultValue="" ref={token} />
      <TextField
        id="pay-email"
        name="email"
        type="email"
        label={t("pay.card.email")}
        defaultValue={card.email}
        autoComplete="email"
        inputMode="email"
        autoCapitalize="none"
        maxLength={254}
        required
      />
      <TextField
        id="pay-name"
        name="name"
        label={t("pay.card.name")}
        hint={nameRefused ? undefined : t("pay.card.name_hint")}
        error={nameRefused ? t("pay.card.name_invalid") : undefined}
        defaultValue={card.name}
        autoComplete="cc-name"
        autoCapitalize="characters"
        minLength={2}
        maxLength={45}
        required
      />
      <FieldLabel htmlFor="pay-card" id="pay-card-label">
        {t("pay.card.label")}
      </FieldLabel>
      <div id="pay-card" role="group" aria-labelledby="pay-card-label" aria-describedby="pay-card-note">
        <WidgetSlot markerClassName={cardSlot} data={{ "data-card": "" }} />
      </div>
      <FormText muted id="pay-card-note">
        {t("pay.card.note")}
      </FormText>
      <FormActions>
        {then !== undefined && <FormText>{then}</FormText>}
        {alert !== undefined && <FormAlert focus={focus && alert === error}>{alert}</FormAlert>}
        <Button kind="primary" type="submit" busy={sending} isDisabled={ready !== "ready"}>
          {submit}
        </Button>
      </FormActions>
    </Form>
  );
}
