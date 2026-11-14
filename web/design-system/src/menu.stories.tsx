// SPDX-License-Identifier: AGPL-3.0-only

// The menus of a post, open at rest under the button that opened it (pressed, as while the menu is open), a story per
// menu: the more menu on another's post, with one item hovered and one focused from the keyboard; the more menu on
// one's own post, bookmarked; and the boost button's menu of a boosted post, its count and on state kept. Their labels
// are samples in the reader's language (`lang`); a label whose action asks for a confirmation ends in "…".

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { IconButton } from "./icon-button";
import { MenuButton, MenuGroup, MenuItem, MenuPanel } from "./menu";
import { Preview, PreviewItems, type PreviewState } from "./preview";

const rem = (px: number) => `${px / rootPx}rem`;

type Lang = "en" | "ja";

const labels = {
  en: {
    boost: "Boost",
    quote: "Quote",
    more: "More",
    link: "Copy link",
    open: "Open original page",
    share: "Share",
    bookmark: "Bookmark",
    unbookmark: "Remove bookmark",
    edit: "Edit",
    mute: "Mute",
    block: "Block…",
    report: "Report…",
    delete: "Delete…",
  },
  ja: {
    boost: "ブースト",
    quote: "引用",
    more: "その他",
    link: "リンクをコピー",
    open: "元のページを開く",
    share: "共有",
    bookmark: "ブックマーク",
    unbookmark: "ブックマークを外す",
    edit: "編集",
    mute: "ミュート",
    block: "ブロック…",
    report: "通報…",
    delete: "削除…",
  },
} as const;

// The space between a button and its menu, as the popover places it, in px.
const menuOffset = 4;

// A post's more menu: what reaches the post (copy link, open original, share), then the bookmark toggle on its own;
// then for another's post the actions on its account (mute, block, report) last and apart, under a caption naming it,
// or for one's own post edit, then delete last and apart.
function MoreItems({ lang, own, bookmarked }: { lang: Lang; own: boolean; bookmarked: boolean }) {
  const l = labels[lang];
  return (
    <>
      <MenuGroup>
        <MenuItem id="link" icon="link">
          {l.link}
        </MenuItem>
        {!own && (
          <MenuItem id="open" icon="open">
            {l.open}
          </MenuItem>
        )}
        <MenuItem id="share" icon="share">
          {l.share}
        </MenuItem>
      </MenuGroup>
      <MenuGroup>
        <MenuItem id="bookmark" icon="bookmark" on={bookmarked}>
          {bookmarked ? l.unbookmark : l.bookmark}
        </MenuItem>
      </MenuGroup>
      {own ? (
        <>
          <MenuGroup>
            <MenuItem id="edit" icon="edit">
              {l.edit}
            </MenuItem>
          </MenuGroup>
          <MenuGroup>
            <MenuItem id="delete" icon="delete">
              {l.delete}
            </MenuItem>
          </MenuGroup>
        </>
      ) : (
        <MenuGroup title="@mika">
          <MenuItem id="mute" icon="mute">
            {l.mute}
          </MenuItem>
          <MenuItem id="block" icon="block">
            {l.block}
          </MenuItem>
          <MenuItem id="report" icon="report">
            {l.report}
          </MenuItem>
        </MenuGroup>
      )}
    </>
  );
}

/** The more button pressed and its menu under it, its end at the button's end; `states` shows items in a state. */
function MoreMenu({
  lang,
  own = false,
  bookmarked = false,
  states = {},
}: {
  lang: Lang;
  own?: boolean;
  bookmarked?: boolean;
  states?: Record<string, PreviewState>;
}) {
  return (
    <div lang={lang} style={{ display: "grid", gap: rem(menuOffset), justifyItems: "end", maxWidth: rem(360) }}>
      <Preview state="pressed">
        <IconButton icon="more" label={labels[lang].more} />
      </Preview>
      <PreviewItems states={states}>
        <MenuPanel label={labels[lang].more}>
          <MoreItems lang={lang} own={own} bookmarked={bookmarked} />
        </MenuPanel>
      </PreviewItems>
    </div>
  );
}

/** The boost button, boosted, with its count, pressed, and its menu under it: boost (on) and quote. */
function BoostMenu({ lang }: { lang: Lang }) {
  const l = labels[lang];
  return (
    <div lang={lang} style={{ display: "grid", gap: rem(menuOffset), justifyItems: "start" }}>
      <Preview state="pressed">
        <MenuButton icon="boost" label={l.boost} count={1_234} pressed align="start">
          <MenuItem id="boost" icon="boost" on>
            {l.boost}
          </MenuItem>
        </MenuButton>
      </Preview>
      <MenuPanel label={l.boost}>
        <MenuGroup>
          <MenuItem id="boost" icon="boost" on>
            {l.boost}
          </MenuItem>
          <MenuItem id="quote" icon="quote">
            {l.quote}
          </MenuItem>
        </MenuGroup>
      </MenuPanel>
    </div>
  );
}

// Tagged `themes`: the story tests run it in dark too.
const meta = {
  title: "Menu",
  tags: ["themes"],
  args: { lang: "en" },
  argTypes: { lang: { control: "select", options: ["en", "ja"], description: "The reader's language" } },
} satisfies Meta<{ lang: Lang }>;
export default meta;

type Story = StoryObj<Meta<{ lang: Lang }>>;

export const OnAnothersPost: Story = {
  render: ({ lang }) => <MoreMenu lang={lang} states={{ open: "hover", block: "focus-visible" }} />,
};
export const OnOwnPost: Story = { render: ({ lang }) => <MoreMenu lang={lang} own bookmarked /> };
export const Boost: Story = { render: ({ lang }) => <BoostMenu lang={lang} /> };
