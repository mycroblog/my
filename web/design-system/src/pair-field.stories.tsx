// SPDX-License-Identifier: AGPL-3.0-only

// The pairs the reader writes (the fields of a profile), in the reader's language (the toolbar's locale; ja, else
// en), on a raised section as a settings screen holds them: one pair not written yet, where the placeholders name its
// fields; four written, one with a value longer than its field; the pairs with what the server refused of them under
// the list; and while the form is sent.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { PairField, PairList } from "./pair-field";
import { SettingsSection } from "./settings";

const rem = (px: number) => `${px / rootPx}rem`;

const words = {
  en: {
    section: "Fields",
    name: "Label",
    value: "Content",
    legend: (n: number) => `Field ${String(n)}`,
    remove: (n: number) => `Remove field ${String(n)}`,
    refused: "This value is not valid. Check what you entered.",
    pairs: [
      ["Photos", "https://yuki.photos.example"],
      ["Pronouns", "she/her"],
      ["Lives in", "A town between the mountains and the sea, an hour from the nearest city by the morning train"],
      ["Languages", "日本語, English"],
    ],
  },
  ja: {
    section: "補足情報",
    name: "ラベル",
    value: "内容",
    legend: (n: number) => `補足情報 ${String(n)}`,
    remove: (n: number) => `補足情報 ${String(n)} を削除`,
    refused: "この値は使えません。入力した内容を確かめてください。",
    pairs: [
      ["写真", "https://yuki.photos.example"],
      ["呼び方", "ゆきさん"],
      ["住んでいる場所", "山と海のあいだの町。いちばん近い都市まで朝の電車で 1 時間かかります"],
      ["言語", "日本語, English"],
    ],
  },
} as const;

const useWords = () => words[new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en"];

function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(600), padding: rem(16) }}>{children}</div>;
}

type Pair = { key: number; name: string; value: string };

/** A list that keeps what the reader writes in its pairs and removes. */
function Pairs({ count, error = false, disabled = false }: { count: number; error?: boolean; disabled?: boolean }) {
  const w = useWords();
  const [pairs, setPairs] = useState<readonly Pair[]>(() =>
    count === 0
      ? [{ key: 0, name: "", value: "" }]
      : w.pairs.slice(0, count).map(([name, value], key) => ({ key, name, value })),
  );
  const change = (key: number, c: Partial<Pair>) => {
    setPairs((was) => was.map((p) => (p.key === key ? { ...p, ...c } : p)));
  };
  return (
    <Column>
      <SettingsSection heading={w.section}>
        <PairList id="pairs" label={w.section} error={error ? w.refused : undefined}>
          {pairs.map((p, i) => (
            <PairField
              key={p.key}
              legend={w.legend(i + 1)}
              nameLabel={w.name}
              name={p.name}
              onName={(name) => {
                change(p.key, { name });
              }}
              valueLabel={w.value}
              value={p.value}
              onValue={(value) => {
                change(p.key, { value });
              }}
              maxLength={255}
              remove={{
                label: w.remove(i + 1),
                onPress: () => {
                  setPairs((was) => was.filter((other) => other.key !== p.key));
                },
              }}
              invalid={error}
              isDisabled={disabled}
            />
          ))}
        </PairList>
      </SettingsSection>
    </Column>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Pair field", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** One pair not written yet: the placeholders name its fields. */
export const Empty: Story = { render: () => <Pairs count={0} /> };

/** Four pairs, one with a value longer than its field. */
export const Written: Story = { render: () => <Pairs count={4} /> };

/** What the server refused of the pairs, under the list; every field is marked. */
export const Refused: Story = { render: () => <Pairs count={2} error /> };

/** While the form is sent, nothing can be changed. */
export const Disabled: Story = { render: () => <Pairs count={2} disabled /> };
