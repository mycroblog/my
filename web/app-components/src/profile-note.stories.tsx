// SPDX-License-Identifier: AGPL-3.0-only

// The reader's private note on a profile, in the reader's language (the toolbar's locale; ja, else en): the line that
// holds it and opens its editor, the line of an account with none, and the editor in the line's place. The words are
// the feature's.

import { useLocale } from "@my/design-system";
import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { NoteEditor, NoteLine } from "./profile";

const rem = (px: number) => `${px / rootPx}rem`;

const words = {
  en: {
    note: "Private note",
    add: "Add a note only you can see",
    edit: "Edit note",
    cancel: "Cancel",
    save: "Save",
    text: "Met at the bakery on Saturday. Ask about the rye starter.",
  },
  ja: {
    note: "非公開のメモ",
    add: "自分だけが見られるメモを追加",
    edit: "メモを編集",
    cancel: "キャンセル",
    save: "保存",
    text: "土曜のパン屋で会った。ライ麦の種のことを聞く。",
  },
} as const;

const useWords = () => words[new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en"];

/** A column of a profile's width, with its side margin, which the line's surface reaches into. */
const Column = ({ children }: { children: ReactNode }) => (
  <div style={{ maxWidth: rem(600), padding: rem(16) }}>{children}</div>
);

function Line({ written }: { written: boolean }) {
  const w = useWords();
  return (
    <Column>
      <NoteLine note={written ? w.text : ""} placeholder={w.add} label={w.edit} onPress={() => undefined} />
    </Column>
  );
}

function Editor() {
  const w = useWords();
  const [text, setText] = useState<string>(w.text);
  return (
    <Column>
      <NoteEditor
        label={w.note}
        value={text}
        onChange={setText}
        maxLength={2000}
        cancel={w.cancel}
        save={w.save}
        onCancel={() => undefined}
        onSave={() => undefined}
      />
    </Column>
  );
}

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "Profile note", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The note, a line that opens its editor. */
export const Written: Story = { render: () => <Line written /> };
/** No note yet: what adding one does, muted. */
export const None: Story = { render: () => <Line written={false} /> };
/** The editor in the line's place: the note in its box, then Cancel and Save. */
export const Editing: Story = { render: () => <Editor /> };
