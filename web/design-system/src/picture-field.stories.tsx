// SPDX-License-Identifier: AGPL-3.0-only

// The field of a picture the reader picks, in the reader's language (the toolbar's locale; ja, else en), on a raised
// section as a settings screen holds it: an avatar's circle and a header's 3:1, each with none, where the button
// chooses one, and with one, where it replaces it and Remove stands after it; one the server refused, with what is
// wrong right under it; and both while the form is sent. Choosing a file shows it at once, as it will show. The marks of
// a server: an icon's square and a logo's line, each shown whole, and a logo drawn for dark backgrounds on the dark.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { color } from "@my/design-tokens";
import { useState, type ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { PictureField } from "./picture-field";
import { images } from "./samples";
import { SettingsSection } from "./settings";

const words = {
  en: {
    section: "Images",
    avatar: "Avatar",
    header: "Header image",
    headerHint: "Shown across the top of your profile.",
    choose: "Choose an image",
    replace: "Replace",
    remove: "Remove",
    refused: "This image could not be used. Choose another one.",
    icon: "Icon",
    logo: "Logo",
    logoDark: "Logo for dark backgrounds",
    svg: "Choose an SVG file",
  },
  ja: {
    section: "画像",
    avatar: "アバター",
    header: "ヘッダー画像",
    headerHint: "プロフィールの上部に表示されます。",
    choose: "画像を選ぶ",
    replace: "画像を変更",
    remove: "外す",
    refused: "この画像は使えませんでした。別の画像を選んでください。",
    icon: "アイコン",
    logo: "ロゴ",
    logoDark: "暗い背景用のロゴ",
    svg: "SVG ファイルを選ぶ",
  },
} as const;

const useWords = () => words[new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en"];

function Column({ children }: { children: ReactNode }) {
  return <div className="grid max-w-[37.5rem] gap-shell-edge p-shell-edge text-body">{children}</div>;
}

type Given = { picture?: string; error?: boolean; disabled?: boolean };

/** A field that keeps the picture the reader picks or removes. */
function Field({ id, shape, given }: { id: string; shape: "round" | "banner"; given: Given }) {
  const w = useWords();
  const [src, setSrc] = useState(given.picture);
  return (
    <PictureField
      id={id}
      label={shape === "round" ? w.avatar : w.header}
      shape={shape}
      name="Yuki"
      src={src}
      hint={shape === "banner" ? w.headerHint : undefined}
      error={given.error === true ? w.refused : undefined}
      choose={src === undefined ? w.choose : w.replace}
      onFile={(file) => {
        setSrc(URL.createObjectURL(file));
      }}
      remove={
        src === undefined
          ? undefined
          : {
              label: w.remove,
              onPress: () => {
                setSrc(undefined);
              },
            }
      }
      isDisabled={given.disabled}
    />
  );
}

function Both({ given }: { given: Given }) {
  const w = useWords();
  return (
    <Column>
      <SettingsSection heading={w.section}>
        <Field id="picture-avatar" shape="round" given={given} />
        <Field id="picture-header" shape="banner" given={given} />
      </SettingsSection>
    </Column>
  );
}

/** A square mark and a line of a logo, shown whole: an SVG drawn as a picture of either shape. */
const mark = (width: number) =>
  `data:image/svg+xml,${encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${width} 100"><rect width="${width}" height="100" rx="18" fill="${color["accent-text"].light}"/><circle cx="50" cy="50" r="28" fill="${color.accent.light}"/></svg>`,
  )}`;

function MarkFields({ given }: { given: Given }) {
  const w = useWords();
  const field = (id: string, label: string, shape: "square" | "wordmark", src: string | undefined, dark = false) => (
    <PictureField
      id={id}
      label={label}
      shape={shape}
      dark={dark}
      accept="image/svg+xml,.svg"
      src={src}
      choose={w.svg}
      onFile={() => undefined}
      remove={src === undefined ? undefined : { label: w.remove, onPress: () => undefined }}
    />
  );
  return (
    <Column>
      <SettingsSection heading={w.section}>
        {field("picture-icon", w.icon, "square", given.picture === undefined ? undefined : mark(100))}
        {field("picture-logo", w.logo, "wordmark", given.picture === undefined ? undefined : mark(400))}
        {field("picture-logo-dark", w.logoDark, "wordmark", given.picture === undefined ? undefined : mark(400), true)}
      </SettingsSection>
    </Column>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Picture field", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** No picture yet: the initial and the surface stand in, and the button chooses one. */
export const Empty: Story = { render: () => <Both given={{}} /> };

/** A picture in each: the button replaces it, and Remove stands after it. */
export const Picked: Story = { render: () => <Both given={{ picture: images.dusk }} /> };

/** A picture the server refused: what is wrong stands right under the field. */
export const Refused: Story = { render: () => <Both given={{ picture: images.snow, error: true }} /> };

/** While the form is sent, the buttons cannot be pressed. */
export const Disabled: Story = { render: () => <Both given={{ picture: images.dusk, disabled: true }} /> };

/** A server's marks: an icon's square and a logo's line, the logo for dark backgrounds on the dark, each shown whole. */
export const Marks: Story = { render: () => <MarkFields given={{ picture: "set" }} /> };

/** No mark yet: the glyph of a picture stands in each place. */
export const MarksEmpty: Story = { render: () => <MarkFields given={{}} /> };
