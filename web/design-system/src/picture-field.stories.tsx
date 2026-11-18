// SPDX-License-Identifier: AGPL-3.0-only

// The field of a picture the reader picks, in the reader's language (the toolbar's locale; ja, else en), on a raised
// section as a settings screen holds it: an avatar's circle and a header's 3:1, each with none, where the button
// chooses one, and with one, where it replaces it and Remove stands after it; one the server refused, with what is
// wrong right under it; and both while the form is sent. Choosing a file shows it at once, as it will show.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { PictureField } from "./picture-field";
import { images } from "./samples";
import { SettingsSection } from "./settings";

const rem = (px: number) => `${px / rootPx}rem`;

const words = {
  en: {
    section: "Pictures",
    avatar: "Avatar",
    header: "Header",
    headerHint: "Shown across the top of your profile.",
    choose: "Choose a picture",
    replace: "Replace",
    remove: "Remove",
    refused: "This picture could not be used. Choose another one.",
  },
  ja: {
    section: "画像",
    avatar: "アバター",
    header: "ヘッダー画像",
    headerHint: "プロフィールの上部に表示されます。",
    choose: "画像を選ぶ",
    replace: "変更",
    remove: "削除",
    refused: "この画像は使えませんでした。別の画像を選んでください。",
  },
} as const;

const useWords = () => words[new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en"];

function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(600), padding: rem(16) }}>{children}</div>;
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
