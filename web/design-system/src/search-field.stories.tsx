// SPDX-License-Identifier: AGPL-3.0-only

// The field that finds as the reader writes, in the reader's language (the toolbar's locale; ja, else en): empty,
// under its label and hint; with words, clear at its end; busy while an account of another server is looked up; and
// with an error under it. The words are placeholders.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useId } from "react";
import { useLocale } from "react-aria-components";
import { SearchField } from "./search-field";

const rem = (px: number) => `${px / rootPx}rem`;

const words = {
  en: {
    label: "Search",
    hint: "People by name or handle, hashtags, and posts by their words",
    clear: "Clear",
    member: "Add an account you follow",
    limit: "A list can have at most 1,000 members. Remove one first.",
  },
  ja: {
    label: "検索",
    hint: "名前やハンドルで人を、ハッシュタグを、言葉で投稿を探せます",
    clear: "消去",
    member: "フォロー中のアカウントを追加",
    limit: "リストのメンバーは 1000 人までです。先にどれかのメンバーを外してください。",
  },
} as const;

function Field({ state }: { state: "empty" | "words" | "busy" | "error" }) {
  const w = words[new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en"];
  const id = useId();
  const member = state === "error";
  return (
    <div style={{ maxWidth: rem(600), padding: rem(16) }}>
      <SearchField
        id={id}
        label={member ? w.member : w.label}
        hint={member ? undefined : w.hint}
        defaultValue={state === "empty" ? "" : state === "busy" ? "@kiri@far.example" : "sam"}
        onChange={() => undefined}
        busy={state === "busy"}
        error={member ? w.limit : undefined}
        clearLabel={w.clear}
      />
    </div>
  );
}

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "Search field", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Empty: Story = { render: () => <Field state="empty" /> };
export const WithWords: Story = { render: () => <Field state="words" /> };
export const LookingUp: Story = { render: () => <Field state="busy" /> };
export const WithError: Story = { render: () => <Field state="error" /> };
