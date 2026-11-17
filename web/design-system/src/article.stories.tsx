// SPDX-License-Identifier: AGPL-3.0-only

// An article's text set for long reading (ADR-0160), in the reader's language (the toolbar's locale), in a column of
// the timeline's width: every block of the article's allowlist, a heading of each level, a table wider than a phone,
// a code line longer than the column, an image of the author's upload, and a link that stands for an image the server
// did not keep. Its head: the title and the summary, and the cover, plain and sensitive.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { ArticleCover, ArticleText, ArticleTitle } from "./article";
import { articleCover as cover, articleText } from "./article.samples";

const rem = (px: number) => `${px / rootPx}rem`;

const useLang = () => (new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en");

const Column = ({ children }: { children: ReactNode }) => (
  <div style={{ maxWidth: rem(600), paddingInline: rem(16), display: "grid", gap: rem(16) }}>{children}</div>
);

const head = {
  en: {
    title: "Running a small server for a year",
    summary: "What it took, what it cost, and what we would do again.",
  },
  ja: { title: "小さなサーバーを一年続けて", summary: "必要だったこと、かかった費用、もう一度やるならどうするか。" },
} as const;

function TextShown() {
  const lang = useLang();
  return (
    <Column>
      <ArticleText lang={lang}>{articleText[lang]}</ArticleText>
    </Column>
  );
}

function HeadShown({ sensitive }: { sensitive: boolean }) {
  const lang = useLang();
  return (
    <Column>
      <ArticleTitle title={head[lang].title} summary={head[lang].summary} lang={lang} />
      <ArticleCover cover={cover} sensitive={sensitive} />
    </Column>
  );
}

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Article", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Text: Story = { render: () => <TextShown /> };
export const Head: Story = { render: () => <HeadShown sensitive={false} /> };
export const HeadSensitive: Story = { render: () => <HeadShown sensitive /> };
