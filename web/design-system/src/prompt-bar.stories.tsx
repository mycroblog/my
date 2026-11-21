// SPDX-License-Identifier: AGPL-3.0-only

// The bar that opens a place to write, in the reader's language (the toolbar's locale; ja, else en): as wide as a
// screen's column, and narrower than its prompt, which is cut at its end on one line. The words are the board's
// (events.board.write); the hover, the press and the focus ring are the pointer's and the keyboard's.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useLocale } from "react-aria-components";
import { Avatar } from "./avatar";
import { Stack } from "./layout";
import { PromptBar } from "./prompt-bar";
import { images } from "./samples";

const rem = (px: number) => `${px / rootPx}rem`;
const none = () => undefined;

const words = {
  en: { write: "Write on the board" },
  ja: { write: "掲示板に書く" },
} as const;

function Bars() {
  const w = words[new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en"];
  const avatar = <Avatar name="Yuki" src={images.snow} size="post" decorative />;
  return (
    <div style={{ display: "grid", gap: rem(28), padding: rem(16) }}>
      <div style={{ maxWidth: rem(600) }}>
        <Stack>
          <PromptBar avatar={avatar} onPress={none}>
            {w.write}
          </PromptBar>
        </Stack>
      </div>
      <div style={{ maxWidth: rem(140) }}>
        <Stack>
          <PromptBar avatar={avatar} onPress={none}>
            {w.write}
          </PromptBar>
        </Stack>
      </div>
    </div>
  );
}

const meta = { title: "Prompt bar", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The bar in a screen's column, and in one narrower than its prompt. */
export const Widths: Story = { render: () => <Bars /> };
