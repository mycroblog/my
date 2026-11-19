// SPDX-License-Identifier: AGPL-3.0-only

// The confirmation with a text to type (#3258, ADR-0110, NFR-MOD-7), before what is grave as well as final: banning an
// account, the text its handle; closing the server, its domain; deleting a role, its name. Each in the flow of the page
// as the modal would place it, and once over the page: empty, its confirmation disabled; a text that is not the one
// stated, still disabled; the text typed, enabled; and the ban in flight. The words are samples in the toolbar's
// language (ja, else en), as the moderation and administration screens would write them.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useRef, type ReactNode } from "react";
import { useLocale } from "react-aria-components";
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

/** The stories' words, in the toolbar's language (ja, else en), as the screens would write them. */
const words = {
  en: {
    cancel: "Cancel",
    ban: {
      title: "Ban @spam_bot_7731@social.example?",
      text: "Their account is suspended on this server, their posts and media are deleted, and the servers that follow them are told. This cannot be undone.",
      confirm: "Ban account",
    },
    close: {
      title: "Close this server?",
      text: "Every account is suspended, sign-ups stop, and other servers are told this server is gone. This cannot be undone.",
      confirm: "Close server",
    },
    role: {
      title: "Delete the role Trusted reviewer?",
      text: "The 4 people who have it lose its permissions, and the invitations that grant it stop working. This cannot be undone.",
      confirm: "Delete role",
      phrase: "Trusted reviewer",
    },
  },
  ja: {
    cancel: "キャンセル",
    ban: {
      title: "@spam_bot_7731@social.example を BAN しますか?",
      text: "このサーバーでアカウントを凍結し、投稿とメディアを削除して、フォローしているサーバーに知らせます。取り消せません。",
      confirm: "BAN する",
    },
    close: {
      title: "このサーバーを閉じますか?",
      text: "すべてのアカウントを凍結し、新規登録を止め、ほかのサーバーにこのサーバーがなくなることを知らせます。取り消せません。",
      confirm: "サーバーを閉じる",
    },
    role: {
      title: "ロール「信頼できる審査者」を削除しますか?",
      text: "このロールを持つ 4 人は権限を失い、このロールを与える招待も使えなくなります。取り消せません。",
      confirm: "ロールを削除",
      phrase: "信頼できる審査者",
    },
  },
} as const;

const useWords = () => words[new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en"];

/** The handle the ban asks to type, the same in every language. */
const handle = "spam_bot_7731@social.example";

/** The ban's confirmation, in the flow of the page, or over it. */
function Ban({ busy = false, layer = false }: { busy?: boolean; layer?: boolean }) {
  const w = useWords();
  return (
    <ConfirmDialog
      layer={layer}
      title={w.ban.title}
      cancel={w.cancel}
      confirm={w.ban.confirm}
      phrase={handle}
      busy={busy}
      onCancel={none}
      onConfirm={none}
    >
      {w.ban.text}
    </ConfirmDialog>
  );
}

/** Closing the server, its domain to type; deleting a role, its name, which has a space. */
function Others() {
  const w = useWords();
  return (
    <Stack gap="entry-room">
      <ConfirmDialog
        layer={false}
        title={w.close.title}
        cancel={w.cancel}
        confirm={w.close.confirm}
        phrase="mycroblog.example"
        onCancel={none}
        onConfirm={none}
      >
        {w.close.text}
      </ConfirmDialog>
      <ConfirmDialog
        layer={false}
        title={w.role.title}
        cancel={w.cancel}
        confirm={w.role.confirm}
        phrase={w.role.phrase}
        onCancel={none}
        onConfirm={none}
      >
        {w.role.text}
      </ConfirmDialog>
    </Stack>
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
      <TypedIn text={handle}>
        <Ban />
      </TypedIn>
    </Column>
  ),
};

/** The ban in flight: the confirmation spins, Cancel and close are disabled, and the dialog stays. */
export const TypedConfirmBusy: Story = {
  render: () => (
    <Column>
      <TypedIn text={handle}>
        <Ban busy />
      </TypedIn>
    </Column>
  ),
};

/** Closing the server, its domain to type; deleting a role, its name, which has a space. */
export const TypedConfirmOthers: Story = {
  render: () => (
    <Column>
      <Others />
    </Column>
  ),
};

/** The ban's confirmation over the page, the focus in its field. */
export const TypedConfirmOverThePage: Story = {
  render: () => <Ban layer />,
};
