// SPDX-License-Identifier: AGPL-3.0-only

// The emoji picker on the sample emoji, in the reader's language (the toolbar's locale): open at rest on its popover's
// surface under the react button that opened it (pressed, as while it is open), at the top of its sections; with the
// reader's own reaction knocked out; with a skin tone chosen; a search by a custom emoji's alias, and one that matches
// none; a server without custom emoji; in a sheet, as a phone opens it; and opened by its button, live.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { useLocale } from "react-aria-components";
import { waitFor } from "storybook/test";
import { EmojiPicker, type EmojiPickerProps } from "./emoji-picker";
import { EmojiPickerPopover, EmojiPickerSheet, EmojiPickerSurface } from "./emoji-picker-overlay";
import { pickerSections, pickerStrings, searchPicker } from "./emoji.samples";
import { IconButton } from "./icon-button";
import { Preview } from "./preview";

const rem = (px: number) => `${px / rootPx}rem`;

type Lang = "en" | "ja";
const useLang = (): Lang => (new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en");

const react = { en: "React", ja: "リアクション" } as const;
const sheet = { en: { title: "React", close: "Close" }, ja: { title: "リアクション", close: "閉じる" } } as const;

/** The picker of the samples, with what a story gives it. */
function SamplePicker(props: Partial<EmojiPickerProps> & { noCustom?: boolean }) {
  const lang = useLang();
  const { noCustom = false, ...rest } = props;
  return (
    <EmojiPicker
      sections={pickerSections(lang, { noCustom })}
      search={searchPicker}
      labels={pickerStrings[lang]}
      onSelect={() => undefined}
      {...rest}
    />
  );
}

/** The react button pressed and the picker's popover under it, at its start, open at rest. */
function Opened(props: Partial<EmojiPickerProps> & { noCustom?: boolean }) {
  const lang = useLang();
  return (
    <div style={{ display: "grid", gap: rem(4), justifyItems: "start" }}>
      <Preview state="pressed">
        <IconButton icon="react" label={react[lang]} />
      </Preview>
      <EmojiPickerSurface label={react[lang]}>
        <SamplePicker {...props} />
      </EmojiPickerSurface>
    </div>
  );
}

/** The sheet of a phone, open, the picker filling it. */
function InSheet() {
  const lang = useLang();
  const [open, setOpen] = useState(true);
  return (
    <>
      <IconButton
        icon="react"
        label={react[lang]}
        onPress={() => {
          setOpen(true);
        }}
      />
      <EmojiPickerSheet
        open={open}
        onClose={() => {
          setOpen(false);
        }}
        title={sheet[lang].title}
        closeLabel={sheet[lang].close}
      >
        <SamplePicker fill />
      </EmojiPickerSheet>
    </>
  );
}

/** The react button, which opens the picker in its popover; what was picked last under it. */
function Live() {
  const lang = useLang();
  const [picked, setPicked] = useState("");
  const [open, setOpen] = useState(false);
  return (
    <div style={{ display: "grid", gap: rem(8), justifyItems: "start" }}>
      <EmojiPickerPopover
        label={react[lang]}
        open={open}
        onOpenChange={setOpen}
        trigger={<IconButton icon="react" label={react[lang]} />}
      >
        <SamplePicker
          current={picked}
          onSelect={(emoji, text) => {
            setPicked(emoji.key === picked ? "" : text);
            setOpen(false);
          }}
        />
      </EmojiPickerPopover>
      <output>{picked}</output>
    </div>
  );
}

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "Emoji picker", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Popover: Story = { render: () => <Opened /> };
/** The reader reacted with 🎉: picking it again takes it back, another moves the reaction there. */
export const OwnReaction: Story = { render: () => <Opened current="🎉" /> };
export const SkinTone: Story = { render: () => <Opened tone={3} /> };
/** A query that matches custom emoji by shortcode, first, then Unicode emoji by name and keyword. */
export const Search: Story = { render: () => <Opened defaultQuery="bl" /> };
export const SearchEmpty: Story = { render: () => <Opened defaultQuery="xylophone" /> };
/** A server with no custom emoji: no section and no category for them. */
export const NoCustomEmoji: Story = { render: () => <Opened noCustom /> };
/** On a phone: a sheet from the bottom, titled, the picker filling it. */
export const Sheet: Story = {
  render: () => <InSheet />,
  // The story tests check the sheet once it has risen, as the reader reads it, not while it fades in.
  play: async ({ canvasElement }) => {
    await waitFor(
      () => {
        const panel = canvasElement.ownerDocument.querySelector('[role="dialog"]')?.parentElement;
        const style = panel ? getComputedStyle(panel) : undefined;
        if (style?.opacity !== "1" || !["none", "matrix(1, 0, 0, 1, 0, 0)"].includes(style.transform))
          throw new Error("the sheet has not risen yet");
      },
      { timeout: 3000 },
    );
  },
};
export const Live_: Story = { name: "Live", render: () => <Live /> };
