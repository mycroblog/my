// SPDX-License-Identifier: AGPL-3.0-only

// The confirmation with a text to type (#3258, ADR-0110, NFR-MOD-7), before what is grave as well as final: banning an
// account, the text its handle; closing the server, its domain; deleting a role, its name. Each in the flow of the page
// as the modal would place it, and once over the page: empty, its confirmation disabled; a text that is not the one
// stated, still disabled; the text typed, enabled; and the ban in flight. Then the confirmation with a field the reader
// may fill in (#3217), the appeal of a post the server's automated review refused, with what the moderators should
// know: empty, its confirmation open all the same; written in; refused under the field; in flight; and over the page.
// The words are samples in the toolbar's language (ja, else en), as the moderation and administration screens and the
// composer would write them.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useRef, type ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { ConfirmDialog } from "./dialog";
import { TextField } from "./form-page";
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
    appeal: {
      title: "Appeal this refusal?",
      text: "The text of your post is sent to this server's moderators, who review it. You can appeal a refusal once. This cannot be undone.",
      confirm: "Send appeal",
      label: "What the moderators should know (optional)",
      hint: "Up to 2,000 characters. It is sent with the text of your post.",
      written:
        "This is a recipe I wrote myself. The link goes to my own blog, where the same recipe has the photos of each step.",
      over: "Shorten this to 2,000 characters or fewer.",
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
    appeal: {
      title: "この判定に異議を申し立てますか？",
      text: "投稿の本文がこのサーバーのモデレーターに送られ、モデレーターが審査します。異議申し立ては判定 1 件につき 1 回だけです。取り消せません。",
      confirm: "申し立てる",
      label: "モデレーターに伝えたいこと (任意)",
      hint: "2,000 文字まで書けます。投稿の本文と一緒に送られます。",
      written: "自分で書いたレシピです。リンク先は自分のブログで、同じレシピに手順ごとの写真を載せています。",
      over: "2,000 文字以内にしてください。",
    },
  },
} as const;

const useWords = () => words[new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en"];

/** The handle the ban asks to type, the same in every language. */
const handle = "spam_bot_7731@social.example";

/** The ban's confirmation, in the flow of the page, or over it. */
function Ban({ busy = false, pending = false, layer = false }: { busy?: boolean; pending?: boolean; layer?: boolean }) {
  const w = useWords();
  return (
    <ConfirmDialog
      layer={layer}
      title={w.ban.title}
      cancel={w.cancel}
      confirm={w.ban.confirm}
      phrase={handle}
      busy={busy}
      pending={pending}
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

/**
 * The appeal's confirmation with what the moderators should know, in the flow of the page or over it: `written` in the
 * field as it opens, `over` refused under it.
 */
function Appeal({
  written = false,
  over = false,
  busy = false,
  layer = false,
}: {
  written?: boolean;
  over?: boolean;
  busy?: boolean;
  layer?: boolean;
}) {
  const w = useWords();
  return (
    <ConfirmDialog
      layer={layer}
      title={w.appeal.title}
      cancel={w.cancel}
      confirm={w.appeal.confirm}
      busy={busy}
      onCancel={none}
      onConfirm={none}
      field={
        <TextField
          id="appeal-comment"
          name="comment"
          label={w.appeal.label}
          multiline
          readOnly={busy}
          defaultValue={written || over ? w.appeal.written : undefined}
          hint={over ? undefined : w.appeal.hint}
          error={over ? w.appeal.over : undefined}
        />
      }
    >
      {w.appeal.text}
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
      <TypedIn text={handle}>
        <Ban />
      </TypedIn>
    </Column>
  ),
};

/** The handle typed while what the reader reads first is on its way: the confirmation stays disabled. */
export const TypedConfirmPending: Story = {
  render: () => (
    <Column>
      <TypedIn text={handle}>
        <Ban pending />
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

/**
 * The appeal of a refused post, nothing written for the moderators: the field says that it may stay empty and how
 * long it may be, and the confirmation is open all the same.
 */
export const FieldConfirm: Story = {
  render: () => (
    <Column>
      <Appeal />
    </Column>
  ),
};

/** What the moderators should know, written in the field, a few lines tall. */
export const FieldConfirmWritten: Story = {
  render: () => (
    <Column>
      <Appeal written />
    </Column>
  ),
};

/** What was written is longer than the server takes: the rule stands under the field, which is marked invalid. */
export const FieldConfirmRefused: Story = {
  render: () => (
    <Column>
      <Appeal over />
    </Column>
  ),
};

/** The appeal in flight: the confirmation spins, Cancel and close are disabled, and the field cannot be changed. */
export const FieldConfirmBusy: Story = {
  render: () => (
    <Column>
      <Appeal written busy />
    </Column>
  ),
};

/** The appeal's confirmation over the page. */
export const FieldConfirmOverThePage: Story = {
  render: () => <Appeal layer />,
};
