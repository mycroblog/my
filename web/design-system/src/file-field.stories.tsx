// SPDX-License-Identifier: AGPL-3.0-only

// The field of a file the reader picks, in the reader's language (the toolbar's locale; ja, else en), on a raised
// section as a settings screen holds it: with none, where the button chooses one and the field says that it holds
// none; with one, named after the button, which then chooses another; with a long name, which breaks where it must;
// one that was refused, with what is wrong right under it; and while the form is sent. Choosing a file names it.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { FileField } from "./file-field";
import { SettingsSection } from "./settings";

const words = {
  en: {
    section: "File",
    label: "CSV file",
    hint: "A file exported from this server or from Mastodon.",
    choose: "Choose a file",
    another: "Choose another file",
    none: "No file chosen",
    refused: "This file is not a CSV file. Choose another one.",
  },
  ja: {
    section: "ファイル",
    label: "CSV ファイル",
    hint: "このサーバーか Mastodon でエクスポートしたファイル。",
    choose: "ファイルを選ぶ",
    another: "別のファイルを選ぶ",
    none: "ファイルが選ばれていません",
    refused: "このファイルは CSV ファイルではありません。別のファイルを選んでください。",
  },
} as const;

const useWords = () => words[new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en"];

function Column({ children }: { children: ReactNode }) {
  return <div className="grid max-w-[37.5rem] gap-shell-edge p-shell-edge text-body">{children}</div>;
}

type Given = { file?: string; error?: boolean; disabled?: boolean };

/** A field that keeps the name of the file the reader picks. */
function Field({ given }: { given: Given }) {
  const w = useWords();
  const [file, setFile] = useState(given.file);
  return (
    <Column>
      <SettingsSection heading={w.section}>
        <FileField
          id="file-field"
          label={w.label}
          hint={w.hint}
          error={given.error === true ? w.refused : undefined}
          choose={file === undefined ? w.choose : w.another}
          file={file}
          none={w.none}
          accept=".csv,text/csv"
          onFile={(picked) => {
            setFile(picked.name);
          }}
          isDisabled={given.disabled}
        />
      </SettingsSection>
    </Column>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "File field", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** No file yet: the button chooses one, and the field says that it holds none. */
export const Empty: Story = { render: () => <Field given={{}} /> };

/** A file chosen: its name after the button, which chooses another. */
export const Chosen: Story = { render: () => <Field given={{ file: "following_accounts.csv" }} /> };

/** A file with a long name: the name breaks where it must, under the button where the two cannot share a line. */
export const LongName: Story = {
  render: () => <Field given={{ file: "following_accounts_exported_from_my_old_server_2026-11-18_final_copy.csv" }} />,
};

/** A file that was refused: what is wrong stands right under the field. */
export const Refused: Story = { render: () => <Field given={{ file: "holiday.png", error: true }} /> };

/** While the form is sent, the button cannot be pressed. */
export const Disabled: Story = { render: () => <Field given={{ file: "following_accounts.csv", disabled: true }} /> };
