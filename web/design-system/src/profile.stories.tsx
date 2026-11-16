// SPDX-License-Identifier: AGPL-3.0-only

// A profile's relations (#3123), in the reader's language (the toolbar's locale; ja, else en): a moved account, whose
// main action leads to the account it moved to; and an account that is not there (deleted, deactivated, or never was).
// The words are placeholders of the feature's (appfeatures:profile). On PC the screens sit in the app's columns; here,
// in a column of the timeline's width.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useId, type ReactNode } from "react";
import { useLocale } from "react-aria-components";
import { Avatar } from "./avatar";
import { LinkButton, TextLink } from "./button";
import { CaptionLine } from "./entry";
import { FeedNotice } from "./feed";
import { BackButton, ScreenStack } from "./list";
import { MenuButton, MenuGroup, MenuItem } from "./menu";
import { FieldList, MetaCount, MetaLine, ProfileHead } from "./profile";
import { images } from "./samples";

const rem = (px: number) => `${px / rootPx}rem`;

type Lang = "en" | "ja";
const useLang = (): Lang => (new Intl.Locale(useLocale().locale).language === "ja" ? "ja" : "en");

type Person = { name: string; handle: string; avatar?: string | undefined };

const owner: Record<Lang, Person> = {
  en: { name: "Sam Rivera", handle: "@sam@social.example", avatar: images.snow },
  ja: { name: "サム・リベラ", handle: "@sam@social.example", avatar: images.snow },
};

const movedTo = "@sam@photos.example";

const words = {
  en: {
    bio: "Landscape photographer. Ridges, coastlines and the light just before it rains.",
    site: "Website",
    joined: "Joined March 2023",
    followingCount: "Following",
    followersCount: "Followers",
    more: "More",
    openOriginal: "Open original page",
    mute: "Mute",
    block: "Block…",
    muteServer: "Mute server",
    movedTo: "Moved to",
    goToNew: "Go to new account",
    gone: (handle: string) => `${handle} was not found. The account may no longer exist, or the link may be wrong.`,
    goneBack: "Back",
  },
  ja: {
    bio: "風景写真を撮っています。尾根、海岸線、雨が降る直前の光。",
    site: "ウェブサイト",
    joined: "2023年3月に登録",
    followingCount: "フォロー",
    followersCount: "フォロワー",
    more: "その他",
    openOriginal: "元のページを表示",
    mute: "ミュート",
    block: "ブロック…",
    muteServer: "サーバーをミュート",
    movedTo: "移転先:",
    goToNew: "新しいアカウントに移動",
    gone: (handle: string) =>
      `${handle} は見つかりませんでした。アカウントがもう存在しないか、リンクが間違っている可能性があります。`,
    goneBack: "戻る",
  },
} as const;

type W = (typeof words)[Lang];

const domainOf = (handle: string) => handle.slice(handle.lastIndexOf("@") + 1);
const userOf = (handle: string) => handle.slice(0, handle.lastIndexOf("@"));

/** A column of the timeline's width, with its side margin. */
const Column = ({ children }: { children: ReactNode }) => (
  <div style={{ maxWidth: rem(600), paddingInline: rem(16), paddingBlock: rem(16) }}>{children}</div>
);

/** More, at the far end of the head's row: the remote account's page, the account's actions, its server's. */
const More = ({ w }: { w: W }) => (
  <MenuButton icon="more" label={w.more}>
    <MenuGroup>
      <MenuItem id="open" icon="open">
        {w.openOriginal}
      </MenuItem>
    </MenuGroup>
    <MenuGroup title={userOf(owner.en.handle)}>
      <MenuItem id="mute" icon="mute">
        {w.mute}
      </MenuItem>
      <MenuItem id="block" icon="block">
        {w.block}
      </MenuItem>
    </MenuGroup>
    <MenuGroup title={domainOf(owner.en.handle)}>
      <MenuItem id="mute-server" icon="mute">
        {w.muteServer}
      </MenuItem>
    </MenuGroup>
  </MenuButton>
);

/**
 * A moved account: its main action leads to the account it moved to, whose own profile offers follow; a line names
 * that account, its handle the link. No "Follows you": the followers moved with it.
 */
function Moved() {
  const lang = useLang();
  const w = words[lang];
  const p = owner[lang];
  const nameId = useId();
  return (
    <Column>
      <ProfileHead
        nameId={nameId}
        banner={images.dusk}
        avatar={<Avatar name={p.name} src={p.avatar} size="preview" decorative />}
        actions={
          <>
            <LinkButton href="#" kind="primary" size="timeline">
              {w.goToNew}
            </LinkButton>
            <More w={w} />
          </>
        }
        name={p.name}
        handle={p.handle}
      >
        <CaptionLine icon="moved">
          {w.movedTo}{" "}
          <bdi dir="ltr">
            <TextLink href="#">{movedTo}</TextLink>
          </bdi>
        </CaptionLine>
        <p className="m-0" dir="auto">
          {w.bio}
        </p>
        <FieldList items={[{ name: w.site, value: "ridgeline.example" }]} />
        <MetaLine>
          <span>{w.joined}</span>
          <MetaCount label={w.followingCount} value="312" href="#following" />
          <MetaCount label={w.followersCount} value="1,284" href="#followers" />
        </MetaLine>
      </ProfileHead>
    </Column>
  );
}

/** The profile of an account that is not there, whether deleted, deactivated or never made: back, then one line. */
function Gone() {
  const w = words[useLang()];
  return (
    <Column>
      <ScreenStack>
        <BackButton onPress={() => undefined}>{w.goneBack}</BackButton>
        <FeedNotice start>{w.gone("@sam@my.example")}</FeedNotice>
      </ScreenStack>
    </Column>
  );
}

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "Profile relations", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const MovedAccount: Story = { render: () => <Moved /> };
export const AccountNotThere: Story = { render: () => <Gone /> };
