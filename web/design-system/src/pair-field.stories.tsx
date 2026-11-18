// SPDX-License-Identifier: AGPL-3.0-only

// The pairs the reader writes (the fields of a profile), in the reader's language (the toolbar's locale; ja, else
// en), on a raised section as a settings screen holds them: one pair not written yet, where the placeholders name its
// fields; four written, one with a value longer than its field; one field the server refused, marked alone, with
// what is wrong directly under its pair; what is wrong with the pairs as a whole, under the list; and while the form
// is sent.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { PairField, PairList } from "./pair-field";
import { SettingsSection } from "./settings";

const words = {
  en: {
    section: "Fields",
    name: "Label",
    value: "Content",
    legend: (n: number) => `Field ${String(n)}`,
    remove: (n: number) => `Remove field ${String(n)}`,
    refused: "This value is not valid. Check what you entered.",
    tooMany: "You can have at most 4.",
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
    remove: (n: number) => `補足情報 ${String(n)} を外す`,
    refused: "この値は使えません。入力した内容を確かめてください。",
    tooMany: "4 個までにしてください。",
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
  return <div className="grid max-w-[37.5rem] gap-shell-edge p-shell-edge text-body">{children}</div>;
}

type Pair = { key: number; name: string; value: string };

/** A list that keeps what the reader writes in its pairs and removes. */
function Pairs({
  count,
  refused,
  tooMany = false,
  disabled = false,
}: {
  count: number;
  /** The pair (from 0) and its field that the server refused. */
  refused?: { pair: number; of: "name" | "value" };
  tooMany?: boolean;
  disabled?: boolean;
}) {
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
        <PairList id="pairs" label={w.section} error={tooMany ? w.tooMany : undefined}>
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
              error={refused?.pair === i ? { of: refused.of, text: w.refused } : undefined}
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

/** A name the server refused: it alone is marked, and what is wrong stands directly under it. */
export const Refused: Story = { render: () => <Pairs count={3} refused={{ pair: 1, of: "name" }} /> };

/** A content the server refused: it alone is marked, and what is wrong stands directly under it. */
export const RefusedContent: Story = { render: () => <Pairs count={3} refused={{ pair: 1, of: "value" }} /> };

/** What is wrong with the pairs as a whole, under the list; no field is marked. */
export const RefusedWhole: Story = { render: () => <Pairs count={4} tooMany /> };

/** While the form is sent, nothing can be changed. */
export const Disabled: Story = { render: () => <Pairs count={2} disabled /> };
