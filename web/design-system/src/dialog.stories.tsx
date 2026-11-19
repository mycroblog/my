// SPDX-License-Identifier: AGPL-3.0-only

// The confirmation with a text to type (#3258, ADR-0110, NFR-MOD-7), before what is grave as well as final: banning an
// account, the text its handle; closing the server, its domain; deleting a role, its name. Each in the flow of the page
// as the modal would place it, and once over the page: empty, its confirmation disabled; a text that is not the one
// stated, still disabled; the text typed, enabled; and the ban in flight. The words are samples in English, as the
// moderation and administration screens would write them.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useRef, type ReactNode } from "react";
import { ConfirmDialog } from "./dialog";
import { Stack } from "./layout";

const rem = (px: number) => `${px / rootPx}rem`;
const none = () => undefined;

/** The column of a screen: as wide as the app's main column at most, the frame's edge around it. */
function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(600), padding: rem(16) }}>{children}</div>;
}

/** Types `text` into the field inside, as the reader would, once it is there. */
function TypedIn({ text, children }: { text: string; children: ReactNode }) {
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const input = root.current?.querySelector("input");
    if (input === null || input === undefined) return;
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set?.call(input, text);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  }, [text]);
  return <div ref={root}>{children}</div>;
}

const ban = {
  title: "Ban @spam_bot_7731@social.example?",
  text: "Their account is suspended on this server, their posts and media are deleted, and the servers that follow them are told. This cannot be undone.",
  phrase: "spam_bot_7731@social.example",
  confirm: "Ban account",
} as const;

/** The ban's confirmation, in the flow of the page. */
function Ban({ busy = false }: { busy?: boolean }) {
  return (
    <ConfirmDialog
      layer={false}
      title={ban.title}
      cancel="Cancel"
      confirm={ban.confirm}
      phrase={ban.phrase}
      busy={busy}
      onCancel={none}
      onConfirm={none}
    >
      {ban.text}
    </ConfirmDialog>
  );
}

const meta = { title: "Dialogs", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** Banning an account, nothing typed yet: the field labelled with the handle to type, the confirmation disabled. */
export const TypedConfirm: Story = {
  render: () => (
    <Column>
      <Ban />
    </Column>
  ),
};

/** The handle typed without its server: not the stated text, so the confirmation stays disabled. */
export const TypedConfirmNotYet: Story = {
  render: () => (
    <Column>
      <TypedIn text="spam_bot_7731">
        <Ban />
      </TypedIn>
    </Column>
  ),
};

/** The handle typed: the confirmation is enabled, and Enter in the field confirms. */
export const TypedConfirmTyped: Story = {
  render: () => (
    <Column>
      <TypedIn text={ban.phrase}>
        <Ban />
      </TypedIn>
    </Column>
  ),
};

/** The ban in flight: the confirmation spins, Cancel and close are disabled, and the dialog stays. */
export const TypedConfirmBusy: Story = {
  render: () => (
    <Column>
      <TypedIn text={ban.phrase}>
        <Ban busy />
      </TypedIn>
    </Column>
  ),
};

/** Closing the server, its domain to type; deleting a role, its name, which has a space. */
export const TypedConfirmOthers: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-room">
        <ConfirmDialog
          layer={false}
          title="Close this server?"
          cancel="Cancel"
          confirm="Close server"
          phrase="mycroblog.example"
          onCancel={none}
          onConfirm={none}
        >
          Every account is suspended, sign-ups stop, and other servers are told this server is gone. This cannot be
          undone.
        </ConfirmDialog>
        <ConfirmDialog
          layer={false}
          title="Delete the role Trusted reviewer?"
          cancel="Cancel"
          confirm="Delete role"
          phrase="Trusted reviewer"
          onCancel={none}
          onConfirm={none}
        >
          The 4 people who have it lose its permissions, and the invitations that grant it stop working. This cannot be
          undone.
        </ConfirmDialog>
      </Stack>
    </Column>
  ),
};

/** The ban's confirmation over the page, the focus in its field. */
export const TypedConfirmOverThePage: Story = {
  render: () => (
    <ConfirmDialog
      title={ban.title}
      cancel="Cancel"
      confirm={ban.confirm}
      phrase={ban.phrase}
      onCancel={none}
      onConfirm={none}
    >
      {ban.text}
    </ConfirmDialog>
  ),
};
